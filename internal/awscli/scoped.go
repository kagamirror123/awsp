package awscli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	stssdk "github.com/aws/aws-sdk-go-v2/service/sts"
)

// ScopedMaxDuration は GetScopedCredentials の DurationSeconds の上限
const ScopedMaxDuration = 36 * time.Hour

// ScopedRequest は絞った認証情報の発行条件
type ScopedRequest struct {
	Profile string
	// ConfigFile は元の認証情報を取る config。空なら SDK の既定(AWS_CONFIG_FILE か ~/.aws/config)
	ConfigFile string
	// Policies は重ねる順のポリシー(JSON)。1 つ以上
	Policies    []string
	Duration    time.Duration
	Description string
}

// ScopedCredentials は sts get-scoped-credentials の結果
// 値は子プロセスの環境変数に渡すためだけに使い 出力やログに載せない
type ScopedCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string //nolint:gosec // 子プロセスの環境変数に渡すための値で 出力しない
	Expiration      time.Time
	ARN             string
	Region          string
	PolicyLayers    int
}

// ScopedCredentials は profile の認証情報にポリシーを順に重ねて 絞った認証情報を発行する
// 期限は最初の発行で決め 2 層目以降は前の層の期限を引き継ぐ
func (c *Client) ScopedCredentials(ctx context.Context, req ScopedRequest) (ScopedCredentials, error) {
	if len(req.Policies) == 0 {
		return ScopedCredentials{}, errors.New("重ねるポリシーがありません")
	}
	if req.Duration < time.Second || req.Duration > ScopedMaxDuration {
		return ScopedCredentials{}, errors.New("有効期間は 1 秒から 36 時間の間にしてください")
	}

	opts := []func(*config.LoadOptions) error{config.WithSharedConfigProfile(req.Profile)}
	if req.ConfigFile != "" {
		opts = append(opts, config.WithSharedConfigFiles([]string{req.ConfigFile}))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return ScopedCredentials{}, fmt.Errorf("AWS SDK 設定の読み込みに失敗: %w", err)
	}
	if cfg.Region == "" {
		return ScopedCredentials{}, fmt.Errorf(
			"profile %q に region がありません: GetScopedCredentials はリージョンの STS でしか呼べないので、config に region を書いてください",
			req.Profile,
		)
	}

	result := ScopedCredentials{Region: cfg.Region}
	for i, policy := range req.Policies {
		input := &stssdk.GetScopedCredentialsInput{
			Policy:                   aws.String(policy),
			ScopedSessionDescription: aws.String(req.Description),
		}
		if i == 0 {
			//nolint:gosec // 上で 36 時間以下と確かめてある
			input.DurationSeconds = aws.Int32(int32(req.Duration / time.Second))
		}
		output, err := stssdk.NewFromConfig(cfg).GetScopedCredentials(ctx, input)
		if err != nil {
			if i > 0 {
				return ScopedCredentials{}, fmt.Errorf(
					"%d 層目のポリシーを重ねられませんでした(前の層が sts:GetScopedCredentials を許可している必要があります): %w",
					i+1, err,
				)
			}
			return ScopedCredentials{}, fmt.Errorf("絞った認証情報の発行に失敗: %w", err)
		}
		if output.Credentials == nil {
			return ScopedCredentials{}, errors.New("絞った認証情報の発行に失敗: 応答に認証情報がありません")
		}

		result.AccessKeyID = awsString(output.Credentials.AccessKeyId)
		result.SecretAccessKey = awsString(output.Credentials.SecretAccessKey)
		result.SessionToken = awsString(output.Credentials.SessionToken)
		if output.Credentials.Expiration != nil {
			result.Expiration = *output.Credentials.Expiration
		}
		if output.SessionInfo != nil {
			result.ARN = awsString(output.SessionInfo.Arn)
		}
		result.PolicyLayers = i + 1

		cfg.Credentials = credentials.NewStaticCredentialsProvider(
			result.AccessKeyID, result.SecretAccessKey, result.SessionToken,
		)
	}
	return result, nil
}
