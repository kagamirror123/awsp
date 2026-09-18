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
	"github.com/kagamirror123/awsp/internal/awsconfig"
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

	err := runner.Run(context.Background(), "", RunOptions{Shell: ShellSyntaxPOSIX})
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

func TestRunShellModeFish(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "it's-dev"}}},
		Selector: stubSelector{selected: "it's-dev"},
		AWS:      &stubAWSClient{identity: awscli.Identity{Account: "1", UserID: "u", ARN: "a"}},
		Stdout:   stdout,
		Stderr:   &bytes.Buffer{},
	})

	if err := runner.Run(context.Background(), "", RunOptions{Shell: ShellSyntaxFish}); err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"set -gx AWS_SDK_LOAD_CONFIG 1;\n",
		"set -gx AWS_PROFILE 'it\\'s-dev';\n",
		"set -q AWS_ACCESS_KEY_ID; and set -e -g AWS_ACCESS_KEY_ID;\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fish 出力に %q がない: %s", want, got)
		}
	}
	if strings.Contains(got, "export ") || strings.Contains(got, "unset ") {
		t.Fatalf("fish 出力に POSIX 構文が混ざっている: %s", got)
	}
}

func TestRunShellModeFishUnset(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: unsetSelection},
		AWS:      &stubAWSClient{},
		Stdout:   stdout,
		Stderr:   &bytes.Buffer{},
	})

	if err := runner.Run(context.Background(), unsetSelection, RunOptions{Shell: ShellSyntaxFish}); err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}
	if !strings.Contains(stdout.String(), "set -q AWS_PROFILE; and set -e -g AWS_PROFILE;\n") {
		t.Fatalf("fish の解除出力が想定外: %s", stdout.String())
	}
}

func TestParseShellSyntax(t *testing.T) {
	t.Parallel()

	cases := map[string]ShellSyntax{"": ShellSyntaxNone, "posix": ShellSyntaxPOSIX, "bash": ShellSyntaxPOSIX, "zsh": ShellSyntaxPOSIX, "fish": ShellSyntaxFish}
	for in, want := range cases {
		got, err := ParseShellSyntax(in)
		if err != nil || got != want {
			t.Fatalf("ParseShellSyntax(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseShellSyntax("powershell"); err == nil {
		t.Fatal("未対応シェルがエラーにならなかった")
	}
}

func TestRunLoginRetry(t *testing.T) {
	t.Parallel()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	aws := &stubAWSClient{
		identity:      awscli.Identity{Account: "1", UserID: "u", ARN: "a"},
		callerErrOnce: errors.New("token has expired"),
	}

	loginCount := 0
	var loginSession awsconfig.SSOSession
	var loginProfile string

	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev", SSOSession: "corp"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Login: func(_ context.Context, session awsconfig.SSOSession, profile string) (LoginResult, error) {
			loginCount++
			loginSession = session
			loginProfile = profile
			return LoginResult{
				SchemaVersion: 1,
				Session:       session.CacheKey(),
				Identity: &Identity{
					Profile: profile,
					Account: "1",
					UserID:  "u",
					ARN:     "a",
				},
			}, nil
		},
		Stdout: stdout,
		Stderr: stderr,
	})

	err := runner.Run(context.Background(), "", RunOptions{})
	if err != nil {
		t.Fatalf("Run が失敗: %v", err)
	}

	if loginCount != 1 {
		t.Fatalf("Login 呼び出し回数が想定外: %d", loginCount)
	}
	if loginProfile != "dev" {
		t.Fatalf("Login に渡した profile が想定外: %s", loginProfile)
	}
	if loginSession.CacheKey() != "corp" {
		t.Fatalf("Login に渡した session が想定外: %+v", loginSession)
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

	loginCalled := false
	runner := NewRunner(RunnerOptions{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Profiles: stubProfileStore{profiles: []Profile{{Name: "dev"}}},
		Selector: stubSelector{selected: "dev"},
		AWS:      aws,
		Login: func(_ context.Context, _ awsconfig.SSOSession, _ string) (LoginResult, error) {
			loginCalled = true
			return LoginResult{}, nil
		},
		Stdout: stdout,
		Stderr: io.Discard,
	})

	err := runner.Run(context.Background(), "", RunOptions{})
	if err == nil {
		t.Fatal("非認証エラーで失敗しなかった")
	}
	if loginCalled {
		t.Fatal("非認証エラーで login が呼ばれた")
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

func (s stubProfileStore) Sessions(_ context.Context) ([]awsconfig.SSOSession, error) {
	return nil, nil
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
