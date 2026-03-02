// Package card は CLI 出力向けのカード描画を提供する
package card

import (
	"strings"

	"github.com/pterm/pterm"
)

// Render はタイトルと本文行を罫線付きカードで描画する
func Render(title string, lines []string) string {
	body := strings.Join(lines, "\n")
	titleStyle := pterm.NewStyle(pterm.FgLightCyan, pterm.Bold)
	borderStyle := pterm.NewStyle(pterm.FgBlue)

	return pterm.DefaultBox.
		WithTitle(titleStyle.Sprint(title)).
		WithTitleTopCenter(true).
		WithBoxStyle(borderStyle).
		WithLeftPadding(1).
		WithRightPadding(1).
		WithTopPadding(0).
		WithBottomPadding(0).
		Sprint(body)
}
