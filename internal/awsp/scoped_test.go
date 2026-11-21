package awsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePolicy(t *testing.T, dir string, name string, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadScopedPolicies(t *testing.T) {
	t.Run("名前で引き 値を置き換えて詰める", func(t *testing.T) {
		dir := t.TempDir()
		writePolicy(t, dir, "cfn.json", `{
  "Statement": [{"Effect": "Allow", "Action": "cloudformation:*", "Resource": "arn:aws:cloudformation:*:*:stack/{{ stack }}/*"}]
}`)
		policies, err := LoadScopedPolicies(ScopedPolicyOptions{
			Dir: dir, Names: []string{"cfn"}, Vars: map[string]string{"stack": "api"},
		})
		if err != nil {
			t.Fatal(err)
		}
		want := `{"Statement":[{"Effect":"Allow","Action":"cloudformation:*","Resource":"arn:aws:cloudformation:*:*:stack/api/*"}]}`
		if len(policies) != 1 || policies[0].Document != want {
			t.Fatalf("got %+v", policies)
		}
	})

	t.Run("always.json があれば 1 層目に置く", func(t *testing.T) {
		dir := t.TempDir()
		always := writePolicy(t, dir, "always.json", `{"Statement":[]}`)
		writePolicy(t, dir, "query.json", `{"Statement":[]}`)
		policies, err := LoadScopedPolicies(ScopedPolicyOptions{Dir: dir, Names: []string{"query"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(policies) != 2 || policies[0].Source != always {
			t.Fatalf("got %+v", policies)
		}
	})

	t.Run("パスでも読める", func(t *testing.T) {
		dir := t.TempDir()
		path := writePolicy(t, dir, "adhoc.json", `{"Statement":[]}`)
		policies, err := LoadScopedPolicies(ScopedPolicyOptions{Dir: t.TempDir(), Names: []string{path}})
		if err != nil || len(policies) != 1 {
			t.Fatalf("got %+v err %v", policies, err)
		}
	})

	errorCases := []struct {
		name  string
		setup func(dir string) ScopedPolicyOptions
		want  string
	}{
		{"ポリシーの指定なし", func(dir string) ScopedPolicyOptions {
			return ScopedPolicyOptions{Dir: dir}
		}, "--policy を 1 つ以上"},
		{"ファイルが無い", func(dir string) ScopedPolicyOptions {
			return ScopedPolicyOptions{Dir: dir, Names: []string{"none"}}
		}, "ポリシーが見つかりません"},
		{"値が足りない", func(dir string) ScopedPolicyOptions {
			writePolicy(t, dir, "cfn.json", `{"Resource":"{{stack}}"}`)
			return ScopedPolicyOptions{Dir: dir, Names: []string{"cfn"}}
		}, "--set stack=<値>"},
		{"JSON でない", func(dir string) ScopedPolicyOptions {
			writePolicy(t, dir, "bad.json", `{"Statement":`)
			return ScopedPolicyOptions{Dir: dir, Names: []string{"bad"}}
		}, "JSON として読めません"},
		{"長すぎる", func(dir string) ScopedPolicyOptions {
			writePolicy(t, dir, "long.json", `{"Sid":"`+strings.Repeat("a", ScopedPolicyMaxLength)+`"}`)
			return ScopedPolicyOptions{Dir: dir, Names: []string{"long"}}
		}, "文字までです"},
		{"層が多すぎる", func(dir string) ScopedPolicyOptions {
			writePolicy(t, dir, "always.json", `{}`)
			writePolicy(t, dir, "a.json", `{}`)
			return ScopedPolicyOptions{Dir: dir, Names: []string{"a", "a", "a"}}
		}, "3 層までです"},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadScopedPolicies(tc.setup(t.TempDir()))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestParseScopedVars(t *testing.T) {
	vars, err := ParseScopedVars([]string{"stack=prod-api-*", "bucket=my.bucket/path"})
	if err != nil || vars["stack"] != "prod-api-*" || vars["bucket"] != "my.bucket/path" {
		t.Fatalf("got %v err %v", vars, err)
	}
	// 引用符を通すと ポリシーの JSON を書き換えて権限を広げられる
	for _, bad := range []string{"stack", "=x", `stack=a","Action":"*`, "stack=a b", "stack="} {
		if _, err := ParseScopedVars([]string{bad}); err == nil {
			t.Fatalf("%q はエラーになるべき", bad)
		}
	}
}
