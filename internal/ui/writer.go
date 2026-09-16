package ui

import (
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
)

// NewWriter は w をラップし 出力先が非 TTY か NO_COLOR の場合に ANSI 装飾を落とす
// Lip Gloss v2 の Style.Render は常に ANSI を出すため 実際の出力箇所ではこの Writer を経由させる(D8)
func NewWriter(w io.Writer) io.Writer {
	return colorprofile.NewWriter(w, os.Environ())
}
