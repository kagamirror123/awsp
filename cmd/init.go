package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// passthroughSubcommands は親シェルの関数がそのままバイナリへ渡すサブコマンド名
var passthroughSubcommands = []string{
	"current", "list", "completion", "help", "init", "version", "status", "preflight", "login", "whoami", "mcp",
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "シェル連携スクリプトを出力(zsh / bash / fish)",
		Example: "  eval \"$(awsp init zsh)\"\n  eval \"$(awsp init bash)\"\n  awsp init fish | source",
	}

	cmd.AddCommand(newInitShellCmd("zsh", "連携関数を出力 (zsh 用)", func(bin string) string {
		return renderPosixInitScript("zsh", bin)
	}))
	cmd.AddCommand(newInitShellCmd("bash", "連携関数を出力 (bash 用)", func(bin string) string {
		return renderPosixInitScript("bash", bin)
	}))
	cmd.AddCommand(newInitShellCmd("fish", "連携関数を出力 (fish 用)", renderFishInitScript))
	return cmd
}

// newInitShellCmd はシェル名ごとの init サブコマンドを作る render には quote 済みのバイナリパスを渡す
func newInitShellCmd(shell, short string, render func(quotedBinary string) string) *cobra.Command {
	return &cobra.Command{
		Use:                   shell,
		Short:                 short,
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), render(quotedExecutablePath()))
			return err
		},
	}
}

// quotedExecutablePath は関数に埋め込む自分自身の絶対パスを quote して返す(PATH の変化に影響されないため)
// quote は Go の文字列リテラル形式で bash / zsh / fish のいずれも二重引用符内の \" と \\ を同じ意味で解釈する
func quotedExecutablePath() string {
	awspPath := "awsp"
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		awspPath = exePath
	}
	return strconv.Quote(awspPath)
}

// renderPosixInitScript は親シェルへ反映するための bash / zsh 共通の関数を生成する(D24)
// shell は見出しに使うシェル名 awspBinary は quote 済みのバイナリパス
func renderPosixInitScript(shell, awspBinary string) string {
	return fmt.Sprintf(`# awsp %[2]s integration
awsp() {
  local _arg
  for _arg in "$@"; do
    if [[ "$_arg" == "--shell" || "$_arg" == --shell=* ]]; then
      %[1]s "$@"
      return $?
    fi
  done

  case "$1" in
    %[3]s)
      %[1]s "$@"
      return $?
      ;;
  esac

  if [[ "$1" == -* ]]; then
    %[1]s "$@"
    return $?
  fi

  local _awsp_exports
  _awsp_exports="$(%[1]s "$@" --shell)"
  local _status=$?
  if [[ $_status -ne 0 ]]; then
    return $_status
  fi

  eval "$_awsp_exports"

  if [[ -n "${AWS_PROFILE:-}" ]]; then
    echo "✅ Set AWS_PROFILE=${AWS_PROFILE}"
  else
    echo "🧹 AWS_PROFILE を解除しました"
  fi
}
`, awspBinary, shell, strings.Join(passthroughSubcommands, "|"))
}

// renderFishInitScript は親シェルへ反映するための fish 関数を生成する(D24)
// --shell=fish の出力は行ごとに ; で終わるため リストを eval で連結してもコマンド境界が残る
func renderFishInitScript(awspBinary string) string {
	return fmt.Sprintf(`# awsp fish integration
function awsp --description 'AWS profile switcher'
    for _arg in $argv
        if string match -q -r -- '^--shell(=.*)?$' "$_arg"
            %[1]s $argv
            return $status
        end
    end

    switch "$argv[1]"
        case %[2]s
            %[1]s $argv
            return $status
    end

    if string match -q -- '-*' "$argv[1]"
        %[1]s $argv
        return $status
    end

    set -l _awsp_exports (%[1]s $argv --shell=fish)
    set -l _status $status
    if test $_status -ne 0
        return $_status
    end

    eval $_awsp_exports

    if set -q AWS_PROFILE
        echo "✅ Set AWS_PROFILE=$AWS_PROFILE"
    else
        echo "🧹 AWS_PROFILE を解除しました"
    end
end
`, awspBinary, strings.Join(passthroughSubcommands, " "))
}
