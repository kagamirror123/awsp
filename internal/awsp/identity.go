package awsp

import (
	"context"
	"fmt"
	"time"

	"github.com/kagamirror123/awsp/internal/awscli"
)

// AWSIdentityClient は STS の呼び出し主体確認を提供する
// SSO セッションの確立は行わない(確立済みセッションの利用だけ)
type AWSIdentityClient interface {
	CallerIdentity(ctx context.Context, profile string) (awscli.Identity, error)
}

// Identity は STS の呼び出し主体情報
// トークン値やクレデンシャル本体は含まない
type Identity struct {
	// Profile は問い合わせに使った profile 名
	Profile string `json:"profile"`
	// Account は AWS アカウント ID
	Account string `json:"account"`
	// UserID は sts:GetCallerIdentity の UserId
	UserID string `json:"userId"`
	// ARN は呼び出し主体の ARN
	ARN string `json:"arn"`
}

// IdentityReport は CLI/MCP 共通の whoami 出力。identity はフラットに保つ。
type IdentityReport struct {
	SchemaVersion int `json:"schemaVersion"`
	Identity
}

// CurrentReport は `awsp current --json` の出力
type CurrentReport struct {
	// SchemaVersion は出力形式のバージョン
	SchemaVersion int `json:"schemaVersion"`
	// GeneratedAt はこのレポートを生成した時刻
	GeneratedAt time.Time `json:"generatedAt"`
	// ConfigFile は判定に使った AWS config のパス(D5)
	ConfigFile string `json:"configFile"`
	// Source は profile の取得元(現状は常に "env")
	Source string `json:"source"`
	// Identity は caller identity
	Identity Identity `json:"identity"`
}

// Whoami は指定 profile の caller identity を返す
// 自動ログインは行わない 認証エラーの場合は次に打つコマンドを含めて返す
func Whoami(ctx context.Context, client AWSIdentityClient, profile string) (Identity, error) {
	identity, err := client.CallerIdentity(ctx, profile)
	if err != nil {
		if awscli.IsAuthRelatedError(err) {
			return Identity{}, fmt.Errorf("認証が必要です: 'awsp login %s' を実行してください: %w", profile, err)
		}
		return Identity{}, fmt.Errorf("caller identity の取得に失敗: %w", err)
	}

	return Identity{
		Profile: profile,
		Account: identity.Account,
		UserID:  identity.UserID,
		ARN:     identity.ARN,
	}, nil
}
