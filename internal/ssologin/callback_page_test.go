package ssologin

import (
	"strings"
	"testing"
)

// コールバック画面は認可直後のブラウザが読む唯一の出力なので
// 置換漏れが無いことと 外部リソースを読みに行かないことを固定する
func TestCallbackPages(t *testing.T) {
	pages := map[string]struct {
		html        string
		wantHeading string
		wantAccent  string
	}{
		"success":    {successCallbackPage(), "承認を受け付けました", callbackAccentSuccess},
		"failure":    {failureCallbackPage(), "認証に失敗しました", callbackAccentError},
		"unexpected": {unexpectedCallbackPage(), "想定外のリクエストです", callbackAccentNeutral},
	}

	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(page.html, "{{") {
				t.Fatalf("テンプレートの置換漏れがある")
			}
			if !strings.Contains(page.html, page.wantHeading) {
				t.Fatalf("見出しが含まれていない: %s", page.wantHeading)
			}
			if !strings.Contains(page.html, page.wantAccent) {
				t.Fatalf("アクセント色が反映されていない: %s", page.wantAccent)
			}
			if !strings.HasPrefix(page.html, "<!doctype html>") {
				t.Fatalf("doctype で始まっていない")
			}

			// 認可直後はネットワークが不安定なこともあるため 1 枚で完結させる
			for _, external := range []string{"http://", "https://", "<script", "<link", "<img"} {
				if strings.Contains(page.html, external) {
					t.Fatalf("外部リソースまたはスクリプトを含んでいる: %s", external)
				}
			}
		})
	}
}

// 空の note は要素ごと出さない(空の箱が残ると余白が崩れるため)
func TestCallbackPageOmitsEmptyNote(t *testing.T) {
	if strings.Contains(successCallbackPage(), `class="note"`) {
		t.Fatal("note が空なのに要素が出力されている")
	}
	if !strings.Contains(callbackPage(callbackAccentNeutral, "", "見出し", "本文", "補足"), `class="note"`) {
		t.Fatal("note を渡したのに要素が出力されていない")
	}
}
