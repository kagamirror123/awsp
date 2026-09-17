package mcp

import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/smithy-go"
	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

type identityFunc func(context.Context, string) (awscli.Identity, error)

func (f identityFunc) CallerIdentity(ctx context.Context, p string) (awscli.Identity, error) {
	return f(ctx, p)
}

func TestLogin_RevokedUnexpiredSessionReauthenticates(t *testing.T) {
	t.Parallel()
	oidc := successfulOIDC()
	deps := loginTestDeps(t, oidc, func(string) error { return nil })
	writeTokenFile(t, deps.SSOCacheDir, "corp", time.Now().Add(time.Hour), true)
	var calls atomic.Int32
	deps.AWS = identityFunc(func(context.Context, string) (awscli.Identity, error) {
		if calls.Add(1) == 1 {
			return awscli.Identity{}, &smithy.GenericAPIError{Code: "UnauthorizedException", Message: "Session token not found or invalid"}
		}
		return awscli.Identity{Account: "123456789012"}, nil
	})
	cs := testServer(t.Context(), t, deps)
	res, out := callLogin(t.Context(), t, cs, LoginInput{Profile: "dev", UseDeviceCode: true})
	if res.IsError || out.Status != "ok" || out.SchemaVersion != 1 || out.Identity == nil {
		t.Fatalf("res=%+v out=%+v", res, out)
	}
	if r, _, _ := oidc.counts(); r != 1 {
		t.Fatalf("flows=%d", r)
	}
}

func successfulOIDC() *fakeOIDCClient {
	return &fakeOIDCClient{registerOutput: fakeRegisterOutput(), startOutput: fakeStartOutput(), createToken: func(int) (*ssooidc.CreateTokenOutput, error) {
		return &ssooidc.CreateTokenOutput{AccessToken: strPtr("test-token"), ExpiresIn: 3600}, nil
	}}
}

func TestLogin_CacheReadIsInsideSharedOperation(t *testing.T) {
	t.Parallel()
	oidc := successfulOIDC()
	deps := loginTestDeps(t, oidc, func(string) error { return nil })
	entered, resume := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	deps.Now = func() time.Time {
		if reads.Add(1) == 1 {
			close(entered)
			<-resume
		}
		return time.Now()
	}
	h := &handlers{deps: deps, manager: newLoginManager(), rootCtx: t.Context()}
	first := make(chan error, 1)
	go func() {
		_, _, err := h.login(t.Context(), nil, LoginInput{Profile: "dev", UseDeviceCode: true})
		first <- err
	}()
	<-entered
	// 先行処理がキャッシュ判定中でも、後続は新しいフローを作らず合流する。
	_, out, err := h.login(t.Context(), nil, LoginInput{Profile: "dev", UseDeviceCode: true, TimeoutSeconds: 1})
	close(resume)
	if firstErr := <-first; firstErr != nil {
		t.Fatal(firstErr)
	}
	if err != nil || out.Status != "pending" || out.Phase != "starting" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	_, out, err = h.login(t.Context(), nil, LoginInput{Profile: "dev", UseDeviceCode: true})
	if err != nil || out.Status != "ok" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if r, _, _ := oidc.counts(); r != 1 {
		t.Fatalf("二重起動: flows=%d", r)
	}
}

type blockingRegisterOIDC struct {
	*fakeOIDCClient
	deadline chan time.Time
	release  chan struct{}
	canceled chan struct{}
}

func (c *blockingRegisterOIDC) RegisterClient(ctx context.Context, _ *ssooidc.RegisterClientInput, _ ...func(*ssooidc.Options)) (*ssooidc.RegisterClientOutput, error) {
	deadline, _ := ctx.Deadline()
	c.deadline <- deadline
	select {
	case <-c.release:
		return fakeRegisterOutput(), nil
	case <-ctx.Done():
		close(c.canceled)
		return nil, ctx.Err()
	}
}

func TestLogin_StartupIsBoundedAndCallerTimeoutPreservesFlow(t *testing.T) {
	t.Parallel()
	client := &blockingRegisterOIDC{fakeOIDCClient: successfulOIDC(), deadline: make(chan time.Time, 1), release: make(chan struct{}), canceled: make(chan struct{})}
	deps := loginTestDeps(t, client.fakeOIDCClient, func(string) error { return nil })
	deps.NewOIDCClient = func(string) ssologin.OIDCClient { return client }
	root, cancel := context.WithCancel(t.Context())
	defer cancel()
	h := &handlers{deps: deps, manager: newLoginManager(), rootCtx: root}
	started := time.Now()
	_, out, err := h.login(t.Context(), nil, LoginInput{Profile: "dev", UseDeviceCode: true, TimeoutSeconds: 1})
	if err != nil || out.Status != "pending" || out.Phase != "starting" || out.AuthorizationURL != "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("呼び出しの待機上限を超過しました")
	}
	deadline := <-client.deadline
	if deadline.IsZero() || deadline.Sub(started) > 31*time.Second {
		t.Fatalf("開始処理の上限がありません: %v", deadline)
	}
	select {
	case <-client.canceled:
		t.Fatal("呼び出しの期限で共有フローが中断されました")
	default:
	}
	close(client.release)
	_, out, err = h.login(t.Context(), nil, LoginInput{Profile: "dev", UseDeviceCode: true})
	if err != nil || out.Status != "ok" {
		t.Fatalf("再合流に失敗: out=%+v err=%v", out, err)
	}
}

func TestClampLoginTimeoutDoesNotOverflow(t *testing.T) {
	if got := clampLoginTimeout(math.MaxInt); got != maxLoginTimeout {
		t.Fatalf("timeout=%s", got)
	}
}

func TestLogin_StartupCancellationReleasesEntry(t *testing.T) {
	t.Parallel()
	client := &blockingRegisterOIDC{fakeOIDCClient: successfulOIDC(), deadline: make(chan time.Time, 1), release: make(chan struct{}), canceled: make(chan struct{})}
	deps := loginTestDeps(t, client.fakeOIDCClient, func(string) error { return nil })
	deps.NewOIDCClient = func(string) ssologin.OIDCClient { return client }
	root, cancel := context.WithCancel(t.Context())
	defer cancel()
	h := &handlers{deps: deps, manager: newLoginManager(), rootCtx: root}
	done := make(chan error, 1)
	go func() { _, _, err := h.login(t.Context(), nil, LoginInput{Profile: "dev"}); done <- err }()
	<-client.deadline
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	// release は done の直後。次の acquire で再利用できることを確認する。
	h.manager.mu.Lock()
	entry := h.manager.flows["corp"]
	h.manager.mu.Unlock()
	if entry != nil {
		select {
		case <-entry.done:
		default:
			t.Fatal("中断後も未完了のエントリが残っています")
		}
	}
}

func TestLogin_JoinedProfilesDoNotShareIdentityErrors(t *testing.T) {
	t.Parallel()
	deps := loginTestDeps(t, successfulOIDC(), func(string) error { return nil })
	h := &handlers{deps: deps, manager: newLoginManager(), rootCtx: t.Context()}
	entry, _ := h.manager.acquire("corp")
	entry.profile = "restricted"
	entry.result = LoginOutput{SchemaVersion: 1, Status: "ok", Session: "corp"}
	entry.err = &smithy.GenericAPIError{Code: "AccessDenied", Message: "restricted role"}
	close(entry.ready)
	close(entry.done)
	_, out, err := h.login(t.Context(), nil, LoginInput{Profile: "dev"})
	if err != nil || out.Identity == nil || out.Identity.Profile != "dev" {
		t.Fatalf("別の profile のエラーが波及しました: out=%+v err=%v", out, err)
	}
}
