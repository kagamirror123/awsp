package cmd

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// 有効で自動更新が効くセッションは Remaining 列に分数ではなく "自動更新" を出す(D34)
func TestSessionRemainingLabel(t *testing.T) {
	t.Parallel()

	remaining := int64(52 * 60)
	expired := int64(-2 * 60 * 60)

	cases := []struct {
		name    string
		session awsp.SessionStatus
		want    string
	}{
		{"自動更新が効く ok", awsp.SessionStatus{State: ssocache.StateOK, RemainingSeconds: &remaining, AutoRefresh: true}, "自動更新"},
		{"自動更新が効かない ok は残り時間", awsp.SessionStatus{State: ssocache.StateOK, RemainingSeconds: &remaining}, "52m"},
		{"warning は経過時間のまま", awsp.SessionStatus{State: ssocache.StateWarning, RemainingSeconds: &expired, AutoRefresh: true}, "-2h"},
		{"未ログインは -", awsp.SessionStatus{State: ssocache.StateUnknown}, "-"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ansi.Strip(sessionRemainingLabel(tc.session)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
