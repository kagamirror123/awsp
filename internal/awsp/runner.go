// Package awsp は awsp コマンドのユースケースを提供する
package awsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ui"
)

const unsetSelection = "(unset)"

// Profile は選択対象の AWS プロファイル情報
// インタラクティブ表示と検証で利用する
type Profile = ProfileInfo

// IsSSO は SSO 関連設定があるかを返す(TUI の状態マーク切り分けに使う D9)
func (p ProfileInfo) IsSSO() bool {
	return p.SSOSession != "" || p.SSOStartURL != "" || p.SSOAccountID != "" || p.SSORoleName != ""
}

// toConfigProfile は awsconfig.ResolveSession に渡すための変換
func (p ProfileInfo) toConfigProfile() awsconfig.Profile {
	return awsconfig.Profile{
		Name:          p.Name,
		Region:        p.Region,
		Output:        p.Output,
		SSOSession:    p.SSOSession,
		SSOStartURL:   p.SSOStartURL,
		SSORegion:     p.SSORegion,
		SSOAccountID:  p.SSOAccountID,
		SSORoleName:   p.SSORoleName,
		RoleARN:       p.RoleARN,
		SourceProfile: p.SourceProfile,
	}
}

// ProfileStore は利用可能な AWS プロファイルと sso-session 一覧を返す
type ProfileStore interface {
	Profiles(ctx context.Context) ([]Profile, error)
	Sessions(ctx context.Context) ([]awsconfig.SSOSession, error)
}

// Selector は対話的にプロファイルを選ぶ
type Selector interface {
	Select(ctx context.Context, profiles []Profile) (string, error)
}

// LoginFunc は認証エラー時に SSO ログインを実行する関数
// 本番では ssologin を使った Login 実装を注入し テストではフェイクに差し替える
type LoginFunc func(ctx context.Context, session awsconfig.SSOSession, profile string) (LoginResult, error)

// RunnerOptions は Runner 初期化時の依存をまとめる
type RunnerOptions struct {
	Logger   *slog.Logger
	Profiles ProfileStore
	Selector Selector
	AWS      AWSIdentityClient
	Login    LoginFunc
	Stdout   io.Writer
	Stderr   io.Writer
}

// Runner は awsp のユースケースを実行する
type Runner struct {
	logger   *slog.Logger
	profiles ProfileStore
	selector Selector
	aws      AWSIdentityClient
	login    LoginFunc
	stdout   io.Writer
	stderr   io.Writer
}

// ShellSyntax は --shell 出力(親シェルへ反映するコマンド)の構文(D24)
type ShellSyntax string

const (
	// ShellSyntaxNone は人間向けの表示(--shell なし)
	ShellSyntaxNone ShellSyntax = ""
	// ShellSyntaxPOSIX は export / unset を使う構文(bash zsh)
	ShellSyntaxPOSIX ShellSyntax = "posix"
	// ShellSyntaxFish は set -gx / set -e を使う構文(fish)
	ShellSyntaxFish ShellSyntax = "fish"
)

// ParseShellSyntax は --shell の値を ShellSyntax に変換する
// 値なしの --shell は posix として扱う bash zsh は posix の別名
func ParseShellSyntax(value string) (ShellSyntax, error) {
	switch value {
	case "":
		return ShellSyntaxNone, nil
	case "posix", "bash", "zsh":
		return ShellSyntaxPOSIX, nil
	case "fish":
		return ShellSyntaxFish, nil
	default:
		return ShellSyntaxNone, fmt.Errorf("--shell の値が不正です: %q: posix か fish を指定してください", value)
	}
}

// RunOptions は実行時の挙動を制御する
type RunOptions struct {
	// Shell が空でなければ人間向け表示の代わりに親シェル用のコマンドを stdout へ出す
	Shell     ShellSyntax
	SkipLogin bool
	LoginOnly bool
}

// shellMode は --shell 出力モードかどうかを返す
func (o RunOptions) shellMode() bool {
	return o.Shell != ShellSyntaxNone
}

// NewRunner は依存を束ねて Runner を作る
func NewRunner(options RunnerOptions) *Runner {
	return &Runner{
		logger:   options.Logger,
		profiles: options.Profiles,
		selector: options.Selector,
		aws:      options.AWS,
		login:    options.Login,
		stdout:   options.Stdout,
		stderr:   options.Stderr,
	}
}

// Run は awsp のメイン処理
func (r *Runner) Run(ctx context.Context, profileArg string, options RunOptions) error {
	if err := options.validate(); err != nil {
		return err
	}

	profileList, err := r.profiles.Profiles(ctx)
	if err != nil {
		return err
	}

	selected, err := r.resolveSelection(ctx, profileArg, profileList)
	if err != nil {
		return err
	}

	if selected == unsetSelection {
		if options.LoginOnly {
			return errors.New("login-only では (unset) を選択できません")
		}
		r.emitUnset(options.Shell)
		return nil
	}

	if !options.SkipLogin {
		if err := r.ensureLoggedIn(ctx, selected, options.shellMode()); err != nil {
			return err
		}
	}

	if options.LoginOnly {
		if options.shellMode() {
			return nil
		}
		_, _ = fmt.Fprintln(r.stdout, ui.SuccessLine(fmt.Sprintf("Login status OK for profile=%s", selected)))
		return nil
	}

	r.emitProfile(options.Shell, selected)
	return nil
}

func (r *Runner) resolveSelection(ctx context.Context, profileArg string, profiles []Profile) (string, error) {
	if profileArg != "" {
		if profileArg == unsetSelection {
			return unsetSelection, nil
		}
		if !containsProfileName(profiles, profileArg) {
			return "", fmt.Errorf("指定プロファイルが見つかりません: %s", profileArg)
		}
		return profileArg, nil
	}

	if len(profiles) == 0 {
		return "", errors.New("利用可能なプロファイルがありません: ~/.aws/config を確認してください")
	}

	selected, err := r.selector.Select(ctx, profiles)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", errors.New("キャンセルしました")
		}
		return "", err
	}
	if selected == "" {
		return "", errors.New("プロファイルが選択されませんでした")
	}

	return selected, nil
}

func (r *Runner) ensureLoggedIn(ctx context.Context, profile string, shellMode bool) error {
	writer := r.stdout
	if shellMode {
		writer = r.stderr
	}

	// まず SDK で identity を取得して既存セッションの有効性を確認する
	// 失敗時はセッション期限切れを想定して SSO ログイン(internal/ssologin)へフォールバックする
	identity, err := r.aws.CallerIdentity(ctx, profile)
	if err != nil {
		if !awscli.IsAuthRelatedError(err) {
			return fmt.Errorf("caller identity の取得に失敗: %w", err)
		}

		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, ui.WarnLine("SSO ログインが必要です"))
		_, _ = fmt.Fprintln(writer, ui.InfoLine("ブラウザ認証を開始します"))
		_, _ = fmt.Fprintln(writer)

		session, sessionErr := r.resolveSession(ctx, profile)
		if sessionErr != nil {
			return sessionErr
		}

		if r.login == nil {
			return errors.New("ログイン処理が未設定です")
		}

		result, loginErr := r.login(ctx, session, profile)
		if loginErr != nil {
			return loginErr
		}
		if result.Identity == nil {
			return errors.New("ログイン後も identity を取得できません")
		}

		identity = awscli.Identity{
			Account: result.Identity.Account,
			UserID:  result.Identity.UserID,
			ARN:     result.Identity.ARN,
		}
	}

	_, _ = fmt.Fprintln(writer, renderIdentityCard(profile, identity))
	return nil
}

func (r *Runner) resolveSession(ctx context.Context, profile string) (awsconfig.SSOSession, error) {
	profiles, err := r.profiles.Profiles(ctx)
	if err != nil {
		return awsconfig.SSOSession{}, fmt.Errorf("profile 一覧の取得に失敗: %w", err)
	}
	sessions, err := r.profiles.Sessions(ctx)
	if err != nil {
		return awsconfig.SSOSession{}, fmt.Errorf("sso-session の取得に失敗: %w", err)
	}

	for _, p := range profiles {
		if p.Name != profile {
			continue
		}
		session, ok := awsconfig.ResolveSession(p.toConfigProfile(), sessions)
		if !ok {
			return awsconfig.SSOSession{}, fmt.Errorf("profile %q は SSO を使っていません: 認証情報を見直してください", profile)
		}
		return session, nil
	}

	return awsconfig.SSOSession{}, fmt.Errorf("指定プロファイルが見つかりません: %s", profile)
}

// staticCredentialVars は profile 切り替え時に解除する静的認証情報の環境変数
var staticCredentialVars = []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}

func (r *Runner) emitUnset(shell ShellSyntax) {
	switch shell {
	case ShellSyntaxPOSIX:
		_, _ = fmt.Fprintln(r.stdout, "unset AWS_PROFILE "+strings.Join(staticCredentialVars, " "))
	case ShellSyntaxFish:
		r.emitFishErase(append([]string{"AWS_PROFILE"}, staticCredentialVars...))
	default:
		_, _ = fmt.Fprintln(r.stdout, ui.InfoLine("AWS_PROFILE と静的認証情報を解除しました"))
	}
}

func (r *Runner) emitProfile(shell ShellSyntax, profile string) {
	switch shell {
	case ShellSyntaxPOSIX:
		_, _ = fmt.Fprintln(r.stdout, "export AWS_SDK_LOAD_CONFIG=1")
		_, _ = fmt.Fprintf(r.stdout, "export AWS_PROFILE=%q\n", profile)
		_, _ = fmt.Fprintln(r.stdout, "unset "+strings.Join(staticCredentialVars, " "))
	case ShellSyntaxFish:
		// fish の関数は出力を行のリストとして受け取り eval で 1 つに連結するため
		// 行末の ; で連結後もコマンド境界が残るようにする
		_, _ = fmt.Fprintln(r.stdout, "set -gx AWS_SDK_LOAD_CONFIG 1;")
		_, _ = fmt.Fprintf(r.stdout, "set -gx AWS_PROFILE %s;\n", fishQuote(profile))
		r.emitFishErase(staticCredentialVars)
	default:
		_, _ = fmt.Fprintln(r.stdout, ui.SuccessLine(fmt.Sprintf("Profile validated: %s", profile)))
		_, _ = fmt.Fprintln(r.stdout, ui.InfoLine("この実行では親シェルの AWS_PROFILE は変更されません"))
	}
}

// emitFishErase は fish でグローバル変数を消すコマンドを 1 変数 1 行で出す
// 未定義の変数を set -e すると status 1 になるため set -q で存在確認してから消す
func (r *Runner) emitFishErase(names []string) {
	for _, name := range names {
		_, _ = fmt.Fprintf(r.stdout, "set -q %s; and set -e -g %s;\n", name, name)
	}
}

// fishQuote は fish の単一引用符で値を包む 単一引用符内で意味を持つのは \ と ' だけ
func fishQuote(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return "'" + escaped + "'"
}

func (o RunOptions) validate() error {
	if o.SkipLogin && o.LoginOnly {
		return errors.New("no-login と login-only は同時に指定できません")
	}
	return nil
}

func containsProfileName(profiles []Profile, profile string) bool {
	names := make([]string, 0, len(profiles))
	for _, item := range profiles {
		names = append(names, item.Name)
	}
	return slices.Contains(names, profile)
}

func renderIdentityCard(profile string, identity awscli.Identity) string {
	return ui.RenderCard("🪪 AWS Caller Identity", []string{
		fmt.Sprintf("🔐 Profile : %s", profile),
		fmt.Sprintf("🧾 Account : %s", identity.Account),
		fmt.Sprintf("👤 UserId  : %s", identity.UserID),
		fmt.Sprintf("🌍 ARN     : %s", identity.ARN),
	})
}
