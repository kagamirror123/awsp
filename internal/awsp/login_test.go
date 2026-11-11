package awsp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

func TestResolveLoginSession_ByProfile(t *testing.T) {
	t.Parallel()

	profiles := []awsconfig.Profile{
		{Name: "dev", SSOSession: "corp"},
	}
	sessions := []awsconfig.SSOSession{
		{Name: "corp", StartURL: "https://example.awsapps.com/start", Region: "us-west-2"},
	}

	session, profile, err := ResolveLoginSession(LoginTarget{Profile: "dev"}, profiles, sessions)
	if err != nil {
		t.Fatalf("ResolveLoginSession が失敗: %v", err)
	}
	if profile != "dev" {
		t.Fatalf("profile が想定外: %s", profile)
	}
	if session.CacheKey() != "corp" {
		t.Fatalf("session が想定外: %+v", session)
	}
}

func TestResolveLoginSession_ProfileWithoutSSO(t *testing.T) {
	t.Parallel()

	profiles := []awsconfig.Profile{
		{Name: "static", RoleARN: "arn:aws:iam::123456789012:role/x", SourceProfile: "base"},
	}

	_, _, err := ResolveLoginSession(LoginTarget{Profile: "static"}, profiles, nil)
	if err == nil {
		t.Fatal("SSO を使わない profile なのにエラーにならなかった")
	}
}

func TestResolveLoginSession_UnknownProfile(t *testing.T) {
	t.Parallel()

	_, _, err := ResolveLoginSession(LoginTarget{Profile: "missing"}, nil, nil)
	if err == nil {
		t.Fatal("存在しない profile なのにエラーにならなかった")
	}
}

func TestResolveLoginSession_BySessionName(t *testing.T) {
	t.Parallel()

	sessions := []awsconfig.SSOSession{
		{Name: "corp", StartURL: "https://example.awsapps.com/start"},
	}

	session, profile, err := ResolveLoginSession(LoginTarget{SSOSessionName: "corp"}, nil, sessions)
	if err != nil {
		t.Fatalf("ResolveLoginSession が失敗: %v", err)
	}
	if profile != "" {
		t.Fatalf("profile が空でない: %s", profile)
	}
	if session.CacheKey() != "corp" {
		t.Fatalf("session が想定外: %+v", session)
	}
}

func TestResolveLoginSession_UnknownSessionName(t *testing.T) {
	t.Parallel()

	_, _, err := ResolveLoginSession(LoginTarget{SSOSessionName: "missing"}, nil, nil)
	if err == nil {
		t.Fatal("存在しない sso-session なのにエラーにならなかった")
	}
}

func TestResolveLoginSession_NoTarget_SingleSession(t *testing.T) {
	t.Parallel()

	sessions := []awsconfig.SSOSession{
		{Name: "corp", StartURL: "https://example.awsapps.com/start"},
	}

	session, profile, err := ResolveLoginSession(LoginTarget{}, nil, sessions)
	if err != nil {
		t.Fatalf("ResolveLoginSession が失敗: %v", err)
	}
	if profile != "" {
		t.Fatalf("profile が空でない: %s", profile)
	}
	if session.CacheKey() != "corp" {
		t.Fatalf("session が想定外: %+v", session)
	}
}

func TestResolveLoginSession_NoTarget_MultipleSessions(t *testing.T) {
	t.Parallel()

	sessions := []awsconfig.SSOSession{
		{Name: "corp-a", StartURL: "https://a.awsapps.com/start"},
		{Name: "corp-b", StartURL: "https://b.awsapps.com/start"},
	}

	_, _, err := ResolveLoginSession(LoginTarget{}, nil, sessions)
	if err == nil {
		t.Fatal("複数 sso-session があるのにエラーにならなかった")
	}
	if !strings.Contains(err.Error(), "corp-a") || !strings.Contains(err.Error(), "corp-b") {
		t.Fatalf("候補が列挙されていない: %v", err)
	}
}

func TestResolveLoginSession_NoTarget_NoSessions(t *testing.T) {
	t.Parallel()

	_, _, err := ResolveLoginSession(LoginTarget{}, nil, nil)
	if err == nil {
		t.Fatal("sso-session が無いのにエラーにならなかった")
	}
}

func TestResolveLoginSession_ProfileAndSessionConflict(t *testing.T) {
	t.Parallel()

	_, _, err := ResolveLoginSession(LoginTarget{Profile: "dev", SSOSessionName: "corp"}, nil, nil)
	if err == nil {
		t.Fatal("profile と --sso-session の同時指定でエラーにならなかった")
	}
}

// Force はログイン済みでもフローを起こし直す(--force)
func TestLogin_ForceStartsFlowEvenWhenSessionIsValid(t *testing.T) {
	session := awsconfig.SSOSession{Name: "corp", StartURL: "https://example.awsapps.com/start", Region: "us-west-2"}
	cacheDir := t.TempDir()
	writeValidToken(t, cacheDir, session.CacheKey())

	t.Run("既定では OIDC を呼ばない", func(t *testing.T) {
		called := false
		_, err := Login(context.Background(), session, "", LoginDeps{}, LoginOptions{
			CacheDir: cacheDir,
			NewOIDCClient: func(string) ssologin.OIDCClient {
				called = true
				return nil
			},
		})
		if err != nil {
			t.Fatalf("有効なセッションで失敗した: %v", err)
		}
		if called {
			t.Fatal("有効なセッションなのにログインフローを起こしている")
		}
	})

	t.Run("Force なら OIDC を呼ぶ", func(t *testing.T) {
		called := false
		_, _ = Login(context.Background(), session, "", LoginDeps{}, LoginOptions{
			CacheDir: cacheDir,
			Force:    true,
			NewOIDCClient: func(region string) ssologin.OIDCClient {
				called = true
				if region != session.Region {
					t.Errorf("region が渡っていない: %s", region)
				}
				// クライアントを返さずに Start を失敗させる(ネットワークへ出ない)
				return nil
			},
		})
		if !called {
			t.Fatal("Force なのにログインフローを起こしていない")
		}
	})
}

func TestLogin_PicksUpLoginByAnotherProcess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		force   bool
		wantErr bool
	}{
		{name: "承認待ちの間に書かれたトークンを使う"},
		{name: "Force では打ち切らない", force: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			session := awsconfig.SSOSession{Name: "corp", StartURL: "https://example.invalid/start", Region: "us-west-2"}
			cacheDir := t.TempDir()
			result, err := Login(context.Background(), session, "", LoginDeps{}, LoginOptions{
				CacheDir: cacheDir,
				Force:    tc.force,
				Timeout:  time.Second,
				// 認可画面を開いた直後に 別のプロセスがログインを済ませてキャッシュを書いた想定
				OpenBrowser: func(string) error {
					writeValidToken(t, cacheDir, session.CacheKey())
					return nil
				},
				NewOIDCClient:      func(string) ssologin.OIDCClient { return &recoveryOIDC{} },
				cacheWatchInterval: 10 * time.Millisecond,
			})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if !tc.wantErr && (result.Status != "ok" || result.ExpiresAt == nil || !result.ExpiresAt.After(time.Now())) {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestLogin_RefreshesBeforeOpeningBrowser(t *testing.T) {
	for _, tc := range []struct {
		name        string
		refreshErr  error
		wantBrowser bool
	}{
		{name: "更新できればブラウザを開かない"},
		{name: "更新できなければブラウザを開く", refreshErr: &ssooidctypes.InvalidGrantException{}, wantBrowser: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			session := awsconfig.SSOSession{Name: "corp", StartURL: "https://example.invalid/start", Region: "us-west-2"}
			cacheDir := t.TempDir()
			writeExpiredToken(t, cacheDir, session.CacheKey())
			opened := false
			result, err := Login(context.Background(), session, "", LoginDeps{}, LoginOptions{
				CacheDir:      cacheDir,
				Timeout:       time.Second,
				OpenBrowser:   func(string) error { opened = true; return nil },
				NewOIDCClient: func(string) ssologin.OIDCClient { return &refreshOIDC{refreshErr: tc.refreshErr} },
			})
			if opened != tc.wantBrowser {
				t.Fatalf("opened=%v err=%v", opened, err)
			}
			if !tc.wantBrowser && (err != nil || result.Status != "ok" || result.ExpiresAt == nil || !result.ExpiresAt.After(time.Now())) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

// refreshOIDC は refresh_token grant だけを失敗させられる
type refreshOIDC struct {
	recoveryOIDC
	refreshErr error
}

func (c *refreshOIDC) CreateToken(ctx context.Context, in *ssooidc.CreateTokenInput, optFns ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	if in.GrantType != nil && *in.GrantType == "refresh_token" && c.refreshErr != nil {
		return nil, c.refreshErr
	}
	return c.recoveryOIDC.CreateToken(ctx, in, optFns...)
}

func TestDualStackState(t *testing.T) {
	t.Parallel()

	appAWS := awsconfig.SSOSession{StartURL: "https://ssoins-0123456789abcdef.portal.us-west-2.app.aws"}
	awsapps := awsconfig.SSOSession{StartURL: "https://d-0123456789.awsapps.com/start"}
	for _, tc := range []struct {
		name    string
		session awsconfig.SSOSession
		env     map[string]string
		want    aws.DualStackEndpointState
	}{
		{name: "app.aws のポータルは dual-stack", session: appAWS, want: aws.DualStackEndpointStateEnabled},
		{name: "awsapps.com のポータルは既定のまま", session: awsapps, want: aws.DualStackEndpointStateUnset},
		{name: "環境変数で有効にできる", session: awsapps, env: map[string]string{"AWS_USE_DUALSTACK_ENDPOINT": "true"}, want: aws.DualStackEndpointStateEnabled},
		{name: "環境変数で無効にできる", session: appAWS, env: map[string]string{"AWS_USE_DUALSTACK_ENDPOINT": "false"}, want: aws.DualStackEndpointStateDisabled},
		{name: "読めない値は無視する", session: appAWS, env: map[string]string{"AWS_USE_DUALSTACK_ENDPOINT": "yes please"}, want: aws.DualStackEndpointStateEnabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lookup := func(key string) (string, bool) {
				value, ok := tc.env[key]
				return value, ok
			}
			if got := dualStackState(tc.session, lookup); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// app.aws のポータルなら dual-stack の認可 URL(oidc.<region>.api.aws)を開く(D37)
func TestLogin_OpensDualStackAuthorizeURLForAppAWSPortal(t *testing.T) {
	if _, ok := os.LookupEnv("AWS_USE_DUALSTACK_ENDPOINT"); ok {
		t.Skip("AWS_USE_DUALSTACK_ENDPOINT が設定されている")
	}

	session := awsconfig.SSOSession{Name: "corp", StartURL: "https://ssoins-0123456789abcdef.portal.us-west-2.app.aws", Region: "us-west-2"}
	opened := ""
	_, _ = Login(context.Background(), session, "", LoginDeps{}, LoginOptions{
		CacheDir:      t.TempDir(),
		Timeout:       time.Second,
		OpenBrowser:   func(rawURL string) error { opened = rawURL; return nil },
		NewOIDCClient: func(string) ssologin.OIDCClient { return &recoveryOIDC{} },
	})
	if !strings.HasPrefix(opened, "https://oidc.us-west-2.api.aws/authorize?") {
		t.Fatalf("opened=%s", opened)
	}
}

// writeExpiredToken は期限切れで refreshToken のあるトークンキャッシュを書く
func writeExpiredToken(t *testing.T, cacheDir string, key string) {
	t.Helper()

	body := `{"startUrl":"https://example.invalid/start","region":"us-west-2","accessToken":"dummy",` +
		`"clientId":"test-client","clientSecret":"test-secret","refreshToken":"dummy-refresh",` +
		`"expiresAt":"` + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339) + `"}`
	if err := os.WriteFile(ssocache.TokenPath(cacheDir, key), []byte(body), 0o600); err != nil {
		t.Fatalf("トークンキャッシュを書けません: %v", err)
	}
}

// writeValidToken は有効期限内のトークンキャッシュを書く
func writeValidToken(t *testing.T, cacheDir string, key string) {
	t.Helper()

	body := `{"startUrl":"https://example.awsapps.com/start","region":"us-west-2",` +
		`"accessToken":"dummy","expiresAt":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}`
	if err := os.WriteFile(ssocache.TokenPath(cacheDir, key), []byte(body), 0o600); err != nil {
		t.Fatalf("トークンキャッシュを書けません: %v", err)
	}
}
