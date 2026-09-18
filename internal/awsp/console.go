package awsp

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kagamirror123/awsp/internal/awsconfig"
)

// ConsoleReport は `awsp console --json` の出力(D31)
type ConsoleReport struct {
	// SchemaVersion は出力形式のバージョン
	SchemaVersion int `json:"schemaVersion"`
	// Profile は対象の profile 名
	Profile string `json:"profile"`
	// Account と Role は URL に載せた sso_account_id と sso_role_name
	Account string `json:"account"`
	Role    string `json:"role"`
	// Destination は指定されたコンソール内の行き先 URL(省略可)
	Destination string `json:"destination,omitempty"`
	// URL は開くアクセスポータルの deep link。トークンは含まない
	URL string `json:"url"`
}

// consoleDestinationSuffixes は destination に許すホスト名の末尾
// AWS のコンソール以外へ飛ばす URL を組まないための制限
var consoleDestinationSuffixes = []string{
	".aws.amazon.com",
	".amazonaws.com",
	".amazonaws.com.cn",
	".amazonaws-us-gov.com",
}

// BuildConsoleURL は profile の account / role でコンソールを開くアクセスポータルの deep link を組む(D31)
// 形式: <start_url>/#/console?account_id=<id>&role_name=<role>[&destination=<url>]
// ブラウザに SSO のセッションが残っていれば再認証なしでコンソールに入れる。トークンは扱わない
func BuildConsoleURL(profile awsconfig.Profile, session awsconfig.SSOSession, destination string) (ConsoleReport, error) {
	if profile.SSOAccountID == "" || profile.SSORoleName == "" {
		return ConsoleReport{}, fmt.Errorf(
			"profile %q は SSO(sso_account_id / sso_role_name)を使っていません: コンソールの deep link は SSO の profile だけ組めます",
			profile.Name,
		)
	}
	base := strings.TrimRight(strings.TrimSpace(session.StartURL), "#/")
	if base == "" {
		return ConsoleReport{}, fmt.Errorf("profile %q の sso_start_url が空です: ~/.aws/config を確認してください", profile.Name)
	}

	query := url.Values{}
	query.Set("account_id", profile.SSOAccountID)
	query.Set("role_name", profile.SSORoleName)
	if destination != "" {
		if err := validateConsoleDestination(destination); err != nil {
			return ConsoleReport{}, err
		}
		query.Set("destination", destination)
	}

	return ConsoleReport{
		SchemaVersion: 1,
		Profile:       profile.Name,
		Account:       profile.SSOAccountID,
		Role:          profile.SSORoleName,
		Destination:   destination,
		URL:           base + "/#/console?" + query.Encode(),
	}, nil
}

// validateConsoleDestination は行き先が https の AWS コンソール URL であることを確認する
func validateConsoleDestination(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("行き先 URL を解釈できません: %w", err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return errors.New("行き先 URL は https のコンソール URL を指定してください(例: https://console.aws.amazon.com/cloudwatch/home)")
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range consoleDestinationSuffixes {
		if strings.HasSuffix(host, suffix) || host == strings.TrimPrefix(suffix, ".") {
			return nil
		}
	}
	return fmt.Errorf("行き先 URL のホスト %q は AWS のコンソールではありません", u.Hostname())
}
