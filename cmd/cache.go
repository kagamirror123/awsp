package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// cliCacheDirOrEmpty は ~/.aws/cli/cache のディレクトリを返す(§6.2)
func cliCacheDirOrEmpty() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリを取得できません: %w", err)
	}
	return filepath.Join(homeDir, ".aws", "cli", "cache"), nil
}
