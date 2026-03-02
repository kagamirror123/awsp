package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/kagamirror123/awsp/internal/awscli"
)

type callerIdentityResult struct {
	identity awscli.Identity
	err      error
}

type stubCurrentIdentityClient struct {
	callerResults []callerIdentityResult
	callerCalls   int

	ssoSession string
	ssoErr     error

	loginErr          error
	loginCalls        int
	lastLoginProfile  string
	lastLoginSession  string
	lastCallerProfile string
}

func (s *stubCurrentIdentityClient) CallerIdentity(_ context.Context, profile string) (awscli.Identity, error) {
	s.callerCalls++
	s.lastCallerProfile = profile

	if len(s.callerResults) == 0 {
		return awscli.Identity{}, nil
	}

	result := s.callerResults[0]
	if len(s.callerResults) > 1 {
		s.callerResults = s.callerResults[1:]
	}

	return result.identity, result.err
}

func (s *stubCurrentIdentityClient) SSOSession(_ context.Context, _ string) (string, error) {
	return s.ssoSession, s.ssoErr
}

func (s *stubCurrentIdentityClient) Login(_ context.Context, profile string, ssoSession string) error {
	s.loginCalls++
	s.lastLoginProfile = profile
	s.lastLoginSession = ssoSession
	return s.loginErr
}

func TestFetchCurrentIdentity_WithoutLogin(t *testing.T) {
	t.Parallel()

	client := &stubCurrentIdentityClient{
		callerResults: []callerIdentityResult{
			{identity: awscli.Identity{Account: "123", UserID: "AID", ARN: "arn:aws:iam::123:user/dev"}},
		},
	}

	got, err := fetchCurrentIdentity(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		client,
		"dev",
		io.Discard,
	)
	if err != nil {
		t.Fatalf("fetchCurrentIdentity が失敗: %v", err)
	}

	if got.Account != "123" {
		t.Fatalf("Account が想定外: %s", got.Account)
	}
	if client.loginCalls != 0 {
		t.Fatalf("不要なログインが呼ばれている: %d", client.loginCalls)
	}
}

func TestFetchCurrentIdentity_WithLoginFallback(t *testing.T) {
	t.Parallel()

	client := &stubCurrentIdentityClient{
		callerResults: []callerIdentityResult{
			{err: errors.New("failed to refresh cached credentials")},
			{identity: awscli.Identity{Account: "999", UserID: "AID2", ARN: "arn:aws:sts::999:assumed-role/Admin/me"}},
		},
		ssoSession: "corp-sso",
	}

	var stderr bytes.Buffer
	got, err := fetchCurrentIdentity(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		client,
		"prod",
		&stderr,
	)
	if err != nil {
		t.Fatalf("fetchCurrentIdentity が失敗: %v", err)
	}

	if got.Account != "999" {
		t.Fatalf("Account が想定外: %s", got.Account)
	}
	if client.loginCalls != 1 {
		t.Fatalf("ログイン呼び出し回数が想定外: %d", client.loginCalls)
	}
	if client.lastLoginProfile != "prod" || client.lastLoginSession != "corp-sso" {
		t.Fatalf("ログイン引数が想定外: profile=%s session=%s", client.lastLoginProfile, client.lastLoginSession)
	}
}

func TestFetchCurrentIdentity_NonAuthError(t *testing.T) {
	t.Parallel()

	client := &stubCurrentIdentityClient{
		callerResults: []callerIdentityResult{
			{err: errors.New("dial tcp timeout")},
		},
	}

	_, err := fetchCurrentIdentity(
		context.Background(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		client,
		"dev",
		io.Discard,
	)
	if err == nil {
		t.Fatal("非認証エラーなのに成功している")
	}
	if client.loginCalls != 0 {
		t.Fatalf("非認証エラーでログインすべきではない: %d", client.loginCalls)
	}
}
