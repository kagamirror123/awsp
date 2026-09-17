package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "シェル連携スクリプトを出力",
		Example: "  awsp init zsh",
	}

	cmd.AddCommand(newInitZshCmd())
	return cmd
}

func newInitZshCmd() *cobra.Command {
	return &cobra.Command{
		Use:                   "zsh",
		Short:                 "連携関数を出力 (zsh 用)",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 関数には自分自身の絶対パスを埋め込む(PATH の変化に影響されないため)
			awspPath := "awsp"
			if exePath, err := os.Executable(); err == nil && exePath != "" {
				awspPath = exePath
			}

			_, err := io.WriteString(cmd.OutOrStdout(), renderZshInitScript(strconv.Quote(awspPath)))
			return err
		},
	}
}

// renderZshInitScript は親シェルへ反映するための zsh 関数を生成する
// awspBinary は quote 済みのバイナリパス
func renderZshInitScript(awspBinary string) string {
	return fmt.Sprintf(`# awsp zsh integration
awsp() {
  local _arg
  for _arg in "$@"; do
    if [[ "$_arg" == "--shell" ]]; then
      %s "$@"
      return $?
    fi
  done

  case "$1" in
    current|list|completion|help|init|version|status|preflight|login|whoami|mcp)
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
`, awspBinary)
}
