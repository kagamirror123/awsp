package mcp

import (
	"context"
	"errors"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagamirror123/awsp/internal/awsp"
)

// WhoamiInput は whoami ツールの入力
type WhoamiInput struct {
	// Profile は caller identity を確認する対象の AWS profile 名(list_profiles で得られる名前)
	Profile string `json:"profile" jsonschema:"AWS profile name from list_profiles to check the caller identity for"`
}

// whoami は whoami ツールのハンドラ
// STS を叩いて caller identity を取得する 自動ログインは行わない(失敗時は login を案内する)
func (h *handlers) whoami(ctx context.Context, _ *sdkmcp.CallToolRequest, in WhoamiInput) (*sdkmcp.CallToolResult, awsp.Identity, error) {
	if in.Profile == "" {
		return nil, awsp.Identity{}, errors.New("profile は必須です: list_profiles で名前を確認してください")
	}

	identity, err := awsp.Whoami(ctx, h.deps.AWS, in.Profile)
	if err != nil {
		return nil, awsp.Identity{}, err
	}

	return nil, identity, nil
}
