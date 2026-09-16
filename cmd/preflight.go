package cmd

import (
	"fmt"
	"time"

	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/spf13/cobra"
)

func newPreflightCmd() *cobra.Command {
	var grace time.Duration

	cmd := &cobra.Command{
		Use:           "preflight",
		Short:         "フック向けに AWS SSO の状態を 1 行で確認(SessionStart 用)",
		Example:       "  awsp preflight\n  awsp preflight --grace 8h",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := loadStatusReport(cmd.Context(), grace)
			if err != nil {
				return err
			}

			line, exitCode := awsp.PreflightLine(report)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			if exitCode != 0 {
				return newSilentExit(exitCode)
			}
			return nil
		},
	}

	cmd.Flags().DurationVar(&grace, "grace", defaultGrace, "失効後に自動更新を見込む猶予時間")
	return cmd
}
