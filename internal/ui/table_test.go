package ui

import (
	"bytes"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestTable_MaxWidthKeepsEveryLineWithinLimit(t *testing.T) {
	t.Parallel()

	const limit = 60

	table := NewTable("Session", "State", "Expires", "Remaining", "Profiles").
		MaxWidth(limit).
		DropWhenNarrow("Remaining", "Expires")

	table.AddRow(
		"corp-sso-with-a-very-long-descriptive-name-that-does-not-fit",
		"ok",
		"09-16 12:00",
		"11h",
		"16",
	)
	table.AddRow(
		"another-extremely-long-sso-session-name-for-good-measure",
		"error",
		"-",
		"-11h",
		"3",
	)

	rendered := table.Render()

	for i, line := range strings.Split(rendered, "\n") {
		if w := lipgloss.Width(line); w > limit {
			t.Fatalf("line %d の幅が上限を超えている: width=%d limit=%d line=%q", i, w, limit, line)
		}
	}
}

func TestTable_NoMaxWidthRendersNaturalWidth(t *testing.T) {
	t.Parallel()

	table := NewTable("a", "b").AddRow("1", "2")
	rendered := table.Render()

	if rendered == "" {
		t.Fatal("自然幅の描画結果が空")
	}
}

func TestTable_DropWhenNarrowDropsInOrderUntilItFits(t *testing.T) {
	t.Parallel()

	headers := []string{"Profile", "Region", "Account", "Role", "Source", "State", "Expires", "Last used"}
	longCell := strings.Repeat("x", 40)

	table := NewTable(headers...).
		DropWhenNarrow("Last used", "Source", "Region", "Expires")
	table.AddRow(longCell, longCell, longCell, longCell, longCell, longCell, longCell, longCell)

	// 十分広い上限では 1 列も落とさず全列残る
	wide := NewTable(headers...).
		DropWhenNarrow("Last used", "Source", "Region", "Expires").
		MaxWidth(1000)
	wide.AddRow(longCell, longCell, longCell, longCell, longCell, longCell, longCell, longCell)
	renderedWide := wide.Render()
	for _, h := range headers {
		if !strings.Contains(renderedWide, h) {
			t.Fatalf("MaxWidth に余裕があるのに列 %q が落ちている:\n%s", h, renderedWide)
		}
	}

	// 狭い上限では DropWhenNarrow の指定順に落ち 収まった時点で止まる
	table.MaxWidth(100)
	rendered := table.Render()

	if strings.Contains(rendered, "Last used") {
		t.Fatalf("最優先で落ちるはずの Last used 列が残っている:\n%s", rendered)
	}

	for _, line := range strings.Split(rendered, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			// 全部落としても収まらない場合は折り返しに任せてよいが
			// このケースは残り列(Profile/State)で十分収まるはず
			t.Fatalf("列を落としても幅が上限を超えている: width=%d line=%q", w, line)
		}
	}
}

func TestTable_TruncateAddsEllipsisAndCountsFullWidthChars(t *testing.T) {
	t.Parallel()

	table := NewTable("Role").Truncate("Role", 8)
	table.AddRow("AdministratorAccess")
	table.AddRow("短い")
	table.AddRow("日本語のとても長いロール名です")

	rendered := table.Render()
	lines := strings.Split(rendered, "\n")

	var dataLines []string
	for _, line := range lines {
		if strings.Contains(line, "…") {
			dataLines = append(dataLines, line)
		}
	}

	if len(dataLines) != 2 {
		t.Fatalf("「…」を含む行数が想定と違う: got=%d rendered=%s", len(dataLines), rendered)
	}

	if strings.Contains(rendered, "AdministratorAccess") {
		t.Fatalf("8 桁を超えるセルが切り詰められていない:\n%s", rendered)
	}
	if strings.Contains(rendered, "日本語のとても長いロール名です") {
		t.Fatalf("全角セルが切り詰められていない:\n%s", rendered)
	}
	if !strings.Contains(rendered, "短い") {
		t.Fatalf("8 桁に収まるセルが変化してしまっている:\n%s", rendered)
	}
}

func TestTerminalWidth_NonTerminalUsesColumnsEnv(t *testing.T) {
	t.Setenv("COLUMNS", "80")

	var buf bytes.Buffer
	if got := TerminalWidth(&buf); got != 80 {
		t.Fatalf("COLUMNS=80 のとき got=%d want=80", got)
	}
}

func TestTerminalWidth_NonTerminalWithoutColumnsDefaultsTo120(t *testing.T) {
	t.Setenv("COLUMNS", "")

	var buf bytes.Buffer
	if got := TerminalWidth(&buf); got != 120 {
		t.Fatalf("COLUMNS 未設定のとき got=%d want=120", got)
	}
}

func TestTerminalWidth_ClampsToMinimum(t *testing.T) {
	t.Setenv("COLUMNS", "10")

	var buf bytes.Buffer
	if got := TerminalWidth(&buf); got != 40 {
		t.Fatalf("COLUMNS=10 のとき下限 40 に丸められるはず got=%d", got)
	}
}

func TestTerminalWidth_WrappedNonFileWriterFallsBackToColumns(t *testing.T) {
	t.Setenv("COLUMNS", "80")

	var buf bytes.Buffer
	wrapped := NewWriter(&buf)
	if got := TerminalWidth(wrapped); got != 80 {
		t.Fatalf("NewWriter でラップした非端末 writer は COLUMNS を使うはず got=%d", got)
	}
}
