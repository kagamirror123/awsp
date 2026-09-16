package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
)

// fakeProfileStore は internal/mcp.ProfileStore を満たすテスト用フェイク
// AWS config ファイルには触れない
type fakeProfileStore struct {
	profiles   []awsconfig.Profile
	sessions   []awsconfig.SSOSession
	configPath string
}

func (f *fakeProfileStore) ProfileDetails(context.Context) ([]awsconfig.Profile, error) {
	return f.profiles, nil
}

func (f *fakeProfileStore) SSOSessions(context.Context) ([]awsconfig.SSOSession, error) {
	return f.sessions, nil
}

func (f *fakeProfileStore) ConfigPath() string {
	return f.configPath
}

// fakeAWSClient は awsp.AWSIdentityClient を満たすテスト用フェイク STS は一切叩かない
type fakeAWSClient struct {
	mu sync.Mutex
	// byProfile は profile 名ごとの応答 未登録の profile はエラーを返す
	byProfile map[string]awscli.Identity
	err       error
	calls     int
}

func (f *fakeAWSClient) CallerIdentity(_ context.Context, profile string) (awscli.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++

	if f.err != nil {
		return awscli.Identity{}, f.err
	}
	identity, ok := f.byProfile[profile]
	if !ok {
		return awscli.Identity{}, errors.New("fakeAWSClient: 未登録の profile: " + profile)
	}
	return identity, nil
}

// fakeOIDCClient は ssologin.OIDCClient を満たすテスト用フェイク 実 API は一切呼ばない
// internal/ssologin/ssologin_test.go の同名フェイクを参考にした internal/mcp 用の独立実装
type fakeOIDCClient struct {
	mu sync.Mutex

	registerOutput *ssooidc.RegisterClientOutput
	registerErr    error
	registerCalls  int

	startOutput *ssooidc.StartDeviceAuthorizationOutput
	startErr    error
	startCalls  int

	// createToken は CreateToken が呼ばれるたびに呼ばれる 戻り値をそのまま返す
	// nil ならデフォルトで AuthorizationPendingException を返し続ける
	createToken func(calls int) (*ssooidc.CreateTokenOutput, error)
	createCalls int
}

func (f *fakeOIDCClient) RegisterClient(
	_ context.Context,
	_ *ssooidc.RegisterClientInput,
	_ ...func(*ssooidc.Options),
) (*ssooidc.RegisterClientOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registerCalls++
	return f.registerOutput, f.registerErr
}

func (f *fakeOIDCClient) StartDeviceAuthorization(
	_ context.Context,
	_ *ssooidc.StartDeviceAuthorizationInput,
	_ ...func(*ssooidc.Options),
) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	return f.startOutput, f.startErr
}

func (f *fakeOIDCClient) CreateToken(
	_ context.Context,
	_ *ssooidc.CreateTokenInput,
	_ ...func(*ssooidc.Options),
) (*ssooidc.CreateTokenOutput, error) {
	f.mu.Lock()
	calls := f.createCalls
	f.createCalls++
	fn := f.createToken
	f.mu.Unlock()

	if fn == nil {
		return nil, &ssooidctypes.AuthorizationPendingException{}
	}
	return fn(calls)
}

func (f *fakeOIDCClient) counts() (register int, start int, create int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.registerCalls, f.startCalls, f.createCalls
}

func strPtr(v string) *string { return &v }

// writeJSONFile はテスト用に JSON ファイルを書く(存在しないディレクトリは作る)
func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("ディレクトリ作成に失敗: %s: %v", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("JSON エンコードに失敗: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("ファイル書き込みに失敗: %s: %v", path, err)
	}
}
