// Package awsconfig は AWS shared config から profile と sso-session を読み取る
package awsconfig

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	defaultSection   = "default"
	profilePrefix    = "profile "
	ssoSessionPrefix = "sso-session "

	// defaultSSORegistrationScope は sso_registration_scopes 未設定時の既定値
	defaultSSORegistrationScope = "sso:account:access"
)

// Profile は ~/.aws/config から抽出した profile 情報
// list とインタラクティブ UI の表示に使う
type Profile struct {
	Name          string `json:"name"`
	Region        string `json:"region,omitempty"`
	Output        string `json:"output,omitempty"`
	SSOSession    string `json:"ssoSession,omitempty"`
	SSOStartURL   string `json:"ssoStartUrl,omitempty"`
	SSORegion     string `json:"ssoRegion,omitempty"`
	SSOAccountID  string `json:"ssoAccountId,omitempty"`
	SSORoleName   string `json:"ssoRoleName,omitempty"`
	RoleARN       string `json:"roleArn,omitempty"`
	SourceProfile string `json:"sourceProfile,omitempty"`
}

// IsSSO は SSO 関連設定があるかを返す
func (p Profile) IsSSO() bool {
	return p.SSOSession != "" || p.SSOStartURL != "" || p.SSOAccountID != "" || p.SSORoleName != ""
}

// SSOSession は `[sso-session NAME]` セクションから抽出した情報
// legacy 形式(profile に sso_start_url を直書きし sso-session を使わない形式)は
// ResolveSession が Name を空にした合成セッションとして返す
type SSOSession struct {
	// Name は sso-session の名前 legacy 形式では空文字
	Name string
	// StartURL は sso_start_url
	StartURL string
	// Region は sso_region
	Region string
	// RegistrationScopes は sso_registration_scopes をカンマ区切りで分解したもの
	// 未設定時は defaultSSORegistrationScope を 1 件持つ
	RegistrationScopes []string
}

// CacheKey は ~/.aws/sso/cache のトークンファイル名算出に使うキーを返す
// sso-session 形式ならセッション名 legacy 形式なら start URL を返す
func (s SSOSession) CacheKey() string {
	if s.Name != "" {
		return s.Name
	}
	return s.StartURL
}

// IsLegacy は sso-session を使わない旧形式(profile 直書き)かどうかを返す
func (s SSOSession) IsLegacy() bool {
	return s.Name == ""
}

// ResolveSession は profile が属する sso-session を解決する
// 戻り値の bool が false の場合 profile は SSO を使っていない(静的認証情報や role のみ等)
func ResolveSession(profile Profile, sessions []SSOSession) (SSOSession, bool) {
	if profile.SSOSession != "" {
		for _, session := range sessions {
			if session.Name == profile.SSOSession {
				return session, true
			}
		}
		// [sso-session] ブロックが見つからなくてもセッション名自体は
		// キャッシュキーとして機能するため合成して返す
		return SSOSession{
			Name:               profile.SSOSession,
			RegistrationScopes: []string{defaultSSORegistrationScope},
		}, true
	}

	if profile.SSOStartURL != "" {
		return SSOSession{
			StartURL:           profile.SSOStartURL,
			Region:             profile.SSORegion,
			RegistrationScopes: []string{defaultSSORegistrationScope},
		}, true
	}

	return SSOSession{}, false
}

// ProfileStore は ~/.aws/config からプロファイルと sso-session を読み取る
// [profile xxx] と [sso-session xxx] を対象にして [default] は除外する
type ProfileStore struct {
	configPath string
}

// NewProfileStore は config ファイルのパスを受け取る
func NewProfileStore(configPath string) *ProfileStore {
	return &ProfileStore{configPath: configPath}
}

// ConfigPath は読み取り対象の config ファイルパスを返す
func (s *ProfileStore) ConfigPath() string {
	return s.configPath
}

// Profiles は config から利用可能なプロファイル名一覧を返す
func (s *ProfileStore) Profiles(ctx context.Context) ([]string, error) {
	items, err := s.ProfileDetails(ctx)
	if err != nil {
		return nil, err
	}

	profiles := make([]string, 0, len(items))
	for _, item := range items {
		profiles = append(profiles, item.Name)
	}
	sort.Strings(profiles)

	return profiles, nil
}

// ProfileDetails は config から profile の詳細情報を返す
func (s *ProfileStore) ProfileDetails(ctx context.Context) ([]Profile, error) {
	profiles, _, err := s.parse(ctx)
	return profiles, err
}

// SSOSessions は config から [sso-session] セクション一覧を返す
func (s *ProfileStore) SSOSessions(ctx context.Context) ([]SSOSession, error) {
	_, sessions, err := s.parse(ctx)
	return sessions, err
}

func (s *ProfileStore) parse(ctx context.Context) ([]Profile, []SSOSession, error) {
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}

	file, err := os.Open(s.configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("AWS config を開けません: %s: %w", s.configPath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	profileSet := make(map[string]*Profile)
	sessionSet := make(map[string]*SSOSession)

	var currentProfile *Profile
	var currentSession *SSOSession

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		sectionType, sectionName, isSection := parseSectionHeader(line)
		if isSection {
			currentProfile, currentSession = nil, nil
			switch sectionType {
			case "profile":
				if _, exists := profileSet[sectionName]; !exists {
					profileSet[sectionName] = &Profile{Name: sectionName}
				}
				currentProfile = profileSet[sectionName]
			case "sso-session":
				if _, exists := sessionSet[sectionName]; !exists {
					sessionSet[sectionName] = &SSOSession{Name: sectionName}
				}
				currentSession = sessionSet[sectionName]
			}
			continue
		}

		key, value, ok := parseKeyValue(line)
		if !ok {
			continue
		}
		if currentProfile != nil {
			applyProfileField(currentProfile, key, value)
		}
		if currentSession != nil {
			applySessionField(currentSession, key, value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("AWS config の読み取りに失敗: %s: %w", s.configPath, err)
	}

	profiles := make([]Profile, 0, len(profileSet))
	for _, profile := range profileSet {
		profiles = append(profiles, *profile)
	}
	sort.Slice(profiles, func(i int, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	sessions := make([]SSOSession, 0, len(sessionSet))
	for _, session := range sessionSet {
		if len(session.RegistrationScopes) == 0 {
			session.RegistrationScopes = []string{defaultSSORegistrationScope}
		}
		sessions = append(sessions, *session)
	}
	sort.Slice(sessions, func(i int, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	return profiles, sessions, nil
}

func parseProfileSection(line string) (string, bool) {
	sectionType, sectionName, ok := parseSectionHeader(line)
	if !ok || sectionType != "profile" {
		return "", false
	}
	return sectionName, true
}

func parseSectionHeader(line string) (string, string, bool) {
	if line == "" {
		return "", "", false
	}
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false
	}
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", "", false
	}

	section := strings.TrimSpace(line[1 : len(line)-1])
	if section == defaultSection {
		return "other", "", true
	}
	if strings.HasPrefix(section, profilePrefix) {
		name := strings.TrimSpace(strings.TrimPrefix(section, profilePrefix))
		if name == "" {
			return "", "", false
		}
		return "profile", name, true
	}
	if strings.HasPrefix(section, ssoSessionPrefix) {
		name := strings.TrimSpace(strings.TrimPrefix(section, ssoSessionPrefix))
		if name == "" {
			return "", "", false
		}
		return "sso-session", name, true
	}

	return "other", "", true
}

func parseKeyValue(line string) (string, string, bool) {
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	key := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])
	value = strings.Trim(value, "\"'")
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func applyProfileField(profile *Profile, key string, value string) {
	switch key {
	case "region":
		profile.Region = value
	case "output":
		profile.Output = value
	case "sso_session":
		profile.SSOSession = value
	case "sso_start_url":
		profile.SSOStartURL = value
	case "sso_region":
		profile.SSORegion = value
	case "sso_account_id":
		profile.SSOAccountID = value
	case "sso_role_name":
		profile.SSORoleName = value
	case "role_arn":
		profile.RoleARN = value
	case "source_profile":
		profile.SourceProfile = value
	}
}

func applySessionField(session *SSOSession, key string, value string) {
	switch key {
	case "sso_start_url":
		session.StartURL = value
	case "sso_region":
		session.Region = value
	case "sso_registration_scopes":
		session.RegistrationScopes = splitScopes(value)
	}
}

func splitScopes(value string) []string {
	parts := strings.Split(value, ",")
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		scope := strings.TrimSpace(part)
		if scope != "" {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}
