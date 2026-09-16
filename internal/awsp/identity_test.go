package awsp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kagamirror123/awsp/internal/awscli"
)

type fakeIdentityClient struct {
	identity awscli.Identity
	err      error
}

func (f fakeIdentityClient) CallerIdentity(_ context.Context, _ string) (awscli.Identity, error) {
	return f.identity, f.err
}

func TestWhoami_Success(t *testing.T) {
	t.Parallel()

	client := fakeIdentityClient{identity: awscli.Identity{Account: "123", UserID: "AID", ARN: "arn:aws:iam::123:user/dev"}}

	identity, err := Whoami(context.Background(), client, "dev")
	if err != nil {
		t.Fatalf("Whoami が失敗: %v", err)
	}
	if identity.Profile != "dev" || identity.Account != "123" {
		t.Fatalf("identity が想定外: %+v", identity)
	}
}

func TestWhoami_AuthError(t *testing.T) {
	t.Parallel()

	client := fakeIdentityClient{err: errors.New("token has expired")}

	_, err := Whoami(context.Background(), client, "dev")
	if err == nil {
		t.Fatal("認証エラーなのに成功した")
	}
	if !strings.Contains(err.Error(), "awsp login dev") {
		t.Fatalf("次のコマンドが案内されていない: %v", err)
	}
}

func TestWhoami_NonAuthError(t *testing.T) {
	t.Parallel()

	client := fakeIdentityClient{err: errors.New("dial tcp: i/o timeout")}

	_, err := Whoami(context.Background(), client, "dev")
	if err == nil {
		t.Fatal("非認証エラーなのに成功した")
	}
	if strings.Contains(err.Error(), "awsp login") {
		t.Fatalf("非認証エラーなのにログイン案内が出た: %v", err)
	}
}
