package awsp

import (
	"strings"
	"testing"

	"github.com/kagamirror123/awsp/internal/awsconfig"
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
