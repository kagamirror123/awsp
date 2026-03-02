// Package awsp は awsp コマンドのユースケースを提供する
package awsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/card"
	"github.com/pterm/pterm"
)

const unsetSelection = "(unset)"

// Profile は選択対象の AWS プロファイル情報
// インタラクティブ表示と検証で利用する
type Profile struct {
	Name          string
	Region        string
	Output        string
	SSOSession    string
	SSOStartURL   string
	SSORegion     string
	SSOAccountID  string
	SSORoleName   string
	RoleARN       string
	SourceProfile string
}

// ProfileStore は利用可能な AWS プロファイル一覧を返す
type ProfileStore interface {
	Profiles(ctx context.Context) ([]Profile, error)
}

// Selector は対話的にプロファイルを選ぶ
type Selector interface {
	Select(ctx context.Context, profiles []Profile) (string, error)
}

// AWSClient は AWS 認証処理の抽象
type AWSClient interface {
	CallerIdentity(ctx context.Context, profile string) (awscli.Identity, error)
	SSOSession(ctx context.Context, profile string) (string, error)
	Login(ctx context.Context, profile string, ssoSession string) error
}

// RunnerOptions は Runner 初期化時の依存をまとめる
type RunnerOptions struct {
	Logger   *slog.Logger
	Profiles ProfileStore
	Selector Selector
	AWS      AWSClient
	Stdout   io.Writer
	Stderr   io.Writer
}

// Runner は awsp のユースケースを実行する
type Runner struct {
	logger   *slog.Logger
	profiles ProfileStore
	selector Selector
	aws      AWSClient
	stdout   io.Writer
	stderr   io.Writer
}

// RunOptions は実行時の挙動を制御する
type RunOptions struct {
	ShellMode bool
	SkipLogin bool
	LoginOnly bool
}

// NewRunner は依存を束ねて Runner を作る
func NewRunner(options RunnerOptions) *Runner {
	return &Runner{
		logger:   options.Logger,
		profiles: options.Profiles,
		selector: options.Selector,
		aws:      options.AWS,
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
		r.emitUnset(options.ShellMode)
		return nil
	}

	if !options.SkipLogin {
		if err := r.ensureLoggedIn(ctx, selected, options.ShellMode); err != nil {
			return err
		}
	}

	if options.LoginOnly {
		if options.ShellMode {
			return nil
		}
		_, _ = fmt.Fprintln(r.stdout, renderSuccessLine(fmt.Sprintf("Login status OK for profile=%s", selected)))
		return nil
	}

	r.emitProfile(options.ShellMode, selected)
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
	// 失敗時はセッション期限切れを想定して CLI ログインへフォールバックする
	// ログイン導線を CLI に寄せる理由は internal/awscli/client.go の説明を参照
	identity, err := r.aws.CallerIdentity(ctx, profile)
	if err != nil {
		if !awscli.IsAuthRelatedError(err) {
			return fmt.Errorf("caller identity の取得に失敗: %w", err)
		}

		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, renderWarnLine("SSO ログインが必要です"))
		_, _ = fmt.Fprintln(writer, renderInfoLine("ブラウザ認証を開始します"))
		_, _ = fmt.Fprintln(writer)

		ssoSession, sessionErr := r.aws.SSOSession(ctx, profile)
		if sessionErr != nil && r.logger != nil {
			r.logger.Debug("sso_session の取得に失敗", "error", sessionErr)
		}

		if err := r.aws.Login(ctx, profile, ssoSession); err != nil {
			return err
		}

		identity, err = r.aws.CallerIdentity(ctx, profile)
		if err != nil {
			return fmt.Errorf("ログイン後も identity を取得できません: %w", err)
		}
	}

	_, _ = fmt.Fprintln(writer, renderIdentityCard(profile, identity))
	return nil
}

func (r *Runner) emitUnset(shellMode bool) {
	if shellMode {
		_, _ = fmt.Fprintln(r.stdout, "unset AWS_PROFILE AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN")
		return
	}
	_, _ = fmt.Fprintln(r.stdout, renderInfoLine("AWS_PROFILE と静的認証情報を解除しました"))
}

func (r *Runner) emitProfile(shellMode bool, profile string) {
	if shellMode {
		_, _ = fmt.Fprintln(r.stdout, "export AWS_SDK_LOAD_CONFIG=1")
		_, _ = fmt.Fprintf(r.stdout, "export AWS_PROFILE=%q\n", profile)
		_, _ = fmt.Fprintln(r.stdout, "unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN")
		return
	}
	_, _ = fmt.Fprintln(r.stdout, renderSuccessLine(fmt.Sprintf("Profile validated: %s", profile)))
	_, _ = fmt.Fprintln(r.stdout, renderInfoLine("この実行では親シェルの AWS_PROFILE は変更されません"))
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
	return card.Render("🪪 AWS Caller Identity", []string{
		fmt.Sprintf("🔐 Profile : %s", profile),
		fmt.Sprintf("🧾 Account : %s", identity.Account),
		fmt.Sprintf("👤 UserId  : %s", identity.UserID),
		fmt.Sprintf("🌍 ARN     : %s", identity.ARN),
	})
}

func renderSuccessLine(message string) string {
	return pterm.NewStyle(pterm.FgLightGreen, pterm.Bold).Sprintf("✅ %s", message)
}

func renderInfoLine(message string) string {
	return pterm.NewStyle(pterm.FgLightBlue).Sprintf("ℹ️ %s", message)
}

func renderWarnLine(message string) string {
	return pterm.NewStyle(pterm.FgLightYellow, pterm.Bold).Sprintf("⚠️ %s", message)
}
