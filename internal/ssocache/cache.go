// Package ssocache は ~/.aws/sso/cache と ~/.aws/cli/cache の状態だけを読む
// ネットワークは一切使わない ローカルファイルの存在と期限だけを見る
// トークン値(accessToken / refreshToken / clientSecret など)は読んでも公開しない
// 読むのは expiresAt と refreshToken の「有無」だけに限定する
package ssocache

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // AWS CLI 互換のキャッシュキー算出に使用 秘匿情報のハッシュ化ではない
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
)

// EvaluationState は SSO セッションの状態を表す文字列 enum
type EvaluationState string

const (
	// StateOK は expiresAt が未来でセッションが有効な状態
	StateOK EvaluationState = "ok"
	// StateWarning は期限切れだが refreshToken があり猶予内で自動更新が見込める状態
	StateWarning EvaluationState = "warning"
	// StateError は refreshToken が無い または猶予を超過した状態
	StateError EvaluationState = "error"
	// StateUnknown はトークンキャッシュファイルが無い(未ログイン)状態
	StateUnknown EvaluationState = "unknown"
)

// TokenMeta は sso/cache のトークンファイルから読んだ最小限の情報
// トークン値そのものは保持しない
type TokenMeta struct {
	// Exists はトークンキャッシュファイルが存在するか
	Exists bool
	// ExpiresAt はアクセストークンの有効期限
	ExpiresAt time.Time
	// HasRefreshToken は refreshToken フィールドが空でないか
	HasRefreshToken bool
}

// Evaluation は Evaluate の判定結果
type Evaluation struct {
	// State は判定された状態
	State EvaluationState
	// ExpiresAt は判定に使った有効期限 unknown の場合はゼロ値
	ExpiresAt time.Time
	// Remaining は ExpiresAt までの残り時間 期限切れ後は負値になる
	Remaining time.Duration
	// AutoRefresh は SDK / CLI が refreshToken でアクセストークンを取り直せるか(D34)
	// true なら ok の Remaining はアクセストークンの残りにすぎず ログインし直しまでの時間ではない
	AutoRefresh bool
}

// DefaultCacheDir は sso トークンキャッシュの既定ディレクトリを返す
func DefaultCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリを取得できません: %w", err)
	}
	return filepath.Join(homeDir, ".aws", "sso", "cache"), nil
}

// TokenPath は sso/cache のトークンファイルパスを返す
// key は sso-session なら「セッション名」 legacy 形式なら「start URL」
func TokenPath(cacheDir string, key string) string {
	return filepath.Join(cacheDir, sha1Hex(key)+".json")
}

// ReadTokenMeta は path のトークンファイルから expiresAt と refreshToken の有無だけを読む
// ファイルが無ければ Exists=false を error なしで返す
func ReadTokenMeta(path string) (TokenMeta, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path は sha1 済みキャッシュキーから算出した固定パス
	if err != nil {
		if os.IsNotExist(err) {
			return TokenMeta{}, nil
		}
		return TokenMeta{}, fmt.Errorf("sso トークンキャッシュを読み込めません: %s: %w", path, err)
	}

	var parsed struct {
		ExpiresAt    string `json:"expiresAt"`
		RefreshToken string `json:"refreshToken"` //nolint:gosec // 値は読むが Exists 有無しか公開しない(パッケージの制約)
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return TokenMeta{}, fmt.Errorf("sso トークンキャッシュの形式が不正です: %s: %w", path, err)
	}

	expiresAt, err := parseExpiresAt(parsed.ExpiresAt)
	if err != nil {
		return TokenMeta{}, fmt.Errorf("sso トークンキャッシュの expiresAt を解釈できません: %s: %w", path, err)
	}

	return TokenMeta{
		Exists:          true,
		ExpiresAt:       expiresAt,
		HasRefreshToken: strings.TrimSpace(parsed.RefreshToken) != "",
	}, nil
}

// Evaluate は TokenMeta と現在時刻から SSO セッションの状態を判定する
// grace は期限切れ後に refreshToken による自動更新を見込む猶予時間(D18 既定 8h)
func Evaluate(meta TokenMeta, now time.Time, grace time.Duration) Evaluation {
	if !meta.Exists {
		return Evaluation{State: StateUnknown}
	}

	eval := Evaluation{ExpiresAt: meta.ExpiresAt, Remaining: meta.ExpiresAt.Sub(now), AutoRefresh: meta.HasRefreshToken}
	switch {
	case eval.Remaining > 0:
		eval.State = StateOK
	case meta.HasRefreshToken && now.Before(meta.ExpiresAt.Add(grace)):
		eval.State = StateWarning
	default:
		eval.State = StateError
	}
	return eval
}

// RoleCredentialKey は ~/.aws/cli/cache のキー算出に使う入力
// sso-session 形式なら SessionName を legacy 形式なら StartURL を設定する(両方は設定しない)
type RoleCredentialKey struct {
	AccountID   string
	RoleName    string
	SessionName string
	StartURL    string
}

// RoleCredentialMeta は ~/.aws/cli/cache のキャッシュファイルから読んだ情報
type RoleCredentialMeta struct {
	// Exists はキャッシュファイルが存在するか
	Exists bool
	// Expiration は Credentials.Expiration
	Expiration time.Time
	// LastUsed はキャッシュファイルの mtime(認証情報の取得・更新時刻)
	LastUsed time.Time
}

// RoleCredentialPath は ~/.aws/cli/cache のキャッシュファイルパスを返す
// sha1 の入力は key をキー昇順 区切り "," ":"(空白なし)で直列化した JSON
func RoleCredentialPath(cacheDir string, key RoleCredentialKey) string {
	fields := make(map[string]string, 3)
	fields["accountId"] = key.AccountID
	fields["roleName"] = key.RoleName
	if key.SessionName != "" {
		fields["sessionName"] = key.SessionName
	} else {
		fields["startUrl"] = key.StartURL
	}

	// encoding/json は map[string]string を marshal する際キーを昇順にソートし
	// 既定のセパレータ(",", ":")を空白なしで使うため botocore の
	// json.dumps(sort_keys=True, separators=(",", ":")) と同じ表現になる
	// ただし既定の HTML エスケープ(< > & の \u 化)は botocore に無いので無効にする
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(fields); err != nil {
		// fields は string のみで構成されるため実質発生しない
		buf.Reset()
		buf.WriteString("{}")
	}
	// Python の json.dumps は ensure_ascii=true。非 ASCII を UTF-16 の \u 表記にする。
	var ascii strings.Builder
	for _, r := range strings.TrimSuffix(buf.String(), "\n") {
		switch {
		case r < 0x7f:
			ascii.WriteRune(r)
		case r <= 0xffff:
			fmt.Fprintf(&ascii, `\u%04x`, r)
		default:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&ascii, `\u%04x\u%04x`, hi, lo)
		}
	}
	data := ascii.String()

	return filepath.Join(cacheDir, sha1Hex(data)+".json")
}

// ReadRoleCredentialMeta は ~/.aws/cli/cache のキャッシュファイルから
// Credentials.Expiration と mtime だけを読む Credentials の他フィールドは読まない
func ReadRoleCredentialMeta(path string) (RoleCredentialMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RoleCredentialMeta{}, nil
		}
		return RoleCredentialMeta{}, fmt.Errorf("role credential キャッシュの参照に失敗: %s: %w", path, err)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path は sha1 済みキャッシュキーから算出した固定パス
	if err != nil {
		return RoleCredentialMeta{}, fmt.Errorf("role credential キャッシュを読み込めません: %s: %w", path, err)
	}

	var parsed struct {
		Credentials struct {
			Expiration time.Time `json:"Expiration"`
		} `json:"Credentials"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return RoleCredentialMeta{}, fmt.Errorf("role credential キャッシュの形式が不正です: %s: %w", path, err)
	}

	return RoleCredentialMeta{
		Exists:     true,
		Expiration: parsed.Credentials.Expiration,
		LastUsed:   info.ModTime(),
	}, nil
}

var expiresAtLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05MST",
}

func parseExpiresAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("expiresAt が空です")
	}

	var lastErr error
	for _, layout := range expiresAtLayouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func sha1Hex(value string) string {
	sum := sha1.Sum([]byte(value)) //nolint:gosec // AWS CLI 互換のキャッシュキー算出に使用
	return hex.EncodeToString(sum[:])
}
