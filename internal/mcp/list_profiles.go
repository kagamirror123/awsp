package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagamirror123/awsp/internal/awsp"
)

// ListProfilesInput は list_profiles ツールの入力(パラメータなし)
type ListProfilesInput struct{}

// listProfiles は list_profiles ツールのハンドラ
// ローカルの AWS config と sso/cli キャッシュだけを読み ネットワークは使わない
// D11: profile の切り替えは人間側のシェルが行う エージェントはここで得た名前を
// aws コマンドの --profile に渡す
func (h *handlers) listProfiles(ctx context.Context, _ *sdkmcp.CallToolRequest, _ ListProfilesInput) (*sdkmcp.CallToolResult, awsp.ProfileList, error) {
	profiles, err := h.deps.Profiles.ProfileDetails(ctx)
	if err != nil {
		return nil, awsp.ProfileList{}, err
	}
	sessions, err := h.deps.Profiles.SSOSessions(ctx)
	if err != nil {
		return nil, awsp.ProfileList{}, err
	}

	list, err := awsp.BuildProfileList(profiles, sessions, awsp.ProfileListOptions{
		ConfigFile:  h.deps.Profiles.ConfigPath(),
		SSOCacheDir: h.deps.SSOCacheDir,
		CLICacheDir: h.deps.CLICacheDir,
		Grace:       defaultGrace,
		Now:         h.deps.now(),
	})
	if err != nil {
		return nil, awsp.ProfileList{}, err
	}

	return nil, list, nil
}
