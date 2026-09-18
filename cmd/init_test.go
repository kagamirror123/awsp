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
				`current|list|completion|help|init|version|status|preflight|login|whoami|console|mcp|__complete|__completeNoDesc)`,
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
		`case current list completion help init version status preflight login whoami console mcp __complete __completeNoDesc`,
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

func TestCompleteProfileNames(t *testing.T) {
	setHomeWithAWSConfig(t, "[profile dev]\nregion = ap-northeast-1\n[profile prod]\nregion = ap-northeast-1\n[profile dev-admin]\nregion = ap-northeast-1\n")

	t.Run("前方一致で profile 名を返す", func(t *testing.T) {
		root := newRootCmd()
		out := &strings.Builder{}
		root.SetOut(out)
		root.SetArgs([]string{"__complete", "de"})
		if err := root.Execute(); err != nil {
			t.Fatalf("__complete が失敗: %v", err)
		}
		got := out.String()
		for _, want := range []string{"dev\n", "dev-admin\n"} {
			if !strings.Contains(got, want) {
				t.Fatalf("補完候補に %q がない:\n%s", want, got)
			}
		}
		if strings.Contains(got, "prod") {
			t.Fatalf("前方一致しない prod が含まれている:\n%s", got)
		}
		if !strings.Contains(got, ":4\n") { // ShellCompDirectiveNoFileComp
			t.Fatalf("ファイル名補完を抑止する directive が無い:\n%s", got)
		}
	})

	t.Run("login と whoami の引数も補完する", func(t *testing.T) {
		for _, sub := range []string{"login", "whoami"} {
			root := newRootCmd()
			out := &strings.Builder{}
			root.SetOut(out)
			root.SetArgs([]string{"__complete", sub, "pr"})
			if err := root.Execute(); err != nil {
				t.Fatalf("__complete %s が失敗: %v", sub, err)
			}
			if !strings.Contains(out.String(), "prod\n") {
				t.Fatalf("%s の補完候補に prod がない:\n%s", sub, out.String())
			}
		}
	})

	t.Run("profile を指定済みなら候補を返さない", func(t *testing.T) {
		root := newRootCmd()
		out := &strings.Builder{}
		root.SetOut(out)
		root.SetArgs([]string{"__complete", "dev", ""})
		if err := root.Execute(); err != nil {
			t.Fatalf("__complete が失敗: %v", err)
		}
		if strings.Contains(out.String(), "prod\n") {
			t.Fatalf("2 つ目の引数に profile 候補を返している:\n%s", out.String())
		}
	})
}
