// Package cmd は awsp の CLI エントリとサブコマンドを提供する
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/charmbracelet/fang"
	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/prompt"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	verbose   bool
	shell     bool
	noLogin   bool
	loginOnly bool
}

// cliError は表示メッセージの抑制と exit code を制御するエラー
// silent=true の場合 Execute は追加のエラー表示をしない(呼び出し側が表示済みの場合に使う)
type cliError struct {
	err    error
	code   int
	silent bool
}

func (e *cliError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *cliError) Unwrap() error { return e.err }

// newExitError は表示メッセージ付きで exit code を指定するエラーを作る(D10 など)
func newExitError(code int, err error) error {
	return &cliError{err: err, code: code}
}

// newSilentExit はメッセージを表示済みの状態で exit code だけ指定する(preflight など)
func newSilentExit(code int) error {
	return &cliError{err: errors.New("silent exit"), code: code, silent: true}
}

// Execute は awsp コマンドのエントリポイント 戻り値はプロセスの exit code
// ヘルプ・エラー表示・配色は Fang に委ねる cliError の silent / code は WithErrorHandler で維持する
func Execute() int {
	rootCmd := newRootCmd()

	err := fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithVersion(versionLine()),
		fang.WithoutCompletions(), // awsp 自前の completion コマンドと重複するため無効化する
		fang.WithoutManpage(),
		fang.WithColorSchemeFunc(fangColorScheme),
		fang.WithErrorHandler(handleCLIError),
	)
	if err == nil {
		return 0
	}

	var typed *cliError
	if errors.As(err, &typed) {
		return typed.code
	}

	return 1
}

// handleCLIError は Fang のエラー表示を差し込む
// cliError.silent なら何も表示しない(preflight など呼び出し側が表示済みの場合)
// Fang 既定の表示は先頭語の Title Case 化と末尾ピリオドの付与でメッセージを加工するため
// (「AWS_PROFILE」が「Aws_profile」になる)メッセージは原文のまま出す
// stderr が端末でなければ装飾なしの 1 行にする(エージェントやパイプが読む前提)
func handleCLIError(w io.Writer, styles fang.Styles, err error) {
	var typed *cliError
	if errors.As(err, &typed) && typed.silent {
		return
	}

	// Fang は w をラップした writer で渡すため w 自身では端末判定できない
	// エラー出力先は常に stderr なので os.Stderr で判定する
	if !isatty.IsTerminal(os.Stderr.Fd()) {
		_, _ = fmt.Fprintln(w, err.Error())
		return
	}

	_, _ = fmt.Fprintln(w, styles.ErrorHeader.String())
	_, _ = fmt.Fprintln(w, styles.ErrorText.UnsetTransform().Render(err.Error()))
	_, _ = fmt.Fprintln(w)
}

// fangColorScheme は Fang のヘルプ・エラー表示の配色を internal/ui と同じパレットから作る(D8)
func fangColorScheme(ld lipgloss.LightDarkFunc) fang.ColorScheme {
	base := ld(lipgloss.Color("0"), lipgloss.Color("15"))
	return fang.ColorScheme{
		Base:           base,
		Title:          ui.ColorHeading,
		Description:    base,
		Codeblock:      ld(lipgloss.Color("253"), lipgloss.Color("235")),
		Program:        ui.ColorInfo,
		Command:        ui.ColorInfo,
		DimmedArgument: ui.ColorMuted,
		Comment:        ui.ColorMuted,
		Flag:           ui.ColorSuccess,
		FlagDefault:    ui.ColorMuted,
		Argument:       base,
		QuotedString:   ui.ColorWarning,
		Help:           ui.ColorMuted,
		Dash:           ui.ColorMuted,
		ErrorHeader:    [2]color.Color{ld(lipgloss.Color("15"), lipgloss.Color("0")), ui.ColorError},
		ErrorDetails:   ui.ColorError,
	}
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}

	cmd := &cobra.Command{
		Use:     "awsp [profile]",
		Short:   "プロファイルを選択して AWS に接続する CLI",
		Long:    "プロファイルを選択して AWS への接続確認まで行う CLI",
		Version: versionLine(),
		Example: strings.Join([]string{
			"  awsp",
			"  awsp dev",
			"  awsp --version",
			"  awsp --login-only",
			"  awsp current",
			"  awsp list",
			"  awsp status",
			"  awsp preflight",
			"  awsp login",
			"  awsp whoami dev",
			"  awsp init zsh",
		}, "\n"),
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(opts.verbose, cmd.ErrOrStderr())

			profileStore := newProfileStore()

			var profileArg string
			if len(args) == 1 {
				profileArg = args[0]
			}

			// D10: 対話 UI が必要になる場面(profile 未指定)で stdin が端末でなければ
			// TUI を起こさず次の行動を含むエラーで終わる
			if profileArg == "" && !isStdinTTY() {
				return newExitError(2, errors.New(
					"対話 UI には端末が必要です: 'awsp <profile>' か 'awsp list --json' を使ってください",
				))
			}

			// --shell では stdout が export 出力専用になるため
			// 対話UIとログイン案内(認可 URL、device code なら Code も含む)の表示は stderr 側へ出す
			stdout := ui.NewWriter(cmd.OutOrStdout())
			stderr := ui.NewWriter(cmd.ErrOrStderr())
			loginOutput := stdout
			// Selector(TUI)は bubbletea 自身が出力先から色プロファイルを判定するため
			// colorprofile.Writer で包まない生の Writer を渡す
			selectorOutput := cmd.OutOrStdout()
			if opts.shell {
				selectorOutput = cmd.ErrOrStderr()
				loginOutput = stderr
			}

			selector := prompt.NewSelectorWithIO(os.Stdin, selectorOutput)
			awsClient := awscli.NewClient()

			runner := awsp.NewRunner(awsp.RunnerOptions{
				Logger:   logger,
				Profiles: profileStoreAdapter{store: profileStore},
				Selector: selector,
				AWS:      awsClient,
				Login:    newLoginFunc(awsClient, loginOutput),
				Stdout:   stdout,
				Stderr:   stderr,
			})

			runOptions := awsp.RunOptions{
				ShellMode: opts.shell,
				SkipLogin: opts.noLogin,
				LoginOnly: opts.loginOnly,
			}

			if !opts.shell {
				_, _ = fmt.Fprintln(stdout)
			}
			if err := runner.Run(cmd.Context(), profileArg, runOptions); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "詳細ログを表示")
	cmd.Flags().BoolVar(&opts.shell, "shell", false, "親シェルへ反映するためのコマンドを標準出力へ出す")
	cmd.Flags().BoolVar(
		&opts.noLogin,
		"no-login",
		false,
		"認証確認とログインを行わず profile の反映処理だけ行う",
	)
	cmd.Flags().BoolVar(
		&opts.loginOnly,
		"login-only",
		false,
		"ログイン状態の確認だけ行い profile は反映しない",
	)
	cmd.SetVersionTemplate("{{printf \"%s\\n\" .Version}}")

	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newCurrentCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newPreflightCmd())
	cmd.AddCommand(newLoginCmd(opts))
	cmd.AddCommand(newWhoamiCmd())
	cmd.AddCommand(newCompletionCmd(cmd))
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newMCPCmd())

	return cmd
}

// newLoginFunc は Runner から使う awsp.LoginFunc の実運用実装を作る
// ログインの URL(device code なら Code も)は output に表示する
func newLoginFunc(client awsp.AWSIdentityClient, output io.Writer) awsp.LoginFunc {
	return func(ctx context.Context, session awsconfig.SSOSession, profile string) (awsp.LoginResult, error) {
		return awsp.Login(ctx, session, profile, awsp.LoginDeps{AWS: client}, awsp.LoginOptions{
			Output: output,
		})
	}
}

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
			_, _ = fmt.Fprintln(out, renderCurrentCard(profile, source, identity))
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

			// 人間向け表示は D9 の状態/残り時間/最終使用を足すため
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

// resolveConfigFilePath は AWS config ファイルのパスを決める(D5)
// AWS_CONFIG_FILE が設定されていればそれを優先し 未設定なら SDK 標準の ~/.aws/config を使う
func resolveConfigFilePath() string {
	if path := strings.TrimSpace(os.Getenv("AWS_CONFIG_FILE")); path != "" {
		return path
	}
	return awssdkconfig.DefaultSharedConfigFilename()
}

func newProfileStore() *awsconfig.ProfileStore {
	return awsconfig.NewProfileStore(resolveConfigFilePath())
}

func resolveCurrentProfile() (string, string, error) {
	profileFromEnv := os.Getenv("AWS_PROFILE")
	if profileFromEnv != "" {
		return profileFromEnv, "env", nil
	}

	return "", "", errors.New("AWS_PROFILE が未設定です: `awsp <profile>` で選択してから実行してください")
}

func ensureKnownProfile(ctx context.Context, profile string, source string) error {
	profileStore := newProfileStore()

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

func isStdinTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
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

// Profiles は config の profile 一覧に D9 の認証状態(sso-session の状態 / 残り時間 / 最終使用)を付与して返す
// awsp.BuildProfileList と同じくローカルファイル(ssocache)だけを読み ネットワークは使わない
func (a profileStoreAdapter) Profiles(ctx context.Context) ([]awsp.Profile, error) {
	items, err := a.store.ProfileDetails(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := a.store.SSOSessions(ctx)
	if err != nil {
		return nil, err
	}

	ssoCacheDir, err := ssocache.DefaultCacheDir()
	if err != nil {
		return nil, err
	}
	cliCacheDir, err := cliCacheDirOrEmpty()
	if err != nil {
		return nil, err
	}

	list, err := awsp.BuildProfileList(items, sessions, awsp.ProfileListOptions{
		ConfigFile:  a.store.ConfigPath(),
		SSOCacheDir: ssoCacheDir,
		CLICacheDir: cliCacheDir,
		Grace:       defaultGrace,
	})
	if err != nil {
		return nil, err
	}

	profiles := make([]awsp.Profile, 0, len(items))
	for i, item := range items {
		info := list.Profiles[i]
		profiles = append(profiles, awsp.Profile{
			Name:                item.Name,
			Region:              item.Region,
			Output:              item.Output,
			SSOSession:          item.SSOSession,
			SSOStartURL:         item.SSOStartURL,
			SSORegion:           item.SSORegion,
			SSOAccountID:        item.SSOAccountID,
			SSORoleName:         item.SSORoleName,
			RoleARN:             item.RoleARN,
			SourceProfile:       item.SourceProfile,
			SessionState:        info.SessionState,
			SessionExpiresAt:    info.SessionExpiresAt,
			LastUsedAt:          info.LastUsedAt,
			CredentialExpiresAt: info.CredentialExpiresAt,
		})
	}

	return profiles, nil
}

func (a profileStoreAdapter) Sessions(ctx context.Context) ([]awsconfig.SSOSession, error) {
	return a.store.SSOSessions(ctx)
}

func renderCurrentCard(profile string, source string, identity awscli.Identity) string {
	return ui.RenderCard("🪪 Current AWS Identity", []string{
		fmt.Sprintf("🔐 Profile : %s", profile),
		fmt.Sprintf("📍 Source  : %s", source),
		fmt.Sprintf("🧾 Account : %s", identity.Account),
		fmt.Sprintf("👤 UserId  : %s", identity.UserID),
		fmt.Sprintf("🌍 ARN     : %s", identity.ARN),
	})
}

// renderProfileList は `awsp list` の人間向け表を描画する
// D9 で足した State / Expires / Last used も列に含める
// Current / # / Auth の列は廃止し(D22) 現在の profile は名前の前に "▶ " を付けて強調する
func renderProfileList(profiles []awsp.Profile, stdout io.Writer) string {
	if len(profiles) == 0 {
		return ui.WarnLine("🫥 profile が見つかりません")
	}

	current := os.Getenv("AWS_PROFILE")
	now := time.Now()

	table := ui.NewTable("Profile", "Region", "Account", "Role", "Source", "State", "Expires", "Last used").
		Truncate("Role", 24).
		Truncate("Account", 14).
		DropWhenNarrow("Last used", "Source", "Region", "Expires").
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

// profileLastUsedLabel は ~/.aws/cli/cache の最終使用時刻を返す
func profileLastUsedLabel(profile awsp.Profile) string {
	if profile.LastUsedAt == nil {
		return ui.Muted("-")
	}
	// 表に収めるため status の Expires 列と同じ短い書式にする
	return profile.LastUsedAt.Local().Format("01-02 15:04")
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
