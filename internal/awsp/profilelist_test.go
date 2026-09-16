package awsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

func TestBuildProfileList_SessionStateAndCredential(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	ssoCacheDir := t.TempDir()
	cliCacheDir := t.TempDir()

	writeTokenFixture(t, ssoCacheDir, "corp", `{
		"startUrl": "https://example.awsapps.com/start",
		"region": "us-west-2",
		"accessToken": "dummy",
		"expiresAt": "2026-09-16T13:00:00Z"
	}`)

	roleKey := ssocache.RoleCredentialKey{
		AccountID:   "123456789012",
		RoleName:    "AdministratorAccess",
		SessionName: "corp",
	}
	rolePath := ssocache.RoleCredentialPath(cliCacheDir, roleKey)
	if err := os.MkdirAll(filepath.Dir(rolePath), 0o700); err != nil {
		t.Fatalf("cli cache dir 作成に失敗: %v", err)
	}
	roleContent, err := json.Marshal(map[string]any{
		"Credentials": map[string]any{
			"AccessKeyId":     "AKIAEXAMPLE",
			"SecretAccessKey": "dummy",
			"SessionToken":    "dummy",
			"Expiration":      "2026-09-16T13:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("role fixture の marshal に失敗: %v", err)
	}
	if err := os.WriteFile(rolePath, roleContent, 0o600); err != nil {
		t.Fatalf("role fixture 作成に失敗: %v", err)
	}

	profiles := []awsconfig.Profile{
		{Name: "dev", SSOSession: "corp", SSOAccountID: "123456789012", SSORoleName: "AdministratorAccess"},
	}
	sessions := []awsconfig.SSOSession{
		{Name: "corp", StartURL: "https://example.awsapps.com/start", Region: "us-west-2"},
	}

	list, err := BuildProfileList(profiles, sessions, ProfileListOptions{
		ConfigFile:  "/tmp/config",
		SSOCacheDir: ssoCacheDir,
		CLICacheDir: cliCacheDir,
		Grace:       8 * time.Hour,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("BuildProfileList が失敗: %v", err)
	}

	if list.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion が想定外: %d", list.SchemaVersion)
	}
	if len(list.Profiles) != 1 {
		t.Fatalf("Profiles 件数が想定外: %d", len(list.Profiles))
	}

	info := list.Profiles[0]
	if info.SessionState != ssocache.StateOK {
		t.Fatalf("SessionState が想定外: %s", info.SessionState)
	}
	if info.LastUsedAt == nil {
		t.Fatal("LastUsedAt が nil")
	}
	if info.CredentialExpiresAt == nil || !info.CredentialExpiresAt.Equal(time.Date(2026, 9, 16, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("CredentialExpiresAt が想定外: %v", info.CredentialExpiresAt)
	}
}

func TestBuildProfileList_NonSSOProfile(t *testing.T) {
	t.Parallel()

	profiles := []awsconfig.Profile{
		{Name: "static", RoleARN: "arn:aws:iam::123456789012:role/x", SourceProfile: "base"},
	}

	list, err := BuildProfileList(profiles, nil, ProfileListOptions{SSOCacheDir: t.TempDir(), Now: time.Now()})
	if err != nil {
		t.Fatalf("BuildProfileList が失敗: %v", err)
	}
	info := list.Profiles[0]
	if info.SessionState != "" {
		t.Fatalf("SSO を使わない profile の SessionState が空でない: %s", info.SessionState)
	}
	if info.LastUsedAt != nil || info.CredentialExpiresAt != nil {
		t.Fatalf("SSO を使わない profile に認証情報が付与された: %+v", info)
	}
}
