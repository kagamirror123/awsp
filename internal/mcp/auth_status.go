package mcp

import (
	"context"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagamirror123/awsp/internal/awsp"
)

// AuthStatusInput は auth_status ツールの入力
type AuthStatusInput struct {
	// GraceSeconds は失効後に自動更新を見込む猶予秒数(D18) 未指定/0 以下なら 28800(8h)を使う
	GraceSeconds int `json:"grace_seconds,omitempty" jsonschema:"seconds after token expiry that a session is still considered auto-refreshable via its refresh token; default 28800 (8h)"`
}

// authStatus は auth_status ツールのハンドラ
// ローカルの SSO トークンキャッシュだけを読み ネットワークは使わない
func (h *handlers) authStatus(ctx context.Context, _ *sdkmcp.CallToolRequest, in AuthStatusInput) (*sdkmcp.CallToolResult, awsp.StatusReport, error) {
	grace := time.Duration(in.GraceSeconds) * time.Second
	if grace <= 0 {
		grace = defaultGrace
	}

	profiles, err := h.deps.Profiles.ProfileDetails(ctx)
	if err != nil {
		return nil, awsp.StatusReport{}, err
	}
	sessions, err := h.deps.Profiles.SSOSessions(ctx)
	if err != nil {
		return nil, awsp.StatusReport{}, err
	}

	report, err := awsp.BuildStatusReport(profiles, sessions, awsp.StatusOptions{
		ConfigFile: h.deps.Profiles.ConfigPath(),
		CacheDir:   h.deps.SSOCacheDir,
		Grace:      grace,
		Now:        h.deps.now(),
	})
	if err != nil {
		return nil, awsp.StatusReport{}, err
	}

	return nil, report, nil
}
