// Package ssologin は AWS IAM Identity Center の OIDC ログインフローを実装する
// aws CLI を exec せず AWS SDK for Go v2(ssooidc)だけでブラウザ認証を完結させる(D12)
// 既定は Authorization Code + PKCE(localhost へのリダイレクト) device code は opt-in
// 書き出すトークンキャッシュは aws CLI / SDK と互換の形式にする
package ssologin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

const (
	deviceGrantType   = "urn:ietf:params:oauth:grant-type:device_code"
	authCodeGrantType = "authorization_code"
	defaultClientName = "awsp"
	publicClientType  = "public"

	// MethodDeviceCode は device authorization flow を表す Flow.Method の値
	MethodDeviceCode = "device_code"
	// MethodAuthorizationCode は Authorization Code + PKCE flow を表す Flow.Method の値(既定)
	MethodAuthorizationCode = "authorization_code"

	// defaultPollInterval は StartDeviceAuthorization が interval を返さなかったときのポーリング間隔
	// RFC 8628 の既定値に合わせる
	defaultPollInterval = 5 * time.Second

	// timeLayout は書き出す時刻の形式(UTC、"2026-09-16T12:34:56Z")
	// time.RFC3339 は UTC の time.Time を Format すると同じ形式になる
	timeLayout = time.RFC3339

	// redirectPath は PKCE のローカルコールバックサーバーが受けるパス
	redirectPath = "/oauth/callback"
	// redirectURIWithoutPort は RegisterClient に渡す redirect URI(ポート無し)
	// PKCE は毎回ポートが変わるため RegisterClient にはポート無しの固定値を渡す(aws CLI と同じ)
	redirectURIWithoutPort = "http://127.0.0.1" + redirectPath

	// defaultListenAddr は PKCE のローカルコールバックサーバーの既定 listen アドレス
	defaultListenAddr = "127.0.0.1:0"

	// authCodeOverallTimeout は PKCE の待ち上限(開始から) aws CLI の _OVERALL_TIMEOUT と同じ
	// device code と違い認可コード自体に有効期限が無いため awsp 側で上限を持つ
	authCodeOverallTimeout = 10 * time.Minute

	// codeVerifierAlphabet は PKCE code_verifier に使う文字集合(RFC 7636 の unreserved 文字)
	codeVerifierAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	// codeVerifierLength は code_verifier の長さ
	codeVerifierLength = 64
)

// authGrantTypes は PKCE 用クライアント登録で指定する grantTypes
var authGrantTypes = []string{authCodeGrantType, "refresh_token"}

// OIDCClient は ssooidc.Client のうちログインフローに必要なメソッドだけを抜き出したインターフェース
// テストではフェイクに差し替える *ssooidc.Client はこれを自然に満たす
type OIDCClient interface {
	RegisterClient(
		ctx context.Context,
		params *ssooidc.RegisterClientInput,
		optFns ...func(*ssooidc.Options),
	) (*ssooidc.RegisterClientOutput, error)
	StartDeviceAuthorization(
		ctx context.Context,
		params *ssooidc.StartDeviceAuthorizationInput,
		optFns ...func(*ssooidc.Options),
	) (*ssooidc.StartDeviceAuthorizationOutput, error)
	CreateToken(
		ctx context.Context,
		params *ssooidc.CreateTokenInput,
		optFns ...func(*ssooidc.Options),
	) (*ssooidc.CreateTokenOutput, error)
}

// Options は Start の挙動を制御する
type Options struct {
	// Client は OIDC クライアント 必須
	Client OIDCClient
	// CacheDir はトークンキャッシュの保存先 未指定時は ssocache.DefaultCacheDir()
	CacheDir string
	// OpenBrowser は認可 URL を開く関数 未指定時は OS 標準のオープンコマンド
	OpenBrowser func(url string) error
	// ClientName は RegisterClient に渡すクライアント名 未指定時は "awsp"
	ClientName string
	// UseDeviceCode を true にすると device authorization flow を使う(opt-in)
	// 既定(false)は Authorization Code + PKCE(D12 2026-09-16 改訂)
	UseDeviceCode bool
	// ListenAddr は PKCE のローカルコールバックサーバーの listen アドレス
	// 未指定時は "127.0.0.1:0"(空きポート) テストでの固定に使う想定
	ListenAddr string
	// AuthorizeBaseURL は認可 URL のベース(スキーム+ホスト、末尾スラッシュ無し)
	// 未指定時は sso_region から "https://oidc.<region>.amazonaws.com"(cn- は .com.cn)を導出する
	// テストで実サーバーの代わりに差し替えるためのフック
	AuthorizeBaseURL string
}

// Flow はログインフロー開始後の状態を保持する 方式(device code / PKCE)に依存しない形にしてある
// Start はブロックせずに返し Wait が承認完了までポーリング/待受する
type Flow struct {
	// Method はこの Flow の方式 MethodAuthorizationCode か MethodDeviceCode
	Method string
	// AuthorizationURL は人が開くべき認可 URL device では verificationUriComplete(無ければ verificationUri)
	AuthorizationURL string
	// UserCode は device code のときだけ設定する AuthorizationURL とは別に入力するコード PKCE では空
	UserCode string
	// ExpiresAt はこの Flow 自体の有効期限 device code は認可コードの期限 PKCE は開始から 10 分固定
	ExpiresAt time.Time
	// Interval は device code の CreateToken ポーリング間隔 PKCE では使わない
	Interval time.Duration
	// BrowserErr はブラウザ起動に失敗した場合の理由
	// nil でなければ呼び出し側が URL(と device code なら UserCode)を人に見せる必要がある
	BrowserErr error

	client                OIDCClient
	session               awsconfig.SSOSession
	tokenPath             string
	clientID              string
	clientSecret          string
	registrationExpiresAt time.Time
	sleepFunc             func(context.Context, time.Duration) error

	// deviceCode は device code のときだけ使う
	deviceCode string

	// codeVerifier state redirectURI callback は PKCE のときだけ使う
	codeVerifier string
	state        string
	redirectURI  string
	callback     *callbackServer
}

// Result は Wait が成功したときの結果 トークン値は含まない
type Result struct {
	// ExpiresAt はアクセストークンの有効期限
	ExpiresAt time.Time
	// HasRefreshToken は refreshToken が発行されたか
	HasRefreshToken bool
}

// Start はログインフローを開始し Flow を返す 承認完了までは待たない
// Options.UseDeviceCode が false(既定)なら Authorization Code + PKCE true なら device code を使う(D12)
func Start(ctx context.Context, session awsconfig.SSOSession, opts Options) (*Flow, error) {
	if opts.Client == nil {
		return nil, errors.New("ssologin: Options.Client が未設定です")
	}
	if session.StartURL == "" {
		return nil, errors.New("ssologin: sso-session に start URL がありません: ~/.aws/config を確認してください")
	}

	cacheDir := opts.CacheDir
	if cacheDir == "" {
		dir, err := ssocache.DefaultCacheDir()
		if err != nil {
			return nil, err
		}
		cacheDir = dir
	}

	clientName := opts.ClientName
	if clientName == "" {
		clientName = defaultClientName
	}

	tokenPath := ssocache.TokenPath(cacheDir, session.CacheKey())

	clientID, clientSecret, registrationExpiresAt, err := registerClient(ctx, opts.Client, clientName, session, opts.UseDeviceCode)
	if err != nil {
		return nil, err
	}

	if opts.UseDeviceCode {
		return startDeviceFlow(ctx, opts, session, tokenPath, clientID, clientSecret, registrationExpiresAt)
	}
	return startAuthCodeFlow(opts, session, tokenPath, clientID, clientSecret, registrationExpiresAt)
}

// startDeviceFlow は StartDeviceAuthorization を呼び device code 用の Flow を組み立てる
func startDeviceFlow(
	ctx context.Context,
	opts Options,
	session awsconfig.SSOSession,
	tokenPath, clientID, clientSecret string,
	registrationExpiresAt time.Time,
) (*Flow, error) {
	authOutput, err := opts.Client.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     &clientID,
		ClientSecret: &clientSecret,
		StartUrl:     &session.StartURL,
	})
	if err != nil {
		return nil, fmt.Errorf("デバイス認可の開始に失敗: %w", err)
	}

	interval := time.Duration(authOutput.Interval) * time.Second
	if interval <= 0 {
		interval = defaultPollInterval
	}

	authURL := strVal(authOutput.VerificationUriComplete)
	if authURL == "" {
		authURL = strVal(authOutput.VerificationUri)
	}

	flow := &Flow{
		Method:                MethodDeviceCode,
		AuthorizationURL:      authURL,
		UserCode:              strVal(authOutput.UserCode),
		ExpiresAt:             time.Now().Add(time.Duration(authOutput.ExpiresIn) * time.Second),
		Interval:              interval,
		client:                opts.Client,
		session:               session,
		tokenPath:             tokenPath,
		clientID:              clientID,
		clientSecret:          clientSecret,
		deviceCode:            strVal(authOutput.DeviceCode),
		registrationExpiresAt: registrationExpiresAt,
	}

	openBrowserAt(opts, flow, flow.AuthorizationURL)
	return flow, nil
}

// startAuthCodeFlow はローカルコールバックサーバーを起動し PKCE 用の Flow を組み立てる(D12)
// listen をブラウザ起動より先に始める(コールバックの取りこぼしを防ぐ)
func startAuthCodeFlow(
	opts Options,
	session awsconfig.SSOSession,
	tokenPath, clientID, clientSecret string,
	registrationExpiresAt time.Time,
) (*Flow, error) {
	listenAddr := opts.ListenAddr
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}

	callback, err := startCallbackServer(listenAddr)
	if err != nil {
		return nil, err
	}

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		callback.close()
		return nil, fmt.Errorf("PKCE の code_verifier 生成に失敗: %w", err)
	}

	state, err := newState()
	if err != nil {
		callback.close()
		return nil, fmt.Errorf("state の生成に失敗: %w", err)
	}

	redirectURI := callback.redirectURIWithPort()
	authorizationURL := buildAuthorizationURL(opts, session, authorizeURLParams{
		clientID:      clientID,
		redirectURI:   redirectURI,
		state:         state,
		codeChallenge: computeCodeChallenge(codeVerifier),
		scopes:        session.RegistrationScopes,
	})

	flow := &Flow{
		Method:                MethodAuthorizationCode,
		AuthorizationURL:      authorizationURL,
		ExpiresAt:             time.Now().Add(authCodeOverallTimeout),
		client:                opts.Client,
		session:               session,
		tokenPath:             tokenPath,
		clientID:              clientID,
		clientSecret:          clientSecret,
		registrationExpiresAt: registrationExpiresAt,
		codeVerifier:          codeVerifier,
		state:                 state,
		redirectURI:           redirectURI,
		callback:              callback,
	}

	openBrowserAt(opts, flow, authorizationURL)
	return flow, nil
}

// openBrowserAt は認可 URL をブラウザで開く 失敗したら flow.BrowserErr に理由を残す
func openBrowserAt(opts Options, flow *Flow, authURL string) {
	if authURL == "" {
		return
	}
	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = defaultOpenBrowser
	}
	if err := openBrowser(authURL); err != nil {
		flow.BrowserErr = err
	}
}

// Wait は承認完了まで待ち成功したらトークンファイルを書く 方式は f.Method で分岐する
// Method が空(直接組み立てた Flow など)は device code として扱う
func (f *Flow) Wait(ctx context.Context) (Result, error) {
	if f.Method == MethodAuthorizationCode {
		return f.waitAuthCode(ctx)
	}
	return f.waitDeviceCode(ctx)
}

// waitDeviceCode は device code の CreateToken をポーリングする
// ctx の期限切れ AuthorizationPendingException 以外の失敗系はすべてエラーを返す
func (f *Flow) waitDeviceCode(ctx context.Context) (Result, error) {
	sleep := f.sleepFunc
	if sleep == nil {
		sleep = sleepContext
	}

	interval := f.Interval
	if interval < 0 {
		interval = 0
	}

	for {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("ログイン待ちがタイムアウトしました: %w", err)
		}
		if time.Now().After(f.ExpiresAt) {
			return Result{}, errors.New("認可コードの有効期限が切れました: 'awsp login' をやり直してください")
		}

		output, err := f.client.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     &f.clientID,
			ClientSecret: &f.clientSecret,
			GrantType:    strPtr(deviceGrantType),
			DeviceCode:   &f.deviceCode,
		})
		if err == nil {
			return f.persistToken(output)
		}

		var pending *ssooidctypes.AuthorizationPendingException
		if errors.As(err, &pending) {
			if sleepErr := sleep(ctx, interval); sleepErr != nil {
				return Result{}, sleepErr
			}
			continue
		}

		var slowDown *ssooidctypes.SlowDownException
		if errors.As(err, &slowDown) {
			interval += 5 * time.Second
			if sleepErr := sleep(ctx, interval); sleepErr != nil {
				return Result{}, sleepErr
			}
			continue
		}

		if msg, ok := describeTokenError(err); ok {
			return Result{}, errors.New(msg)
		}
		return Result{}, fmt.Errorf("トークン取得に失敗: %w", err)
	}
}

// waitAuthCode はローカルコールバックの受信を待ち CreateToken(authorization_code)まで行う(D12)
func (f *Flow) waitAuthCode(ctx context.Context) (Result, error) {
	defer f.callback.close()

	var deadline <-chan time.Time
	if !f.ExpiresAt.IsZero() {
		d := time.Until(f.ExpiresAt)
		if d < 0 {
			d = 0
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case result := <-f.callback.resultCh:
		if result.err != nil {
			return Result{}, result.err
		}
		if result.state != f.state {
			return Result{}, errors.New("state が一致しません: 'awsp login' をやり直してください")
		}
		return f.exchangeAuthCode(ctx, result.code)

	case <-ctx.Done():
		return Result{}, fmt.Errorf("ログイン待ちがタイムアウトしました: %w", ctx.Err())

	case <-deadline:
		return Result{}, errors.New("認可の待ち上限(10分)を超えました: 'awsp login' をやり直してください")
	}
}

// exchangeAuthCode は認可コードを CreateToken(authorization_code)でトークンに交換する
func (f *Flow) exchangeAuthCode(ctx context.Context, code string) (Result, error) {
	output, err := f.client.CreateToken(ctx, &ssooidc.CreateTokenInput{
		ClientId:     &f.clientID,
		ClientSecret: &f.clientSecret,
		GrantType:    strPtr(authCodeGrantType),
		Code:         strPtr(code),
		RedirectUri:  strPtr(f.redirectURI),
		CodeVerifier: strPtr(f.codeVerifier),
	})
	if err != nil {
		if msg, ok := describeTokenError(err); ok {
			return Result{}, errors.New(msg)
		}
		return Result{}, fmt.Errorf("トークン取得に失敗: %w", err)
	}
	return f.persistToken(output)
}

// describeTokenError は CreateToken の既知のエラーを利用者向けメッセージに変換する
// 変換できた場合だけ ok=true を返す
func describeTokenError(err error) (message string, ok bool) {
	var expired *ssooidctypes.ExpiredTokenException
	if errors.As(err, &expired) {
		return "承認の期限切れです: 'awsp login' をやり直してください", true
	}
	var denied *ssooidctypes.AccessDeniedException
	if errors.As(err, &denied) {
		return "ログイン承認が拒否されました: 'awsp login' をやり直してください", true
	}
	return "", false
}

func (f *Flow) persistToken(output *ssooidc.CreateTokenOutput) (Result, error) {
	expiresAt := time.Now().UTC().Add(time.Duration(output.ExpiresIn) * time.Second)

	file := tokenFile{
		StartURL:              f.session.StartURL,
		Region:                f.session.Region,
		AccessToken:           strVal(output.AccessToken),
		ExpiresAt:             expiresAt.Format(timeLayout),
		ClientID:              f.clientID,
		ClientSecret:          f.clientSecret,
		RegistrationExpiresAt: f.registrationExpiresAt.UTC().Format(timeLayout),
		RefreshToken:          strVal(output.RefreshToken),
	}

	if err := writeTokenFileAtomic(f.tokenPath, file); err != nil {
		return Result{}, err
	}

	return Result{
		ExpiresAt:       expiresAt,
		HasRefreshToken: file.RefreshToken != "",
	}, nil
}

// tokenFile は ~/.aws/sso/cache のトークンキャッシュファイルの形式
// aws CLI / SDK と完全互換にする
type tokenFile struct {
	StartURL              string `json:"startUrl"`
	Region                string `json:"region"`
	AccessToken           string `json:"accessToken,omitempty"` //nolint:gosec // aws CLI 互換のキャッシュ形式に必須
	ExpiresAt             string `json:"expiresAt,omitempty"`
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"` //nolint:gosec // aws CLI 互換のキャッシュ形式に必須
	RegistrationExpiresAt string `json:"registrationExpiresAt"`
	RefreshToken          string `json:"refreshToken,omitempty"` //nolint:gosec // aws CLI 互換のキャッシュ形式に必須
}

// registerClient は OIDC クライアントを毎回新規登録する(device / PKCE 共通の入口)
//
// 既存トークンファイルの clientId / clientSecret を再利用しないのは意図的
// PKCE は redirect URI のポートが毎回変わるため登録の使い回しができない(D12)
// device code 側も同じ理由(旧実装からの経緯)で新規登録のままにしてある
// RegisterClient は無認証で即時に完了しコストは無い
//
// useDeviceCode=false(PKCE 既定)のときは grantTypes/redirectUris/issuerUrl を付けて登録する
// これらを付けない登録は device code と refresh_token の両方に使える(旧 CLI と同じ形)
func registerClient(
	ctx context.Context,
	client OIDCClient,
	clientName string,
	session awsconfig.SSOSession,
	useDeviceCode bool,
) (clientID string, clientSecret string, registrationExpiresAt time.Time, err error) {
	input := &ssooidc.RegisterClientInput{
		ClientName: strPtr(clientName),
		ClientType: strPtr(publicClientType),
		Scopes:     session.RegistrationScopes,
	}
	if !useDeviceCode {
		input.GrantTypes = authGrantTypes
		input.RedirectUris = []string{redirectURIWithoutPort}
		input.IssuerUrl = strPtr(session.StartURL)
	}

	output, registerErr := client.RegisterClient(ctx, input)
	if registerErr != nil {
		return "", "", time.Time{}, fmt.Errorf("OIDC クライアント登録に失敗: %w", registerErr)
	}

	return strVal(output.ClientId), strVal(output.ClientSecret), time.Unix(output.ClientSecretExpiresAt, 0).UTC(), nil
}

// authorizeURLParams は buildAuthorizationURL の入力
type authorizeURLParams struct {
	clientID      string
	redirectURI   string
	state         string
	codeChallenge string
	scopes        []string
}

// buildAuthorizationURL は PKCE の認可 URL を組み立てる
// クエリキーは AWS 独自の "scopes"(複数形)を含め aws CLI(SSOTokenFetcherAuth)と同じ形にする
func buildAuthorizationURL(opts Options, session awsconfig.SSOSession, p authorizeURLParams) string {
	base := strings.TrimSuffix(opts.AuthorizeBaseURL, "/")
	if base == "" {
		base = defaultAuthorizeBaseURL(session.Region)
	}

	query := url.Values{}
	query.Set("response_type", "code")
	query.Set("client_id", p.clientID)
	query.Set("redirect_uri", p.redirectURI)
	query.Set("state", p.state)
	query.Set("code_challenge_method", "S256")
	query.Set("scopes", strings.Join(p.scopes, " "))
	query.Set("code_challenge", p.codeChallenge)

	return base + "/authorize?" + query.Encode()
}

// defaultAuthorizeBaseURL は sso_region から認可エンドポイントのベース URL を導出する
// 中国リージョン(cn- 始まり)は .amazonaws.com.cn を使う
func defaultAuthorizeBaseURL(region string) string {
	domain := "amazonaws.com"
	if strings.HasPrefix(region, "cn-") {
		domain = "amazonaws.com.cn"
	}
	return fmt.Sprintf("https://oidc.%s.%s", region, domain)
}

// generateCodeVerifier は PKCE の code_verifier を crypto/rand で生成する
// [A-Za-z0-9-._~] から 64 文字 剰余バイアスを避けるため棄却法で選ぶ
func generateCodeVerifier() (string, error) {
	maxByte := byte(256 - (256 % len(codeVerifierAlphabet)))

	result := make([]byte, 0, codeVerifierLength)
	buf := make([]byte, codeVerifierLength)
	for len(result) < codeVerifierLength {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b >= maxByte {
				continue
			}
			result = append(result, codeVerifierAlphabet[int(b)%len(codeVerifierAlphabet)])
			if len(result) == codeVerifierLength {
				break
			}
		}
	}
	return string(result), nil
}

// computeCodeChallenge は PKCE の code_challenge(S256)を計算する
// base64url でパディング("=")を含めない(Go は RawURLEncoding)
func computeCodeChallenge(codeVerifier string) string {
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// newState は認可 URL の state パラメータに使うランダムな UUID v4 文字列を生成する
func newState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// callbackResult は PKCE のローカルコールバックサーバーが受け取った 1 回分の結果
type callbackResult struct {
	code  string
	state string
	err   error
}

// callbackServer は PKCE のリダイレクトを受けるローカル HTTP サーバー
// ブラウザを開く前に listen を始め 1 回リクエストを受けたら閉じる
type callbackServer struct {
	listener  net.Listener
	server    *http.Server
	resultCh  chan callbackResult
	closeOnce sync.Once
}

// startCallbackServer は addr で listen し PKCE のコールバックを待つサーバーを起動する
func startCallbackServer(addr string) (*callbackServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ローカルコールバックサーバーの起動に失敗: %w", err)
	}

	cs := &callbackServer{
		listener: listener,
		resultCh: make(chan callbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(redirectPath, cs.handle)
	cs.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = cs.server.Serve(listener)
	}()

	return cs, nil
}

// port はローカルコールバックサーバーが実際に listen しているポート番号を返す
func (c *callbackServer) port() int {
	if tcpAddr, ok := c.listener.Addr().(*net.TCPAddr); ok {
		return tcpAddr.Port
	}
	return 0
}

// redirectURIWithPort はポート付きの redirect URI を返す 認可 URL と CreateToken に使う
func (c *callbackServer) redirectURIWithPort() string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", c.port(), redirectPath)
}

// handle は OAuth コールバックを処理する
// error クエリがあれば失敗 code と state があれば取り込む それ以外(favicon 等)は無視する
func (c *callbackServer) handle(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	switch {
	case query.Get("error") != "":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, failureCallbackPage())
		c.send(callbackResult{err: fmt.Errorf("認可が拒否されました: %s", query.Get("error"))})

	case query.Get("code") != "":
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, successCallbackPage())
		c.send(callbackResult{code: query.Get("code"), state: query.Get("state")})

	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, unexpectedCallbackPage())
	}
}

// send は 1 回だけ resultCh へ結果を送る(バッファ 1) 2 回目以降のリクエストは捨てる
func (c *callbackServer) send(result callbackResult) {
	select {
	case c.resultCh <- result:
	default:
	}
}

func (c *callbackServer) close() {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = c.server.Shutdown(ctx)
	})
}

func writeTokenFileAtomic(path string, file tokenFile) (err error) {
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return fmt.Errorf("sso トークンキャッシュ用ディレクトリを作成できません: %s: %w", dir, mkErr)
	}

	data, marshalErr := json.MarshalIndent(file, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("sso トークンキャッシュのエンコードに失敗: %w", marshalErr)
	}

	tmp, createErr := os.CreateTemp(dir, ".awsp-token-*.tmp")
	if createErr != nil {
		return fmt.Errorf("sso トークンキャッシュの一時ファイルを作成できません: %w", createErr)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if chmodErr := tmp.Chmod(0o600); chmodErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("sso トークンキャッシュの権限設定に失敗: %w", chmodErr)
	}
	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("sso トークンキャッシュの書き込みに失敗: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("sso トークンキャッシュのクローズに失敗: %w", closeErr)
	}

	if renameErr := os.Rename(tmpPath, path); renameErr != nil { //nolint:gosec // path は ssocache.TokenPath が算出する sha1 済み固定パス
		return fmt.Errorf("sso トークンキャッシュの確定に失敗: %w", renameErr)
	}
	renamed = true

	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("ログイン待ちがタイムアウトしました: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// defaultOpenBrowser は OS 標準のコマンドでブラウザを起動する
func defaultOpenBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL) //nolint:gosec // 固定バイナリ 引数は AWS が返す検証 URL
	case "linux":
		cmd = exec.Command("xdg-open", rawURL) //nolint:gosec // 固定バイナリ 引数は AWS が返す検証 URL
	default:
		return fmt.Errorf("このOSではブラウザの自動起動に対応していません: %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// 終了を待たないがゾンビにしないため回収だけ行う
	go func() { _ = cmd.Wait() }()
	return nil
}

func strPtr(v string) *string {
	return &v
}

func strVal(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
