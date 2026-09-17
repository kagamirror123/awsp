package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:          "list",
		Short:        "利用可能なプロファイル一覧を表示",
		Example:      "  awsp list\n  awsp list --json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileStore := newProfileStore()

			if jsonOutput {
				profiles, err := profileStore.ProfileDetails(cmd.Context())
				if err != nil {
					return err
				}
				sessions, err := profileStore.SSOSessions(cmd.Context())
				if err != nil {
					return err
				}

				ssoCacheDir, err := ssocache.DefaultCacheDir()
				if err != nil {
					return err
				}
				cliCacheDir, err := cliCacheDirOrEmpty()
				if err != nil {
					return err
				}

				list, err := awsp.BuildProfileList(profiles, sessions, awsp.ProfileListOptions{
					ConfigFile:  profileStore.ConfigPath(),
					SSOCacheDir: ssoCacheDir,
					CLICacheDir: cliCacheDir,
					Grace:       defaultGrace,
				})
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(list)
			}

			// 人間向け表示は D9 の状態/残り時間/認証情報取得を足すため
			// profileStoreAdapter で awsp.BuildProfileList 相当の情報を埋めた Profile を使う
			profiles, err := (profileStoreAdapter{store: profileStore}).Profiles(cmd.Context())
			if err != nil {
				return err
			}

			out := ui.NewWriter(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, renderProfileList(profiles, out))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "出力を JSON 形式にする")
	return cmd
}

// renderProfileList は `awsp list` の人間向け表を描画する
// D9 で足した State / Expires / Fetched も列に含める
// Current / # / Auth の列は廃止し(D22) 現在の profile は名前の前に "▶ " を付けて強調する
func renderProfileList(profiles []awsp.Profile, stdout io.Writer) string {
	if len(profiles) == 0 {
		return ui.WarnLine("🫥 profile が見つかりません")
	}

	current := os.Getenv("AWS_PROFILE")
	now := time.Now()

	table := ui.NewTable("Profile", "Region", "Account", "Role", "Source", "State", "Expires", "Fetched").
		Truncate("Role", 24).
		Truncate("Account", 14).
		DropWhenNarrow("Fetched", "Source", "Region", "Expires").
		MaxWidth(ui.TerminalWidth(stdout))

	for _, profile := range profiles {
		isCurrent := current != "" && current == profile.Name

		// 現在の profile は "▶ " を付けて強調し 他は先頭 2 桁を空白で揃える(D22)
		name := "  " + profile.Name
		if isCurrent {
			name = ui.Badge(ui.BadgeOK, "▶ "+profile.Name)
		}

		table.AddRow(
			name,
			ui.DashIfEmpty(profile.Region),
			ui.DashIfEmpty(profile.SSOAccountID),
			ui.DashIfEmpty(profile.SSORoleName),
			ui.DashIfEmpty(profile.SourceProfile),
			profileStateBadge(profile),
			profileExpiresLabel(profile, now),
			profileLastUsedLabel(profile),
		)
	}

	currentView := "-"
	if strings.TrimSpace(current) != "" {
		currentView = current
	}

	lines := []string{
		ui.Heading("📚 Available profiles"),
		ui.Muted(fmt.Sprintf("total=%d  current=%s", len(profiles), currentView)),
		"",
		table.Render(),
	}
	for _, profile := range profiles {
		for _, diagnostic := range profile.Diagnostics {
			lines = append(lines, ui.WarnLine(profile.Name+": "+diagnostic))
		}
	}
	return strings.Join(lines, "\n")
}

// profileStateBadge は profile の sso-session 状態を色付きで返す
// SSO を使わない(static な)profile は Auth 列の廃止に伴い 🪪 を返す(D22)
func profileStateBadge(profile awsp.Profile) string {
	if !profile.IsSSO() {
		return "🪪"
	}

	switch profile.SessionState {
	case ssocache.StateOK:
		return ui.Badge(ui.BadgeOK, string(profile.SessionState))
	case ssocache.StateWarning:
		return ui.Badge(ui.BadgeWarning, string(profile.SessionState))
	case ssocache.StateError:
		return ui.Badge(ui.BadgeError, string(profile.SessionState))
	default:
		return ui.Badge(ui.BadgeUnknown, string(ssocache.StateUnknown))
	}
}

// profileExpiresLabel は sso-session の残り時間を返す(D9 と同じ書式 期限切れは負)
func profileExpiresLabel(profile awsp.Profile, now time.Time) string {
	if profile.SessionExpiresAt == nil {
		return ui.Muted("-")
	}
	return awsp.FormatRemaining(profile.SessionExpiresAt.Sub(now))
}

// profileLastUsedLabel は ~/.aws/cli/cache の認証情報キャッシュの取得・更新時刻を返す
func profileLastUsedLabel(profile awsp.Profile) string {
	if profile.LastUsedAt == nil {
		return ui.Muted("-")
	}
	// 表に収めるため status の Expires 列と同じ短い書式にする
	return profile.LastUsedAt.Local().Format("01-02 15:04")
}
