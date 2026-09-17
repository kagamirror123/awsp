package ssologin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/kagamirror123/awsp/internal/awsconfig"
)

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

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("PKCE の code_verifier 生成に失敗: %w", err)
	}
	state, err := newState()
	if err != nil {
		return nil, fmt.Errorf("state の生成に失敗: %w", err)
	}
	callback, err := startCallbackServer(listenAddr, state)
	if err != nil {
		return nil, err
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
