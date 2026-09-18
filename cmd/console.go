package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssologin"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/spf13/cobra"
)

// newConsoleCmd は profile の account / role でマネジメントコンソールをブラウザで開くコマンドを作る(D31)
// アクセスポータルの deep link を組んで開くだけで トークンや認証情報には触れない
func newConsoleCmd() *cobra.Command {
	var noBrowser bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "console [profile] [destination-url]",
		Short: "指定 profile の account / role でマネジメントコンソールをブラウザで開く(省略時は AWS_PROFILE)",
		Long: "指定 profile の account / role でマネジメントコンソールをブラウザで開く(省略時は AWS_PROFILE)。\n" +
			"IAM Identity Center のアクセスポータルの deep link を組んで開くので、ブラウザに SSO のセッションが残っていれば再認証なしで入れる。\n" +
			"2 つ目の引数にコンソール内の URL を渡すと、そのアカウントでそのページを開く。",
		Example: "  awsp console\n  awsp console prod\n" +
			"  awsp console prod https://ap-northeast-1.console.aws.amazon.com/cloudwatch/home\n" +
			"  awsp console prod --no-browser",
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeProfileNames,
		SilenceUsage:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName, source, err := resolveConsoleProfile(args)
			if err != nil {
				return err
			}
			var destination string
			if len(args) == 2 {
				destination = args[1]
			}

			profileStore := newProfileStore()
			profiles, err := profileStore.ProfileDetails(cmd.Context())
			if err != nil {
				return err
			}
			sessions, err := profileStore.SSOSessions(cmd.Context())
			if err != nil {
				return err
			}

			profile, session, err := findSSOProfile(profileName, profiles, sessions)
			if err != nil {
				return err
			}

			report, err := awsp.BuildConsoleURL(profile, session, destination)
			if err != nil {
				return err
			}

			if !noBrowser {
				if err := ssologin.OpenBrowser(report.URL); err != nil {
					// 起動に失敗しても URL は出すので 人が手で開ける
					_, _ = fmt.Fprintln(ui.NewWriter(cmd.ErrOrStderr()), ui.WarnLine(err.Error()))
				}
			}

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			}

			out := ui.NewWriter(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, renderConsoleCard(report, source))
			return nil
		},
	}

	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "ブラウザを開かず URL の表示だけ行う")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "出力を JSON 形式にする")
	return cmd
}

// resolveConsoleProfile は引数の profile を優先し 無ければ AWS_PROFILE を使う
func resolveConsoleProfile(args []string) (string, string, error) {
	if len(args) >= 1 && args[0] != "" {
		return args[0], "arg", nil
	}
	return resolveCurrentProfile()
}

// findSSOProfile は名前で profile を探し その sso-session と共に返す
// 見つからない・SSO を使っていない場合は次の行動を含むエラーを返す
func findSSOProfile(name string, profiles []awsconfig.Profile, sessions []awsconfig.SSOSession) (awsconfig.Profile, awsconfig.SSOSession, error) {
	for _, profile := range profiles {
		if profile.Name != name {
			continue
		}
		session, ok := awsconfig.ResolveSession(profile, sessions)
		if !ok {
			return awsconfig.Profile{}, awsconfig.SSOSession{}, fmt.Errorf(
				"profile %q は SSO を使っていません: コンソールの deep link は SSO の profile だけ組めます(`awsp list` で確認してください)", name,
			)
		}
		return profile, session, nil
	}
	return awsconfig.Profile{}, awsconfig.SSOSession{}, fmt.Errorf(
		"指定プロファイルが見つかりません: %s: `awsp list` で確認してください", name,
	)
}

func renderConsoleCard(report awsp.ConsoleReport, source string) string {
	lines := []string{
		fmt.Sprintf("🔐 Profile : %s", report.Profile),
		fmt.Sprintf("📍 Source  : %s", source),
		fmt.Sprintf("🧾 Account : %s", report.Account),
		fmt.Sprintf("🎭 Role    : %s", report.Role),
	}
	if report.Destination != "" {
		lines = append(lines, fmt.Sprintf("🎯 Dest    : %s", report.Destination))
	}
	lines = append(lines, fmt.Sprintf("🔗 URL     : %s", report.URL))
	return ui.RenderCard("🖥️ AWS Console", lines)
}
