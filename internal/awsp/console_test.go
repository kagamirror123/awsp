package awsp

import (
	"net/url"
	"strings"
	"testing"

	"github.com/kagamirror123/awsp/internal/awsconfig"
)

func TestBuildConsoleURL(t *testing.T) {
	t.Parallel()

	profile := awsconfig.Profile{Name: "dev", SSOSession: "corp", SSOAccountID: "123456789012", SSORoleName: "AdministratorAccess"}

	t.Run("start URL の末尾の / や # を吸収する", func(t *testing.T) {
		for _, start := range []string{
			"https://d-xxx.awsapps.com/start",
			"https://d-xxx.awsapps.com/start/",
			"https://d-xxx.awsapps.com/start/#",
			"https://d-xxx.awsapps.com/start/#/",
		} {
			got, err := BuildConsoleURL(profile, awsconfig.SSOSession{StartURL: start}, "")
			if err != nil {
				t.Fatalf("%s: %v", start, err)
			}
			want := "https://d-xxx.awsapps.com/start/#/console?account_id=123456789012&role_name=AdministratorAccess"
			if got.URL != want {
				t.Fatalf("%s: URL が想定外\n got=%s\nwant=%s", start, got.URL, want)
			}
		}
	})

	t.Run("destination は URL エンコードして載せる", func(t *testing.T) {
		dest := "https://ap-northeast-1.console.aws.amazon.com/cloudwatch/home?region=ap-northeast-1#logsV2:log-groups"
		got, err := BuildConsoleURL(profile, awsconfig.SSOSession{StartURL: "https://d-xxx.awsapps.com/start"}, dest)
		if err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(strings.TrimPrefix(got.URL, "https://d-xxx.awsapps.com/start/#/console"))
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("destination") != dest {
			t.Fatalf("destination が往復しない: %s", got.URL)
		}
		if got.Destination != dest || got.Account != "123456789012" || got.Role != "AdministratorAccess" || got.SchemaVersion != 1 {
			t.Fatalf("Report が想定外: %+v", got)
		}
	})

	t.Run("AWS 以外の行き先は拒否する", func(t *testing.T) {
		for _, dest := range []string{
			"https://example.com/",
			"http://console.aws.amazon.com/",
			"https://evil.com/console.aws.amazon.com",
			"console.aws.amazon.com/ec2",
		} {
			if _, err := BuildConsoleURL(profile, awsconfig.SSOSession{StartURL: "https://d-xxx.awsapps.com/start"}, dest); err == nil {
				t.Fatalf("%s が通ってしまった", dest)
			}
		}
	})

	t.Run("SSO を使わない profile はエラー", func(t *testing.T) {
		static := awsconfig.Profile{Name: "legacy", SourceProfile: "dev", RoleARN: "arn:aws:iam::1:role/x"}
		_, err := BuildConsoleURL(static, awsconfig.SSOSession{StartURL: "https://d-xxx.awsapps.com/start"}, "")
		if err == nil || !strings.Contains(err.Error(), "SSO") {
			t.Fatalf("エラーが想定外: %v", err)
		}
	})
}
