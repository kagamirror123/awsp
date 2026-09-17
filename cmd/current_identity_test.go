package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/kagamirror123/awsp/internal/awscli"
)

func TestFetchCurrentIdentity_WithoutLogin(t *testing.T) {
	t.Parallel()

	callerCalls := 0
	loginCalls := 0
	deps := currentIdentityDeps{
		CallerIdentity: func(_ context.Context, _ string) (awscli.Identity, error) {
			callerCalls++
			return awscli.Identity{Account: "123", UserID: "AID", ARN: "arn:aws:iam::123:user/dev"}, nil
		},
		Login: func(_ context.Context, _ string, _ io.Writer) (awscli.Identity, error) {
			loginCalls++
			return awscli.Identity{}, nil
		},
	}

	got, err := fetchCurrentIdentity(context.Background(), "dev", io.Discard, deps)
	if err != nil {
		t.Fatalf("fetchCurrentIdentity が失敗: %v", err)
	}

	if got.Account != "123" {
		t.Fatalf("Account が想定外: %s", got.Account)
	}
	if loginCalls != 0 {
		t.Fatalf("不要なログインが呼ばれている: %d", loginCalls)
	}
	if callerCalls != 1 {
		t.Fatalf("CallerIdentity の呼び出し回数が想定外: %d", callerCalls)
	}
}

func TestFetchCurrentIdentity_WithLoginFallback(t *testing.T) {
	t.Parallel()

	var lastLoginProfile string
	deps := currentIdentityDeps{
		CallerIdentity: func(_ context.Context, _ string) (awscli.Identity, error) {
			return awscli.Identity{}, errors.New("SSOProviderInvalidToken: the SSO session has expired or is invalid")
		},
		Login: func(_ context.Context, profile string, _ io.Writer) (awscli.Identity, error) {
			lastLoginProfile = profile
			return awscli.Identity{Account: "999", UserID: "AID2", ARN: "arn:aws:sts::999:assumed-role/Admin/me"}, nil
		},
	}

	var stderr bytes.Buffer
	got, err := fetchCurrentIdentity(context.Background(), "prod", &stderr, deps)
	if err != nil {
		t.Fatalf("fetchCurrentIdentity が失敗: %v", err)
	}

	if got.Account != "999" {
		t.Fatalf("Account が想定外: %s", got.Account)
	}
	if lastLoginProfile != "prod" {
		t.Fatalf("Login に渡した profile が想定外: %s", lastLoginProfile)
	}
	if stderr.Len() == 0 {
		t.Fatal("ログイン案内が出力されていない")
	}
}

func TestFetchCurrentIdentity_NonAuthError(t *testing.T) {
	t.Parallel()

	loginCalls := 0
	deps := currentIdentityDeps{
		CallerIdentity: func(_ context.Context, _ string) (awscli.Identity, error) {
			return awscli.Identity{}, errors.New("dial tcp timeout")
		},
		Login: func(_ context.Context, _ string, _ io.Writer) (awscli.Identity, error) {
			loginCalls++
			return awscli.Identity{}, nil
		},
	}

	_, err := fetchCurrentIdentity(context.Background(), "dev", io.Discard, deps)
	if err == nil {
		t.Fatal("非認証エラーなのに成功している")
	}
	if loginCalls != 0 {
		t.Fatalf("非認証エラーでログインすべきではない: %d", loginCalls)
	}
}
