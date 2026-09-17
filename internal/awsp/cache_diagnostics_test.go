package awsp

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

func TestCacheFailureIsLocalized(t *testing.T) {
	t.Parallel()
	cache, roles := t.TempDir(), t.TempDir()
	profiles := []awsconfig.Profile{{Name: "broken", SSOSession: "broken", SSOAccountID: "123", SSORoleName: "ReadOnly"}, {Name: "healthy", SSOSession: "healthy"}, {Name: "static"}}
	sessions := []awsconfig.SSOSession{{Name: "broken"}, {Name: "healthy"}}
	writeTokenFixture(t, cache, "broken", "{")
	writeValidToken(t, cache, "healthy")
	if err := os.WriteFile(ssocache.RoleCredentialPath(roles, ssocache.RoleCredentialKey{SessionName: "broken", AccountID: "123", RoleName: "ReadOnly"}), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := BuildProfileList(profiles, sessions, ProfileListOptions{SSOCacheDir: cache, CLICacheDir: roles})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Profiles) != 3 || list.Profiles[0].SessionState != ssocache.StateError || len(list.Profiles[0].Diagnostics) != 2 || list.Profiles[0].SessionExpiresAt != nil || list.Profiles[1].SessionState != ssocache.StateOK || list.Profiles[2].SessionState != "" {
		t.Fatalf("list=%+v", list)
	}
	report, err := BuildStatusReport(profiles, sessions, StatusOptions{CacheDir: cache})
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall != ssocache.StateError || len(report.Sessions) != 2 || report.Sessions[0].Diagnostic == "" || report.Sessions[0].ExpiresAt != nil || report.Sessions[1].State != ssocache.StateOK {
		t.Fatalf("report=%+v", report)
	}
	line, code := PreflightLine(report)
	if code != 1 || !strings.Contains(line, "キャッシュエラー") || strings.Contains(line, "0s 前") {
		t.Fatalf("preflight=%s code=%d", line, code)
	}
}

func TestLegacyTokenCannotPromiseRefresh(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	cache := t.TempDir()
	profile := awsconfig.Profile{Name: "legacy", SSOStartURL: "https://example.invalid/start", SSORegion: "us-west-2"}
	writeTokenFixture(t, cache, profile.SSOStartURL, `{"expiresAt":"`+now.Add(-time.Minute).Format(time.RFC3339)+`","refreshToken":"test"}`)
	report, err := BuildStatusReport([]awsconfig.Profile{profile}, nil, StatusOptions{CacheDir: cache, Now: now, Grace: 8 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, code := PreflightLine(report); code != 1 || report.Overall != ssocache.StateError {
		t.Fatalf("report=%+v", report)
	}
	list, err := BuildProfileList([]awsconfig.Profile{profile}, nil, ProfileListOptions{SSOCacheDir: cache, Now: now, Grace: 8 * time.Hour})
	if err != nil || list.Profiles[0].SessionState != ssocache.StateError {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}
