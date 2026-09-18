package ssologin

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// openBrowserAt は認可 URL をブラウザで開く 失敗したら flow.BrowserErr に理由を残す
func openBrowserAt(opts Options, flow *Flow, authURL string) {
	if authURL == "" {
		return
	}
	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = defaultOpenBrowser
	}
	if err := openBrowser(authURL); err != nil {
		flow.BrowserErr = err
	}
}

// OpenBrowser は OS 標準のコマンドで URL をブラウザで開く
// ログイン以外(awsp console など)からも同じ起動処理を使うために公開する
func OpenBrowser(rawURL string) error {
	return defaultOpenBrowser(rawURL)
}

// defaultOpenBrowser は OS 標準のコマンドでブラウザを起動する
func defaultOpenBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL) //nolint:gosec // 固定バイナリ 引数は AWS が返す検証 URL
	case "linux":
		cmd = exec.Command("xdg-open", rawURL) //nolint:gosec // 固定バイナリ 引数は AWS が返す検証 URL
	default:
		return fmt.Errorf("このOSではブラウザの自動起動に対応していません: %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// open/xdg-open の即時の失敗は呼び出し側へ返す。
	// ブラウザの寿命と連動するランチャーもあるため、待機は 3 秒に制限する。
	// 待機後も回収は続けるが、起動したブラウザを強制終了しない。
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errors.New("ブラウザの起動結果を確認できませんでした。認可 URL を手動で開いてください")
	}
}
