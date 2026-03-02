// Package awsconfig は AWS shared config から profile を読み取る
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
	defaultSection = "default"
	profilePrefix  = "profile "
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

// ProfileStore は ~/.aws/config からプロファイル情報を読み取る
// [profile xxx] を対象にして [default] や [sso-session xxx] は除外する
type ProfileStore struct {
	configPath string
}

// NewProfileStore は config ファイルのパスを受け取る
func NewProfileStore(configPath string) *ProfileStore {
	return &ProfileStore{configPath: configPath}
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
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	file, err := os.Open(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("AWS config を開けません: %s: %w", s.configPath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	profileSet := make(map[string]*Profile)
	currentProfile := ""

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		sectionType, sectionName, isSection := parseSectionHeader(line)
		if isSection {
			if sectionType == "profile" {
				currentProfile = sectionName
				if _, exists := profileSet[currentProfile]; !exists {
					profileSet[currentProfile] = &Profile{Name: currentProfile}
				}
			} else {
				currentProfile = ""
			}
			continue
		}

		if currentProfile == "" {
			continue
		}

		key, value, ok := parseKeyValue(line)
		if !ok {
			continue
		}
		applyProfileField(profileSet[currentProfile], key, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("AWS config の読み取りに失敗: %s: %w", s.configPath, err)
	}

	profiles := make([]Profile, 0, len(profileSet))
	for _, profile := range profileSet {
		profiles = append(profiles, *profile)
	}
	sort.Slice(profiles, func(i int, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})

	return profiles, nil
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
	if !strings.HasPrefix(section, profilePrefix) {
		return "other", "", true
	}

	name := strings.TrimSpace(strings.TrimPrefix(section, profilePrefix))
	if name == "" {
		return "", "", false
	}

	return "profile", name, true
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
