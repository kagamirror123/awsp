package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/spf13/cobra"
)

func newCurrentCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:          "current",
		Short:        "現在の AWS_PROFILE の caller identity を表示",
		Example:      "  awsp current\n  awsp current --json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, source, err := resolveCurrentProfile()
			if err != nil {
				return err
			}
			if err := ensureKnownProfile(cmd.Context(), profile, source); err != nil {
				return err
			}

			output := ui.NewWriter(cmd.ErrOrStderr())
			client := awscli.NewClient()
			deps := currentIdentityDeps{
				CallerIdentity: client.CallerIdentity,
				Login: func(ctx context.Context, profile string, output io.Writer) (awscli.Identity, error) {
					return loginForProfile(ctx, profile, client, output)
				},
			}

			identity, err := fetchCurrentIdentity(cmd.Context(), profile, output, deps)
			if err != nil {
				return fmt.Errorf("現在の identity を取得できません: %w", err)
			}

			if jsonOutput {
				report := awsp.CurrentReport{
					SchemaVersion: 1,
					GeneratedAt:   time.Now(),
					ConfigFile:    newProfileStore().ConfigPath(),
					Source:        source,
					Identity: awsp.Identity{
						Profile: profile,
						Account: identity.Account,
						UserID:  identity.UserID,
						ARN:     identity.ARN,
					},
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			}

			out := ui.NewWriter(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, renderCurrentCard(profile, source, identity, ui.TerminalWidth(out)))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "出力を JSON 形式にする")
	return cmd
}

// currentIdentityDeps は fetchCurrentIdentity が使う外部依存
// Login は認証エラー時に呼ばれる(実運用では loginForProfile がラップする)
type currentIdentityDeps struct {
	CallerIdentity func(ctx context.Context, profile string) (awscli.Identity, error)
	Login          func(ctx context.Context, profile string, output io.Writer) (awscli.Identity, error)
}

func fetchCurrentIdentity(
	ctx context.Context,
	profile string,
	output io.Writer,
	deps currentIdentityDeps,
) (awscli.Identity, error) {
	identity, err := deps.CallerIdentity(ctx, profile)
	if err == nil {
		return identity, nil
	}
	if !awscli.IsAuthRelatedError(err) {
		return awscli.Identity{}, err
	}

	_, _ = fmt.Fprintln(output)
	_, _ = fmt.Fprintln(output, ui.WarnLine("SSO ログインが必要です"))
	_, _ = fmt.Fprintln(output, ui.InfoLine("ブラウザ認証を開始します"))
	_, _ = fmt.Fprintln(output)

	return deps.Login(ctx, profile, output)
}

// loginForProfile は profile の属する sso-session を config から解決して awsp.Login を実行する
func loginForProfile(ctx context.Context, profile string, client awsp.AWSIdentityClient, output io.Writer) (awscli.Identity, error) {
	profileStore := newProfileStore()

	profiles, err := profileStore.ProfileDetails(ctx)
	if err != nil {
		return awscli.Identity{}, err
	}
	sessions, err := profileStore.SSOSessions(ctx)
	if err != nil {
		return awscli.Identity{}, err
	}

	session, _, err := awsp.ResolveLoginSession(awsp.LoginTarget{Profile: profile}, profiles, sessions)
	if err != nil {
		return awscli.Identity{}, err
	}

	result, err := awsp.Login(ctx, session, profile, awsp.LoginDeps{AWS: client}, awsp.LoginOptions{
		Output: output,
	})
	if err != nil {
		return awscli.Identity{}, err
	}
	if result.Identity == nil {
		return awscli.Identity{}, errors.New("ログイン後も identity を取得できません")
	}

	return awscli.Identity{
		Account: result.Identity.Account,
		UserID:  result.Identity.UserID,
		ARN:     result.Identity.ARN,
	}, nil
}

// renderCurrentCard は current / whoami のカードを描画する ARN も載せ 収まらない端末では枠を外す(D33)
func renderCurrentCard(profile string, source string, identity awscli.Identity, maxWidth int) string {
	return ui.RenderCard("🪪 Current AWS Identity", []string{
		fmt.Sprintf("🔐 Profile : %s", profile),
		fmt.Sprintf("📍 Source  : %s", source),
		fmt.Sprintf("🧾 Account : %s", identity.Account),
		fmt.Sprintf("👤 UserId  : %s", identity.UserID),
		fmt.Sprintf("🌍 ARN     : %s", identity.ARN),
	}, maxWidth)
}
