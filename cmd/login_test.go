package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

func TestLoginJSONKeepsPromptsOnStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[sso-session corp]\nsso_start_url = https://example.invalid/start\nsso_region = us-west-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", path)
	var stdout, stderr bytes.Buffer
	command := newLoginCmdWithRunner(&rootOptions{}, func(_ context.Context, _ awsconfig.SSOSession, _ string, _ awsp.LoginDeps, opts awsp.LoginOptions) (awsp.LoginResult, error) {
		_, err := fmt.Fprintln(opts.Output, "ブラウザで承認してください: https://example.invalid/")
		return awsp.LoginResult{SchemaVersion: 1, Status: "ok", Session: "corp", State: ssocache.StateOK}, err
	})
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"--json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var result awsp.LoginResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout 全体を JSON として解釈できません: %s: %v", stdout.String(), err)
	}
	if result.SchemaVersion != 1 || result.Status != "ok" || !strings.Contains(stderr.String(), "ブラウザで承認") {
		t.Fatalf("result=%+v stderr=%s", result, stderr.String())
	}
}
