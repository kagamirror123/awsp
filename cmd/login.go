package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/spf13/cobra"
)

func newLoginCmd(opts *rootOptions) *cobra.Command {
	var ssoSessionName string
	var timeout time.Duration
	var noBrowser bool
	var useDeviceCode bool
	var force bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "login [profile]",
		Short: "ブラウザ承認で AWS SSO にログイン(既定: Authorization Code + PKCE)",
		Example: "  awsp login\n  awsp login dev\n  awsp login --sso-session corp\n  " +
			"awsp login --timeout 3m\n  awsp login --force\n  awsp login --use-device-code",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var profileArg string
			if len(args) == 1 {
				profileArg = args[0]
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

			session, profile, err := awsp.ResolveLoginSession(
				awsp.LoginTarget{Profile: profileArg, SSOSessionName: ssoSessionName},
				profiles,
				sessions,
			)
			if err != nil {
				return err
			}

			output := ui.NewWriter(cmd.OutOrStdout())
			if opts.shell {
				output = ui.NewWriter(cmd.ErrOrStderr())
			}

			result, err := awsp.Login(cmd.Context(), session, profile, awsp.LoginDeps{
				AWS: awscli.NewClient(),
			}, awsp.LoginOptions{
				Timeout:       timeout,
				Force:         force,
				OpenBrowser:   openBrowserOption(noBrowser),
				Output:        output,
				UseDeviceCode: useDeviceCode,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}

			out := ui.NewWriter(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, renderLoginResult(result))
			return nil
		},
	}

	cmd.Flags().StringVar(&ssoSessionName, "sso-session", "", "対象の sso-session 名")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "承認待ちの上限時間")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "ブラウザを開かず URL(device code なら Code も)の表示だけ行う")
	cmd.Flags().BoolVar(&force, "force", false, "有効なセッションが残っていてもログインし直す")
	cmd.Flags().BoolVar(
		&useDeviceCode,
		"use-device-code",
		false,
		"認可を device code 方式で行う(既定は Authorization Code + PKCE)",
	)
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "出力を JSON 形式にする")
	return cmd
}

// openBrowserOption は --no-browser 指定時にブラウザを起動しない関数を返す
// 未指定時は nil を返し ssologin の既定(OS 標準のオープンコマンド)に委ねる
func openBrowserOption(noBrowser bool) func(string) error {
	if !noBrowser {
		return nil
	}
	return func(string) error { return nil }
}

func renderLoginResult(result awsp.LoginResult) string {
	lines := []string{
		fmt.Sprintf("🔐 Session : %s", result.Session),
		fmt.Sprintf("📶 State   : %s", result.State),
	}
	if result.ExpiresAt != nil {
		lines = append(lines, fmt.Sprintf("⏳ Expires : %s", result.ExpiresAt.Format(time.RFC3339)))
	}
	if result.Identity != nil {
		lines = append(lines,
			fmt.Sprintf("🪪 Profile : %s", result.Identity.Profile),
			fmt.Sprintf("🧾 Account : %s", result.Identity.Account),
			fmt.Sprintf("👤 UserId  : %s", result.Identity.UserID),
			fmt.Sprintf("🌍 ARN     : %s", result.Identity.ARN),
		)
	}
	return ui.RenderCard("✅ AWS SSO Login", lines)
}
