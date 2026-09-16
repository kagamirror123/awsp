package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCurrentProfile(t *testing.T) {
	t.Run("AWS_PROFILE が設定されている場合", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "dev")

		profile, source, err := resolveCurrentProfile()
		if err != nil {
			t.Fatalf("resolveCurrentProfile が失敗: %v", err)
		}
		if profile != "dev" || source != "env" {
			t.Fatalf("resolveCurrentProfile が想定外: profile=%q source=%q", profile, source)
		}
	})

	t.Run("AWS_PROFILE が未設定の場合", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "")

		_, _, err := resolveCurrentProfile()
		if err == nil {
			t.Fatal("未設定なのにエラーにならなかった")
		}
	})
}

func TestEnsureKnownProfile(t *testing.T) {
	t.Run("存在するプロファイルは通過", func(t *testing.T) {
		setHomeWithAWSConfig(t, "[profile dev]\nregion = ap-northeast-1\n")

		err := ensureKnownProfile(context.Background(), "dev", "env")
		if err != nil {
			t.Fatalf("ensureKnownProfile が失敗: %v", err)
		}
	})

	t.Run("環境変数由来で未登録なら unset を促す", func(t *testing.T) {
		setHomeWithAWSConfig(t, "[profile dev]\nregion = ap-northeast-1\n")

		err := ensureKnownProfile(context.Background(), "missing", "env")
		if err == nil {
			t.Fatal("未登録なのにエラーにならなかった")
		}
		if !strings.Contains(err.Error(), "unset AWS_PROFILE") {
			t.Fatalf("エラーメッセージが想定外: %v", err)
		}
	})
}

func TestRenderZshInitScript(t *testing.T) {
	t.Run("フラグ指定を profile と誤解釈しない", func(t *testing.T) {
		script := renderZshInitScript(`"/usr/local/bin/awsp"`)

		if !strings.Contains(script, `if [[ "$1" == -* ]]; then`) {
			t.Fatalf("フラグ素通しの分岐が存在しない")
		}
		if !strings.Contains(script, `current|list|completion|help|init|version|status|preflight|login|whoami|mcp`) {
			t.Fatalf("サブコマンド素通しの分岐が存在しない")
		}
	})

	t.Run("実行コマンドを差し替えられる", func(t *testing.T) {
		script := renderZshInitScript(`my-wrapper awsp`)

		if !strings.Contains(script, `my-wrapper awsp "$@"`) {
			t.Fatalf("差し替えコマンドが通常実行に使われていない")
		}
		if !strings.Contains(script, `my-wrapper awsp "$@" --shell`) {
			t.Fatalf("差し替えコマンドが shell mode 実行に使われていない")
		}
	})
}

func TestResolveConfigFilePath(t *testing.T) {
	t.Run("AWS_CONFIG_FILE が優先される", func(t *testing.T) {
		t.Setenv("AWS_CONFIG_FILE", "/tmp/custom-aws-config")

		got := resolveConfigFilePath()
		if got != "/tmp/custom-aws-config" {
			t.Fatalf("AWS_CONFIG_FILE が尊重されていない: %s", got)
		}
	})

	t.Run("未設定時は SDK 既定の ~/.aws/config を使う", func(t *testing.T) {
		homeDir := t.TempDir()
		t.Setenv("HOME", homeDir)
		t.Setenv("AWS_CONFIG_FILE", "")

		got := resolveConfigFilePath()
		want := filepath.Join(homeDir, ".aws", "config")
		if got != want {
			t.Fatalf("既定パスが想定外: got=%s want=%s", got, want)
		}
	})
}

func setHomeWithAWSConfig(t *testing.T, content string) {
	t.Helper()

	homeDir := t.TempDir()
	awsDir := filepath.Join(homeDir, ".aws")
	if err := os.MkdirAll(awsDir, 0o700); err != nil {
		t.Fatalf(".aws ディレクトリ作成に失敗: %v", err)
	}

	configPath := filepath.Join(awsDir, "config")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("config 作成に失敗: %v", err)
	}

	t.Setenv("HOME", homeDir)
	// AWS_CONFIG_FILE が実行環境で設定されていても(D5)テストの HOME を優先させる
	t.Setenv("AWS_CONFIG_FILE", "")
}
