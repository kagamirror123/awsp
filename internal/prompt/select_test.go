package prompt

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kagamirror123/awsp/internal/awsp"
)

func TestShouldStartFiltering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  tea.KeyMsg
		mode bool
		want bool
	}{
		{
			name: "通常文字はフィルタ開始",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")},
			mode: false,
			want: true,
		},
		{
			name: "日本語入力でもフィルタ開始",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("あ")},
			mode: false,
			want: true,
		},
		{
			name: "j は通常文字としてフィルタ開始",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
			mode: false,
			want: true,
		},
		{
			name: "k は通常文字としてフィルタ開始",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")},
			mode: false,
			want: true,
		},
		{
			name: "q は終了キーとして扱う",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")},
			mode: false,
			want: false,
		},
		{
			name: "slash はフィルタ起動キーとして扱う",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
			mode: false,
			want: false,
		},
		{
			name: "Alt 修飾はフィルタ開始しない",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a"), Alt: true},
			mode: false,
			want: false,
		},
		{
			name: "非表示文字は開始しない",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}},
			mode: false,
			want: false,
		},
		{
			name: "すでにフィルタ中なら開始しない",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")},
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
		msg  tea.KeyMsg
		want bool
	}{
		{
			name: "up は移動キー",
			msg:  tea.KeyMsg{Type: tea.KeyUp},
			want: true,
		},
		{
			name: "j は移動キーではない",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")},
			want: false,
		},
		{
			name: "通常文字は移動キーではない",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")},
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

	model := newSelectModel([]list.Item{
		profileItem{profile: awsp.Profile{Name: "dev", Region: "us-west-2"}},
		profileItem{profile: awsp.Profile{Name: "prod", Region: "us-west-2"}},
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
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

	updated, _ = current.Update(tea.KeyMsg{Type: tea.KeyDown})
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
