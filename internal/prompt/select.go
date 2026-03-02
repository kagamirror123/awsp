// Package prompt は対話 UI の選択処理を提供する
package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// UnsetOption は環境変数解除を表す擬似プロファイル
const UnsetOption = "(unset)"

// Selector はインタラクティブなプロファイル選択を提供する
// 左に一覧 右に詳細を表示する
type Selector struct {
	input  io.Reader
	output io.Writer
}

// NewSelector は Selector を作る
func NewSelector() *Selector {
	return NewSelectorWithIO(os.Stdin, os.Stdout)
}

// NewSelectorWithIO は入出力を指定して Selector を作る
func NewSelectorWithIO(input io.Reader, output io.Writer) *Selector {
	selectorColorOnce.Do(configureSelectorColorProfile)

	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	return &Selector{
		input:  input,
		output: output,
	}
}

func configureSelectorColorProfile() {
	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
		return
	}

	// 端末差で色が消えるケースを避けるため
	// 対話画面は ANSI256 以上を明示する
	if strings.TrimSpace(os.Getenv("COLORTERM")) != "" {
		lipgloss.SetColorProfile(termenv.TrueColor)
		return
	}
	lipgloss.SetColorProfile(termenv.ANSI256)
}

// Select は候補を表示して 1 つ選択する
func (s *Selector) Select(ctx context.Context, profiles []awsp.Profile) (string, error) {
	items := make([]list.Item, 0, len(profiles)+1)
	items = append(items, profileItem{profile: awsp.Profile{Name: UnsetOption}})
	for _, profile := range profiles {
		items = append(items, profileItem{profile: profile})
	}

	model := newSelectModel(items)
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(s.input),
		tea.WithOutput(s.output),
	)
	result, err := program.Run()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "", context.Canceled
		}
		return "", err
	}

	finalModel, ok := result.(selectModel)
	if !ok {
		return "", errors.New("選択画面の終了状態を取得できません")
	}
	if finalModel.aborted {
		return "", context.Canceled
	}
	if finalModel.selected == "" {
		return "", errors.New("プロファイルが選択されませんでした")
	}

	return finalModel.selected, nil
}

type profileItem struct {
	profile awsp.Profile
}

func (i profileItem) FilterValue() string {
	return strings.Join([]string{
		i.profile.Name,
		i.profile.Region,
		i.profile.SSOAccountID,
		i.profile.SSORoleName,
		i.profile.SSOSession,
		i.profile.SourceProfile,
		i.profile.RoleARN,
	}, " ")
}

func (i profileItem) Title() string {
	return fmt.Sprintf("%s %s", profileIcon(i.profile), i.profile.Name)
}

func (i profileItem) Description() string {
	return ""
}

type listPaneSize struct {
	leftWidth  int
	rightWidth int
	bodyHeight int
}

type selectModel struct {
	list     list.Model
	selected string
	aborted  bool
	width    int
	height   int
}

func newSelectModel(items []list.Item) selectModel {
	listModel := list.New(items, newProfileDelegate(), 0, 0)
	listModel.Title = "🧭 Profiles"
	listModel.SetShowTitle(true)
	listModel.SetShowStatusBar(false)
	listModel.SetShowPagination(false)
	listModel.SetShowHelp(false)
	listModel.SetFilteringEnabled(true)
	listModel.SetShowFilter(true)
	listModel.FilterInput.Prompt = "🔎 "
	listModel.FilterInput.Placeholder = "profile / region / account / role"
	listModel.FilterInput.CharLimit = 128
	listModel.FilterInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	listModel.FilterInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230"))
	listModel.FilterInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	listModel.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	listModel.Styles.TitleBar = lipgloss.NewStyle().Padding(0, 1)
	listModel.Styles.NoItems = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Padding(1, 1)
	listModel.KeyMap.CursorUp.SetKeys("up")
	listModel.KeyMap.CursorUp.SetHelp("↑", "up")
	listModel.KeyMap.CursorDown.SetKeys("down")
	listModel.KeyMap.CursorDown.SetHelp("↓", "down")

	model := selectModel{
		list:   listModel,
		width:  120,
		height: 30,
	}
	model.applyLayout(120, 30)
	return model
}

type profileDelegate struct{}

func newProfileDelegate() profileDelegate {
	return profileDelegate{}
}

func (d profileDelegate) Height() int {
	return 1
}

func (d profileDelegate) Spacing() int {
	return 0
}

func (d profileDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d profileDelegate) Render(writer io.Writer, model list.Model, index int, item list.Item) {
	profile, ok := item.(profileItem)
	if !ok {
		return
	}

	isSelected := index == model.Index()
	marker := "  "
	lineStyle := listItemNormalStyle
	if isSelected {
		marker = "▶ "
		lineStyle = listItemSelectedStyle
	}

	line := marker + profile.Title()
	_, _ = fmt.Fprint(writer, lineStyle.Render(line))
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.applyLayout(typed.Width, typed.Height)
		return m, nil

	case tea.KeyMsg:
		if cmd, handled := m.handleKeyInput(typed); handled {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *selectModel) handleKeyInput(msg tea.KeyMsg) (tea.Cmd, bool) {
	if isQuitKey(msg) {
		m.aborted = true
		return tea.Quit, true
	}

	// フィルタ中に移動キーが押されたら入力モードを抜けて移動へ渡す
	// これで「絞り込み後に矢印が効かない」状態を避ける
	if m.list.SettingFilter() && isNavigationKey(msg) {
		m.list.SetFilterState(list.FilterApplied)
		switch msg.String() {
		case "up":
			m.list.CursorUp()
		case "down":
			m.list.CursorDown()
		}
		return nil, true
	}

	// 文字入力を検知したら即フィルタ入力へ入る
	// `/` を押さなくても絞り込みできるようにする
	if shouldStartFiltering(msg, m.list.SettingFilter()) {
		return m.startFiltering(msg), true
	}

	if msg.String() == "enter" && !m.list.SettingFilter() {
		selected, ok := currentProfile(m.list.SelectedItem())
		if !ok {
			return nil, true
		}
		m.selected = selected.Name
		return tea.Quit, true
	}

	return nil, false
}

func (m *selectModel) startFiltering(msg tea.KeyMsg) tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.list, cmd = m.list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	m.list, cmd = m.list.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

func (m selectModel) View() string {
	size := resolvePaneSize(m.width, m.height)
	help := helpStyle.Render("↑↓: move  Enter: select  type or /: filter  Esc: clear  q: quit")
	left := panelStyle.
		Width(size.leftWidth).
		Height(size.bodyHeight).
		Render(m.list.View())
	right := panelStyle.
		Width(size.rightWidth).
		Height(size.bodyHeight).
		Render(m.renderDetail())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return strings.Join([]string{
		titleStyle.Render("☁️  awsp profile selector"),
		help,
		body,
	}, "\n")
}

func (m selectModel) renderDetail() string {
	profile, ok := currentProfile(m.list.SelectedItem())
	if !ok {
		return detailMutedStyle.Render("🔍 条件に一致するプロファイルがありません")
	}

	if profile.Name == UnsetOption {
		body := []string{
			detailTitleStyle.Render("🧹 Unset mode"),
			detailMutedStyle.Render("現在の環境変数を解除"),
			"",
			"unset AWS_PROFILE",
			"unset AWS_ACCESS_KEY_ID",
			"unset AWS_SECRET_ACCESS_KEY",
			"unset AWS_SESSION_TOKEN",
		}
		return strings.Join(body, "\n")
	}

	body := []string{
		detailTitleStyle.Render("📋 Profile detail"),
		renderDetailLine("🔐", "name", profile.Name),
		renderDetailLine("🌏", "region", fallbackValue(profile.Region)),
		renderDetailLine("🧾", "output", fallbackValue(profile.Output)),
		renderDetailLine("🧩", "sso session", fallbackValue(profile.SSOSession)),
		renderDetailLine("🏢", "account id", fallbackValue(profile.SSOAccountID)),
		renderDetailLine("🛡", "role name", fallbackValue(profile.SSORoleName)),
		renderDetailLine("🎭", "role arn", fallbackValue(profile.RoleARN)),
		renderDetailLine("🔗", "source", fallbackValue(profile.SourceProfile)),
		"",
		detailMutedStyle.Render("接続時は caller identity を取得して表示"),
	}

	return strings.Join(body, "\n")
}

func fallbackValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func (m *selectModel) applyLayout(width int, height int) {
	size := resolvePaneSize(width, height)
	m.width = width
	m.height = height
	m.list.SetSize(maxInt(14, size.leftWidth-4), maxInt(8, size.bodyHeight-2))
}

func resolvePaneSize(width int, height int) listPaneSize {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 30
	}

	bodyHeight := maxInt(12, height-3)
	leftWidth := maxInt(52, width/2)
	rightWidth := width - leftWidth - 1

	const minRightWidth = 46
	if rightWidth < minRightWidth {
		rightWidth = minRightWidth
		leftWidth = width - rightWidth - 1
	}
	if leftWidth < 42 {
		leftWidth = 42
		rightWidth = maxInt(30, width-leftWidth-1)
	}

	return listPaneSize{
		leftWidth:  leftWidth,
		rightWidth: rightWidth,
		bodyHeight: bodyHeight,
	}
}

func currentProfile(item list.Item) (awsp.Profile, bool) {
	profile, ok := item.(profileItem)
	if !ok {
		return awsp.Profile{}, false
	}
	return profile.profile, true
}

func renderDetailLine(icon string, key string, value string) string {
	paddedIcon := padDisplayRight(icon, 2)
	paddedKey := padDisplayRight(key, 11)
	return fmt.Sprintf("%s %s %s", paddedIcon, detailKeyStyle.Render(paddedKey), value)
}

func padDisplayRight(value string, width int) string {
	displayWidth := runewidth.StringWidth(value)
	if displayWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-displayWidth)
}

func shouldStartFiltering(msg tea.KeyMsg, filtering bool) bool {
	if filtering {
		return false
	}

	if msg.Alt {
		return false
	}

	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return false
	}

	if len(msg.Runes) == 1 {
		switch msg.String() {
		case "g", "G", "q", "/":
			return false
		}
	}

	for _, r := range msg.Runes {
		if !unicode.IsPrint(r) {
			return false
		}
	}

	return true
}

func isNavigationKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "down":
		return true
	default:
		return false
	}
}

func isQuitKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+c", "q":
		return true
	default:
		return false
	}
}

func profileIcon(profile awsp.Profile) string {
	switch {
	case profile.Name == UnsetOption:
		return "🧹"
	case profile.SSOAccountID != "" || profile.SSOSession != "":
		return "🔐"
	default:
		return "🪪"
	}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	titleStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	helpStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	panelStyle          = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	listItemNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)
	listItemSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("16")).
				Background(lipgloss.Color("220")).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("214")).
				Padding(0, 1)
	detailTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	detailKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("109")).Bold(true)
	detailMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	selectorColorOnce sync.Once
)
