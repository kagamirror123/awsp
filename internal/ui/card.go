package ui

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

var cardBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorBorder).
	Padding(0, 1)

// RenderCard はタイトルと本文行を見出し付きの罫線ボックスで描画する
func RenderCard(title string, lines []string) string {
	heading := Heading(title)
	body := cardBoxStyle.Render(strings.Join(lines, "\n"))
	return heading + "\n" + body
}
