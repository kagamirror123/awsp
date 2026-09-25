package ui

import (
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderCard_KeepsBorderWhenItFits(t *testing.T) {
	t.Parallel()

	lines := []string{"🔐 Profile : dev", "🧾 Account : 123456789012"}
	framed := cardBoxStyle.Render(strings.Join(lines, "\n"))

	// ちょうど収まる幅なら枠を残す(D33)
	got := ansi.Strip(RenderCard("Title", lines, lipgloss.Width(framed)))
	if !strings.Contains(got, "╭") || !strings.Contains(got, "╰") {
		t.Fatalf("幅に収まるのに枠が無い:\n%s", got)
	}
}

func TestRenderCard_DropsBorderWhenTooWide(t *testing.T) {
	t.Parallel()

	lines := []string{"🔐 Profile : dev", "🌍 ARN     : arn:aws:sts::123456789012:assumed-role/Example/you@example.com"}
	framed := cardBoxStyle.Render(strings.Join(lines, "\n"))

	got := ansi.Strip(RenderCard("Title", lines, lipgloss.Width(framed)-1))
	if strings.ContainsAny(got, "╭╮╰╯│") {
		t.Fatalf("はみ出すのに枠が残っている:\n%s", got)
	}
	// 字下げだけを足し 短い行を最長行の幅まで空白で埋めない(埋めた空白が端末で折り返すため)
	want := strings.Join([]string{"Title", "  " + lines[0], "  " + lines[1]}, "\n")
	if got != want {
		t.Fatalf("枠なしの描画が想定外\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestRenderCard_NoLimitWhenMaxWidthIsZero(t *testing.T) {
	t.Parallel()

	got := ansi.Strip(RenderCard("Title", []string{strings.Repeat("x", 200)}, 0))
	if !strings.Contains(got, "╭") {
		t.Fatalf("maxWidth=0 なのに枠が外れた:\n%s", got)
	}
}
