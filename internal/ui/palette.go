// Package ui は awsp の CLI 出力(カード・表・行・バッジ)を Lip Gloss v2 で一本化する(D8)
// 配色はここに集約し ダーク・ライト両方の背景で読めるよう ANSI 16〜256 色を基本にする
package ui

import (
	"image/color"

	lipgloss "charm.land/lipgloss/v2"
)

// 配色は 1 か所に集約する(成功 / 情報 / 警告 / エラー / 見出し / 罫線 / 薄字)
// cmd パッケージの Fang 配色もここから作る(同じパレットを共有する)
var (
	// ColorSuccess は成功メッセージの色
	ColorSuccess color.Color = lipgloss.Color("10")
	// ColorInfo は情報メッセージの色
	ColorInfo color.Color = lipgloss.Color("12")
	// ColorWarning は警告メッセージの色
	ColorWarning color.Color = lipgloss.Color("11")
	// ColorError はエラーメッセージの色
	ColorError color.Color = lipgloss.Color("9")
	// ColorHeading は見出しの色
	ColorHeading color.Color = lipgloss.Color("14")
	// ColorBorder は罫線の色
	ColorBorder color.Color = lipgloss.Color("12")
	// ColorMuted は薄字(補助情報 未設定値など)の色
	ColorMuted color.Color = lipgloss.Color("8")
)
