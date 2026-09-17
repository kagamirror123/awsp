// Package ssologin は AWS IAM Identity Center の OIDC ログインフローを実装する
// aws CLI を exec せず AWS SDK for Go v2(ssooidc)だけでブラウザ認証を完結させる(D12)
// 既定は Authorization Code + PKCE(localhost へのリダイレクト) device code は opt-in
// 書き出すトークンキャッシュは aws CLI / SDK と互換の形式にする
package ssologin

import (
	"context"
	"errors"
	"fmt"
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

// Wait は承認完了まで待ち成功したらトークンファイルを書く 方式は f.Method で分岐する
// Method が空(直接組み立てた Flow など)は device code として扱う
func (f *Flow) Wait(ctx context.Context) (Result, error) {
	if f.Method == MethodAuthorizationCode {
		return f.waitAuthCode(ctx)
	}
	return f.waitDeviceCode(ctx)
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

func strPtr(v string) *string {
	return &v
}

func strVal(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
