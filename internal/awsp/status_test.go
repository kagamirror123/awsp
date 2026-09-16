package awsp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

func writeTokenFixture(t *testing.T, cacheDir string, key string, content string) {
	t.Helper()
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("cacheDir 作成に失敗: %v", err)
	}
	path := ssocache.TokenPath(cacheDir, key)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("トークンファイル作成に失敗: %v", err)
	}
}

func TestBuildStatusReport_SingleSessionOK(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	writeTokenFixture(t, cacheDir, "corp-sso", `{
		"startUrl": "https://example.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "dummy",
		"expiresAt": "2026-09-16T12:52:00Z"
	}`)

	profiles := []awsconfig.Profile{
		{Name: "dev", SSOSession: "corp-sso", SSOAccountID: "123456789012", SSORoleName: "AdministratorAccess"},
	}
	sessions := []awsconfig.SSOSession{
		{Name: "corp-sso", StartURL: "https://example.awsapps.com/start", Region: "us-west-2"},
	}

	report, err := BuildStatusReport(profiles, sessions, StatusOptions{
		ConfigFile: "/tmp/config",
		CacheDir:   cacheDir,
		Grace:      8 * time.Hour,
		Now:        now,
	})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}

	if report.Overall != ssocache.StateOK {
		t.Fatalf("Overall が想定外: %s", report.Overall)
	}
	if len(report.Sessions) != 1 {
		t.Fatalf("Sessions 件数が想定外: %d", len(report.Sessions))
	}
	session := report.Sessions[0]
	if session.State != ssocache.StateOK {
		t.Fatalf("session.State が想定外: %s", session.State)
	}
	if len(session.Profiles) != 1 || session.Profiles[0] != "dev" {
		t.Fatalf("session.Profiles が想定外: %v", session.Profiles)
	}
	if session.RemainingSeconds == nil || *session.RemainingSeconds != 52*60 {
		t.Fatalf("RemainingSeconds が想定外: %v", session.RemainingSeconds)
	}

	line, exitCode := PreflightLine(report)
	if exitCode != 0 {
		t.Fatalf("exitCode が想定外: %d", exitCode)
	}
	want := "awsp preflight: AWS SSO 有効(corp-sso 残り 52m)"
	if line != want {
		t.Fatalf("PreflightLine が想定外\nwant=%s\ngot=%s", want, line)
	}
}

func TestBuildStatusReport_ErrorState(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	writeTokenFixture(t, cacheDir, "corp-sso", `{
		"startUrl": "https://example.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "dummy",
		"expiresAt": "2026-09-16T01:00:00Z"
	}`)

	profiles := []awsconfig.Profile{
		{Name: "dev", SSOSession: "corp-sso"},
	}
	sessions := []awsconfig.SSOSession{
		{Name: "corp-sso", StartURL: "https://example.awsapps.com/start", Region: "us-west-2"},
	}

	report, err := BuildStatusReport(profiles, sessions, StatusOptions{
		CacheDir: cacheDir,
		Grace:    8 * time.Hour,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}
	if report.Overall != ssocache.StateError {
		t.Fatalf("Overall が想定外: %s", report.Overall)
	}

	line, exitCode := PreflightLine(report)
	if exitCode != 1 {
		t.Fatalf("exitCode が想定外: %d", exitCode)
	}
	want := "awsp preflight: AWS SSO 失効(corp-sso 11h 前)。AWS を使う前に 'awsp login --sso-session corp-sso' を実行してください"
	if line != want {
		t.Fatalf("PreflightLine が想定外\nwant=%s\ngot=%s", want, line)
	}
}

func TestBuildStatusReport_UnknownState(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()

	profiles := []awsconfig.Profile{
		{Name: "dev", SSOSession: "corp-sso"},
	}
	sessions := []awsconfig.SSOSession{
		{Name: "corp-sso", StartURL: "https://example.awsapps.com/start", Region: "us-west-2"},
	}

	report, err := BuildStatusReport(profiles, sessions, StatusOptions{
		CacheDir: cacheDir,
		Grace:    8 * time.Hour,
		Now:      time.Now(),
	})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}
	if report.Overall != ssocache.StateUnknown {
		t.Fatalf("Overall が想定外: %s", report.Overall)
	}

	line, exitCode := PreflightLine(report)
	if exitCode != 1 {
		t.Fatalf("exitCode が想定外: %d", exitCode)
	}
	want := "awsp preflight: AWS SSO 未ログイン(corp-sso)。AWS を使う前に 'awsp login --sso-session corp-sso' を実行してください"
	if line != want {
		t.Fatalf("PreflightLine が想定外\nwant=%s\ngot=%s", want, line)
	}
}

func TestBuildStatusReport_NoSessionsIsOK(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	profiles := []awsconfig.Profile{
		{Name: "static", RoleARN: "arn:aws:iam::123456789012:role/x", SourceProfile: "base"},
	}

	report, err := BuildStatusReport(profiles, nil, StatusOptions{CacheDir: cacheDir, Now: time.Now()})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}
	if report.Overall != ssocache.StateOK {
		t.Fatalf("Overall が想定外: %s", report.Overall)
	}
	if len(report.Sessions) != 0 {
		t.Fatalf("Sessions が想定外: %v", report.Sessions)
	}

	line, exitCode := PreflightLine(report)
	if exitCode != 0 {
		t.Fatalf("exitCode が想定外: %d", exitCode)
	}
	if line != "awsp preflight: AWS SSO 設定なし" {
		t.Fatalf("PreflightLine が想定外: %s", line)
	}
}

func TestBuildStatusReport_WorstStateWins(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	// session-ok は有効 session-error は refreshToken なしで期限切れ
	writeTokenFixture(t, cacheDir, "session-ok", `{
		"startUrl": "https://ok.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "dummy",
		"expiresAt": "2026-09-16T13:00:00Z"
	}`)
	writeTokenFixture(t, cacheDir, "session-error", `{
		"startUrl": "https://error.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "dummy",
		"expiresAt": "2026-09-16T01:00:00Z"
	}`)

	sessions := []awsconfig.SSOSession{
		{Name: "session-ok", StartURL: "https://ok.awsapps.com/start"},
		{Name: "session-error", StartURL: "https://error.awsapps.com/start"},
	}

	report, err := BuildStatusReport(nil, sessions, StatusOptions{CacheDir: cacheDir, Grace: 8 * time.Hour, Now: now})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}
	if report.Overall != ssocache.StateError {
		t.Fatalf("Overall が想定外(悪い方が優先されるべき): %s", report.Overall)
	}
	if len(report.Sessions) != 2 {
		t.Fatalf("Sessions 件数が想定外: %d", len(report.Sessions))
	}
}

func TestBuildStatusReport_LegacySession(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	writeTokenFixture(t, cacheDir, "https://legacy.awsapps.com/start", `{
		"startUrl": "https://legacy.awsapps.com/start",
		"region": "ap-northeast-1",
		"accessToken": "dummy",
		"expiresAt": "2026-09-16T13:00:00Z"
	}`)

	profiles := []awsconfig.Profile{
		{Name: "legacy-dev", SSOStartURL: "https://legacy.awsapps.com/start", SSORegion: "ap-northeast-1"},
	}

	report, err := BuildStatusReport(profiles, nil, StatusOptions{CacheDir: cacheDir, Now: now})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}
	if len(report.Sessions) != 1 {
		t.Fatalf("Sessions 件数が想定外: %d", len(report.Sessions))
	}
	if report.Sessions[0].Name != "" {
		t.Fatalf("legacy セッションの Name が空でない: %s", report.Sessions[0].Name)
	}
	if report.Sessions[0].StartURL != "https://legacy.awsapps.com/start" {
		t.Fatalf("legacy セッションの StartURL が想定外: %s", report.Sessions[0].StartURL)
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{52 * time.Minute, "52m"},
		{11 * time.Hour, "11h"},
		{3 * 24 * time.Hour, "3d"},
		{-90 * time.Minute, "1h"},
	}

	for _, tc := range tests {
		if got := formatDuration(tc.d); got != tc.want {
			t.Fatalf("formatDuration(%s) が想定外: got=%s want=%s", tc.d, got, tc.want)
		}
	}
}

func TestGroupSessions_UnreferencedNamedSessionIncluded(t *testing.T) {
	t.Parallel()

	sessions := []awsconfig.SSOSession{
		{Name: "unused", StartURL: "https://unused.awsapps.com/start"},
	}

	groups := groupSessions(nil, sessions)
	if len(groups) != 1 {
		t.Fatalf("groups 件数が想定外: %d", len(groups))
	}
	if len(groups[0].profiles) != 0 {
		t.Fatalf("profiles が想定外: %v", groups[0].profiles)
	}
}

func TestConfigFileFieldIsPassedThrough(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	report, err := BuildStatusReport(nil, nil, StatusOptions{
		ConfigFile: filepath.Join("testdata", "config"),
		CacheDir:   cacheDir,
		Now:        time.Now(),
	})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}
	if report.ConfigFile != filepath.Join("testdata", "config") {
		t.Fatalf("ConfigFile が想定外: %s", report.ConfigFile)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion が想定外: %d", report.SchemaVersion)
	}
}
