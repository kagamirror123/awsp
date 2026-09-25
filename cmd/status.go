package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/spf13/cobra"
)

const defaultGrace = 8 * time.Hour

func newStatusCmd() *cobra.Command {
	var jsonOutput bool
	var grace time.Duration

	cmd := &cobra.Command{
		Use:          "status",
		Short:        "認証状態を表示 (AWS SSO セッション)",
		Example:      "  awsp status\n  awsp status --json\n  awsp status --grace 8h",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := loadStatusReport(cmd.Context(), grace)
			if err != nil {
				return err
			}

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			}

			out := ui.NewWriter(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, renderStatusReport(report, out))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "出力を JSON 形式にする")
	cmd.Flags().DurationVar(&grace, "grace", defaultGrace, "失効後に自動更新を見込む猶予時間")
	return cmd
}

// loadStatusReport は現在の AWS config と sso/cache から StatusReport を組み立てる
func loadStatusReport(ctx context.Context, grace time.Duration) (awsp.StatusReport, error) {
	profileStore := newProfileStore()

	profiles, err := profileStore.ProfileDetails(ctx)
	if err != nil {
		return awsp.StatusReport{}, err
	}
	sessions, err := profileStore.SSOSessions(ctx)
	if err != nil {
		return awsp.StatusReport{}, err
	}

	cacheDir, err := ssocache.DefaultCacheDir()
	if err != nil {
		return awsp.StatusReport{}, err
	}

	return awsp.BuildStatusReport(profiles, sessions, awsp.StatusOptions{
		ConfigFile: profileStore.ConfigPath(),
		CacheDir:   cacheDir,
		Grace:      grace,
	})
}

// renderStatusReport は `awsp status` の人間向け表を描画する
// 表には Profiles の名前は並べず件数だけを載せ(D22) ok 以外のセッションがあれば
// その Summary(次に打つコマンド入り)を表の下に 1 行ずつ足す
// generatedAt は人が見ると実行時刻でしかなく 狭い端末で行を折り返させるだけなので JSON にだけ載せる
func renderStatusReport(report awsp.StatusReport, stdout io.Writer) string {
	lines := []string{
		ui.Heading(fmt.Sprintf("🔐 AWS SSO Status (%s)", report.Overall)),
		ui.Muted("configFile=" + report.ConfigFile),
		"",
	}

	if len(report.Sessions) == 0 {
		lines = append(lines, ui.WarnLine("SSO セッションが見つかりません"))
		return strings.Join(lines, "\n")
	}

	table := ui.NewTable("Session", "State", "Expires", "Remaining", "Profiles").
		MaxWidth(ui.TerminalWidth(stdout)).
		DropWhenNarrow("Remaining", "Expires")

	var warnings []string
	for _, session := range report.Sessions {
		name := session.Name
		if name == "" {
			name = session.StartURL
		}

		table.AddRow(
			name,
			stateBadge(session.State),
			sessionExpiresLabel(session),
			sessionRemainingLabel(session),
			strconv.Itoa(len(session.Profiles)),
		)

		if session.State != ssocache.StateOK {
			warnings = append(warnings, sessionSummaryLine(name, session))
		}
	}
	lines = append(lines, table.Render())
	lines = append(lines, warnings...)
	return strings.Join(lines, "\n")
}

// sessionExpiresLabel はローカル時刻の "01-02 15:04" 表記を返す(D22) unknown は "-"
func sessionExpiresLabel(session awsp.SessionStatus) string {
	if session.ExpiresAt == nil {
		return ui.Muted("-")
	}
	return session.ExpiresAt.Local().Format("01-02 15:04")
}

// sessionRemainingLabel は awsp.FormatRemaining と同じ書式の残り時間を返す 期限切れは負(D22) unknown は "-"
// 有効で自動更新が効くときは 残り時間がアクセストークンの残りにすぎないので "自動更新" と出す(D34)
func sessionRemainingLabel(session awsp.SessionStatus) string {
	if session.RemainingSeconds == nil {
		return ui.Muted("-")
	}
	if session.State == ssocache.StateOK && session.AutoRefresh {
		return "自動更新"
	}
	return awsp.FormatRemaining(time.Duration(*session.RemainingSeconds) * time.Second)
}

// sessionSummaryLine は ok 以外のセッションについて 次に打つコマンド入りの Summary を 1 行で返す
// warning は WarnLine error/unknown は ErrorLine で強調する
func sessionSummaryLine(name string, session awsp.SessionStatus) string {
	message := fmt.Sprintf("%s: %s", name, session.Summary)
	if session.Diagnostic != "" {
		message += "\n" + session.Diagnostic
	}
	if session.State == ssocache.StateWarning {
		return ui.WarnLine(message)
	}
	return ui.ErrorLine(message)
}

func stateBadge(state ssocache.EvaluationState) string {
	switch state {
	case ssocache.StateOK:
		return ui.Badge(ui.BadgeOK, string(state))
	case ssocache.StateWarning:
		return ui.Badge(ui.BadgeWarning, string(state))
	case ssocache.StateError:
		return ui.Badge(ui.BadgeError, string(state))
	default:
		return ui.Badge(ui.BadgeUnknown, string(state))
	}
}
