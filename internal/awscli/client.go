// Package awscli は AWS SDK for Go v2 を使った caller identity の確認を提供する
package awscli

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	stssdk "github.com/aws/aws-sdk-go-v2/service/sts"
)

// Identity は sts get-caller-identity の結果
// json タグは CLI 出力に合わせる
// AWS のキー名に合わせて大文字を維持
type Identity struct {
	Account string `json:"Account"`
	UserID  string `json:"UserId"`
	ARN     string `json:"Arn"`
}

// Client は AWS SDK を使った caller identity 確認を提供する
// SSO セッションの確立(device authorization flow)は internal/ssologin が担い
// このパッケージは確立済みのセッションを使った identity 確認だけに責務を絞る
// 以前は SSO ログイン開始を aws CLI の exec(sso login / configure get)へ委譲していたが
// D12(docs/design.md)で SDK 内製の認可フロー(PKCE / device code) へ切り替えたため
// このパッケージから aws CLI への依存を完全に無くした
type Client struct{}

// NewClient は Client を作る
func NewClient() *Client {
	return &Client{}
}

// CallerIdentity は指定プロファイルで呼び出し主体を取得する
func (c *Client) CallerIdentity(ctx context.Context, profile string) (Identity, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return Identity{}, fmt.Errorf("AWS SDK 設定の読み込みに失敗: %w", err)
	}
	stsClient := stssdk.NewFromConfig(cfg)
	output, err := stsClient.GetCallerIdentity(ctx, &stssdk.GetCallerIdentityInput{})
	if err != nil {
		return Identity{}, fmt.Errorf("caller identity の取得に失敗: %w", err)
	}
	return Identity{
		Account: awsString(output.Account),
		UserID:  awsString(output.UserId),
		ARN:     awsString(output.Arn),
	}, nil
}

func awsString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
