package cmd

import (
	"strings"
	"testing"
)

func TestRenderPosixInitScript(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			script := renderPosixInitScript(shell, `"/usr/local/bin/awsp"`)

			for _, want := range []string{
				"# awsp " + shell + " integration",
				`if [[ "$1" == -* ]]; then`,
				`if [[ "$_arg" == "--shell" || "$_arg" == --shell=* ]]; then`,
				`current|list|completion|help|init|version|status|preflight|login|whoami|mcp)`,
				`"/usr/local/bin/awsp" "$@" --shell)`,
			} {
				if !strings.Contains(script, want) {
					t.Fatalf("%s の関数に %q がない:\n%s", shell, want, script)
				}
			}
		})
	}
}

func TestRenderFishInitScript(t *testing.T) {
	script := renderFishInitScript(`"/usr/local/bin/awsp"`)

	for _, want := range []string{
		"function awsp",
		`case current list completion help init version status preflight login whoami mcp`,
		`string match -q -r -- '^--shell(=.*)?$' "$_arg"`,
		`string match -q -- '-*' "$argv[1]"`,
		`set -l _awsp_exports ("/usr/local/bin/awsp" $argv --shell=fish)`,
		"eval $_awsp_exports",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("fish の関数に %q がない:\n%s", want, script)
		}
	}
	if strings.Contains(script, "[[") || strings.Contains(script, "eval \"$") {
		t.Fatalf("fish の関数に POSIX 構文が混ざっている:\n%s", script)
	}
}

func TestInitCommandShells(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "fish"} {
		t.Run(shell, func(t *testing.T) {
			root := newRootCmd()
			out := &strings.Builder{}
			root.SetOut(out)
			root.SetArgs([]string{"init", shell})
			if err := root.Execute(); err != nil {
				t.Fatalf("init %s が失敗: %v", shell, err)
			}
			if !strings.Contains(out.String(), "# awsp "+shell+" integration") {
				t.Fatalf("init %s の出力が想定外:\n%s", shell, out.String())
			}
		})
	}
}
