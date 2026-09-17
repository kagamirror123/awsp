package awsp

import (
	"time"

	"github.com/kagamirror123/awsp/internal/awsconfig"
	"github.com/kagamirror123/awsp/internal/ssocache"
)

// ProfileInfo は `awsp list --json` の 1 profile 分の情報
type ProfileInfo struct {
	// Diagnostics はこの profile のキャッシュを読み取れなかった理由
	Diagnostics   []string `json:"diagnostics,omitempty"`
	Name          string   `json:"name"`
	Region        string   `json:"region,omitempty"`
	Output        string   `json:"output,omitempty"`
	SSOSession    string   `json:"ssoSession,omitempty"`
	SSOStartURL   string   `json:"ssoStartUrl,omitempty"`
	SSORegion     string   `json:"ssoRegion,omitempty"`
	SSOAccountID  string   `json:"ssoAccountId,omitempty"`
	SSORoleName   string   `json:"ssoRoleName,omitempty"`
	RoleARN       string   `json:"roleArn,omitempty"`
	SourceProfile string   `json:"sourceProfile,omitempty"`
	// SessionState はこの profile が属する sso-session の状態(SSO を使わない profile では空)
	SessionState ssocache.EvaluationState `json:"sessionState,omitempty"`
	// SessionExpiresAt は sso-session のトークン有効期限 未使用/未算出なら省略(D9 の TUI/list 表示用)
	SessionExpiresAt *time.Time `json:"sessionExpiresAt,omitempty"`
	// LastUsedAt はロール認証情報キャッシュの取得・更新時刻(mtime)。JSON 名は互換性のため維持する。
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	// CredentialExpiresAt は ~/.aws/cli/cache のロール認証情報の有効期限 未使用/未算出なら省略
	CredentialExpiresAt *time.Time `json:"credentialExpiresAt,omitempty"`
}

// ProfileList は `awsp list --json` の出力
type ProfileList struct {
	// SchemaVersion は出力形式のバージョン
	SchemaVersion int `json:"schemaVersion"`
	// ConfigFile は読み取り元の AWS config パス(D5)
	ConfigFile string `json:"configFile"`
	// Profiles は profile 一覧
	Profiles []ProfileInfo `json:"profiles"`
}

// ProfileListOptions は BuildProfileList の入力
type ProfileListOptions struct {
	// ConfigFile は出力にそのまま載せる AWS config のパス
	ConfigFile string
	// SSOCacheDir は ~/.aws/sso/cache のディレクトリ 未指定時は ssocache.DefaultCacheDir()
	SSOCacheDir string
	// CLICacheDir は ~/.aws/cli/cache のディレクトリ 未指定時は算出しない(§6.2 の付加情報は空になる)
	CLICacheDir string
	// Grace は失効後に自動更新を見込む猶予時間(D18 既定 8h)
	Grace time.Duration
	// Now は判定時刻 未指定時は time.Now()
	Now time.Time
}

// BuildProfileList は profile 一覧に sso-session の認証状態と認証情報取得情報を付与する
func BuildProfileList(profiles []awsconfig.Profile, sessions []awsconfig.SSOSession, opts ProfileListOptions) (ProfileList, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	ssoCacheDir := opts.SSOCacheDir
	if ssoCacheDir == "" {
		dir, err := ssocache.DefaultCacheDir()
		if err != nil {
			return ProfileList{}, err
		}
		ssoCacheDir = dir
	}

	infos := make([]ProfileInfo, 0, len(profiles))
	for _, profile := range profiles {
		info := ProfileInfo{
			Name:          profile.Name,
			Region:        profile.Region,
			Output:        profile.Output,
			SSOSession:    profile.SSOSession,
			SSOStartURL:   profile.SSOStartURL,
			SSORegion:     profile.SSORegion,
			SSOAccountID:  profile.SSOAccountID,
			SSORoleName:   profile.SSORoleName,
			RoleARN:       profile.RoleARN,
			SourceProfile: profile.SourceProfile,
		}

		if session, ok := awsconfig.ResolveSession(profile, sessions); ok {
			tokenPath := ssocache.TokenPath(ssoCacheDir, session.CacheKey())
			meta, err := ssocache.ReadTokenMeta(tokenPath)
			eval := evaluateSession(session, meta, now, opts.Grace)
			info.SessionState = eval.State
			if err != nil {
				info.SessionState = ssocache.StateError
				info.Diagnostics = append(info.Diagnostics, err.Error())
			} else if eval.State != ssocache.StateUnknown {
				expiresAt := eval.ExpiresAt
				info.SessionExpiresAt = &expiresAt
			}
		}

		if opts.CLICacheDir != "" && profile.SSOAccountID != "" && profile.SSORoleName != "" {
			key := ssocache.RoleCredentialKey{
				AccountID: profile.SSOAccountID,
				RoleName:  profile.SSORoleName,
			}
			if profile.SSOSession != "" {
				key.SessionName = profile.SSOSession
			} else {
				key.StartURL = profile.SSOStartURL
			}

			roleMeta, err := ssocache.ReadRoleCredentialMeta(ssocache.RoleCredentialPath(opts.CLICacheDir, key))
			if err != nil {
				info.Diagnostics = append(info.Diagnostics, err.Error())
			} else if roleMeta.Exists {
				lastUsed := roleMeta.LastUsed
				info.LastUsedAt = &lastUsed
				expiration := roleMeta.Expiration
				info.CredentialExpiresAt = &expiration
			}
		}

		infos = append(infos, info)
	}

	return ProfileList{
		SchemaVersion: 1,
		ConfigFile:    opts.ConfigFile,
		Profiles:      infos,
	}, nil
}
