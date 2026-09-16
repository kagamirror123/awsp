package ui

import (
	lipgloss "charm.land/lipgloss/v2"
)

var (
	successStyle = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(ColorInfo)
	warnStyle    = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	headingStyle = lipgloss.NewStyle().Foreground(ColorHeading).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
)

// SuccessLine は成功メッセージを 1 行で装飾する
func SuccessLine(message string) string {
	return successStyle.Render("✅ " + message)
}

// InfoLine は情報メッセージを 1 行で装飾する
func InfoLine(message string) string {
	return infoStyle.Render("ℹ️ " + message)
}

// WarnLine は警告メッセージを 1 行で装飾する
func WarnLine(message string) string {
	return warnStyle.Render("⚠️ " + message)
}

// ErrorLine はエラーメッセージを 1 行で装飾する
func ErrorLine(message string) string {
	return errorStyle.Render("❌ " + message)
}

// Heading は見出しを装飾する
func Heading(text string) string {
	return headingStyle.Render(text)
}

// Muted は補助情報を薄字で装飾する
func Muted(text string) string {
	return mutedStyle.Render(text)
}

// DashIfEmpty は値が空なら薄字の "-" を返す
func DashIfEmpty(value string) string {
	if value == "" {
		return Muted("-")
	}
	return value
}
