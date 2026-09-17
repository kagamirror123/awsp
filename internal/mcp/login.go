package mcp

import (
	"context"
	"fmt"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

const (
	// defaultLoginTimeout は login ツールの承認待ち上限の既定値(D13)
	defaultLoginTimeout = 5 * time.Minute
	// maxLoginTimeout は login ツールの承認待ち上限の上限値(D13)
	maxLoginTimeout = 15 * time.Minute
)

// LoginInput は login ツールの入力
// Profile と SSOSession は同時に指定しない(両方空なら config の sso-session が 1 つの場合だけ自動解決する)
type LoginInput struct {
	// Profile はログイン対象を profile 名で指定する sso_session とは同時に指定できない
	Profile string `json:"profile,omitempty" jsonschema:"AWS profile name to establish an SSO session for; mutually exclusive with sso_session; omit to auto-resolve when the AWS config has exactly one sso-session"`
	// SSOSession はログイン対象を sso-session 名で指定する profile とは同時に指定できない
	SSOSession string `json:"sso_session,omitempty" jsonschema:"sso-session name (from list_profiles) to log in to; mutually exclusive with profile"`
	// TimeoutSeconds は承認待ちの上限秒数 未指定/0 以下なら 300 900 を超える値は 900 に切り詰める
	TimeoutSeconds int `json:"timeout_seconds,omitempty" jsonschema:"seconds to block waiting for the human's browser approval before returning status=pending; default 300, clamped to a maximum of 900"`
	// UseDeviceCode を true にすると device authorization flow を使う(既定は Authorization Code + PKCE)
	UseDeviceCode bool `json:"use_device_code,omitempty" jsonschema:"use the OIDC device-authorization flow (open a URL on any device and enter a code) instead of the default Authorization Code + PKCE flow (opens a browser on this same machine and completes automatically); useful for remote/headless environments, but device-code grants may be disabled by the organization's Identity Center"`
}

// LoginOutput は login ツールの出力 トークン値は絶対に含めない
type LoginOutput = awsp.LoginResult

// login は login ツールのハンドラ
// 既に有効なら profile の identity を確認して返す(D4)
// 未確立なら loginManager 経由で認可フローを開始/合流し 呼び出し側の timeout まで待つ(D3 D13)
func (h *handlers) login(ctx context.Context, req *sdkmcp.CallToolRequest, in LoginInput) (*sdkmcp.CallToolResult, LoginOutput, error) {
	profiles, err := h.deps.Profiles.ProfileDetails(ctx)
	if err != nil {
		return nil, LoginOutput{}, err
	}
	sessions, err := h.deps.Profiles.SSOSessions(ctx)
	if err != nil {
		return nil, LoginOutput{}, err
	}

	session, profile, err := awsp.ResolveLoginSession(awsp.LoginTarget{
		Profile:        in.Profile,
		SSOSessionName: in.SSOSession,
	}, profiles, sessions)
	if err != nil {
		return nil, LoginOutput{}, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, clampLoginTimeout(in.TimeoutSeconds))
	defer cancel()

	// キャッシュ確認も所有権取得後に行い、直前に完了したログインを見落とさない。
	key := session.CacheKey()
	entry, starter := h.manager.acquire(key)
	if starter {
		h.startFlow(session, profile, key, entry, in)
	}

	var flow *ssologin.Flow
	state := ssocache.StateUnknown
	select {
	case <-entry.ready:
		flow, state = entry.flow, entry.state
	case <-waitCtx.Done():
		return nil, pendingOutput(session, nil, state, "ログインを開始しています。login を再度呼び出すと同じ処理の進捗を確認できます"), nil
	}

	if flow != nil {
		if starter && flow.BrowserErr != nil {
			return nil, pendingOutput(session, flow, state,
				"ブラウザの自動起動を確認できませんでした。URL を手動で開いて承認し、login を再度呼び出してください"), nil
		}
		notifyProgress(ctx, req, flow)
	}

	select {
	case <-entry.done:
		// 認証情報の確認エラーは対象 profile だけに返す。同じセッションの別ロールに波及させない。
		if entry.err != nil && (entry.result.Status != "ok" || entry.profile == profile) {
			return nil, LoginOutput{}, entry.err
		}
		out, err := awsp.WithLoginIdentity(waitCtx, h.deps.AWS, profile, entry.result)
		return nil, out, err
	case <-waitCtx.Done():
		return nil, pendingOutput(session, flow, state,
			"待機を終了しました。ログインはサーバー内で継続しています。login を再度呼び出すと合流して待てます"), nil
	}
}

// startFlow はキャッシュ確認から認証完了までをセッション単位で共有する。
// 個々の呼び出しの期限では中断せず、サーバーの寿命と認証処理の上限に従う。
func (h *handlers) startFlow(session awsconfig.SSOSession, profile, key string, entry *loginFlowEntry, in LoginInput) {
	go func() {
		entry.profile = profile
		started := false
		entry.result, entry.err = awsp.Login(h.rootCtx, session, profile, awsp.LoginDeps{AWS: h.deps.AWS}, awsp.LoginOptions{
			CacheDir:      h.deps.SSOCacheDir,
			Grace:         defaultGrace,
			Now:           h.deps.now,
			Timeout:       maxLoginTimeout,
			NewOIDCClient: h.deps.NewOIDCClient,
			OpenBrowser:   h.deps.OpenBrowser,
			UseDeviceCode: in.UseDeviceCode,
			OnStarted: func(flow *ssologin.Flow, state ssocache.EvaluationState) {
				entry.flow, entry.state = flow, state
				started = true
				close(entry.ready)
			},
		})
		close(entry.done)
		if !started {
			close(entry.ready)
		}
		h.manager.release(key, entry)
	}()
}

// pendingOutput は開始待ち、またはブラウザでの承認待ちを表す。
func pendingOutput(session awsconfig.SSOSession, flow *ssologin.Flow, state ssocache.EvaluationState, message string) LoginOutput {
	out := LoginOutput{
		SchemaVersion: 1,
		Status:        "pending",
		Phase:         "starting",
		Session:       displayName(session),
		State:         state,
		Message:       message,
	}
	if flow != nil {
		out.Phase = "authorizing"
		out.Method = flow.Method
		out.AuthorizationURL = flow.AuthorizationURL
		out.UserCode = flow.UserCode
	}
	return out
}

// notifyProgress はブロック直前に進行通知で認可 URL(device code なら Code も)を流す
// リクエストに progressToken が無ければ何もしない(D13)
func notifyProgress(ctx context.Context, req *sdkmcp.CallToolRequest, flow *ssologin.Flow) {
	if req == nil || req.Session == nil {
		return
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return
	}

	message := fmt.Sprintf("ブラウザで承認してください: %s", flow.AuthorizationURL)
	if flow.UserCode != "" {
		message = fmt.Sprintf("%s (code: %s)", message, flow.UserCode)
	}

	_ = req.Session.NotifyProgress(ctx, &sdkmcp.ProgressNotificationParams{
		ProgressToken: token,
		Message:       message,
	})
}

// clampLoginTimeout は timeout_seconds を既定値/上限値に丸める(D13)
func clampLoginTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultLoginTimeout
	}
	if seconds > int(maxLoginTimeout/time.Second) {
		return maxLoginTimeout
	}
	return time.Duration(seconds) * time.Second
}

// displayName は sso-session の表示名を返す legacy 形式では start URL を使う
func displayName(session awsconfig.SSOSession) string {
	if session.Name != "" {
		return session.Name
	}
	if session.StartURL != "" {
		return session.StartURL
	}
	return "(unnamed)"
}
