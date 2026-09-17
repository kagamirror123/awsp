package awsp

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// StatusReport は `awsp status --json` の出力
// 判定単位は sso-session(D14) 1 つでも error なら Overall は error
type StatusReport struct {
	// SchemaVersion は出力形式のバージョン
	SchemaVersion int `json:"schemaVersion"`
	// GeneratedAt はこのレポートを生成した時刻
	GeneratedAt time.Time `json:"generatedAt"`
	// ConfigFile は判定に使った AWS config のパス(D5)
	ConfigFile string `json:"configFile"`
	// Overall は全セッションのうち最も悪い状態
	// 優先順位は error > unknown > warning > ok(悪い方を採る)
	// sso-session が 1 つも無い場合は ok
	Overall ssocache.EvaluationState `json:"overall"`
	// Sessions は sso-session ごとの状態
	Sessions []SessionStatus `json:"sessions"`
}

// SessionStatus は 1 つの sso-session の認証状態
type SessionStatus struct {
	// Diagnostic はキャッシュを読み取れなかった理由
	Diagnostic string `json:"diagnostic,omitempty"`
	// Name は sso-session の名前 legacy 形式では空文字
	Name string `json:"name"`
	// StartURL は sso_start_url
	StartURL string `json:"startUrl"`
	// Region は sso_region
	Region string `json:"region,omitempty"`
	// State はこのセッションの状態
	State ssocache.EvaluationState `json:"state"`
	// ExpiresAt はトークンの有効期限 未ログイン(unknown)時は nil
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	// RemainingSeconds は ExpiresAt までの残り秒数
	// 期限切れ後は負値 未ログイン(unknown)時は nil
	RemainingSeconds *int64 `json:"remainingSeconds,omitempty"`
	// Summary は人が読むための状態要約 次に打つコマンドを含む
	Summary string `json:"summary"`
	// Profiles はこの sso-session を使う profile 名一覧
	Profiles []string `json:"profiles"`
}

// StatusOptions は BuildStatusReport の入力
type StatusOptions struct {
	// ConfigFile は出力にそのまま載せる AWS config のパス
	ConfigFile string
	// CacheDir は sso/cache のディレクトリ 未指定時は ssocache.DefaultCacheDir()
	CacheDir string
	// Grace は失効後に自動更新を見込む猶予時間(D18 既定 8h)
	Grace time.Duration
	// Now は判定時刻 未指定時は time.Now()
	Now time.Time
}

// BuildStatusReport は AWS config と sso/cache から現在の認証状態を判定する
func BuildStatusReport(profiles []awsconfig.Profile, sessions []awsconfig.SSOSession, opts StatusOptions) (StatusReport, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	cacheDir := opts.CacheDir
	if cacheDir == "" {
		dir, err := ssocache.DefaultCacheDir()
		if err != nil {
			return StatusReport{}, err
		}
		cacheDir = dir
	}

	groups := groupSessions(profiles, sessions)

	statuses := make([]SessionStatus, 0, len(groups))
	for _, group := range groups {
		statuses = append(statuses, buildSessionStatus(cacheDir, group, now, opts.Grace))
	}

	overall := ssocache.StateOK
	if len(statuses) > 0 {
		overall = overallState(statuses)
	}

	return StatusReport{
		SchemaVersion: 1,
		GeneratedAt:   now,
		ConfigFile:    opts.ConfigFile,
		Overall:       overall,
		Sessions:      statuses,
	}, nil
}

// PreflightLine は StatusReport から 1 行・装飾なしの preflight 出力と exit code を作る
// exit code は ok/warning が 0 error/unknown が 1
func PreflightLine(report StatusReport) (string, int) {
	exitCode := 0
	if report.Overall == ssocache.StateError || report.Overall == ssocache.StateUnknown {
		exitCode = 1
	}

	if len(report.Sessions) == 0 {
		return "awsp preflight: AWS SSO 設定なし", exitCode
	}

	group := sessionsWithState(report.Sessions, report.Overall)
	names := joinSessionNames(group)
	hint := loginHint(group)

	switch report.Overall {
	case ssocache.StateOK:
		remaining := safeDuration(group[0].RemainingSeconds)
		return fmt.Sprintf("awsp preflight: AWS SSO 有効(%s 残り %s)", names, formatDuration(remaining)), exitCode

	case ssocache.StateWarning:
		elapsed := -safeDuration(group[0].RemainingSeconds)
		return fmt.Sprintf(
			"awsp preflight: AWS SSO 失効(%s %s 前)。使用時に自動更新を試みます。認証エラーなら '%s' を実行してください",
			names, formatDuration(elapsed), hint,
		), exitCode

	case ssocache.StateError:
		for _, session := range group {
			if session.Diagnostic != "" {
				return fmt.Sprintf("awsp preflight: AWS SSO キャッシュエラー(%s)。'%s' で再ログインしてください。詳細は 'awsp status'", names, hint), exitCode
			}
		}
		elapsed := -safeDuration(group[0].RemainingSeconds)
		return fmt.Sprintf(
			"awsp preflight: AWS SSO 失効(%s %s 前)。AWS を使う前に '%s' を実行してください",
			names, formatDuration(elapsed), hint,
		), exitCode

	default: // unknown
		return fmt.Sprintf(
			"awsp preflight: AWS SSO 未ログイン(%s)。AWS を使う前に '%s' を実行してください",
			names, hint,
		), exitCode
	}
}

// sessionGroup は 1 つの sso-session と それを使う profile 名一覧
type sessionGroup struct {
	session  awsconfig.SSOSession
	profiles []string
}

// groupSessions は profile と sso-session 定義から sso-session 単位のグループを作る
// 名前付き sso-session は profile が無くても対象にする(config に定義があれば判定材料にする)
func groupSessions(profiles []awsconfig.Profile, sessions []awsconfig.SSOSession) []sessionGroup {
	acc := make(map[string]*sessionGroup)

	for _, session := range sessions {
		acc[session.CacheKey()] = &sessionGroup{session: session}
	}

	for _, profile := range profiles {
		session, ok := awsconfig.ResolveSession(profile, sessions)
		if !ok {
			continue
		}
		key := session.CacheKey()
		entry, exists := acc[key]
		if !exists {
			entry = &sessionGroup{session: session}
			acc[key] = entry
		}
		entry.profiles = append(entry.profiles, profile.Name)
	}

	result := make([]sessionGroup, 0, len(acc))
	for _, entry := range acc {
		sort.Strings(entry.profiles)
		result = append(result, *entry)
	}
	sort.Slice(result, func(i int, j int) bool {
		return result[i].session.CacheKey() < result[j].session.CacheKey()
	})

	return result
}

// distinctSessions は config から一意な sso-session 一覧を返す(login の対象解決用)
func distinctSessions(profiles []awsconfig.Profile, sessions []awsconfig.SSOSession) []awsconfig.SSOSession {
	groups := groupSessions(profiles, sessions)
	result := make([]awsconfig.SSOSession, 0, len(groups))
	for _, g := range groups {
		result = append(result, g.session)
	}
	return result
}

func buildSessionStatus(cacheDir string, group sessionGroup, now time.Time, grace time.Duration) SessionStatus {
	tokenPath := ssocache.TokenPath(cacheDir, group.session.CacheKey())
	meta, err := ssocache.ReadTokenMeta(tokenPath)
	eval := evaluateSession(group.session, meta, now, grace)

	profiles := group.profiles
	if profiles == nil {
		profiles = []string{}
	}

	status := SessionStatus{
		Name:     group.session.Name,
		StartURL: group.session.StartURL,
		Region:   group.session.Region,
		State:    eval.State,
		Summary:  summarizeSession(group.session, eval),
		Profiles: profiles,
	}

	if err != nil {
		status.State = ssocache.StateError
		status.Diagnostic = err.Error()
		status.Summary = fmt.Sprintf("キャッシュを読めません。'%s' で再ログインしてください", loginHintForSession(group.session))
	} else if eval.State != ssocache.StateUnknown {
		expiresAt := eval.ExpiresAt
		status.ExpiresAt = &expiresAt
		remainingSeconds := int64(eval.Remaining.Seconds())
		status.RemainingSeconds = &remainingSeconds
	}

	return status
}

func summarizeSession(session awsconfig.SSOSession, eval ssocache.Evaluation) string {
	hint := loginHintForSession(session)

	switch eval.State {
	case ssocache.StateOK:
		return fmt.Sprintf("有効(残り %s)", formatDuration(eval.Remaining))
	case ssocache.StateWarning:
		return fmt.Sprintf("失効(%s 前)。使用時に自動更新を試みます。認証エラーなら '%s' を実行してください", formatDuration(-eval.Remaining), hint)
	case ssocache.StateError:
		return fmt.Sprintf("失効(%s 前)。'%s' を実行してください", formatDuration(-eval.Remaining), hint)
	default: // unknown
		return fmt.Sprintf("未ログイン。'%s' を実行してください", hint)
	}
}

func overallState(sessions []SessionStatus) ssocache.EvaluationState {
	worst := sessions[0].State
	for _, s := range sessions[1:] {
		if stateRank(s.State) > stateRank(worst) {
			worst = s.State
		}
	}
	return worst
}

// stateRank は状態の悪さを表す 大きいほど悪い(error > unknown > warning > ok)
func stateRank(s ssocache.EvaluationState) int {
	switch s {
	case ssocache.StateError:
		return 3
	case ssocache.StateUnknown:
		return 2
	case ssocache.StateWarning:
		return 1
	default: // ok
		return 0
	}
}

func sessionsWithState(sessions []SessionStatus, state ssocache.EvaluationState) []SessionStatus {
	result := make([]SessionStatus, 0, len(sessions))
	for _, s := range sessions {
		if s.State == state {
			result = append(result, s)
		}
	}
	return result
}

func joinSessionNames(sessions []SessionStatus) string {
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, displayNameForStatus(s))
	}
	return strings.Join(names, "、")
}

func loginHint(sessions []SessionStatus) string {
	if len(sessions) == 1 && sessions[0].Name != "" {
		return "awsp login --sso-session " + sessions[0].Name
	}
	return "awsp login"
}

func loginHintForSession(session awsconfig.SSOSession) string {
	if session.Name != "" {
		return "awsp login --sso-session " + session.Name
	}
	return "awsp login"
}

func displayNameForStatus(s SessionStatus) string {
	if s.Name != "" {
		return s.Name
	}
	if s.StartURL != "" {
		return s.StartURL
	}
	return "(unnamed)"
}

func displayNameForSession(session awsconfig.SSOSession) string {
	if session.Name != "" {
		return session.Name
	}
	if session.StartURL != "" {
		return session.StartURL
	}
	return "(unnamed)"
}

func safeDuration(seconds *int64) time.Duration {
	if seconds == nil {
		return 0
	}
	return time.Duration(*seconds) * time.Second
}

// FormatRemaining は残り時間を簡潔な表記にする(例: 52m, 11h, 3d)
// 期限切れは負の値をそのまま渡すと "-11h" のように表現する(D9)
func FormatRemaining(d time.Duration) string {
	if d < 0 {
		return "-" + formatDuration(d)
	}
	return formatDuration(d)
}

// formatDuration は残り時間/経過時間を簡潔な日本語向け表記にする(例: 52m, 11h, 3d)
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// evaluateSession は自動更新に対応しない legacy 設定を期限切れとして扱う。
func evaluateSession(session awsconfig.SSOSession, meta ssocache.TokenMeta, now time.Time, grace time.Duration) ssocache.Evaluation {
	if session.IsLegacy() {
		meta.HasRefreshToken = false
	}
	return ssocache.Evaluate(meta, now, grace)
}
