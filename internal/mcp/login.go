package mcp

import (
	"context"
	"errors"
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
type LoginOutput struct {
	// Status は "ok"(セッション確立済み)か "pending"(承認待ち 別フィールドで案内)
	Status string `json:"status"`
	// Session はログイン対象の sso-session 名 legacy 形式では sso_start_url
	Session string `json:"session"`
	// State はこの応答時点でのセッション状態 status=pending の間はログインフロー開始前の状態を示す
	State ssocache.EvaluationState `json:"state"`
	// ExpiresAt はアクセストークンの有効期限 status=ok のときだけ設定する
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// Identity は対象 profile が分かった場合の caller identity status=ok のときだけ設定する
	Identity *awsp.Identity `json:"identity,omitempty"`
	// Method はログイン方式("authorization_code" または "device_code") status=pending のときだけ設定する
	Method string `json:"method,omitempty"`
	// AuthorizationURL は人が開くべき認可 URL status=pending のときだけ設定する
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	// UserCode は device_code のときだけ設定する AuthorizationURL とは別に入力するコード
	UserCode string `json:"userCode,omitempty"`
	// Message は人 または エージェントが次に何をすべきかの説明
	Message string `json:"message"`
}

// login は login ツールのハンドラ
// 既に有効なら device flow を起こさず即返す(D4)
// 未確立なら loginManager 経由で device flow を開始/合流し 呼び出し側の timeout まで待つ(D3 D13)
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

	timeout := clampLoginTimeout(in.TimeoutSeconds)

	tokenPath := ssocache.TokenPath(h.deps.SSOCacheDir, session.CacheKey())
	meta, err := ssocache.ReadTokenMeta(tokenPath)
	if err != nil {
		return nil, LoginOutput{}, err
	}
	eval := ssocache.Evaluate(meta, h.deps.now(), defaultGrace)

	if eval.State == ssocache.StateOK {
		out := LoginOutput{
			Status:  "ok",
			Session: displayName(session),
			State:   ssocache.StateOK,
			Message: "既に有効な SSO セッションです",
		}
		expiresAt := eval.ExpiresAt
		out.ExpiresAt = &expiresAt
		if err := attachIdentity(ctx, h.deps.AWS, profile, &out); err != nil {
			return nil, LoginOutput{}, err
		}
		return nil, out, nil
	}

	if h.deps.NewOIDCClient == nil {
		return nil, LoginOutput{}, errors.New("mcp: Deps.NewOIDCClient が未設定です")
	}

	key := session.CacheKey()
	entry, starter := h.manager.acquire(key)
	if starter {
		h.startFlow(session, key, entry, in.UseDeviceCode)
	}

	select {
	case <-entry.ready:
	case <-ctx.Done():
		return nil, LoginOutput{}, fmt.Errorf("ログイン開始待ちがキャンセルされました: %w", ctx.Err())
	}

	if entry.startErr != nil {
		return nil, LoginOutput{}, fmt.Errorf("ログインの開始に失敗: %w", entry.startErr)
	}
	flow := entry.flow

	// ブラウザを自動起動できなかった場合は「今このフローを起こした呼び出し」だけ即 pending で返す(D13)
	// 合流した呼び出し(starter=false)はこの時点で既に URL/コードが確定しているので待ちに入る
	if starter && flow.BrowserErr != nil {
		return nil, pendingOutput(session, flow, eval.State,
			"ブラウザの自動起動に失敗しました。以下の URL を開いて承認してください。"+
				"承認後に login を再度呼び出すと状態を確認できます"), nil
	}

	notifyProgress(ctx, req, flow)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-entry.done:
		if entry.waitErr != nil {
			return nil, LoginOutput{}, entry.waitErr
		}
		out := LoginOutput{
			Status:  "ok",
			Session: displayName(session),
			State:   ssocache.StateOK,
			Message: "ログインが完了しました",
		}
		expiresAt := entry.result.ExpiresAt
		out.ExpiresAt = &expiresAt
		if err := attachIdentity(ctx, h.deps.AWS, profile, &out); err != nil {
			return nil, LoginOutput{}, err
		}
		return nil, out, nil

	case <-timer.C:
		return nil, pendingOutput(session, flow, eval.State,
			"承認待ちがタイムアウトしました。ログインはサーバー内で継続しています。"+
				"login を再度呼び出すと合流して続きから待てます"), nil

	case <-ctx.Done():
		return nil, pendingOutput(session, flow, eval.State,
			"呼び出しがキャンセルされました。ログインはサーバー内で継続しています。"+
				"login を再度呼び出すと合流して続きから待てます"), nil
	}
}

// startFlow はログインフローを開始し 完了(または失敗)まで h.rootCtx 上で goroutine を継続する(D3 D4)
// h.rootCtx は MCP サーバーの寿命に紐づく長寿命コンテキストで
// 個々の login 呼び出しの ctx がキャンセル/タイムアウトしてもフローは止まらない
func (h *handlers) startFlow(session awsconfig.SSOSession, key string, entry *loginFlowEntry, useDeviceCode bool) {
	go func() {
		client := h.deps.NewOIDCClient(session.Region)
		flow, err := ssologin.Start(h.rootCtx, session, ssologin.Options{
			Client:        client,
			CacheDir:      h.deps.SSOCacheDir,
			OpenBrowser:   h.deps.OpenBrowser,
			UseDeviceCode: useDeviceCode,
		})
		entry.flow = flow
		entry.startErr = err
		close(entry.ready)

		if err != nil {
			close(entry.done)
			h.manager.release(key, entry)
			return
		}

		// 待ちの上限は device code の有効期限そのもの(呼び出し側の timeout とは無関係)
		waitCtx, cancel := context.WithDeadline(h.rootCtx, flow.ExpiresAt)
		defer cancel()

		result, waitErr := flow.Wait(waitCtx)
		entry.result = result
		entry.waitErr = waitErr
		close(entry.done)
		h.manager.release(key, entry)
	}()
}

// attachIdentity は profile が分かっている場合だけ STS で identity を取得して out に添える
func attachIdentity(ctx context.Context, client awsp.AWSIdentityClient, profile string, out *LoginOutput) error {
	if profile == "" || client == nil {
		return nil
	}
	identity, err := awsp.Whoami(ctx, client, profile)
	if err != nil {
		return fmt.Errorf("ログイン後の identity 取得に失敗: %w", err)
	}
	out.Identity = &identity
	return nil
}

// pendingOutput は承認待ちを表す LoginOutput を組み立てる
func pendingOutput(session awsconfig.SSOSession, flow *ssologin.Flow, state ssocache.EvaluationState, message string) LoginOutput {
	return LoginOutput{
		Status:           "pending",
		Session:          displayName(session),
		State:            state,
		Method:           flow.Method,
		AuthorizationURL: flow.AuthorizationURL,
		UserCode:         flow.UserCode,
		Message:          message,
	}
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
	d := time.Duration(seconds) * time.Second
	if d > maxLoginTimeout {
		return maxLoginTimeout
	}
	return d
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
