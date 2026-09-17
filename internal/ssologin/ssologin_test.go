package ssologin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	ssooidctypes "github.com/aws/aws-sdk-go-v2/service/ssooidc/types"
	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// fakeOIDCClient は OIDCClient を満たすテスト用フェイク 実 API は一切呼ばない
type fakeOIDCClient struct {
	registerOutput *ssooidc.RegisterClientOutput
	registerErr    error
	registerInput  *ssooidc.RegisterClientInput

	startOutput *ssooidc.StartDeviceAuthorizationOutput
	startErr    error

	createResponses []createTokenResponse
	createCalls     int
	createInputs    []*ssooidc.CreateTokenInput
}

type createTokenResponse struct {
	output *ssooidc.CreateTokenOutput
	err    error
}

func (f *fakeOIDCClient) RegisterClient(
	_ context.Context,
	in *ssooidc.RegisterClientInput,
	_ ...func(*ssooidc.Options),
) (*ssooidc.RegisterClientOutput, error) {
	f.registerInput = in
	return f.registerOutput, f.registerErr
}

func (f *fakeOIDCClient) StartDeviceAuthorization(
	_ context.Context,
	_ *ssooidc.StartDeviceAuthorizationInput,
	_ ...func(*ssooidc.Options),
) (*ssooidc.StartDeviceAuthorizationOutput, error) {
	return f.startOutput, f.startErr
}

func (f *fakeOIDCClient) CreateToken(
	_ context.Context,
	in *ssooidc.CreateTokenInput,
	_ ...func(*ssooidc.Options),
) (*ssooidc.CreateTokenOutput, error) {
	f.createInputs = append(f.createInputs, in)
	if f.createCalls >= len(f.createResponses) {
		return nil, errors.New("createResponses が尽きました(テスト設定ミス)")
	}
	resp := f.createResponses[f.createCalls]
	f.createCalls++
	return resp.output, resp.err
}

func testSession() awsconfig.SSOSession {
	return awsconfig.SSOSession{
		Name:               "corp",
		StartURL:           "https://example.awsapps.com/start",
		Region:             "us-west-2",
		RegistrationScopes: []string{"sso:account:access"},
	}
}

func registerOutput() *ssooidc.RegisterClientOutput {
	return &ssooidc.RegisterClientOutput{
		ClientId:              strPtr("client-id"),
		ClientSecret:          strPtr("client-secret"),
		ClientSecretExpiresAt: time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
}

func startOutput() *ssooidc.StartDeviceAuthorizationOutput {
	return &ssooidc.StartDeviceAuthorizationOutput{
		DeviceCode:              strPtr("device-code"),
		UserCode:                strPtr("ABCD-1234"),
		VerificationUri:         strPtr("https://device.sso.us-west-2.amazonaws.com/"),
		VerificationUriComplete: strPtr("https://device.sso.us-west-2.amazonaws.com/?user_code=ABCD-1234"),
		ExpiresIn:               600,
		Interval:                0,
	}
}

func noopOpenBrowser(_ string) error { return nil }

func TestStart_DeviceCode_WritesFlowFieldsAndDoesNotBlock(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	client := &fakeOIDCClient{
		registerOutput: registerOutput(),
		startOutput:    startOutput(),
	}

	flow, err := Start(context.Background(), testSession(), Options{
		Client:        client,
		CacheDir:      cacheDir,
		OpenBrowser:   noopOpenBrowser,
		UseDeviceCode: true,
	})
	if err != nil {
		t.Fatalf("Start が失敗: %v", err)
	}

	if flow.Method != MethodDeviceCode {
		t.Fatalf("Method が想定外: %s", flow.Method)
	}
	if flow.UserCode != "ABCD-1234" {
		t.Fatalf("UserCode が想定外: %s", flow.UserCode)
	}
	if flow.AuthorizationURL == "" {
		t.Fatal("AuthorizationURL が空")
	}
	if flow.BrowserErr != nil {
		t.Fatalf("BrowserErr が想定外: %v", flow.BrowserErr)
	}

	// device code の登録は grantTypes/redirectUris/issuerUrl を付けない(旧 CLI 互換の形)
	if client.registerInput == nil {
		t.Fatal("RegisterClient が呼ばれていない")
	}
	if client.registerInput.GrantTypes != nil {
		t.Fatalf("device code の登録に GrantTypes が付いている: %v", client.registerInput.GrantTypes)
	}
	if client.registerInput.RedirectUris != nil {
		t.Fatalf("device code の登録に RedirectUris が付いている: %v", client.registerInput.RedirectUris)
	}
	if client.registerInput.IssuerUrl != nil {
		t.Fatalf("device code の登録に IssuerUrl が付いている: %v", client.registerInput.IssuerUrl)
	}
}

func TestWait_PendingThenSuccess(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	tokenPath := ssocache.TokenPath(cacheDir, session.CacheKey())

	client := &fakeOIDCClient{
		createResponses: []createTokenResponse{
			{err: &ssooidctypes.AuthorizationPendingException{}},
			{output: &ssooidc.CreateTokenOutput{
				AccessToken:  strPtr("access-token"),
				ExpiresIn:    3600,
				RefreshToken: strPtr("refresh-token"),
			}},
		},
	}

	var sleptDurations []time.Duration
	flow := &Flow{
		client:       client,
		session:      session,
		tokenPath:    tokenPath,
		clientID:     "client-id",
		clientSecret: "client-secret",
		deviceCode:   "device-code",
		ExpiresAt:    time.Now().Add(1 * time.Minute),
		Interval:     2 * time.Second,
		sleepFunc: func(_ context.Context, d time.Duration) error {
			sleptDurations = append(sleptDurations, d)
			return nil
		},
	}

	result, err := flow.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait が失敗: %v", err)
	}
	if !result.HasRefreshToken {
		t.Fatal("HasRefreshToken が false")
	}
	if len(sleptDurations) != 1 || sleptDurations[0] != 2*time.Second {
		t.Fatalf("sleep 回数/間隔が想定外: %v", sleptDurations)
	}
	if client.createCalls != 2 {
		t.Fatalf("CreateToken の呼び出し回数が想定外: %d", client.createCalls)
	}

	assertTokenFileWritten(t, tokenPath, session, true)
}

func TestWait_SlowDownIncreasesInterval(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	tokenPath := ssocache.TokenPath(cacheDir, session.CacheKey())

	client := &fakeOIDCClient{
		createResponses: []createTokenResponse{
			{err: &ssooidctypes.AuthorizationPendingException{}},
			{err: &ssooidctypes.SlowDownException{}},
			{output: &ssooidc.CreateTokenOutput{
				AccessToken: strPtr("access-token"),
				ExpiresIn:   3600,
			}},
		},
	}

	var sleptDurations []time.Duration
	flow := &Flow{
		client:       client,
		session:      session,
		tokenPath:    tokenPath,
		clientID:     "client-id",
		clientSecret: "client-secret",
		deviceCode:   "device-code",
		ExpiresAt:    time.Now().Add(1 * time.Minute),
		Interval:     2 * time.Second,
		sleepFunc: func(_ context.Context, d time.Duration) error {
			sleptDurations = append(sleptDurations, d)
			return nil
		},
	}

	result, err := flow.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait が失敗: %v", err)
	}
	if result.HasRefreshToken {
		t.Fatal("refreshToken を返していないのに HasRefreshToken=true")
	}

	want := []time.Duration{2 * time.Second, 7 * time.Second}
	if len(sleptDurations) != len(want) {
		t.Fatalf("sleep 回数が想定外: %v", sleptDurations)
	}
	for i, d := range want {
		if sleptDurations[i] != d {
			t.Fatalf("sleep[%d] が想定外: got=%s want=%s", i, sleptDurations[i], d)
		}
	}
}

func TestWait_ExpiredTokenFails(t *testing.T) {
	t.Parallel()

	session := testSession()
	client := &fakeOIDCClient{
		createResponses: []createTokenResponse{
			{err: &ssooidctypes.ExpiredTokenException{}},
		},
	}

	flow := &Flow{
		client:       client,
		session:      session,
		tokenPath:    filepath.Join(t.TempDir(), "unused.json"),
		clientID:     "client-id",
		clientSecret: "client-secret",
		deviceCode:   "device-code",
		ExpiresAt:    time.Now().Add(1 * time.Minute),
		sleepFunc: func(_ context.Context, _ time.Duration) error {
			t.Fatal("expired 応答で sleep が呼ばれてはいけない")
			return nil
		},
	}

	_, err := flow.Wait(context.Background())
	if err == nil {
		t.Fatal("エラーにならなかった")
	}
}

func TestWait_ContextDeadlineFails(t *testing.T) {
	t.Parallel()

	session := testSession()
	client := &fakeOIDCClient{
		createResponses: []createTokenResponse{
			{err: &ssooidctypes.AuthorizationPendingException{}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	flow := &Flow{
		client:       client,
		session:      session,
		tokenPath:    filepath.Join(t.TempDir(), "unused.json"),
		clientID:     "client-id",
		clientSecret: "client-secret",
		deviceCode:   "device-code",
		ExpiresAt:    time.Now().Add(1 * time.Minute),
		sleepFunc: func(_ context.Context, _ time.Duration) error {
			t.Fatal("ctx が既に done なので CreateToken/sleep に到達してはいけない")
			return nil
		},
	}

	_, err := flow.Wait(ctx)
	if err == nil {
		t.Fatal("ctx 期限切れなのにエラーにならなかった")
	}
	if client.createCalls != 0 {
		t.Fatalf("ctx が done なのに CreateToken が呼ばれた: %d", client.createCalls)
	}
}

func TestWait_WrittenFileKeyAndFormat(t *testing.T) {
	t.Parallel()

	// MkdirAll は既存ディレクトリの権限を変えないため
	// パーミッション検証にはまだ存在しないサブディレクトリを使う
	cacheDir := filepath.Join(t.TempDir(), "sso", "cache")
	session := testSession()
	tokenPath := ssocache.TokenPath(cacheDir, session.CacheKey())

	client := &fakeOIDCClient{
		createResponses: []createTokenResponse{
			{output: &ssooidc.CreateTokenOutput{
				AccessToken:  strPtr("access-token"),
				ExpiresIn:    3600,
				RefreshToken: strPtr("refresh-token"),
			}},
		},
	}

	flow := &Flow{
		client:                client,
		session:               session,
		tokenPath:             tokenPath,
		clientID:              "client-id",
		clientSecret:          "client-secret",
		deviceCode:            "device-code",
		registrationExpiresAt: time.Now().Add(90 * 24 * time.Hour),
		ExpiresAt:             time.Now().Add(1 * time.Minute),
	}

	if _, err := flow.Wait(context.Background()); err != nil {
		t.Fatalf("Wait が失敗: %v", err)
	}

	assertTokenFileWritten(t, tokenPath, session, true)

	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("os.Stat が失敗: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ファイル権限が想定外: %v", info.Mode().Perm())
	}

	dirInfo, err := os.Stat(filepath.Dir(tokenPath))
	if err != nil {
		t.Fatalf("os.Stat(dir) が失敗: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("ディレクトリ権限が想定外: %v", dirInfo.Mode().Perm())
	}
}

func assertTokenFileWritten(t *testing.T, path string, session awsconfig.SSOSession, wantRefreshToken bool) {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // テストが自分で書いたファイルを読むだけ
	if err != nil {
		t.Fatalf("トークンファイルを読めない: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("トークンファイルの JSON が不正: %v", err)
	}

	if parsed["startUrl"] != session.StartURL {
		t.Fatalf("startUrl が想定外: %v", parsed["startUrl"])
	}
	if parsed["region"] != session.Region {
		t.Fatalf("region が想定外: %v", parsed["region"])
	}
	if parsed["clientId"] != "client-id" || parsed["clientSecret"] != "client-secret" {
		t.Fatalf("client 情報が想定外: %v", parsed)
	}

	expiresAt, ok := parsed["expiresAt"].(string)
	if !ok {
		t.Fatal("expiresAt が文字列でない")
	}
	if _, err := time.Parse(time.RFC3339, expiresAt); err != nil {
		t.Fatalf("expiresAt の形式が RFC3339 でない: %s: %v", expiresAt, err)
	}

	registrationExpiresAt, ok := parsed["registrationExpiresAt"].(string)
	if !ok {
		t.Fatal("registrationExpiresAt が文字列でない")
	}
	if _, err := time.Parse(time.RFC3339, registrationExpiresAt); err != nil {
		t.Fatalf("registrationExpiresAt の形式が RFC3339 でない: %s: %v", registrationExpiresAt, err)
	}

	_, hasRefresh := parsed["refreshToken"]
	if hasRefresh != wantRefreshToken {
		t.Fatalf("refreshToken の有無が想定外: got=%v want=%v", hasRefresh, wantRefreshToken)
	}
}

// startPKCEFlow は PKCE(既定)の Flow を起動するテスト用ヘルパー
func startPKCEFlow(t *testing.T, client *fakeOIDCClient, cacheDir string, session awsconfig.SSOSession) *Flow {
	t.Helper()

	flow, err := Start(context.Background(), session, Options{
		Client:      client,
		CacheDir:    cacheDir,
		OpenBrowser: noopOpenBrowser,
	})
	if err != nil {
		t.Fatalf("Start が失敗: %v", err)
	}
	return flow
}

// callbackRedirectURI は flow.AuthorizationURL の redirect_uri クエリ値を取り出す
func callbackRedirectURI(t *testing.T, flow *Flow) string {
	t.Helper()

	parsed, err := url.Parse(flow.AuthorizationURL)
	if err != nil {
		t.Fatalf("AuthorizationURL の parse に失敗: %v", err)
	}
	redirectURI := parsed.Query().Get("redirect_uri")
	if redirectURI == "" {
		t.Fatal("redirect_uri が空")
	}
	return redirectURI
}

// TestPKCE_RegisterClientAndAuthorizationURL は PKCE の RegisterClient 入力と
// AuthorizationURL のクエリが仕様(aws CLI の SSOTokenFetcherAuth)どおりであることを検証する
func TestPKCE_RegisterClientAndAuthorizationURL(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	client := &fakeOIDCClient{registerOutput: registerOutput()}

	flow := startPKCEFlow(t, client, cacheDir, session)
	t.Cleanup(func() { flow.callback.close() })

	if flow.Method != MethodAuthorizationCode {
		t.Fatalf("Method が想定外: %s", flow.Method)
	}
	if flow.UserCode != "" {
		t.Fatalf("PKCE では UserCode は空のはず: %s", flow.UserCode)
	}
	if flow.BrowserErr != nil {
		t.Fatalf("BrowserErr が想定外: %v", flow.BrowserErr)
	}

	// RegisterClient の入力検証
	if client.registerInput == nil {
		t.Fatal("RegisterClient が呼ばれていない")
	}
	if !reflect.DeepEqual(client.registerInput.GrantTypes, []string{"authorization_code", "refresh_token"}) {
		t.Fatalf("GrantTypes が想定外: %v", client.registerInput.GrantTypes)
	}
	if !reflect.DeepEqual(client.registerInput.RedirectUris, []string{"http://127.0.0.1/oauth/callback"}) {
		t.Fatalf("RedirectUris が想定外(ポート無しのはず): %v", client.registerInput.RedirectUris)
	}
	if strVal(client.registerInput.IssuerUrl) != session.StartURL {
		t.Fatalf("IssuerUrl が想定外: %s", strVal(client.registerInput.IssuerUrl))
	}
	if !reflect.DeepEqual(client.registerInput.Scopes, session.RegistrationScopes) {
		t.Fatalf("Scopes が想定外: %v", client.registerInput.Scopes)
	}

	// AuthorizationURL のクエリ検証
	parsed, err := url.Parse(flow.AuthorizationURL)
	if err != nil {
		t.Fatalf("AuthorizationURL の parse に失敗: %v", err)
	}
	q := parsed.Query()

	if q.Get("response_type") != "code" {
		t.Fatalf("response_type が想定外: %s", q.Get("response_type"))
	}
	if q.Get("client_id") != "client-id" {
		t.Fatalf("client_id が想定外: %s", q.Get("client_id"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method が想定外: %s", q.Get("code_challenge_method"))
	}
	if q.Get("scopes") != strings.Join(session.RegistrationScopes, " ") {
		t.Fatalf("scopes が想定外: %s", q.Get("scopes"))
	}
	if q.Get("state") == "" {
		t.Fatal("state が空")
	}
	if q.Get("code_challenge") == "" {
		t.Fatal("code_challenge が空")
	}

	redirectURI := q.Get("redirect_uri")
	redirectParsed, err := url.Parse(redirectURI)
	if err != nil {
		t.Fatalf("redirect_uri の parse に失敗: %v", err)
	}
	_, port, err := net.SplitHostPort(redirectParsed.Host)
	if err != nil {
		t.Fatalf("redirect_uri のホストが不正: %s: %v", redirectURI, err)
	}
	if port != strconv.Itoa(flow.callback.port()) {
		t.Fatalf("redirect_uri のポートが実際の listen ポートと不一致: got=%s want=%d", port, flow.callback.port())
	}
}

// TestPKCE_StartAndWait_Success はブラウザの代わりにコールバック URL へ直接 GET し
// Wait が成功して CreateToken(authorization_code)が期待どおりの入力で呼ばれ
// code_challenge が CodeVerifier の sha256 と一致し トークンファイルが書かれることを確認する
func TestPKCE_StartAndWait_Success(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	tokenPath := ssocache.TokenPath(cacheDir, session.CacheKey())

	client := &fakeOIDCClient{
		registerOutput: registerOutput(),
		createResponses: []createTokenResponse{
			{output: &ssooidc.CreateTokenOutput{
				AccessToken:  strPtr("access-token"),
				ExpiresIn:    3600,
				RefreshToken: strPtr("refresh-token"),
			}},
		},
	}

	flow := startPKCEFlow(t, client, cacheDir, session)

	parsed, err := url.Parse(flow.AuthorizationURL)
	if err != nil {
		t.Fatalf("AuthorizationURL の parse に失敗: %v", err)
	}
	state := parsed.Query().Get("state")
	codeChallenge := parsed.Query().Get("code_challenge")
	redirectURI := callbackRedirectURI(t, flow)

	//nolint:gosec // テスト用のローカルコールバックへの GET(固定 127.0.0.1)
	resp, err := http.Get(redirectURI + "?code=abc&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("コールバックへの GET に失敗: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "承認を受け付けました") || strings.Contains(string(body), "認証が完了しました") {
		t.Fatal("トークン交換前の表示が完了を断言しています")
	}

	result, err := flow.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait が失敗: %v", err)
	}
	if !result.HasRefreshToken {
		t.Fatal("HasRefreshToken が false")
	}

	if len(client.createInputs) != 1 {
		t.Fatalf("CreateToken の呼び出し回数が想定外: %d", len(client.createInputs))
	}
	createInput := client.createInputs[0]
	if strVal(createInput.GrantType) != "authorization_code" {
		t.Fatalf("GrantType が想定外: %s", strVal(createInput.GrantType))
	}
	if strVal(createInput.Code) != "abc" {
		t.Fatalf("Code が想定外: %s", strVal(createInput.Code))
	}
	if strVal(createInput.RedirectUri) != redirectURI {
		t.Fatalf("RedirectUri が想定外: got=%s want=%s", strVal(createInput.RedirectUri), redirectURI)
	}
	if strVal(createInput.ClientId) != "client-id" || strVal(createInput.ClientSecret) != "client-secret" {
		t.Fatalf("client 情報が想定外: id=%s secret=%s", strVal(createInput.ClientId), strVal(createInput.ClientSecret))
	}

	sum := sha256.Sum256([]byte(strVal(createInput.CodeVerifier)))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if wantChallenge != codeChallenge {
		t.Fatalf("code_challenge が CodeVerifier の sha256 と一致しない: got=%s want=%s", codeChallenge, wantChallenge)
	}

	assertTokenFileWritten(t, tokenPath, session, true)
}

// TestPKCE_StateMismatchFails は state 不一致で失敗し CreateToken が呼ばれないことを確認する
func TestPKCE_StateMismatchFails(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	client := &fakeOIDCClient{registerOutput: registerOutput()}

	flow := startPKCEFlow(t, client, cacheDir, session)
	redirectURI := callbackRedirectURI(t, flow)

	//nolint:gosec // テスト用のローカルコールバックへの GET(固定 127.0.0.1)
	resp, err := http.Get(redirectURI + "?code=abc&state=wrong-state")
	if err != nil {
		t.Fatalf("コールバックへの GET に失敗: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest || strings.Contains(string(body), "承認を受け付けました") {
		t.Fatalf("state 不一致で成功を表示しました: status=%d", resp.StatusCode)
	}

	_, err = flow.Wait(context.Background())
	if err == nil {
		t.Fatal("state 不一致なのにエラーにならなかった")
	}
	if client.createCalls != 0 {
		t.Fatalf("state 不一致なのに CreateToken が呼ばれた: %d", client.createCalls)
	}
}

// TestPKCE_ErrorParamFails は ?error=... のコールバックで失敗することを確認する
func TestPKCE_ErrorParamFails(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	client := &fakeOIDCClient{registerOutput: registerOutput()}

	flow := startPKCEFlow(t, client, cacheDir, session)
	redirectURI := callbackRedirectURI(t, flow)

	//nolint:gosec // テスト用のローカルコールバックへの GET(固定 127.0.0.1)
	resp, err := http.Get(redirectURI + "?error=access_denied&state=" + url.QueryEscape(flow.state))
	if err != nil {
		t.Fatalf("コールバックへの GET に失敗: %v", err)
	}
	_ = resp.Body.Close()

	_, err = flow.Wait(context.Background())
	if err == nil {
		t.Fatal("error クエリがあるのにエラーにならなかった")
	}
	if client.createCalls != 0 {
		t.Fatalf("認可拒否なのに CreateToken が呼ばれた: %d", client.createCalls)
	}
}

// TestPKCE_ContextDeadlineFails は ctx が既に done ならコールバックを待たず失敗することを確認する
func TestPKCE_ContextDeadlineFails(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	session := testSession()
	client := &fakeOIDCClient{registerOutput: registerOutput()}

	flow := startPKCEFlow(t, client, cacheDir, session)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := flow.Wait(ctx)
	if err == nil {
		t.Fatal("ctx 期限切れなのにエラーにならなかった")
	}
	if client.createCalls != 0 {
		t.Fatalf("ctx が done なのに CreateToken が呼ばれた: %d", client.createCalls)
	}
}
