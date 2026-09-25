package ui

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

var cardBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorBorder).
	Padding(0, 1)

// cardPlainIndent は枠を外したときの字下げ 枠ありの本文と同じ桁から始まるようにする
const cardPlainIndent = "  "

// RenderCard はタイトルと本文行を見出し付きの罫線ボックスで描画する
// 枠が maxWidth を超えるときは 端末の折り返しで罫線が割れるので 枠を外して字下げだけで描く(D33)
// 枠なしの行は端末に任せて折り返すので ARN や URL も 1 行のまま選択・クリックできる
// maxWidth が 0 以下なら幅を制限しない
func RenderCard(title string, lines []string, maxWidth int) string {
	heading := Heading(title)
	body := cardBoxStyle.Render(strings.Join(lines, "\n"))
	if maxWidth > 0 && lipgloss.Width(body) > maxWidth {
		// lipgloss で描くと短い行も最長行の幅まで空白で埋まり その空白が折り返すので 行ごとに字下げだけ足す
		plain := make([]string, len(lines))
		for i, line := range lines {
			plain[i] = cardPlainIndent + line
		}
		body = strings.Join(plain, "\n")
	}
	return heading + "\n" + body
}
