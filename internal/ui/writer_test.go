package ui

import (
	"bytes"
	"strings"
	"testing"
)

// styledSample は装飾を含む出力の代表として使う
func styledSample() string {
	return RenderCard("Sample", []string{
		SuccessLine("ok"),
		InfoLine("info"),
		WarnLine("warn"),
		ErrorLine("error"),
		Heading("heading"),
		Muted("muted"),
		Badge(BadgeOK, "ok"),
		NewTable("a", "b").AddRow("1", "2").Render(),
	}, 0)
}

func TestNewWriter_NonTTYStripsANSI(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	writer := NewWriter(&buf)

	if _, err := writer.Write([]byte(styledSample())); err != nil {
		t.Fatalf("Write が失敗: %v", err)
	}

	if strings.ContainsRune(buf.String(), '\x1b') {
		t.Fatalf("非 TTY 出力に ANSI エスケープが残っている: %q", buf.String())
	}
}

func TestNewWriter_NoColorEnvStripsANSI(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	writer := NewWriter(&buf)

	if _, err := writer.Write([]byte(styledSample())); err != nil {
		t.Fatalf("Write が失敗: %v", err)
	}

	if strings.ContainsRune(buf.String(), '\x1b') {
		t.Fatalf("NO_COLOR 指定時に ANSI エスケープが残っている: %q", buf.String())
	}
}
