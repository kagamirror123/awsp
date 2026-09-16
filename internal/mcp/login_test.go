package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

func loginTestSession() awsconfig.SSOSession {
	return awsconfig.SSOSession{
		Name:               "corp",
		StartURL:           "https://example.awsapps.com/start",
		Region:             "us-west-2",
		RegistrationScopes: []string{"sso:account:access"},
	}
}

func loginTestDeps(t *testing.T, oidc *fakeOIDCClient, openBrowser func(string) error) Deps {
	t.Helper()
	deps := baseDeps(t)
	// ssologin.Flow.persistToken は実時計(time.Now)でトークンの expiresAt を書く
	// baseDeps の固定 Now とずらすと 書き込まれたトークンを固定 Now で評価したときに
	// 「既に期限切れ」に見えてしまい device flow が再度起きてしまう
	// (デバイスフローが実際に絡むテストでは実時計のまま使う)
	deps.Now = nil
	deps.Profiles = &fakeProfileStore{
		profiles: []awsconfig.Profile{{Name: "dev", SSOSession: "corp"}},
		sessions: []awsconfig.SSOSession{loginTestSession()},
	}
	deps.AWS = &fakeAWSClient{byProfile: map[string]awscli.Identity{
		"dev": {Account: "123456789012", UserID: "AIDAEXAMPLE", ARN: "arn:aws:iam::123456789012:user/dev"},
	}}
	deps.NewOIDCClient = func(string) ssologin.OIDCClient { return oidc }
	deps.OpenBrowser = openBrowser
	return deps
}

func fakeRegisterOutput() *ssooidc.RegisterClientOutput {
	return &ssooidc.RegisterClientOutput{
		ClientId:              strPtr("client-id"),
		ClientSecret:          strPtr("client-secret"),
		ClientSecretExpiresAt: time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
}

func fakeStartOutput() *ssooidc.StartDeviceAuthorizationOutput {
	return &ssooidc.StartDeviceAuthorizationOutput{
		DeviceCode:              strPtr("device-code"),
		UserCode:                strPtr("ABCD-1234"),
		VerificationUri:         strPtr("https://device.sso.us-west-2.amazonaws.com/"),
		VerificationUriComplete: strPtr("https://device.sso.us-west-2.amazonaws.com/?user_code=ABCD-1234"),
		ExpiresIn:               600,
		Interval:                0,
	}
}

// blockingCreateToken は release() が呼ばれるまで CreateToken をブロックし
// その後 output/err を返すクロージャを作る 実時間の sleep/interval に依存せず
// 「まだ承認されていない」状態を決定的に再現するために使う
func blockingCreateToken(output *ssooidc.CreateTokenOutput, err error) (fn func(int) (*ssooidc.CreateTokenOutput, error), release func()) {
	gate := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	fn = func(int) (*ssooidc.CreateTokenOutput, error) {
		<-gate
		return output, err
	}
	return fn, release
}

func callLogin(ctx context.Context, t *testing.T, cs *sdkmcp.ClientSession, in LoginInput) (*sdkmcp.CallToolResult, LoginOutput) {
	t.Helper()
	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "login", Arguments: in})
	if err != nil {
		t.Fatalf("CallTool(login) がプロトコルエラーになった: %v", err)
	}
	var out LoginOutput
	if !res.IsError {
		decodeStructured(t, res.StructuredContent, &out)
	}
	return res, out
}

// TestLogin_AlreadyValidReturnsOKImmediately は事前判定で有効なセッションなら
// device flow を一切起こさず即 ok を返すことを確認する(D4)
func TestLogin_AlreadyValidReturnsOKImmediately(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	oidc := &fakeOIDCClient{}
	deps := loginTestDeps(t, oidc, func(string) error { return nil })

	// deps.Now(baseDeps 由来)と同じ基準時刻を使う 実時計を混ぜると
	// 固定 Now との差でトークンが「既に期限切れ」判定になり得るため
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	deps.Now = func() time.Time { return now }

	session := loginTestSession()
	writeTokenFile(t, deps.SSOCacheDir, session.CacheKey(), now.Add(1*time.Hour), true)

	cs := testServer(ctx, t, deps)

	res, out := callLogin(ctx, t, cs, LoginInput{Profile: "dev"})
	if res.IsError {
		t.Fatalf("login が IsError を返した: %+v", res.Content)
	}
	if out.Status != "ok" {
		t.Fatalf("Status が想定外: %s", out.Status)
	}
	if out.Identity == nil || out.Identity.Account != "123456789012" {
		t.Fatalf("Identity が想定外: %+v", out.Identity)
	}

	register, start, create := oidc.counts()
	if register != 0 || start != 0 || create != 0 {
		t.Fatalf("既に有効なのに device flow を起こした: register=%d start=%d create=%d", register, start, create)
	}
}

// TestLogin_BrowserFailureReturnsPendingThenJoinsToOK は
// 「ブラウザ起動に失敗したら pending を即返す」→「2 回目の呼び出しが合流して ok になる」を確認する(D13)
func TestLogin_BrowserFailureReturnsPendingThenJoinsToOK(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	createFn, release := blockingCreateToken(&ssooidc.CreateTokenOutput{
		AccessToken:  strPtr("access-token"),
		ExpiresIn:    3600,
		RefreshToken: strPtr("refresh-token"),
	}, nil)

	oidc := &fakeOIDCClient{
		registerOutput: fakeRegisterOutput(),
		startOutput:    fakeStartOutput(),
		createToken:    createFn,
	}
	browserErr := errors.New("ブラウザを開けません(テスト)")
	deps := loginTestDeps(t, oidc, func(string) error { return browserErr })

	cs := testServer(ctx, t, deps)

	// 1 回目: ブラウザ起動に失敗するので即 pending が返る(flow.Wait はブロックしたまま継続)
	res1, out1 := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 60, UseDeviceCode: true})
	if res1.IsError {
		t.Fatalf("1 回目が IsError を返した: %+v", res1.Content)
	}
	if out1.Status != "pending" {
		t.Fatalf("1 回目の Status が想定外: %s", out1.Status)
	}
	if out1.AuthorizationURL == "" || out1.UserCode == "" {
		t.Fatalf("pending なのに URL/コードが空: %+v", out1)
	}

	// 承認完了を許可してから 2 回目を呼ぶ(合流してブロックし ok になる)
	release()

	res2, out2 := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 10, UseDeviceCode: true})
	if res2.IsError {
		t.Fatalf("2 回目が IsError を返した: %+v", res2.Content)
	}
	if out2.Status != "ok" {
		t.Fatalf("2 回目の Status が想定外: %s", out2.Status)
	}
	if out2.Identity == nil || out2.Identity.Account != "123456789012" {
		t.Fatalf("2 回目の Identity が想定外: %+v", out2.Identity)
	}

	register, start, _ := oidc.counts()
	if register != 1 || start != 1 {
		t.Fatalf("RegisterClient/StartDeviceAuthorization が 1 回ずつでない: register=%d start=%d", register, start)
	}
}

// TestLogin_ConcurrentCallsStartDeviceFlowOnce は同じ session への同時 2 呼び出しで
// RegisterClient / StartDeviceAuthorization が 1 回しか呼ばれないことを確認する(D4)
func TestLogin_ConcurrentCallsStartDeviceFlowOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	createFn, release := blockingCreateToken(&ssooidc.CreateTokenOutput{
		AccessToken: strPtr("access-token"),
		ExpiresIn:   3600,
	}, nil)

	oidc := &fakeOIDCClient{
		registerOutput: fakeRegisterOutput(),
		startOutput:    fakeStartOutput(),
		createToken:    createFn,
	}
	deps := loginTestDeps(t, oidc, func(string) error { return nil })

	cs := testServer(ctx, t, deps)

	var wg sync.WaitGroup
	results := make([]*sdkmcp.CallToolResult, 2)
	wg.Add(2)
	for i := range results {
		i := i
		go func() {
			defer wg.Done()
			res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      "login",
				Arguments: LoginInput{Profile: "dev", TimeoutSeconds: 1, UseDeviceCode: true},
			})
			if err != nil {
				t.Errorf("CallTool がプロトコルエラーになった: %v", err)
				return
			}
			results[i] = res
		}()
	}
	wg.Wait()

	for i, res := range results {
		if res == nil {
			continue
		}
		if res.IsError {
			t.Fatalf("呼び出し%d が IsError を返した: %+v", i, res.Content)
		}
		var out LoginOutput
		decodeStructured(t, res.StructuredContent, &out)
		if out.Status != "pending" {
			t.Fatalf("呼び出し%d の Status が想定外(1s タイムアウトで pending のはず): %s", i, out.Status)
		}
	}

	register, start, _ := oidc.counts()
	if register != 1 {
		t.Fatalf("RegisterClient が 1 回でない: %d", register)
	}
	if start != 1 {
		t.Fatalf("StartDeviceAuthorization が 1 回でない: %d", start)
	}

	// バックグラウンドのフローを完了させてから終わる
	// (release だけして goroutine を宙に浮かせたまま t.TempDir() の後始末を迎えると
	//  goroutine のトークン書き込みとディレクトリ削除が競合してテストが不安定になる)
	release()
	res3, out3 := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 10, UseDeviceCode: true})
	if res3.IsError {
		t.Fatalf("後始末用の合流呼び出しが IsError を返した: %+v", res3.Content)
	}
	if out3.Status != "ok" {
		t.Fatalf("後始末用の合流呼び出しの Status が想定外: %s", out3.Status)
	}
}

// TestLogin_DeniedIsToolError は承認拒否が IsError の結果になることを確認する
func TestLogin_DeniedIsToolError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	oidc := &fakeOIDCClient{
		registerOutput: fakeRegisterOutput(),
		startOutput:    fakeStartOutput(),
		createToken: func(int) (*ssooidc.CreateTokenOutput, error) {
			return nil, &ssooidctypes.AccessDeniedException{}
		},
	}
	deps := loginTestDeps(t, oidc, func(string) error { return nil })

	cs := testServer(ctx, t, deps)

	res, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "login",
		Arguments: LoginInput{Profile: "dev", TimeoutSeconds: 30, UseDeviceCode: true},
	})
	if err != nil {
		t.Fatalf("CallTool がプロトコルエラーになった: %v", err)
	}
	if !res.IsError {
		t.Fatal("拒否されたのに IsError が立っていない")
	}
}

// TestLogin_TimeoutReturnsPendingWithoutKillingFlow は承認待ちが呼び出し側の timeout に達したとき
// pending を返しつつフローを継続することを確認する(D4)
func TestLogin_TimeoutReturnsPendingWithoutKillingFlow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	createFn, release := blockingCreateToken(&ssooidc.CreateTokenOutput{
		AccessToken: strPtr("access-token"),
		ExpiresIn:   3600,
	}, nil)

	oidc := &fakeOIDCClient{
		registerOutput: fakeRegisterOutput(),
		startOutput:    fakeStartOutput(),
		createToken:    createFn,
	}
	deps := loginTestDeps(t, oidc, func(string) error { return nil })

	cs := testServer(ctx, t, deps)

	res, out := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 1, UseDeviceCode: true})
	if res.IsError {
		t.Fatalf("timeout 経路が IsError を返した: %+v", res.Content)
	}
	if out.Status != "pending" {
		t.Fatalf("Status が想定外: %s", out.Status)
	}
	if out.AuthorizationURL == "" {
		t.Fatalf("timeout 経路でも URL が入っているはず: %+v", out)
	}

	release()

	// フローが継続していることを確認: 合流して ok になる
	res2, out2 := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 10, UseDeviceCode: true})
	if res2.IsError {
		t.Fatalf("合流呼び出しが IsError を返した: %+v", res2.Content)
	}
	if out2.Status != "ok" {
		t.Fatalf("合流呼び出しの Status が想定外: %s", out2.Status)
	}

	register, start, _ := oidc.counts()
	if register != 1 || start != 1 {
		t.Fatalf("フローが再起動されてしまった: register=%d start=%d", register, start)
	}
}

// TestLogin_PKCE_IsDefaultAndBrowserFailureJoinsToOK は use_device_code を指定しなければ
// PKCE(既定)が使われること ブラウザ起動に失敗したら pending で Method/AuthorizationURL を返すこと
// ローカルコールバックへの到達(ブラウザでの承認の代わり)後に 2 回目の呼び出しが合流して ok になることを確認する
func TestLogin_PKCE_IsDefaultAndBrowserFailureJoinsToOK(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	createFn, release := blockingCreateToken(&ssooidc.CreateTokenOutput{
		AccessToken:  strPtr("access-token"),
		ExpiresIn:    3600,
		RefreshToken: strPtr("refresh-token"),
	}, nil)

	oidc := &fakeOIDCClient{
		registerOutput: fakeRegisterOutput(),
		createToken:    createFn,
	}
	browserErr := errors.New("ブラウザを開けません(テスト)")
	deps := loginTestDeps(t, oidc, func(string) error { return browserErr })

	cs := testServer(ctx, t, deps)

	// use_device_code を指定しない = PKCE(既定)
	res1, out1 := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 60})
	if res1.IsError {
		t.Fatalf("1 回目が IsError を返した: %+v", res1.Content)
	}
	if out1.Status != "pending" {
		t.Fatalf("1 回目の Status が想定外: %s", out1.Status)
	}
	if out1.Method != ssologin.MethodAuthorizationCode {
		t.Fatalf("Method が想定外(PKCE が既定のはず): %s", out1.Method)
	}
	if out1.AuthorizationURL == "" {
		t.Fatalf("pending なのに AuthorizationURL が空: %+v", out1)
	}
	if out1.UserCode != "" {
		t.Fatalf("PKCE では UserCode は空のはず: %s", out1.UserCode)
	}

	parsed, err := url.Parse(out1.AuthorizationURL)
	if err != nil {
		t.Fatalf("AuthorizationURL の parse に失敗: %v", err)
	}
	q := parsed.Query()
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	if redirectURI == "" || state == "" {
		t.Fatalf("redirect_uri/state が空: %s", out1.AuthorizationURL)
	}

	// 「ブラウザでの承認」の代わりにローカルコールバックへ直接 GET する
	//nolint:gosec // テスト用のローカルコールバックへの GET(固定 127.0.0.1)
	resp, err := http.Get(redirectURI + "?code=abc&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("コールバックへの GET に失敗: %v", err)
	}
	_ = resp.Body.Close()

	// AWS 側からの CreateToken 応答を許可してから 2 回目を呼ぶ(合流してブロックし ok になる)
	release()

	res2, out2 := callLogin(ctx, t, cs, LoginInput{Profile: "dev", TimeoutSeconds: 10})
	if res2.IsError {
		t.Fatalf("2 回目が IsError を返した: %+v", res2.Content)
	}
	if out2.Status != "ok" {
		t.Fatalf("2 回目の Status が想定外: %s", out2.Status)
	}
	if out2.Identity == nil || out2.Identity.Account != "123456789012" {
		t.Fatalf("2 回目の Identity が想定外: %+v", out2.Identity)
	}

	register, start, _ := oidc.counts()
	if register != 1 || start != 0 {
		t.Fatalf("RegisterClient=1回/StartDeviceAuthorization=0回のはず: register=%d start=%d", register, start)
	}
}

// TestLoginInput_JSONTags は LoginInput の json タグが設計どおりであることを確認する
func TestLoginInput_JSONTags(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(LoginInput{Profile: "dev", SSOSession: "corp", TimeoutSeconds: 10, UseDeviceCode: true})
	if err != nil {
		t.Fatalf("marshal に失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal に失敗: %v", err)
	}
	for _, key := range []string{"profile", "sso_session", "timeout_seconds", "use_device_code"} {
		if _, ok := m[key]; !ok {
			t.Errorf("キーが無い: %s", key)
		}
	}
}
