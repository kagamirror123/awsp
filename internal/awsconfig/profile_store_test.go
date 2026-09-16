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

func TestSSOSessions(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config")

	content := `[sso-session corp]
sso_start_url = https://example.awsapps.com/start
sso_region = us-west-2
sso_registration_scopes = sso:account:access, sso:read

[sso-session no-scope]
sso_start_url = https://no-scope.awsapps.com/start
sso_region = ap-northeast-1

[profile dev]
sso_session = corp
sso_account_id = 123456789012
sso_role_name = AdministratorAccess
`

	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("config 作成に失敗: %v", err)
	}

	store := NewProfileStore(configPath)
	got, err := store.SSOSessions(context.Background())
	if err != nil {
		t.Fatalf("SSOSessions が失敗: %v", err)
	}

	want := []SSOSession{
		{
			Name:               "corp",
			StartURL:           "https://example.awsapps.com/start",
			Region:             "us-west-2",
			RegistrationScopes: []string{"sso:account:access", "sso:read"},
		},
		{
			Name:               "no-scope",
			StartURL:           "https://no-scope.awsapps.com/start",
			Region:             "ap-northeast-1",
			RegistrationScopes: []string{defaultSSORegistrationScope},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SSOSessions が想定外\nwant=%+v\ngot=%+v", want, got)
	}
}

func TestResolveSession(t *testing.T) {
	t.Parallel()

	sessions := []SSOSession{
		{
			Name:               "corp",
			StartURL:           "https://example.awsapps.com/start",
			Region:             "us-west-2",
			RegistrationScopes: []string{"sso:account:access"},
		},
	}

	t.Run("named session が見つかる", func(t *testing.T) {
		t.Parallel()
		profile := Profile{Name: "dev", SSOSession: "corp"}

		got, ok := ResolveSession(profile, sessions)
		if !ok {
			t.Fatal("ok が false")
		}
		if got.CacheKey() != "corp" || got.IsLegacy() {
			t.Fatalf("ResolveSession が想定外: %+v", got)
		}
	})

	t.Run("named session が config に無くても合成される", func(t *testing.T) {
		t.Parallel()
		profile := Profile{Name: "dev", SSOSession: "missing"}

		got, ok := ResolveSession(profile, sessions)
		if !ok {
			t.Fatal("ok が false")
		}
		if got.CacheKey() != "missing" {
			t.Fatalf("CacheKey が想定外: %s", got.CacheKey())
		}
	})

	t.Run("legacy profile は start URL をキーにする", func(t *testing.T) {
		t.Parallel()
		profile := Profile{
			Name:        "legacy-dev",
			SSOStartURL: "https://legacy.awsapps.com/start",
			SSORegion:   "ap-northeast-1",
		}

		got, ok := ResolveSession(profile, sessions)
		if !ok {
			t.Fatal("ok が false")
		}
		if !got.IsLegacy() {
			t.Fatal("IsLegacy が false")
		}
		if got.CacheKey() != "https://legacy.awsapps.com/start" {
			t.Fatalf("CacheKey が想定外: %s", got.CacheKey())
		}
	})

	t.Run("SSO を使わない profile は ok=false", func(t *testing.T) {
		t.Parallel()
		profile := Profile{Name: "static", RoleARN: "arn:aws:iam::123456789012:role/x", SourceProfile: "base"}

		_, ok := ResolveSession(profile, sessions)
		if ok {
			t.Fatal("SSO を使わない profile なのに ok=true")
		}
	})
}
