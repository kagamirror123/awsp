// Package cmd は awsp の CLI エントリとサブコマンドを提供する
package cmd

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/charmbracelet/fang"
	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/prompt"
	"github.com/kagamirror123/awsp/internal/ui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	verbose bool
	// shell は --shell の値 空なら人間向け表示 値なしの --shell は posix(D24)
	shell     string
	noLogin   bool
	loginOnly bool
}

// shellMode は --shell 出力モードかどうかを返す
func (o *rootOptions) shellMode() bool {
	return o.shell != ""
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
			"  awsp console",
			"  awsp init zsh",
			"  awsp init fish",
		}, "\n"),
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProfileNames,
		SilenceUsage:      true,
		SilenceErrors:     true,
		RunE: func(cmd *cobra.Command, args []string) error {
			shellSyntax, err := awsp.ParseShellSyntax(opts.shell)
			if err != nil {
				return err
			}

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
			if opts.shellMode() {
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
				Shell:     shellSyntax,
				SkipLogin: opts.noLogin,
				LoginOnly: opts.loginOnly,
			}

			if !opts.shellMode() {
				_, _ = fmt.Fprintln(stdout)
			}
			if err := runner.Run(cmd.Context(), profileArg, runOptions); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "詳細ログを表示")
	cmd.Flags().StringVar(
		&opts.shell,
		"shell",
		"",
		"親シェルへ反映するためのコマンドを標準出力へ出す(posix|fish。値なしは posix)",
	)
	// 値なしの --shell を許す(既存の zsh 関数が `--shell` 単独で呼ぶため)。値は --shell=fish の形で渡す
	cmd.Flags().Lookup("shell").NoOptDefVal = "posix"
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
	cmd.AddCommand(newConsoleCmd())
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

func newCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "シェル補完スクリプトを生成",
		Example:               "  awsp completion zsh > ~/.zsh/completions/_awsp\n  awsp completion bash > /etc/bash_completion.d/awsp\n  awsp completion fish > ~/.config/fish/completions/awsp.fish",
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
