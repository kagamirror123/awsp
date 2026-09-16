package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// handlers は 4 ツールのハンドラがまとめて共有する状態を保持する
type handlers struct {
	deps    Deps
	manager *loginManager
	// rootCtx は MCP サーバーの寿命(stdio 接続そのもの)に紐づくコンテキスト
	// login のバックグラウンドのログインフロー(D3 D4)はこれを使い 個々の呼び出しの ctx より長生きする
	rootCtx context.Context
}

// D11 はどの人間向け出力(agentEnv)にも共通で載せる注記
// profile の切り替えは人間のシェル(zsh 関数)が行い エージェント側からは変更できないため
// エージェントは list_profiles で得た名前を aws コマンドの --profile に渡す
const d11Note = "AWS profiles are switched by the human's own shell; " +
	"this MCP server cannot change the human's environment. " +
	"Agents should pass `--profile <name>` on aws commands using names from list_profiles."

const noTokenNote = "Never returns SSO token or credential values."

// NewServer は 4 つのツール(auth_status list_profiles whoami login)を登録した MCP サーバーを作る(D1 D2)
//
// ctx はログイン管理の内部フロー(D3 D4 D13)を回すための長寿命コンテキスト
// stdio 接続そのものの寿命に紐づくものを渡すこと(個々のツール呼び出しの ctx を渡してはいけない)
func NewServer(ctx context.Context, deps Deps, impl *sdkmcp.Implementation) *sdkmcp.Server {
	server := sdkmcp.NewServer(impl, nil)

	h := &handlers{
		deps:    deps,
		manager: newLoginManager(),
		rootCtx: ctx,
	}

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "auth_status",
		Description: "Report whether the AWS SSO sessions found in the AWS config are currently valid. " +
			"Reads only the local SSO token cache; makes no network calls. " +
			"Use this at the start of a task to check whether login is needed before touching AWS. " +
			d11Note + " " + noTokenNote,
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, h.authStatus)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "list_profiles",
		Description: "List the AWS profiles found in the AWS config, along with the SSO session state and " +
			"last-used role credentials for each. Reads only local files; makes no network calls. " +
			d11Note + " " + noTokenNote,
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, h.listProfiles)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "whoami",
		Description: "Get the AWS STS caller identity (account, ARN) for a given profile by calling " +
			"sts:GetCallerIdentity. Does not attempt to log in automatically; if it fails with an auth error, " +
			"call login for that profile first. " + d11Note + " " + noTokenNote,
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(true),
		},
	}, h.whoami)

	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name: "login",
		Description: "Establish an AWS SSO session for a profile or sso-session, blocking until the human " +
			"approves or timeout_seconds elapses. By default uses the Authorization Code + PKCE flow, which " +
			"opens a browser on this same machine and completes automatically once approved there; the " +
			"browser must be reachable from here. Set use_device_code=true to use the OIDC " +
			"device-authorization flow instead, which lets the human approve from any device by opening a " +
			"URL and entering a code (useful for remote/headless environments), but device-code grants may " +
			"be disabled by the organization's Identity Center. " +
			"If the session is already valid, returns status=ok immediately without starting a new flow. " +
			"If the browser cannot be opened automatically, returns status=pending immediately with " +
			"authorizationUrl (and userCode for device_code) for the human to open manually; call login " +
			"again with the same target to join that same in-progress flow and keep waiting. " +
			"Calling login again for a target that is already being logged in joins the same flow instead of " +
			"starting a duplicate one. " + d11Note + " " + noTokenNote,
		Annotations: &sdkmcp.ToolAnnotations{
			ReadOnlyHint:  false,
			OpenWorldHint: boolPtr(true),
		},
	}, h.login)

	return server
}

func boolPtr(v bool) *bool {
	return &v
}
