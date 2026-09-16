// Package awsp は awsp コマンドのユースケースを提供する
package awsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ui"
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

	// SessionState はこの profile が属する sso-session の状態(D9)
	// SSO を使わない profile では空文字のまま
	SessionState ssocache.EvaluationState
	// SessionExpiresAt は sso-session のトークン有効期限 未算出/該当なしは nil
	SessionExpiresAt *time.Time
	// LastUsedAt は ~/.aws/cli/cache の最終使用時刻(mtime) 未算出/該当なしは nil
	LastUsedAt *time.Time
	// CredentialExpiresAt は ~/.aws/cli/cache のロール認証情報の有効期限 未算出/該当なしは nil
	CredentialExpiresAt *time.Time
}

// IsSSO は SSO 関連設定があるかを返す(TUI の状態マーク切り分けに使う D9)
func (p Profile) IsSSO() bool {
	return p.SSOSession != "" || p.SSOStartURL != "" || p.SSOAccountID != "" || p.SSORoleName != ""
}

// toConfigProfile は awsconfig.ResolveSession に渡すための変換
func (p Profile) toConfigProfile() awsconfig.Profile {
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
		_, _ = fmt.Fprintln(r.stdout, ui.SuccessLine(fmt.Sprintf("Login status OK for profile=%s", selected)))
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

func (r *Runner) emitUnset(shellMode bool) {
	if shellMode {
		_, _ = fmt.Fprintln(r.stdout, "unset AWS_PROFILE AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN")
		return
	}
	_, _ = fmt.Fprintln(r.stdout, ui.InfoLine("AWS_PROFILE と静的認証情報を解除しました"))
}

func (r *Runner) emitProfile(shellMode bool, profile string) {
	if shellMode {
		_, _ = fmt.Fprintln(r.stdout, "export AWS_SDK_LOAD_CONFIG=1")
		_, _ = fmt.Fprintf(r.stdout, "export AWS_PROFILE=%q\n", profile)
		_, _ = fmt.Fprintln(r.stdout, "unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN")
		return
	}
	_, _ = fmt.Fprintln(r.stdout, ui.SuccessLine(fmt.Sprintf("Profile validated: %s", profile)))
	_, _ = fmt.Fprintln(r.stdout, ui.InfoLine("この実行では親シェルの AWS_PROFILE は変更されません"))
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
