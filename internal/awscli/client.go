// Package awscli は AWS SDK 呼び出しと AWS CLI 実行を扱う
package awscli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	stssdk "github.com/aws/aws-sdk-go-v2/service/sts"
)

const awsBinary = "aws"

// Identity は sts get-caller-identity の結果
// json タグは CLI 出力に合わせる
// AWS のキー名に合わせて大文字を維持
type Identity struct {
	Account string `json:"Account"`
	UserID  string `json:"UserId"`
	ARN     string `json:"Arn"`
}

// Client は AWS SDK と aws CLI の呼び出しをまとめる
// 認証確認は SDK 認証更新は CLI で実行する
// 理由は SSO ログイン開始フローが CLI で成熟しているため
// SDK だけで完結させると OIDC のデバイス認可やブラウザ誘導や
// トークンキャッシュ管理を自前実装する必要があり保守負荷が高い
// そのためこのツールでは責務を分離して運用安定性を優先する
type Client struct {
	logger *slog.Logger
	stdout io.Writer
	stderr io.Writer
}

// NewClient は実行クライアントを作る
func NewClient(logger *slog.Logger, stdout io.Writer, stderr io.Writer) *Client {
	return &Client{
		logger: logger,
		stdout: stdout,
		stderr: stderr,
	}
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

// SSOSession は config から sso_session 名を読む
// 未設定時は空文字を返す
func (c *Client) SSOSession(ctx context.Context, profile string) (string, error) {
	args := []string{"configure", "get", "sso_session", "--profile", profile}

	output, err := c.runAndCapture(ctx, profile, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// Login は aws sso login を実行する
// ここを SDK 化しないのは意図的
// SDK の sts 呼び出しは既存の SSO セッションを使えるが
// そのセッション自体の確立は aws CLI が公式に担っている
// CLI は IAM Identity Center の対話ログインやキャッシュ更新を標準化している
// この処理を再実装すると型安全性は上がっても運用リスクが増える
// そのため SSO セッション確立は CLI へ委譲している
func (c *Client) Login(ctx context.Context, profile string, ssoSession string) error {
	args := []string{"sso", "login"}
	if ssoSession != "" {
		args = append(args, "--sso-session", ssoSession)
	} else {
		args = append(args, "--profile", profile)
	}

	if err := c.runInteractive(ctx, profile, args...); err != nil {
		return fmt.Errorf("sso login に失敗: %w", err)
	}
	return nil
}

func (c *Client) runAndCapture(ctx context.Context, profile string, args ...string) ([]byte, error) {
	if err := ensureAWSCLI(); err != nil {
		return nil, err
	}

	// #nosec G204 固定バイナリ aws に固定引数のみを渡す
	cmd := exec.CommandContext(ctx, awsBinary, args...)
	cmd.Env = buildAWSEnv(profile)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, &commandError{
			Command: strings.Join(append([]string{awsBinary}, args...), " "),
			Output:  strings.TrimSpace(stderr.String()),
			Err:     err,
		}
	}

	return output, nil
}

func (c *Client) runInteractive(ctx context.Context, profile string, args ...string) error {
	if err := ensureAWSCLI(); err != nil {
		return err
	}

	// #nosec G204 固定バイナリ aws に固定引数のみを渡す
	cmd := exec.CommandContext(ctx, awsBinary, args...)
	cmd.Env = buildAWSEnv(profile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr

	if c.logger != nil {
		c.logger.Debug("aws CLI 実行", "command", strings.Join(append([]string{awsBinary}, args...), " "))
	}

	if err := cmd.Run(); err != nil {
		return &commandError{
			Command: strings.Join(append([]string{awsBinary}, args...), " "),
			Err:     err,
		}
	}

	return nil
}

func buildAWSEnv(profile string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "AWS_ACCESS_KEY_ID=") ||
			strings.HasPrefix(entry, "AWS_SECRET_ACCESS_KEY=") ||
			strings.HasPrefix(entry, "AWS_SESSION_TOKEN=") ||
			strings.HasPrefix(entry, "AWS_PROFILE=") ||
			strings.HasPrefix(entry, "AWS_SDK_LOAD_CONFIG=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "AWS_SDK_LOAD_CONFIG=1")
	env = append(env, "AWS_PROFILE="+profile)
	return env
}

func ensureAWSCLI() error {
	if _, err := exec.LookPath(awsBinary); err != nil {
		return fmt.Errorf("aws CLI が見つかりません: %w", err)
	}
	return nil
}

func awsString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type commandError struct {
	Command string
	Output  string
	Err     error
}

func (e *commandError) Error() string {
	if e.Output == "" {
		return fmt.Sprintf("%s: %v", e.Command, e.Err)
	}
	return fmt.Sprintf("%s: %v: %s", e.Command, e.Err, e.Output)
}

func (e *commandError) Unwrap() error {
	return e.Err
}
