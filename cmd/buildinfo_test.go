package cmd

import (
	"strings"
	"testing"
)

func TestSetBuildInfo(t *testing.T) {
	t.Cleanup(func() {
		buildVersion = "dev"
		buildCommit = "none"
		buildDate = "unknown"
	})

	SetBuildInfo("v0.9.0", "abc1234", "2026-03-03T00:00:00Z")

	if got := versionLine(); got != "awsp v0.9.0" {
		t.Fatalf("versionLine が想定外: %s", got)
	}

	detail := versionDetail()
	for _, word := range []string{"version=v0.9.0", "commit=abc1234", "date=2026-03-03T00:00:00Z"} {
		if !strings.Contains(detail, word) {
			t.Fatalf("versionDetail に期待値がない: %s", detail)
		}
	}
}

func TestSetBuildInfo_EmptyValueIsIgnored(t *testing.T) {
	t.Cleanup(func() {
		buildVersion = "dev"
		buildCommit = "none"
		buildDate = "unknown"
	})

	SetBuildInfo("v1.0.0", "deadbeef", "2026-03-03")
	SetBuildInfo("", "", "")

	if got := versionLine(); got != "awsp v1.0.0" {
		t.Fatalf("version が上書きされている: %s", got)
	}
}
