// Package mcp は awsp の MCP サーバー実装を提供する(D1 D2 D3 D4 D13)
// stdio トランスポートで auth_status / list_profiles / whoami / login の 4 ツールを配信し
// いずれも internal/awsp のユースケース関数をそのまま呼び出す(CLI と実装を共有する D1)
package mcp

import (
	"context"
	"time"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

// defaultGrace は失効後に自動更新を見込む猶予時間の既定値(D18)
const defaultGrace = 8 * time.Hour

// ProfileStore は AWS config から profile と sso-session を読み取る
// *awsconfig.ProfileStore がそのままこれを満たす テストではフェイクに差し替える
type ProfileStore interface {
	ProfileDetails(ctx context.Context) ([]awsconfig.Profile, error)
	SSOSessions(ctx context.Context) ([]awsconfig.SSOSession, error)
	ConfigPath() string
}

// Deps は MCP サーバーが使う外部依存をまとめる
// テストがネットワークと実ファイルを避けられるようすべて差し替え可能にする
type Deps struct {
	// Profiles は profile / sso-session の読み取り元
	Profiles ProfileStore
	// SSOCacheDir は ~/.aws/sso/cache 相当のディレクトリ 未指定時は動作しない(呼び出し側で必須設定)
	SSOCacheDir string
	// CLICacheDir は ~/.aws/cli/cache 相当のディレクトリ(list_profiles の認証情報取得情報に使う)
	// 空文字なら付加情報の算出を省略する(awsp.BuildProfileList の仕様どおり)
	CLICacheDir string
	// AWS は STS の呼び出し主体確認に使う(whoami / login 後の identity 取得)
	AWS awsp.AWSIdentityClient
	// NewOIDCClient は sso-session の region から OIDC クライアントを作る
	// 本番では ssooidc.New(ssooidc.Options{Region: region}) を渡す
	NewOIDCClient func(region string) ssologin.OIDCClient
	// OpenBrowser は認可 URL を開く関数 nil なら ssologin の既定(OS 標準のオープンコマンド)
	OpenBrowser func(url string) error
	// Now は現在時刻を返す 未指定時は time.Now
	Now func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}
