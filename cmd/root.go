// Package cmd は awsp の CLI エントリとサブコマンドを提供する
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	ptext "github.com/jedib0t/go-pretty/v6/text"
	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/card"
	"github.com/kagamirror123/awsp/internal/prompt"
	"github.com/mattn/go-isatty"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	verbose   bool
	shell     bool
	noLogin   bool
	loginOnly bool
}

// Execute は awsp コマンドのエントリポイント
func Execute() error {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "❌ %s\n", err)
		return err
	}
	return nil
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}

	cmd := &cobra.Command{
		Use:           "awsp [profile]",
		Short:         "AWS プロファイルを選択して接続する CLI",
		Long:          "AWS プロファイルを選択して接続確認まで行う CLI",
		Version:       versionLine(),
		Example:       "  awsp\n  awsp dev\n  awsp --version\n  awsp --login-only\n  awsp current\n  awsp list\n  awsp init zsh",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(opts.verbose, cmd.ErrOrStderr())
			configureColorOutput(cmd.OutOrStdout())

			profileStore, err := newProfileStore()
			if err != nil {
				return err
			}

			selectorOutput := cmd.OutOrStdout()
			awsStdout := cmd.OutOrStdout()
			awsStderr := cmd.ErrOrStderr()
			if opts.shell {
				// --shell では stdout が export 出力専用になるため
				// 対話UIと aws sso login の表示は stderr 側へ出す
				selectorOutput = cmd.ErrOrStderr()
				awsStdout = cmd.ErrOrStderr()
			}

			selector := prompt.NewSelectorWithIO(os.Stdin, selectorOutput)
			awsClient := awscli.NewClient(logger, awsStdout, awsStderr)

			runner := awsp.NewRunner(awsp.RunnerOptions{
				Logger:   logger,
				Profiles: profileStoreAdapter{store: profileStore},
				Selector: selector,
				AWS:      awsClient,
				Stdout:   cmd.OutOrStdout(),
				Stderr:   cmd.ErrOrStderr(),
			})

			var profileArg string
			if len(args) == 1 {
				profileArg = args[0]
			}

			runOptions := awsp.RunOptions{
				ShellMode: opts.shell,
				SkipLogin: opts.noLogin,
				LoginOnly: opts.loginOnly,
			}

			if !opts.shell {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}
			if err := runner.Run(cmd.Context(), profileArg, runOptions); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "詳細ログを表示")
	cmd.Flags().BoolVar(&opts.shell, "shell", false, "eval 用にシェルコマンドを標準出力へ出す")
	cmd.Flags().BoolVar(
		&opts.noLogin,
		"no-login",
		false,
		"caller identity と sso login を行わず profile の反映処理だけ行う",
	)
	cmd.Flags().BoolVar(
		&opts.loginOnly,
		"login-only",
		false,
		"profile を反映せずログイン状態の確認だけ行う",
	)
	cmd.SetHelpTemplate(helpTemplate())
	cmd.SetUsageTemplate(usageTemplate())
	cmd.SetVersionTemplate("{{printf \"%s\\n\" .Version}}")

	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newCurrentCmd(opts))
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newCompletionCmd(cmd))
	cmd.AddCommand(newInitCmd())

	return cmd
}

func newCurrentCmd(opts *rootOptions) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:          "current",
		Short:        "現在の AWS_PROFILE の caller identity を表示",
		Example:      "  awsp current\n  awsp current --json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := newLogger(opts.verbose, cmd.ErrOrStderr())
			awsClient := awscli.NewClient(logger, cmd.ErrOrStderr(), cmd.ErrOrStderr())

			profile, source, err := resolveCurrentProfile()
			if err != nil {
				return err
			}
			if err := ensureKnownProfile(cmd.Context(), profile, source); err != nil {
				return err
			}
			identity, err := awsClient.CallerIdentity(cmd.Context(), profile)
			if err != nil {
				if !awscli.IsAuthRelatedError(err) {
					return fmt.Errorf("現在の identity を取得できません: %w", err)
				}

				configureColorOutput(cmd.ErrOrStderr())
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
				_, _ = fmt.Fprintln(
					cmd.ErrOrStderr(),
					pterm.NewStyle(pterm.FgLightYellow, pterm.Bold).Sprint("⚠️ SSO ログインが必要です"),
				)
				_, _ = fmt.Fprintln(
					cmd.ErrOrStderr(),
					pterm.NewStyle(pterm.FgLightBlue).Sprint("ℹ️ ブラウザ認証を開始します"),
				)
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())

				ssoSession, sessionErr := awsClient.SSOSession(cmd.Context(), profile)
				if sessionErr != nil && logger != nil {
					logger.Debug("sso_session の取得に失敗", "error", sessionErr)
				}

				if err := awsClient.Login(cmd.Context(), profile, ssoSession); err != nil {
					return fmt.Errorf("現在の identity を取得できません: %w", err)
				}

				identity, err = awsClient.CallerIdentity(cmd.Context(), profile)
				if err != nil {
					return fmt.Errorf("現在の identity を取得できません: %w", err)
				}
			}

			if jsonOutput {
				payload := map[string]string{
					"profile": profile,
					"source":  source,
					"account": identity.Account,
					"userId":  identity.UserID,
					"arn":     identity.ARN,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			configureColorOutput(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderCurrentCard(profile, source, identity))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "JSON 形式で出力")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "version",
		Short:        "バージョン情報を表示",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), versionDetail())
			return err
		},
	}
}

func newListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:          "list",
		Short:        "利用可能なプロファイル一覧を表示",
		Example:      "  awsp list\n  awsp list --json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profileStore, err := newProfileStore()
			if err != nil {
				return err
			}

			profiles, err := profileStore.ProfileDetails(cmd.Context())
			if err != nil {
				return err
			}

			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(profiles)
			}

			configureColorOutput(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), renderProfileList(profiles))
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "JSON 形式で出力")
	return cmd
}

func newCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "シェル補完スクリプトを生成",
		Example:               "  awsp completion zsh > ~/.zsh/completions/_awsp",
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return rootCmd.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return fmt.Errorf("未対応シェルです: %s", args[0])
			}
		},
	}

	return cmd
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "シェル連携スクリプトを出力",
		Example: "  awsp init zsh",
	}

	cmd.AddCommand(newInitZshCmd())
	return cmd
}

func helpTemplate() string {
	return `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}
{{end}}
{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`
}

func usageTemplate() string {
	return `🚀 Usage{{if .Runnable}}
  {{.UseLine}}{{end}}{{if and (not .Runnable) .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

🔁 Aliases
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

💡 Examples
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{if eq (len .Groups) 0}}

📦 Commands{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $.Commands}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands{{range .Commands}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

⚙️ Flags
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

🌐 Global Flags
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

🧭 Topics{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

📘 More
  {{.CommandPath}} [command] --help{{end}}
`
}

func newInitZshCmd() *cobra.Command {
	return &cobra.Command{
		Use:                   "zsh",
		Short:                 "zsh 用の awsp 連携関数を出力",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		SilenceUsage:          true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			awspPath := "awsp"
			exePath, err := os.Executable()
			if err == nil && exePath != "" {
				awspPath = exePath
			}

			_, err = io.WriteString(cmd.OutOrStdout(), renderZshInitScript(awspPath))
			return err
		},
	}
}

func newProfileStore() (*awsconfig.ProfileStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("ホームディレクトリを取得できません: %w", err)
	}

	return awsconfig.NewProfileStore(filepath.Join(homeDir, ".aws", "config")), nil
}

func resolveCurrentProfile() (string, string, error) {
	profileFromEnv := os.Getenv("AWS_PROFILE")
	if profileFromEnv != "" {
		return profileFromEnv, "env", nil
	}

	return "", "", errors.New("AWS_PROFILE が未設定です: `awsp <profile>` で選択してから実行してください")
}

func ensureKnownProfile(ctx context.Context, profile string, source string) error {
	profileStore, err := newProfileStore()
	if err != nil {
		return err
	}

	profiles, err := profileStore.Profiles(ctx)
	if err != nil {
		return err
	}

	if slices.Contains(profiles, profile) {
		return nil
	}

	if source == "env" {
		return fmt.Errorf(
			"AWS_PROFILE=%q が ~/.aws/config に見つかりません: `unset AWS_PROFILE` して `awsp <profile>` を実行してください",
			profile,
		)
	}

	return fmt.Errorf(
		"指定プロファイル %q が ~/.aws/config に見つかりません: `awsp list` で確認してください",
		profile,
	)
}

func configureColorOutput(writer io.Writer) {
	if os.Getenv("NO_COLOR") != "" {
		ptext.DisableColors()
		pterm.DisableColor()
		return
	}

	file, ok := writer.(*os.File)
	if !ok {
		ptext.DisableColors()
		pterm.DisableColor()
		return
	}

	if isatty.IsTerminal(file.Fd()) {
		ptext.EnableColors()
		pterm.EnableColor()
		return
	}

	ptext.DisableColors()
	pterm.DisableColor()
}

func newLogger(verbose bool, writer io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		AddSource: verbose,
		Level:     level,
	})

	return slog.New(handler)
}

type profileStoreAdapter struct {
	store *awsconfig.ProfileStore
}

func (a profileStoreAdapter) Profiles(ctx context.Context) ([]awsp.Profile, error) {
	items, err := a.store.ProfileDetails(ctx)
	if err != nil {
		return nil, err
	}

	profiles := make([]awsp.Profile, 0, len(items))
	for _, item := range items {
		profiles = append(profiles, awsp.Profile{
			Name:          item.Name,
			Region:        item.Region,
			Output:        item.Output,
			SSOSession:    item.SSOSession,
			SSOStartURL:   item.SSOStartURL,
			SSORegion:     item.SSORegion,
			SSOAccountID:  item.SSOAccountID,
			SSORoleName:   item.SSORoleName,
			RoleARN:       item.RoleARN,
			SourceProfile: item.SourceProfile,
		})
	}

	return profiles, nil
}

func renderCurrentCard(profile string, source string, identity awscli.Identity) string {
	return card.Render("🪪 Current AWS Identity", []string{
		fmt.Sprintf("🔐 Profile : %s", profile),
		fmt.Sprintf("📍 Source  : %s", source),
		fmt.Sprintf("🧾 Account : %s", identity.Account),
		fmt.Sprintf("👤 UserId  : %s", identity.UserID),
		fmt.Sprintf("🌍 ARN     : %s", identity.ARN),
	})
}

func renderProfileList(profiles []awsconfig.Profile) string {
	if len(profiles) == 0 {
		return pterm.NewStyle(pterm.FgLightYellow).Sprint("🫥 profile が見つかりません")
	}

	current := os.Getenv("AWS_PROFILE")
	writer := table.NewWriter()
	tableStyle := table.StyleRounded
	tableStyle.Color = table.ColorOptions{
		Border: ptext.Colors{ptext.FgHiBlue},
		Header: ptext.Colors{ptext.FgHiCyan},
		Row:    ptext.Colors{ptext.FgWhite},
	}
	writer.SetStyle(tableStyle)
	writer.SetColumnConfigs([]table.ColumnConfig{
		{Name: "Current", Align: ptext.AlignCenter},
		{Name: "#", Align: ptext.AlignRight},
		{Name: "Profile", Align: ptext.AlignLeft},
		{Name: "Region", Align: ptext.AlignLeft},
		{Name: "Auth", Align: ptext.AlignLeft},
		{Name: "Account", Align: ptext.AlignLeft},
		{Name: "Role", Align: ptext.AlignLeft},
		{Name: "Source", Align: ptext.AlignLeft},
	})
	writer.AppendHeader(table.Row{"Current", "#", "Profile", "Region", "Auth", "Account", "Role", "Source"})

	for idx, profile := range profiles {
		isCurrent := current != "" && current == profile.Name

		marker := ptext.Faint.Sprint(".")
		if isCurrent {
			marker = ptext.FgGreen.Sprint("*")
		}

		auth := ptext.FgYellow.Sprint("static")
		if profile.IsSSO() {
			auth = ptext.FgCyan.Sprint("sso")
		}

		name := profile.Name
		if isCurrent {
			name = ptext.FgGreen.Sprint(profile.Name)
		}

		writer.AppendRow(table.Row{
			marker,
			idx + 1,
			name,
			dashIfEmpty(profile.Region),
			auth,
			dashIfEmpty(profile.SSOAccountID),
			dashIfEmpty(profile.SSORoleName),
			dashIfEmpty(profile.SourceProfile),
		})
	}

	currentView := "-"
	if strings.TrimSpace(current) != "" {
		currentView = current
	}

	lines := []string{
		pterm.NewStyle(pterm.FgLightCyan, pterm.Bold).Sprint("📚 Available profiles"),
		pterm.NewStyle(pterm.FgGray).Sprintf("total=%d  current=%s", len(profiles), currentView),
		"",
		writer.Render(),
	}
	return strings.Join(lines, "\n")
}

func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return ptext.Faint.Sprint("-")
	}
	return value
}

func renderZshInitScript(awspPath string) string {
	escapedPath := strconv.Quote(awspPath)

	return fmt.Sprintf(`# awsp zsh integration
awsp() {
  local _awsp_bin=%s
  local _arg
  for _arg in "$@"; do
    if [[ "$_arg" == "--shell" ]]; then
      "$_awsp_bin" "$@"
      return $?
    fi
  done

  case "$1" in
    current|list|completion|help|init|version)
      "$_awsp_bin" "$@"
      return $?
      ;;
  esac

  if [[ "$1" == -* ]]; then
    "$_awsp_bin" "$@"
    return $?
  fi

  local _awsp_exports
  _awsp_exports="$("$_awsp_bin" "$@" --shell)"
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
`, escapedPath)
}
