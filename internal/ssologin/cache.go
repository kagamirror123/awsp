package ssologin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
)

func (f *Flow) persistToken(output *ssooidc.CreateTokenOutput) (Result, error) {
	expiresAt := time.Now().UTC().Add(time.Duration(output.ExpiresIn) * time.Second)

	file := tokenFile{
		StartURL:              f.session.StartURL,
		Region:                f.session.Region,
		AccessToken:           strVal(output.AccessToken),
		ExpiresAt:             expiresAt.Format(timeLayout),
		ClientID:              f.clientID,
		ClientSecret:          f.clientSecret,
		RegistrationExpiresAt: f.registrationExpiresAt.UTC().Format(timeLayout),
		RefreshToken:          strVal(output.RefreshToken),
	}

	if err := writeTokenFileAtomic(f.tokenPath, file); err != nil {
		return Result{}, err
	}

	return Result{
		ExpiresAt:       expiresAt,
		HasRefreshToken: file.RefreshToken != "",
	}, nil
}

// tokenFile は ~/.aws/sso/cache のトークンキャッシュファイルの形式
// aws CLI / SDK と完全互換にする
type tokenFile struct {
	StartURL              string `json:"startUrl"`
	Region                string `json:"region"`
	AccessToken           string `json:"accessToken,omitempty"` //nolint:gosec // aws CLI 互換のキャッシュ形式に必須
	ExpiresAt             string `json:"expiresAt,omitempty"`
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"` //nolint:gosec // aws CLI 互換のキャッシュ形式に必須
	RegistrationExpiresAt string `json:"registrationExpiresAt"`
	RefreshToken          string `json:"refreshToken,omitempty"` //nolint:gosec // aws CLI 互換のキャッシュ形式に必須
}

func writeTokenFileAtomic(path string, file tokenFile) (err error) {
	dir := filepath.Dir(path)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return fmt.Errorf("sso トークンキャッシュ用ディレクトリを作成できません: %s: %w", dir, mkErr)
	}

	data, marshalErr := json.MarshalIndent(file, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("sso トークンキャッシュのエンコードに失敗: %w", marshalErr)
	}

	tmp, createErr := os.CreateTemp(dir, ".awsp-token-*.tmp")
	if createErr != nil {
		return fmt.Errorf("sso トークンキャッシュの一時ファイルを作成できません: %w", createErr)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if chmodErr := tmp.Chmod(0o600); chmodErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("sso トークンキャッシュの権限設定に失敗: %w", chmodErr)
	}
	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("sso トークンキャッシュの書き込みに失敗: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("sso トークンキャッシュのクローズに失敗: %w", closeErr)
	}

	if renameErr := os.Rename(tmpPath, path); renameErr != nil { //nolint:gosec // path は ssocache.TokenPath が算出する sha1 済み固定パス
		return fmt.Errorf("sso トークンキャッシュの確定に失敗: %w", renameErr)
	}
	renamed = true

	return nil
}
