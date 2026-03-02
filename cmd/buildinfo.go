// Package cmd は awsp の CLI エントリとサブコマンドを提供する
package cmd

import "strings"

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

// SetBuildInfo はビルド時に埋め込まれた情報を設定する
func SetBuildInfo(version string, commit string, date string) {
	if strings.TrimSpace(version) != "" {
		buildVersion = version
	}
	if strings.TrimSpace(commit) != "" {
		buildCommit = commit
	}
	if strings.TrimSpace(date) != "" {
		buildDate = date
	}
}

func versionLine() string {
	return "awsp " + buildVersion
}

func versionDetail() string {
	return "version=" + buildVersion + " commit=" + buildCommit + " date=" + buildDate
}
