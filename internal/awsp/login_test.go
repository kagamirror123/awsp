package awsp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

// writeValidToken は有効期限内のトークンキャッシュを書く
func writeValidToken(t *testing.T, cacheDir string, key string) {
	t.Helper()

	body := `{"startUrl":"https://example.awsapps.com/start","region":"us-west-2",` +
		`"accessToken":"dummy","expiresAt":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}`
	if err := os.WriteFile(ssocache.TokenPath(cacheDir, key), []byte(body), 0o600); err != nil {
		t.Fatalf("トークンキャッシュを書けません: %v", err)
	}
}
