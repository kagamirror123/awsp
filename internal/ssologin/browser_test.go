package ssologin

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultOpenBrowserReportsLauncherExitFailure(t *testing.T) {
	var name string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "linux":
		name = "xdg-open"
	default:
		t.Skip("自動起動非対応 OS")
	}
	dir := t.TempDir()
	//nolint:gosec // 起動コマンドを代替する実行用スクリプトのため実行権限が必要
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := defaultOpenBrowser("https://example.invalid/"); err == nil {
		t.Fatal("起動コマンドの失敗を見落としました")
	}
}
