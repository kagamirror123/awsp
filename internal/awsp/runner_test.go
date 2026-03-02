package awsp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/kagamirror123/awsp/internal/awscli"
)

func TestRunWithProfileArg(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	aws := &stubAWSClient{
		identity: awscli.Identity{
			Account: "123456789012",
			UserID:  "AIDABCDEFGHIJKLMN",
			ARN:     "arn:aws:iam::123456789012:user/example",
		},
	}

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}, {Name: "prod"}}},
		Selector: stubSelector{selected: ""},
		AWS:      aws,
		Stdout:   stdout,
		Stderr:   stderr,
	})

	err := runner.Run(context.Background(), "dev", RunOptions{})
	if err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	if !reflect.DeepEqual(aws.calledProfiles, []string{"dev"}) {
		t.Fatalf("CallerIdentity 呼び出しが想定外: %v", aws.calledProfiles)
	}

	output := stdout.String()
	if !bytes.Contains([]byte(output), []byte("Profile validated: dev")) {
		t.Fatalf("出力に profile 設定がない: %s", output)
	}
}

func TestRunShellMode(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	aws := &stubAWSClient{
		identity: awscli.Identity{Account: "1", UserID: "u", ARN: "a"},
	}

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Stdout:   stdout,
		Stderr:   stderr,
	})

	err := runner.Run(context.Background(), "", RunOptions{ShellMode: true})
	if err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	shellScript := stdout.String()
	if !bytes.Contains([]byte(shellScript), []byte("export AWS_PROFILE=\"dev\"")) {
		t.Fatalf("shell 出力が不足: %s", shellScript)
	}

	if !bytes.Contains(stderr.Bytes(), []byte("Account : 1")) {
		t.Fatalf("shell mode の情報出力先が想定外: %s", stderr.String())
	}
}

func TestRunLoginRetry(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	aws := &stubAWSClient{
		identity:      awscli.Identity{Account: "1", UserID: "u", ARN: "a"},
		callerErrOnce: errors.New("token has expired"),
		ssoSession:    "corp",
	}

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Stdout:   stdout,
		Stderr:   stderr,
	})

	err := runner.Run(context.Background(), "", RunOptions{})
	if err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	if aws.loginCount != 1 {
		t.Fatalf("Login 呼び出し回数が想定外: %d", aws.loginCount)
	}
}

func TestRunNoLogin(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	aws := &stubAWSClient{}

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Stdout:   stdout,
		Stderr:   io.Discard,
	})

	err := runner.Run(context.Background(), "dev", RunOptions{SkipLogin: true})
	if err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	if len(aws.calledProfiles) != 0 {
		t.Fatalf("no-login なのに caller identity が呼ばれた: %v", aws.calledProfiles)
	}
}

func TestRunLoginOnly(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	aws := &stubAWSClient{identity: awscli.Identity{Account: "1", UserID: "u", ARN: "a"}}

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Stdout:   stdout,
		Stderr:   io.Discard,
	})

	err := runner.Run(context.Background(), "", RunOptions{LoginOnly: true})
	if err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	if strings.Contains(stdout.String(), "export AWS_PROFILE") {
		t.Fatalf("login-only で export が出力された: %s", stdout.String())
	}
}

func TestRunNonAuthErrorDoesNotLogin(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	aws := &stubAWSClient{callerErrOnce: errors.New("dial tcp: i/o timeout")}

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Stdout:   stdout,
		Stderr:   io.Discard,
	})

	err := runner.Run(context.Background(), "", RunOptions{})
	if err == nil {
		t.Fatal("非認証エラーで失敗しなかった")
	}
	if aws.loginCount != 0 {
		t.Fatalf("非認証エラーで login が呼ばれた: %d", aws.loginCount)
	}
}

func TestRunOptionsValidate(t *testing.T) {
	t.Parallel()

	err := RunOptions{SkipLogin: true, LoginOnly: true}.validate()
	if err == nil {
		t.Fatal("不正オプションが許可された")
	}
}

type stubProfileStore struct {
	profiles []Profile
}

func (s stubProfileStore) Profiles(_ context.Context) ([]Profile, error) {
	return s.profiles, nil
}

type stubSelector struct {
	selected string
}

func (s stubSelector) Select(_ context.Context, _ []Profile) (string, error) {
	if s.selected == "" {
		return "dev", nil
	}
	return s.selected, nil
}

type stubAWSClient struct {
	identity       awscli.Identity
	callerErrOnce  error
	ssoSession     string
	loginCount     int
	calledProfiles []string
}

func (s *stubAWSClient) CallerIdentity(_ context.Context, profile string) (awscli.Identity, error) {
	s.calledProfiles = append(s.calledProfiles, profile)
	if s.callerErrOnce != nil {
		err := s.callerErrOnce
		s.callerErrOnce = nil
		return awscli.Identity{}, err
	}
	return s.identity, nil
}

func (s *stubAWSClient) SSOSession(_ context.Context, _ string) (string, error) {
	return s.ssoSession, nil
}

func (s *stubAWSClient) Login(_ context.Context, _ string, _ string) error {
	s.loginCount++
	return nil
}
