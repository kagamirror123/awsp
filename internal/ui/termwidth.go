package ui

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

const (
	// defaultTerminalWidth は幅を判定できないときの既定値(D22)
	defaultTerminalWidth = 120
	// minTerminalWidth は表の可読性を保つための下限(D22)
	minTerminalWidth = 40
)

// TerminalWidth は w の出力先の端末幅を返す(D22)
// w が(NewWriter でラップされていても その先が)端末に繋がる *os.File なら実際の列数を使う
// 端末でなければ環境変数 COLUMNS を使い それも無ければ 120 とする いずれも下限は 40
func TerminalWidth(w io.Writer) int {
	if f, ok := terminalFile(w); ok {
		if cols, _, err := term.GetSize(f.Fd()); err == nil && cols > 0 {
			return max(cols, minTerminalWidth)
		}
	}

	width := defaultTerminalWidth
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			width = n
		}
	}

	return max(width, minTerminalWidth)
}

// terminalFile は w から端末に繋がる *os.File を取り出す
// NewWriter は colorprofile.Writer で元の Writer を包むため Forward を辿って元を探す
func terminalFile(w io.Writer) (*os.File, bool) {
	for {
		switch v := w.(type) {
		case *os.File:
			return v, term.IsTerminal(v.Fd())
		case *colorprofile.Writer:
			w = v.Forward
		default:
			return nil, false
		}
	}
}
