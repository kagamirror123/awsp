package cmd

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kagamirror123/awsp/internal/awscli"
)

func TestScopedEnv(t *testing.T) {
	creds := awscli.ScopedCredentials{
		AccessKeyID: "AKID", SecretAccessKey: "SECRET", SessionToken: "TOKEN",
		Expiration: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC), Region: "us-west-2",
	}

	t.Run("profile と前の認証情報を取り除き 絞った認証情報とリージョンを入れる", func(t *testing.T) {
		env := scopedEnv([]string{
			"PATH=/bin", "AWS_PROFILE=prod", "AWS_ACCESS_KEY_ID=OLD", "AWS_SESSION_TOKEN=OLD",
			"AWS_CONFIG_FILE=/tmp/config",
		}, creds)
		for _, want := range []string{
			"PATH=/bin", "AWS_CONFIG_FILE=/tmp/config", "AWS_ACCESS_KEY_ID=AKID", "AWS_SECRET_ACCESS_KEY=SECRET",
			"AWS_SESSION_TOKEN=TOKEN", "AWS_CREDENTIAL_EXPIRATION=2026-11-21T12:00:00Z",
			"AWS_REGION=us-west-2", "AWS_DEFAULT_REGION=us-west-2",
		} {
			if !slices.Contains(env, want) {
				t.Errorf("%q がない: %v", want, env)
			}
		}
		for _, entry := range env {
			if strings.HasPrefix(entry, "AWS_PROFILE=") || strings.HasSuffix(entry, "=OLD") {
				t.Errorf("%q が残っている", entry)
			}
		}
	})

	t.Run("親にリージョンがあれば上書きしない", func(t *testing.T) {
		env := scopedEnv([]string{"AWS_REGION=ap-northeast-1"}, creds)
		if !slices.Contains(env, "AWS_REGION=ap-northeast-1") || slices.Contains(env, "AWS_REGION=us-west-2") {
			t.Errorf("got %v", env)
		}
	})
}

func TestScopedRequiresDashAndReason(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"scoped", "--profile", "dev", "--policy", "q", "--reason", "test", "aws", "s3", "ls"}, "-- の後ろ"},
		{[]string{"scoped", "--profile", "dev", "--policy", "q", "--", "aws", "s3", "ls"}, "--reason"},
		{[]string{"scoped", "--profile", "dev", "--policy", "q", "--reason", "調査", "--", "aws", "s3", "ls"}, "--reason"},
		{[]string{"scoped", "--policy", "q", "--reason", "test", "--", "aws", "s3", "ls"}, "--profile"},
	}
	for _, tc := range cases {
		t.Setenv("AWS_PROFILE", "")
		root := newRootCmd()
		root.SetArgs(tc.args)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: want error containing %q, got %v", tc.args, tc.want, err)
		}
	}
}
