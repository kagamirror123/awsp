package awsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	// ScopedPolicyMaxLength は GetScopedCredentials の Policy の上限(文字数)
	ScopedPolicyMaxLength = 2048
	// ScopedPolicyMaxLayers は 1 つのセッションに重ねられるポリシーの上限
	ScopedPolicyMaxLayers = 3
	// scopedAlwaysPolicy は ポリシーのディレクトリにあれば必ず 1 層目に重ねるファイル名
	scopedAlwaysPolicy = "always.json"
)

var (
	scopedPlaceholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)
	// 値は JSON の文字列の中へ入るので 引用符やバックスラッシュを通さない
	scopedVarValue = regexp.MustCompile(`^[A-Za-z0-9._/*:@+=,-]+$`)
	scopedVarKey   = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// ScopedPolicy は重ねるポリシー 1 層分
type ScopedPolicy struct {
	// Source は読んだファイルのパス
	Source string
	// Document は置き換えと詰めを済ませた JSON
	Document string
}

// ScopedPolicyOptions はポリシーの解決条件
type ScopedPolicyOptions struct {
	// Dir は名前で引くポリシーの置き場
	Dir string
	// Names は --policy の値。パス区切りか .json を含めばファイルのパス そうでなければ Dir/<名前>.json
	Names []string
	// Vars は {{key}} を置き換える値
	Vars map[string]string
}

// ParseScopedVars は key=value の並びを map にする
func ParseScopedVars(pairs []string) (map[string]string, error) {
	vars := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || !scopedVarKey.MatchString(key) {
			return nil, fmt.Errorf("--set は key=value の形で指定してください: %q", pair)
		}
		if !scopedVarValue.MatchString(value) {
			return nil, fmt.Errorf("--set %s の値に使えない文字があります(英数字と ._/*:@+=,- だけ): %q", key, value)
		}
		vars[key] = value
	}
	return vars, nil
}

// LoadScopedPolicies は重ねる順にポリシーを読む
// Dir に always.json があれば必ず 1 層目に置き そのあとに Names を指定順で並べる
func LoadScopedPolicies(opts ScopedPolicyOptions) ([]ScopedPolicy, error) {
	if len(opts.Names) == 0 {
		return nil, errors.New("--policy を 1 つ以上指定してください")
	}

	paths := make([]string, 0, len(opts.Names)+1)
	always := filepath.Join(opts.Dir, scopedAlwaysPolicy)
	if _, err := os.Stat(always); err == nil {
		paths = append(paths, always)
	}
	for _, name := range opts.Names {
		paths = append(paths, scopedPolicyPath(opts.Dir, name))
	}
	if len(paths) > ScopedPolicyMaxLayers {
		return nil, fmt.Errorf(
			"ポリシーは %d 層までです(%s を含めて %d 層になります)",
			ScopedPolicyMaxLayers, scopedAlwaysPolicy, len(paths),
		)
	}

	policies := make([]ScopedPolicy, 0, len(paths))
	for _, path := range paths {
		document, err := loadScopedPolicy(path, opts.Vars)
		if err != nil {
			return nil, err
		}
		policies = append(policies, ScopedPolicy{Source: path, Document: document})
	}
	return policies, nil
}

func scopedPolicyPath(dir string, name string) string {
	if strings.ContainsRune(name, filepath.Separator) || strings.HasSuffix(name, ".json") {
		return name
	}
	return filepath.Join(dir, name+".json")
}

func loadScopedPolicy(path string, vars map[string]string) (string, error) {
	//nolint:gosec // 利用者が指定したポリシーのファイルを読むのが役目
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("ポリシーが見つかりません: %s", path)
		}
		return "", fmt.Errorf("ポリシーを読めません: %w", err)
	}

	missing := map[string]struct{}{}
	rendered := scopedPlaceholder.ReplaceAllStringFunc(string(raw), func(match string) string {
		key := scopedPlaceholder.FindStringSubmatch(match)[1]
		value, ok := vars[key]
		if !ok {
			missing[key] = struct{}{}
			return match
		}
		return value
	})
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for key := range missing {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("%s の {{%s}} に値がありません: --set %s=<値> を付けてください", path, keys[0], keys[0])
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(rendered)); err != nil {
		return "", fmt.Errorf("%s は JSON として読めません: %w", path, err)
	}
	if n := len([]rune(compact.String())); n > ScopedPolicyMaxLength {
		return "", fmt.Errorf("%s は %d 文字あります: ポリシーは %d 文字までです", path, n, ScopedPolicyMaxLength)
	}
	return compact.String(), nil
}
