package awsconfig

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProfiles(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config")

	content := `[default]
region = ap-northeast-1

[profile dev]
region = ap-northeast-1

[sso-session example]
sso_start_url = https://example.awsapps.com/start

[profile prod]
region = us-east-1
`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("config 作成に失敗: %v", err)
	}

	store := NewProfileStore(configPath)
	got, err := store.Profiles(context.Background())
	if err != nil {
		t.Fatalf("Profiles が失敗: %v", err)
	}

	want := []string{"dev", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Profiles が想定外\nwant=%v\ngot=%v", want, got)
	}
}

func TestProfileDetails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config")

	content := `[profile dev]
region = ap-northeast-1
output = json
sso_session = corp
sso_account_id = 123456789012
sso_role_name = AdministratorAccess

[profile prod]
role_arn = arn:aws:iam::123456789012:role/prod-role
source_profile = base
`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("config 作成に失敗: %v", err)
	}

	store := NewProfileStore(configPath)
	got, err := store.ProfileDetails(context.Background())
	if err != nil {
		t.Fatalf("ProfileDetails が失敗: %v", err)
	}

	want := []Profile{
		{
			Name:         "dev",
			Region:       "ap-northeast-1",
			Output:       "json",
			SSOSession:   "corp",
			SSOAccountID: "123456789012",
			SSORoleName:  "AdministratorAccess",
		},
		{
			Name:          "prod",
			RoleARN:       "arn:aws:iam::123456789012:role/prod-role",
			SourceProfile: "base",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProfileDetails が想定外\nwant=%+v\ngot=%+v", want, got)
	}
}

func TestParseProfileSection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		line   string
		want   string
		wantOK bool
	}{
		{name: "profile section", line: "[profile sandbox]", want: "sandbox", wantOK: true},
		{name: "default section", line: "[default]", want: "", wantOK: false},
		{name: "sso section", line: "[sso-session shared]", want: "", wantOK: false},
		{name: "comment", line: "# comment", want: "", wantOK: false},
		{name: "invalid", line: "region = ap-northeast-1", want: "", wantOK: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseProfileSection(tc.line)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("parseProfileSection が想定外\nwant=(%q,%v)\ngot=(%q,%v)", tc.want, tc.wantOK, got, ok)
			}
		})
	}
}
