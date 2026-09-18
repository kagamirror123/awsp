package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// testServer は Deps から MCP サーバーを作り in-memory transport でクライアントと繋ぐ
// 戻り値の cleanup は必ず defer で呼ぶこと
func testServer(ctx context.Context, t *testing.T, deps Deps) *sdkmcp.ClientSession {
	t.Helper()

	server := NewServer(ctx, deps, &sdkmcp.Implementation{Name: "awsp-test", Version: "test"})

	t1, t2 := sdkmcp.NewInMemoryTransports()

	serverSession, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server.Connect が失敗: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect が失敗: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	return clientSession
}

// decodeStructured は CallToolResult.StructuredContent を目的の型へ変換する
// クライアント側では JSON を一旦汎用値に unmarshal しているため 目的の型へ再変換する
func decodeStructured(t *testing.T, structured any, out any) {
	t.Helper()
	data, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("structuredContent の marshal に失敗: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("structuredContent の unmarshal に失敗: %v", err)
	}
}

func baseDeps(t *testing.T) Deps {
	t.Helper()
	return Deps{
		Profiles:    &fakeProfileStore{configPath: "/tmp/does-not-matter/config"},
		SSOCacheDir: t.TempDir(),
		CLICacheDir: t.TempDir(),
		AWS:         &fakeAWSClient{},
		Now:         func() time.Time { return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC) },
	}
}

func TestToolsList_AllFourToolsHaveOutputSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cs := testServer(ctx, t, baseDeps(t))

	result, err := cs.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools が失敗: %v", err)
	}

	want := map[string]bool{"auth_status": false, "list_profiles": false, "whoami": false, "login": false}
	for _, tool := range result.Tools {
		if _, ok := want[tool.Name]; !ok {
			continue
		}
		want[tool.Name] = true
		if tool.OutputSchema == nil {
			t.Errorf("%s: OutputSchema が付いていない", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("%s: InputSchema が付いていない", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s: Description が空", tool.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("ツールが見つからない: %s", name)
		}
	}
}

func TestAuthStatus_MatchesBuildStatusReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	session := awsconfig.SSOSession{
		Name:               "corp",
		StartURL:           "https://example.awsapps.com/start",
		Region:             "us-west-2",
		RegistrationScopes: []string{"sso:account:access"},
	}
	profiles := []awsconfig.Profile{{Name: "dev", SSOSession: "corp"}}
	sessions := []awsconfig.SSOSession{session}

	ssoCacheDir := t.TempDir()
	writeTokenFile(t, ssoCacheDir, session.CacheKey(), now.Add(1*time.Hour), true)

	deps := Deps{
		Profiles:    &fakeProfileStore{profiles: profiles, sessions: sessions, configPath: "/tmp/config"},
		SSOCacheDir: ssoCacheDir,
		AWS:         &fakeAWSClient{},
		Now:         func() time.Time { return now },
	}

	cs := testServer(ctx, t, deps)

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "auth_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool が失敗: %v", err)
	}
	if res.IsError {
		t.Fatalf("auth_status が IsError を返した: %+v", res.Content)
	}

	var got awsp.StatusReport
	decodeStructured(t, res.StructuredContent, &got)

	want, err := awsp.BuildStatusReport(profiles, sessions, awsp.StatusOptions{
		ConfigFile: "/tmp/config",
		CacheDir:   ssoCacheDir,
		Grace:      defaultGrace,
		Now:        now,
	})
	if err != nil {
		t.Fatalf("BuildStatusReport が失敗: %v", err)
	}

	if got.Overall != want.Overall {
		t.Fatalf("Overall が想定外: got=%s want=%s", got.Overall, want.Overall)
	}
	if len(got.Sessions) != len(want.Sessions) {
		t.Fatalf("Sessions 数が想定外: got=%d want=%d", len(got.Sessions), len(want.Sessions))
	}
	if got.Sessions[0].State != ssocache.StateOK {
		t.Fatalf("State が想定外: got=%s want=ok", got.Sessions[0].State)
	}
}

func TestAuthStatus_GraceSecondsDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := baseDeps(t)
	cs := testServer(ctx, t, deps)

	// grace_seconds を省略しても失敗しないこと(既定 28800 が使われる)
	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "auth_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool が失敗: %v", err)
	}
	if res.IsError {
		t.Fatalf("auth_status が IsError を返した: %+v", res.Content)
	}

	var got awsp.StatusReport
	decodeStructured(t, res.StructuredContent, &got)
	if got.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion が想定外: %d", got.SchemaVersion)
	}
}

func TestListProfiles_ReturnsProfileList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	profiles := []awsconfig.Profile{
		{Name: "dev", Region: "ap-northeast-1"},
		{Name: "prod", Region: "us-east-1"},
	}
	deps := baseDeps(t)
	deps.Profiles = &fakeProfileStore{profiles: profiles, configPath: "/tmp/config"}

	cs := testServer(ctx, t, deps)

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_profiles", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool が失敗: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_profiles が IsError を返した: %+v", res.Content)
	}

	var got awsp.ProfileList
	decodeStructured(t, res.StructuredContent, &got)

	if len(got.Profiles) != 2 {
		t.Fatalf("Profiles 数が想定外: %d", len(got.Profiles))
	}
	if got.ConfigFile != "/tmp/config" {
		t.Fatalf("ConfigFile が想定外: %s", got.ConfigFile)
	}
}

func TestListProfiles_IncludesCurrentProfile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := baseDeps(t)
	deps.Profiles = &fakeProfileStore{profiles: []awsconfig.Profile{{Name: "dev"}}, configPath: "/tmp/config"}
	// 人間用 config にしか無い名前でも そのまま伝える(D30)
	deps.CurrentProfile = func() string { return "prod-admin" }

	cs := testServer(ctx, t, deps)
	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_profiles", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_profiles が失敗: err=%v isError=%v", err, res != nil && res.IsError)
	}

	var got awsp.ProfileList
	decodeStructured(t, res.StructuredContent, &got)
	if got.CurrentProfile != "prod-admin" {
		t.Fatalf("CurrentProfile が想定外: %q", got.CurrentProfile)
	}
}

func TestListProfiles_OmitsCurrentProfileWhenUnset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := baseDeps(t)
	deps.Profiles = &fakeProfileStore{profiles: []awsconfig.Profile{{Name: "dev"}}, configPath: "/tmp/config"}

	cs := testServer(ctx, t, deps)
	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_profiles", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_profiles が失敗: err=%v", err)
	}
	if raw, ok := res.StructuredContent.(map[string]any); ok {
		if _, present := raw["currentProfile"]; present {
			t.Fatalf("AWS_PROFILE 未設定なのに currentProfile が出力に含まれている: %v", raw["currentProfile"])
		}
	}
}

func TestWhoami_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := baseDeps(t)
	deps.AWS = &fakeAWSClient{byProfile: map[string]awscli.Identity{
		"dev": {Account: "123456789012", UserID: "AIDAEXAMPLE", ARN: "arn:aws:iam::123456789012:user/dev"},
	}}

	cs := testServer(ctx, t, deps)

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "whoami",
		Arguments: map[string]any{"profile": "dev"},
	})
	if err != nil {
		t.Fatalf("CallTool が失敗: %v", err)
	}
	if res.IsError {
		t.Fatalf("whoami が IsError を返した: %+v", res.Content)
	}

	var got awsp.Identity
	decodeStructured(t, res.StructuredContent, &got)
	if got.Account != "123456789012" || got.Profile != "dev" {
		t.Fatalf("Identity が想定外: %+v", got)
	}
}

func TestWhoami_AuthErrorIsToolError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := baseDeps(t)
	deps.AWS = &fakeAWSClient{err: errors.New("caller identity の取得に失敗(テスト用)")}

	cs := testServer(ctx, t, deps)

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "whoami",
		Arguments: map[string]any{"profile": "missing"},
	})
	if err != nil {
		t.Fatalf("CallTool 自体はプロトコルエラーになってはいけない: %v", err)
	}
	if !res.IsError {
		t.Fatal("whoami が IsError を返さなかった")
	}
}

// writeTokenFile はテスト用に ssocache 互換のトークンファイルを書く
// トークン値はダミーで良い(ssocache は expiresAt と refreshToken の有無しか読まない)
func writeTokenFile(t *testing.T, cacheDir string, key string, expiresAt time.Time, hasRefreshToken bool) {
	t.Helper()

	refreshToken := ""
	if hasRefreshToken {
		refreshToken = "dummy-refresh-token"
	}

	// ssologin.Options 経由で書かせず ssocache.TokenPath と同じ場所へ直接書く
	// (internal/mcp からは internal/ssologin の非公開書き込み関数を使えないため)
	path := ssocache.TokenPath(cacheDir, key)
	writeJSONFile(t, path, map[string]any{
		"startUrl":              "https://example.awsapps.com/start",
		"region":                "us-west-2",
		"accessToken":           "dummy-access-token",
		"expiresAt":             expiresAt.UTC().Format(time.RFC3339),
		"clientId":              "dummy-client-id",
		"clientSecret":          "dummy-client-secret",
		"registrationExpiresAt": expiresAt.Add(90 * 24 * time.Hour).UTC().Format(time.RFC3339),
		"refreshToken":          refreshToken,
	})
}
