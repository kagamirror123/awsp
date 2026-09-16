package awsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

// LoginResult は `awsp login` の JSON 出力
type LoginResult struct {
	// SchemaVersion は出力形式のバージョン
	SchemaVersion int `json:"schemaVersion"`
	// Session はログイン対象の sso-session 名 legacy 形式では start URL
	Session string `json:"session"`
	// State はログイン後の状態(通常は ok)
	State ssocache.EvaluationState `json:"state"`
	// ExpiresAt はアクセストークンの有効期限
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// Identity は対象 profile が分かった場合の caller identity
	Identity *Identity `json:"identity,omitempty"`
}

// LoginTarget は awsp login の対象指定 Profile と SSOSessionName は同時に指定しない
type LoginTarget struct {
	Profile        string
	SSOSessionName string
}

// LoginOptions は Login の挙動を制御する
type LoginOptions struct {
	// CacheDir は sso/cache のディレクトリ 未指定時は ssocache.DefaultCacheDir()
	CacheDir string
	// Grace は失効後に自動更新を見込む猶予時間(D18 既定 8h)
	Grace time.Duration
	// Timeout は承認待ちの上限時間(D13 既定 5m)
	Timeout time.Duration
	// OpenBrowser は認可 URL を開く関数 未指定時は OS 標準のオープンコマンド
	OpenBrowser func(url string) error
	// Output はログインの URL(device flow ならコードも)を表示する出力先
	Output io.Writer
	// UseDeviceCode を true にすると device authorization flow を使う(opt-in)
	// 既定(false)は Authorization Code + PKCE(D12 2026-09-16 改訂)
	UseDeviceCode bool
}

// LoginDeps は Login が使う外部依存
type LoginDeps struct {
	// AWS はログイン後の identity 確認に使う STS クライアント
	AWS AWSIdentityClient
}

// ResolveLoginSession は target と config から対象の sso-session を 1 つに決める
// target が空の場合は config の sso-session が 1 つならそれを使い
// 複数あれば候補を列挙したエラーを返す
// 戻り値の 2 番目は target が profile 名だった場合のその profile 名(ログイン後の identity 確認に使う)
func ResolveLoginSession(
	target LoginTarget,
	profiles []awsconfig.Profile,
	sessions []awsconfig.SSOSession,
) (awsconfig.SSOSession, string, error) {
	if target.Profile != "" && target.SSOSessionName != "" {
		return awsconfig.SSOSession{}, "", errors.New("profile と --sso-session は同時に指定できません")
	}

	if target.Profile != "" {
		for _, profile := range profiles {
			if profile.Name != target.Profile {
				continue
			}
			session, ok := awsconfig.ResolveSession(profile, sessions)
			if !ok {
				return awsconfig.SSOSession{}, "", fmt.Errorf("profile %q は SSO を使っていません", target.Profile)
			}
			return session, target.Profile, nil
		}
		return awsconfig.SSOSession{}, "", fmt.Errorf(
			"指定プロファイルが見つかりません: %s: `awsp list` で確認してください", target.Profile,
		)
	}

	if target.SSOSessionName != "" {
		for _, session := range sessions {
			if session.Name == target.SSOSessionName {
				return session, "", nil
			}
		}
		return awsconfig.SSOSession{}, "", fmt.Errorf(
			"sso-session %q が見つかりません: ~/.aws/config の [sso-session %s] を確認してください",
			target.SSOSessionName, target.SSOSessionName,
		)
	}

	candidates := distinctSessions(profiles, sessions)
	switch len(candidates) {
	case 0:
		return awsconfig.SSOSession{}, "", errors.New("config に sso-session がありません: ~/.aws/config を確認してください")
	case 1:
		return candidates[0], "", nil
	default:
		names := make([]string, 0, len(candidates))
		for _, s := range candidates {
			names = append(names, displayNameForSession(s))
		}
		return awsconfig.SSOSession{}, "", fmt.Errorf(
			"sso-session が複数あります。どれか指定してください(--sso-session): %s",
			strings.Join(names, ", "),
		)
	}
}

// Login は SSO セッションを確立する 既に有効なセッションであれば
// ログインフローを起こさず即返す(D4) 対象 profile が分かれば identity を添えて返す
func Login(ctx context.Context, session awsconfig.SSOSession, profile string, deps LoginDeps, opts LoginOptions) (LoginResult, error) {
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		dir, err := ssocache.DefaultCacheDir()
		if err != nil {
			return LoginResult{}, err
		}
		cacheDir = dir
	}

	grace := opts.Grace
	if grace <= 0 {
		grace = 8 * time.Hour
	}

	now := time.Now()
	tokenPath := ssocache.TokenPath(cacheDir, session.CacheKey())
	meta, err := ssocache.ReadTokenMeta(tokenPath)
	if err != nil {
		return LoginResult{}, err
	}
	eval := ssocache.Evaluate(meta, now, grace)

	if eval.State == ssocache.StateOK {
		expiresAt := eval.ExpiresAt
		return finishLogin(ctx, session, profile, ssocache.StateOK, &expiresAt, deps)
	}

	oidcClient := ssooidc.New(ssooidc.Options{Region: session.Region})

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	flow, err := ssologin.Start(waitCtx, session, ssologin.Options{
		Client:        oidcClient,
		CacheDir:      cacheDir,
		OpenBrowser:   opts.OpenBrowser,
		UseDeviceCode: opts.UseDeviceCode,
	})
	if err != nil {
		return LoginResult{}, fmt.Errorf("ログインの開始に失敗: %w", err)
	}

	printLoginPrompt(opts.Output, flow)

	result, err := flow.Wait(waitCtx)
	if err != nil {
		return LoginResult{}, err
	}

	return finishLogin(ctx, session, profile, ssocache.StateOK, &result.ExpiresAt, deps)
}

func finishLogin(
	ctx context.Context,
	session awsconfig.SSOSession,
	profile string,
	state ssocache.EvaluationState,
	expiresAt *time.Time,
	deps LoginDeps,
) (LoginResult, error) {
	result := LoginResult{
		SchemaVersion: 1,
		Session:       displayNameForSession(session),
		State:         state,
		ExpiresAt:     expiresAt,
	}

	if profile == "" || deps.AWS == nil {
		return result, nil
	}

	identity, err := deps.AWS.CallerIdentity(ctx, profile)
	if err != nil {
		return result, fmt.Errorf("ログイン後の identity 取得に失敗: %w", err)
	}

	result.Identity = &Identity{
		Profile: profile,
		Account: identity.Account,
		UserID:  identity.UserID,
		ARN:     identity.ARN,
	}
	return result, nil
}

func printLoginPrompt(w io.Writer, flow *ssologin.Flow) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "ブラウザで承認してください:")
	_, _ = fmt.Fprintf(w, "  %s\n", flow.AuthorizationURL)
	if flow.UserCode != "" {
		_, _ = fmt.Fprintf(w, "  Code: %s\n", flow.UserCode)
	} else {
		_, _ = fmt.Fprintln(w, "  ブラウザで承認すると自動で戻ります")
	}
	if flow.BrowserErr != nil {
		_, _ = fmt.Fprintln(w, "ブラウザの自動起動に失敗しました。上記 URL を手動で開いてください")
	}
	_, _ = fmt.Fprintln(w)
}
