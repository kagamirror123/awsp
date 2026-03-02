package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRootVersionFlag(t *testing.T) {
	t.Cleanup(func() {
		buildVersion = "dev"
		buildCommit = "none"
		buildDate = "unknown"
	})

	SetBuildInfo("v9.9.9", "abc1234", "2026-03-03")

	root := newRootCmd()
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--version が失敗: %v", err)
	}

	if got := strings.TrimSpace(output.String()); got != "awsp v9.9.9" {
		t.Fatalf("--version 出力が想定外: %s", got)
	}
}

func TestVersionSubcommand(t *testing.T) {
	t.Cleanup(func() {
		buildVersion = "dev"
		buildCommit = "none"
		buildDate = "unknown"
	})

	SetBuildInfo("v1.2.3", "deadbeef", "2026-03-03")

	root := newRootCmd()
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("version サブコマンドが失敗: %v", err)
	}

	got := strings.TrimSpace(output.String())
	for _, expected := range []string{
		"version=v1.2.3",
		"commit=deadbeef",
		"date=2026-03-03",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version 出力に期待値がない: %s", got)
		}
	}
}
