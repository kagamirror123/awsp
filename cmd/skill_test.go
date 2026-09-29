package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillCommandPattern は SKILL.md のコードスパンに書いた awsp の呼び出しを拾う
var skillCommandPattern = regexp.MustCompile("`(awsp [^`]+)`")

// TestSkillCommandsExist は skills/awsp/SKILL.md に書いたコマンドとフラグが CLI に実在するかを確かめる(D35)
// CLI を変えてスキルだけが古いまま残ると エージェントは存在しないフラグを打つまで気づけない
// 引数は <profile> や [url] の形で書き フラグの値は --timeout=<duration> のように = でつなぐ
func TestSkillCommandsExist(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "skills", "awsp", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md を読めない: %v", err)
	}

	matches := skillCommandPattern.FindAllStringSubmatch(string(body), -1)
	if len(matches) == 0 {
		t.Fatal("SKILL.md に awsp の呼び出しが見つからない")
	}

	root := newRootCmd()
	for _, m := range matches {
		invocation := m[1]
		var path, flags []string
		for _, field := range strings.Fields(invocation)[1:] {
			switch {
			case strings.HasPrefix(field, "--"):
				name, _, _ := strings.Cut(strings.TrimPrefix(field, "--"), "=")
				flags = append(flags, name)
			case strings.HasPrefix(field, "<"), strings.HasPrefix(field, "["):
				// 引数の置き場
			default:
				path = append(path, field)
			}
		}

		cmd, rest, err := root.Find(path)
		if err != nil || len(rest) > 0 {
			t.Errorf("%q: サブコマンドが見つからない(残り %v): %v", invocation, rest, err)
			continue
		}
		for _, name := range flags {
			if cmd.Flags().Lookup(name) == nil && cmd.InheritedFlags().Lookup(name) == nil {
				t.Errorf("%q: %s に --%s が無い", invocation, cmd.CommandPath(), name)
			}
		}
	}
}
