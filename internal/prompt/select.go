// Package prompt は対話 UI の選択処理を提供する
package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/kagamirror123/awsp/internal/ssocache"
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

// Select は候補を表示して 1 つ選択する
func (s *Selector) Select(ctx context.Context, profiles []awsp.Profile) (string, error) {
	model := newSelectModel(buildItems(profiles, time.Now()))
	model.setCurrentProfile(os.Getenv("AWS_PROFILE"))
	program := tea.NewProgram(
		model,
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

func buildItems(profiles []awsp.Profile, now time.Time) []list.Item {
	items := make([]list.Item, 0, len(profiles)+1)
	items = append(items, profileItem{profile: awsp.Profile{Name: UnsetOption}, now: now})
	for _, profile := range profiles {
		items = append(items, profileItem{profile: profile, now: now})
	}
	return items
}

type profileItem struct {
	profile awsp.Profile
	now     time.Time
	current bool
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
	current := ""
	if i.current {
		current = "● "
	}
	return fmt.Sprintf("%s%s %s  %s", current, stateMarker(i.profile), i.profile.Name, remainingLabel(i.profile, i.now))
}

func (i profileItem) Description() string {
	return ""
}

type listPaneSize struct {
	leftWidth   int
	rightWidth  int
	leftHeight  int
	rightHeight int
	stacked     bool
}

type selectModel struct {
	list         list.Model
	selected     string
	aborted      bool
	width        int
	height       int
	detailOffset int
	location     *time.Location
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

	filterStyles := listModel.FilterInput.Styles()
	filterStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	filterStyles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("230"))
	filterStyles.Blurred.Prompt = filterStyles.Focused.Prompt
	filterStyles.Blurred.Text = filterStyles.Focused.Text
	filterStyles.Cursor.Blink = false
	filterStyles.Cursor.Color = lipgloss.Color("205")
	listModel.FilterInput.SetStyles(filterStyles)

	listModel.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	listModel.Styles.TitleBar = lipgloss.NewStyle().Padding(0, 1)
	listModel.Styles.NoItems = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Padding(1, 1)
	listModel.KeyMap.CursorUp.SetKeys("up")
	listModel.KeyMap.CursorUp.SetHelp("↑", "up")
	listModel.KeyMap.CursorDown.SetKeys("down")
	listModel.KeyMap.CursorDown.SetHelp("↓", "down")

	model := selectModel{
		list:     listModel,
		location: time.Local,
		width:    120,
		height:   30,
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
	_, _ = fmt.Fprint(writer, lineStyle.Render(ansi.Truncate(line, max(1, model.Width()-lineStyle.GetHorizontalFrameSize()), "…")))
}

func (m selectModel) Init() tea.Cmd {
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.applyLayout(typed.Width, typed.Height)
		return m, nil

	case tea.KeyPressMsg:
		before := selectedName(m.list.SelectedItem())
		if cmd, handled := m.handleKeyInput(typed); handled {
			if before != selectedName(m.list.SelectedItem()) {
				m.detailOffset = 0
			}
			return m, cmd
		}
	}

	before := selectedName(m.list.SelectedItem())
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if before != selectedName(m.list.SelectedItem()) {
		m.detailOffset = 0
	}
	return m, cmd
}

func (m *selectModel) handleKeyInput(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if isQuitKey(msg) && (msg.String() == "ctrl+c" || !m.list.SettingFilter()) {
		m.aborted = true
		return tea.Quit, true
	}

	if msg.String() == "pgdown" || msg.String() == "pgup" {
		size := resolvePaneSize(m.width, m.height)
		lines := m.detailLines(size.rightWidth - 4)
		page := max(1, size.rightHeight-3)
		step := page
		if msg.String() == "pgup" {
			step = -step
		}
		m.detailOffset = min(max(0, m.detailOffset+step), max(0, len(lines)-page))
		return nil, true
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

func (m *selectModel) startFiltering(msg tea.KeyPressMsg) tea.Cmd {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.list, cmd = m.list.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	m.list, cmd = m.list.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

func (m selectModel) View() tea.View {
	size := resolvePaneSize(m.width, m.height)
	left := renderPanel(m.list.View(), size.leftWidth, size.leftHeight)
	lines := m.detailLines(size.rightWidth - 4)
	page := max(1, size.rightHeight-3)
	offset := min(m.detailOffset, max(0, len(lines)-page))
	detail := strings.Join(lines[offset:min(len(lines), offset+page)], "\n")
	detail = lipgloss.NewStyle().Height(page).Render(detail)
	if len(lines) > page {
		detail += "\n" + detailMutedStyle.Render(fmt.Sprintf("PgUp/PgDn  %d–%d / %d", offset+1, min(len(lines), offset+page), len(lines)))
	}
	right := renderPanel(detail, size.rightWidth, size.rightHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if size.stacked {
		body = left + "\n" + right
	}
	content := strings.Join([]string{
		titleStyle.Render("☁️  awsp  ● 現在の profile"),
		helpStyle.Render("↑↓ 移動  Enter 決定  / 検索  q 終了"),
		body,
	}, "\n")
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

// renderPanel は枠を含めたサイズを指定する。長い値は詳細側で折り返す。
func renderPanel(content string, width, height int) string {
	return panelStyle.Width(width).Height(height).
		MaxWidth(width).MaxHeight(height).Render(content)
}

func (m selectModel) detailLines(width int) []string {
	return strings.Split(ansi.Hardwrap(m.renderDetail(), max(1, width), false), "\n")
}

// setCurrentProfile は環境から渡された現在値を表示し、初期カーソルを合わせる。
func (m *selectModel) setCurrentProfile(name string) {
	if name == "" {
		name = UnsetOption
	}
	items := m.list.Items()
	selected := 0
	for i, item := range items {
		p := item.(profileItem)
		p.current = p.profile.Name == name
		items[i] = p
		if p.current {
			selected = i
		}
	}
	m.list.SetItems(items)
	m.list.Select(selected)
}

func selectedName(item list.Item) string {
	p, _ := currentProfile(item)
	return p.Name
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
		renderDetailLine("🧩", "session", fallbackValue(profile.SSOSession)),
		renderDetailLine("🏢", "account", fallbackValue(profile.SSOAccountID)),
		renderDetailLine("🛡", "role", fallbackValue(profile.SSORoleName)),
		renderDetailLine("🎭", "role arn", fallbackValue(profile.RoleARN)),
		renderDetailLine("🔗", "source", fallbackValue(profile.SourceProfile)),
		"",
		renderDetailLine(stateMarker(profile), "state", sessionStateLabel(profile)),
		renderDetailLine("⏳", "SSO expires", fallbackTime(profile.SessionExpiresAt, m.location)),
		renderDetailLine("🕘", "fetched", fallbackTime(profile.LastUsedAt, m.location)),
		renderDetailLine("🔑", "cred expires", fallbackTime(profile.CredentialExpiresAt, m.location)),
		"",
		detailMutedStyle.Render("接続時は caller identity を取得して表示"),
	}

	if len(profile.Diagnostics) > 0 {
		body = append(body, "", "⚠ キャッシュの読み取りエラー")
		body = append(body, profile.Diagnostics...)
		body = append(body, "キャッシュの内容・アクセス権を確認してください")
	}
	return strings.Join(body, "\n")
}

func fallbackValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// fallbackTime は時刻をローカル時刻の短い形式で表示する 未算出/該当なしは "-"(D9)
func fallbackTime(t *time.Time, location *time.Location) string {
	if t == nil {
		return "-"
	}
	return t.In(location).Format("01/02 15:04 MST")
}

// sessionStateLabel は sso-session の状態を文字列にする SSO を使わない profile は "-"(D9)
func sessionStateLabel(p awsp.Profile) string {
	if !p.IsSSO() {
		return "-"
	}
	if p.SessionState == "" {
		return string(ssocache.StateUnknown)
	}
	return string(p.SessionState)
}

// stateMarker は D9 の一覧行/詳細で使う状態マークを返す
// SSO セッションの状態は ok 🟢 / warning 🟡 / error 🔴 / unknown ⚪ SSO を使わない profile は 🪪
func stateMarker(p awsp.Profile) string {
	switch {
	case p.Name == UnsetOption:
		return "🧹"
	case !p.IsSSO():
		return "🪪"
	default:
		switch p.SessionState {
		case ssocache.StateOK:
			return "🟢"
		case ssocache.StateWarning:
			return "🟡"
		case ssocache.StateError:
			return "🔴"
		default:
			return "⚪"
		}
	}
}

// remainingLabel は一覧行に載せる残り時間を返す(D9)
// awsp.SessionStatus と同じ書式(52m / 11h / 3d) 期限切れは "-11h" のように負で表す
func remainingLabel(p awsp.Profile, now time.Time) string {
	if p.SessionExpiresAt == nil {
		return "-"
	}
	return awsp.FormatRemaining(p.SessionExpiresAt.Sub(now))
}

func (m *selectModel) applyLayout(width int, height int) {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 30
	}
	m.width, m.height = width, height
	m.detailOffset = 0
	size := resolvePaneSize(width, height)
	m.list.SetSize(max(1, size.leftWidth-4), max(1, size.leftHeight-2))
}

func resolvePaneSize(width int, height int) listPaneSize {
	if width <= 0 {
		width = 120
	}
	if height <= 0 {
		height = 30
	}
	body := max(4, height-2)
	if width < 96 {
		listHeight := min(9, max(4, body/3))
		return listPaneSize{
			leftWidth: width, rightWidth: width,
			leftHeight: listHeight, rightHeight: body - listHeight, stacked: true,
		}
	}
	left := width * 2 / 5
	return listPaneSize{
		leftWidth: left, rightWidth: width - left,
		leftHeight: body, rightHeight: body,
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
	paddedKey := padDisplayRight(key, 12)
	return fmt.Sprintf("%s %s %s", paddedIcon, detailKeyStyle.Render(paddedKey), value)
}

func padDisplayRight(value string, width int) string {
	displayWidth := lipgloss.Width(value)
	if displayWidth >= width {
		return value
	}
	return value + strings.Repeat(" ", width-displayWidth)
}

func shouldStartFiltering(msg tea.KeyPressMsg, filtering bool) bool {
	if filtering {
		return false
	}

	key := msg.Key()
	if key.Mod.Contains(tea.ModAlt) {
		return false
	}

	if key.Text == "" {
		return false
	}

	if utf8.RuneCountInString(key.Text) == 1 {
		switch key.Text {
		case "q", "/":
			return false
		}
	}

	for _, r := range key.Text {
		if !unicode.IsPrint(r) {
			return false
		}
	}

	return true
}

func isNavigationKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "up", "down":
		return true
	default:
		return false
	}
}

func isQuitKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "ctrl+c", "q":
		return true
	default:
		return false
	}
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
)
