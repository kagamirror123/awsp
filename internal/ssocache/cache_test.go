package ssocache

import (
	"crypto/sha1" //nolint:gosec // テストで期待値を独自算出するために使用
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenPath(t *testing.T) {
	t.Parallel()

	cacheDir := "/tmp/does-not-matter"
	key := "my-sso-session"

	got := TokenPath(cacheDir, key)

	sum := sha1.Sum([]byte(key)) //nolint:gosec // 期待値算出用
	want := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".json")

	if got != want {
		t.Fatalf("TokenPath が想定外\nwant=%s\ngot=%s", want, got)
	}
}

func TestRoleCredentialPath(t *testing.T) {
	t.Parallel()

	t.Run("sso-session 形式は startUrl を含めない", func(t *testing.T) {
		t.Parallel()

		cacheDir := "/tmp/does-not-matter"
		key := RoleCredentialKey{
			AccountID:   "123456789012",
			RoleName:    "AdministratorAccess",
			SessionName: "corp",
			StartURL:    "should-be-ignored",
		}

		got := RoleCredentialPath(cacheDir, key)

		wantJSON := `{"accountId":"123456789012","roleName":"AdministratorAccess","sessionName":"corp"}`
		sum := sha1.Sum([]byte(wantJSON)) //nolint:gosec // 期待値算出用
		want := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".json")

		if got != want {
			t.Fatalf("RoleCredentialPath が想定外\nwant=%s\ngot=%s", want, got)
		}
	})

	t.Run("legacy 形式は startUrl を使う", func(t *testing.T) {
		t.Parallel()

		cacheDir := "/tmp/does-not-matter"
		key := RoleCredentialKey{
			AccountID: "123456789012",
			RoleName:  "AdministratorAccess",
			StartURL:  "https://example.awsapps.com/start",
		}

		got := RoleCredentialPath(cacheDir, key)

		wantJSON := `{"accountId":"123456789012","roleName":"AdministratorAccess","startUrl":"https://example.awsapps.com/start"}`
		sum := sha1.Sum([]byte(wantJSON)) //nolint:gosec // 期待値算出用
		want := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".json")

		if got != want {
			t.Fatalf("RoleCredentialPath が想定外\nwant=%s\ngot=%s", want, got)
		}
	})
}

func TestReadTokenMeta(t *testing.T) {
	t.Parallel()

	t.Run("ファイルが無ければ Exists=false でエラーなし", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		meta, err := ReadTokenMeta(filepath.Join(dir, "not-exists.json"))
		if err != nil {
			t.Fatalf("ReadTokenMeta が失敗: %v", err)
		}
		if meta.Exists {
			t.Fatal("存在しないのに Exists=true")
		}
	})

	t.Run("refreshToken ありを読める", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "token.json")
		writeFixture(t, path, `{
			"startUrl": "https://example.awsapps.com/start",
			"region": "us-west-2",
			"accessToken": "dummy-access-token",
			"expiresAt": "2026-09-16T00:00:00Z",
			"refreshToken": "dummy-refresh-token"
		}`)

		meta, err := ReadTokenMeta(path)
		if err != nil {
			t.Fatalf("ReadTokenMeta が失敗: %v", err)
		}
		if !meta.Exists {
			t.Fatal("Exists が false")
		}
		if !meta.HasRefreshToken {
			t.Fatal("HasRefreshToken が false")
		}
		want := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
		if !meta.ExpiresAt.Equal(want) {
			t.Fatalf("ExpiresAt が想定外: got=%v want=%v", meta.ExpiresAt, want)
		}
	})

	t.Run("refreshToken なしを読める", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "token.json")
		writeFixture(t, path, `{
			"startUrl": "https://example.awsapps.com/start",
			"region": "us-west-2",
			"accessToken": "dummy-access-token",
			"expiresAt": "2026-09-16T00:00:00Z"
		}`)

		meta, err := ReadTokenMeta(path)
		if err != nil {
			t.Fatalf("ReadTokenMeta が失敗: %v", err)
		}
		if meta.HasRefreshToken {
			t.Fatal("refreshToken が無いのに HasRefreshToken=true")
		}
	})
}

func TestEvaluate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	grace := 8 * time.Hour

	t.Run("unknown: ファイルなし", func(t *testing.T) {
		t.Parallel()
		got := Evaluate(TokenMeta{Exists: false}, now, grace)
		if got.State != StateUnknown {
			t.Fatalf("State が想定外: %s", got.State)
		}
	})

	t.Run("ok: 期限が未来", func(t *testing.T) {
		t.Parallel()
		meta := TokenMeta{Exists: true, ExpiresAt: now.Add(1 * time.Hour)}
		got := Evaluate(meta, now, grace)
		if got.State != StateOK {
			t.Fatalf("State が想定外: %s", got.State)
		}
		if got.Remaining != 1*time.Hour {
			t.Fatalf("Remaining が想定外: %s", got.Remaining)
		}
	})

	t.Run("AutoRefresh は refreshToken の有無をそのまま表す(D34)", func(t *testing.T) {
		t.Parallel()
		withRefresh := Evaluate(TokenMeta{Exists: true, ExpiresAt: now.Add(time.Hour), HasRefreshToken: true}, now, grace)
		if withRefresh.State != StateOK || !withRefresh.AutoRefresh {
			t.Fatalf("refreshToken ありの ok が想定外: %+v", withRefresh)
		}
		withoutRefresh := Evaluate(TokenMeta{Exists: true, ExpiresAt: now.Add(time.Hour)}, now, grace)
		if withoutRefresh.State != StateOK || withoutRefresh.AutoRefresh {
			t.Fatalf("refreshToken なしの ok が想定外: %+v", withoutRefresh)
		}
	})

	t.Run("warning: 期限切れだが refreshToken ありで猶予内", func(t *testing.T) {
		t.Parallel()
		meta := TokenMeta{Exists: true, ExpiresAt: now.Add(-1 * time.Hour), HasRefreshToken: true}
		got := Evaluate(meta, now, grace)
		if got.State != StateWarning {
			t.Fatalf("State が想定外: %s", got.State)
		}
	})

	t.Run("warning から error への猶予境界: 猶予未満は warning", func(t *testing.T) {
		t.Parallel()
		// 期限切れから grace 未満(1 秒手前)は warning
		meta := TokenMeta{Exists: true, ExpiresAt: now.Add(-grace + time.Second), HasRefreshToken: true}
		got := Evaluate(meta, now, grace)
		if got.State != StateWarning {
			t.Fatalf("猶予境界直前が想定外: %s", got.State)
		}
	})

	t.Run("warning から error への猶予境界: 猶予超過は error", func(t *testing.T) {
		t.Parallel()
		// 期限切れから grace ちょうど経過(now == expiresAt+grace)は猶予超過扱い
		meta := TokenMeta{Exists: true, ExpiresAt: now.Add(-grace), HasRefreshToken: true}
		got := Evaluate(meta, now, grace)
		if got.State != StateError {
			t.Fatalf("猶予境界超過が想定外: %s", got.State)
		}
	})

	t.Run("error: refreshToken なし", func(t *testing.T) {
		t.Parallel()
		meta := TokenMeta{Exists: true, ExpiresAt: now.Add(-1 * time.Minute), HasRefreshToken: false}
		got := Evaluate(meta, now, grace)
		if got.State != StateError {
			t.Fatalf("State が想定外: %s", got.State)
		}
	})

	t.Run("error: refreshToken ありでも猶予超過", func(t *testing.T) {
		t.Parallel()
		meta := TokenMeta{Exists: true, ExpiresAt: now.Add(-9 * time.Hour), HasRefreshToken: true}
		got := Evaluate(meta, now, grace)
		if got.State != StateError {
			t.Fatalf("State が想定外: %s", got.State)
		}
	})

	t.Run("ok から warning への境界: expiresAt が now と同時刻は warning 判定", func(t *testing.T) {
		t.Parallel()
		meta := TokenMeta{Exists: true, ExpiresAt: now, HasRefreshToken: true}
		got := Evaluate(meta, now, grace)
		if got.State != StateWarning {
			t.Fatalf("State が想定外: %s", got.State)
		}
	})
}

func TestReadRoleCredentialMeta(t *testing.T) {
	t.Parallel()

	t.Run("ファイルが無ければ Exists=false", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		meta, err := ReadRoleCredentialMeta(filepath.Join(dir, "not-exists.json"))
		if err != nil {
			t.Fatalf("ReadRoleCredentialMeta が失敗: %v", err)
		}
		if meta.Exists {
			t.Fatal("存在しないのに Exists=true")
		}
	})

	t.Run("Expiration と mtime を読める", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "role.json")
		writeFixture(t, path, `{
			"Credentials": {
				"AccessKeyId": "AKIAEXAMPLE",
				"SecretAccessKey": "dummy",
				"SessionToken": "dummy",
				"Expiration": "2026-09-16T09:00:00Z"
			}
		}`)

		meta, err := ReadRoleCredentialMeta(path)
		if err != nil {
			t.Fatalf("ReadRoleCredentialMeta が失敗: %v", err)
		}
		if !meta.Exists {
			t.Fatal("Exists が false")
		}
		want := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
		if !meta.Expiration.Equal(want) {
			t.Fatalf("Expiration が想定外: got=%v want=%v", meta.Expiration, want)
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("os.Stat が失敗: %v", statErr)
		}
		if !meta.LastUsed.Equal(info.ModTime()) {
			t.Fatalf("LastUsed が mtime と一致しない: got=%v want=%v", meta.LastUsed, info.ModTime())
		}
	})
}

func writeFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("フィクスチャ作成に失敗: %s: %v", path, err)
	}
}
