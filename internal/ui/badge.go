package ui

import (
	lipgloss "charm.land/lipgloss/v2"
)

// BadgeKind は Badge の色分けの種類
type BadgeKind int

const (
	// BadgeOK は正常状態
	BadgeOK BadgeKind = iota
	// BadgeWarning は警告状態
	BadgeWarning
	// BadgeError は異常状態
	BadgeError
	// BadgeUnknown は不明/未設定の状態
	BadgeUnknown
)

var badgeStyles = map[BadgeKind]lipgloss.Style{
	BadgeOK:      lipgloss.NewStyle().Foreground(ColorSuccess),
	BadgeWarning: lipgloss.NewStyle().Foreground(ColorWarning),
	BadgeError:   lipgloss.NewStyle().Foreground(ColorError),
	BadgeUnknown: lipgloss.NewStyle().Foreground(ColorMuted),
}

// Badge は状態に応じて色付けした文字列を返す
func Badge(kind BadgeKind, text string) string {
	style, ok := badgeStyles[kind]
	if !ok {
		style = badgeStyles[BadgeUnknown]
	}
	return style.Render(text)
}
