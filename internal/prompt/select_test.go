package prompt

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/kagamirror123/awsp/internal/awsp"
)

func TestShouldStartFiltering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		mode bool
		want bool
	}{
		{
			name: "通常文字はフィルタ開始",
			msg:  tea.KeyPressMsg{Text: "a", Code: 'a'},
			mode: false,
			want: true,
		},
		{
			name: "日本語入力でもフィルタ開始",
			msg:  tea.KeyPressMsg{Text: "あ", Code: 'あ'},
			mode: false,
			want: true,
		},
		{
			name: "j は通常文字としてフィルタ開始",
			msg:  tea.KeyPressMsg{Text: "j", Code: 'j'},
			mode: false,
			want: true,
		},
		{
			name: "k は通常文字としてフィルタ開始",
			msg:  tea.KeyPressMsg{Text: "k", Code: 'k'},
			mode: false,
			want: true,
		},
		{
			name: "q は終了キーとして扱う",
			msg:  tea.KeyPressMsg{Text: "q", Code: 'q'},
			mode: false,
			want: false,
		},
		{
			name: "slash はフィルタ起動キーとして扱う",
			msg:  tea.KeyPressMsg{Text: "/", Code: '/'},
			mode: false,
			want: false,
		},
		{
			name: "Alt 修飾はフィルタ開始しない",
			msg:  tea.KeyPressMsg{Text: "a", Code: 'a', Mod: tea.ModAlt},
			mode: false,
			want: false,
		},
		{
			name: "非表示文字は開始しない",
			msg:  tea.KeyPressMsg{Text: "\n", Code: '\n'},
			mode: false,
			want: false,
		},
		{
			name: "すでにフィルタ中なら開始しない",
			msg:  tea.KeyPressMsg{Text: "a", Code: 'a'},
			mode: true,
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldStartFiltering(tc.msg, tc.mode); got != tc.want {
				t.Fatalf("shouldStartFiltering が想定外: want=%v got=%v", tc.want, got)
			}
		})
	}
}

func TestIsNavigationKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want bool
	}{
		{
			name: "up は移動キー",
			msg:  tea.KeyPressMsg{Code: tea.KeyUp},
			want: true,
		},
		{
			name: "j は移動キーではない",
			msg:  tea.KeyPressMsg{Text: "j", Code: 'j'},
			want: false,
		},
		{
			name: "通常文字は移動キーではない",
			msg:  tea.KeyPressMsg{Text: "a", Code: 'a'},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isNavigationKey(tc.msg); got != tc.want {
				t.Fatalf("isNavigationKey が想定外: want=%v got=%v", tc.want, got)
			}
		})
	}
}

func TestUpdateArrowKeyExitsFilteringAndMovesCursor(t *testing.T) {
	t.Parallel()

	now := time.Now()
	model := newSelectModel([]list.Item{
		profileItem{profile: awsp.Profile{Name: "dev", Region: "us-west-2"}, now: now},
		profileItem{profile: awsp.Profile{Name: "prod", Region: "us-west-2"}, now: now},
	})

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	current, ok := updated.(selectModel)
	if !ok {
		t.Fatal("モデル型の変換に失敗")
	}
	if cmd != nil {
		updated, _ = current.Update(cmd())
		current, ok = updated.(selectModel)
		if !ok {
			t.Fatal("モデル型の変換に失敗")
		}
	}

	if !current.list.SettingFilter() {
		t.Fatal("文字入力後にフィルタ入力モードへ入っていない")
	}

	updated, _ = current.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	current, ok = updated.(selectModel)
	if !ok {
		t.Fatal("モデル型の変換に失敗")
	}

	if current.list.SettingFilter() {
		t.Fatal("矢印入力後もフィルタ入力モードのままになっている")
	}

	if len(current.list.VisibleItems()) == 0 {
		t.Fatal("フィルタ結果が 0 件になっている")
	}
}
