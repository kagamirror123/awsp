package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:          "whoami [profile]",
		Short:        "指定 profile の caller identity を表示(省略時は AWS_PROFILE。自動ログインしない)",
		Example:      "  awsp whoami\n  awsp whoami dev\n  awsp whoami dev --json",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, source, err := resolveWhoamiProfile(args)
			if err != nil {
				return err
			}
			client := awscli.NewClient()

			identity, err := awsp.Whoami(cmd.Context(), client, profile)
			if err != nil {
				return err
			}

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(awsp.IdentityReport{SchemaVersion: 1, Identity: identity})
			}

			out := ui.NewWriter(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, renderCurrentCard(profile, source, awscli.Identity{
				Account: identity.Account,
				UserID:  identity.UserID,
				ARN:     identity.ARN,
			}))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "出力を JSON 形式にする")
	return cmd
}

// resolveWhoamiProfile は引数の profile を優先し 無ければ AWS_PROFILE を使う
// 戻り値の 2 番目は表示用の取得元("arg" か "env")
func resolveWhoamiProfile(args []string) (string, string, error) {
	if len(args) == 1 && args[0] != "" {
		return args[0], "arg", nil
	}
	profile, source, err := resolveCurrentProfile()
	if err != nil {
		return "", "", errors.New(
			"profile が未指定で AWS_PROFILE も未設定です: 'awsp whoami <profile>' か、'awsp <profile>' で選択してから実行してください",
		)
	}
	return profile, source, nil
}
