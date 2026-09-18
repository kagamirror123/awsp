package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

// completeProfileNames は位置引数の profile 名を補完する(D29)
// config を読んで名前を返すだけで ネットワークは使わない
// 既に profile が 1 つ指定されていれば候補を返さない ファイル名補完には落とさない
func completeProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := newProfileStore().Profiles(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	candidates := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, toComplete) {
			candidates = append(candidates, name)
		}
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}
