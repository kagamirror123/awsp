package prompt

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

func realisticModel(t *testing.T, width, height int) tea.Model {
	t.Helper()
	expires := goldenNow.Add(time.Hour)
	profiles := []awsp.Profile{{Name: "production-readonly-ap-northeast-1", Region: "ap-northeast-1", SSOSession: "corporate", SSOAccountID: "123456789012", SSORoleName: "ReadOnlyAccess", SessionState: ssocache.StateOK, SessionExpiresAt: &expires, LastUsedAt: &goldenNow, CredentialExpiresAt: &expires, RoleARN: "arn:aws:iam::123456789012:role/production-readonly", Diagnostics: []string{strings.Repeat("キャッシュ診断 ", 15)}}}
	model := newSelectModel(buildItems(profiles, goldenNow))
	model.location = time.UTC
	model.setCurrentProfile(profiles[0].Name)
	return apply(t, model, tea.WindowSizeMsg{Width: width, Height: height})
}

func TestLayoutFitsTerminalAndDetailsCanScroll(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {80, 24}, {120, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			model := realisticModel(t, size[0], size[1])
			content := model.View().Content
			if lipgloss.Width(content) > size[0] || lipgloss.Height(content) > size[1] {
				t.Fatalf("サイズ超過 %dx%d:\n%s", lipgloss.Width(content), lipgloss.Height(content), ansi.Strip(content))
			}
			if !strings.Contains(ansi.Strip(content), "Enter 決定") || !strings.Contains(ansi.Strip(content), "● 🟢") {
				t.Fatalf("操作案内または現在値がありません:\n%s", ansi.Strip(content))
			}
			model = apply(t, model, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown}, tea.KeyPressMsg{Code: tea.KeyPgDown})
			if !strings.Contains(ansi.Strip(model.View().Content), "アクセス権") {
				t.Fatalf("詳細末尾を読めません:\n%s", ansi.Strip(model.View().Content))
			}
		})
	}
}

func TestSearchAcceptsQAndG(t *testing.T) {
	for _, query := range []string{"qa", "gcp", "GCP"} {
		t.Run(query, func(t *testing.T) {
			var model tea.Model = newSelectModel(buildItems([]awsp.Profile{{Name: query}, {Name: "prod"}}, goldenNow))
			if query == "qa" {
				model = apply(t, model, tea.KeyPressMsg{Text: "/", Code: '/'})
			}
			for _, r := range query {
				model = apply(t, model, tea.KeyPressMsg{Text: string(r), Code: r})
			}
			got := model.(selectModel)
			if got.aborted || got.list.FilterValue() != query || selectedName(got.list.SelectedItem()) != query {
				t.Fatalf("検索に失敗: filter=%q aborted=%v selected=%s", got.list.FilterValue(), got.aborted, selectedName(got.list.SelectedItem()))
			}
		})
	}
}
