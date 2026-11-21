package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kagamirror123/awsp/internal/awscli"
	"github.com/kagamirror123/awsp/internal/awsp"
	"github.com/spf13/cobra"
)

// scopedConfigFileEnv は 元の認証情報を取る config を AWS_CONFIG_FILE と別に指す環境変数
const scopedConfigFileEnv = "AWSP_SCOPED_CONFIG_FILE"

// scopedCredentialEnv は 子プロセスへ渡す前に取り除く環境変数
// profile や前の認証情報が残ると 絞った認証情報より優先されることがある
var scopedCredentialEnv = []string{
	"AWS_PROFILE", "AWS_DEFAULT_PROFILE",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN",
	"AWS_CREDENTIAL_EXPIRATION",
}

func newScopedCmd() *cobra.Command {
	var (
		profile      string
		policyNames  []string
		vars         []string
		reason       string
		duration     time.Duration
		sourceConfig string
	)

	cmd := &cobra.Command{
		Use:   "scoped --profile <profile> --policy <名前|パス> --reason <用途> -- <コマンド> [引数...]",
		Short: "ポリシーで絞った一時認証情報を発行し、指定したコマンドにだけ渡して実行",
		Long: strings.Join([]string{
			"ポリシーで絞った一時認証情報を発行し、-- の後ろのコマンドにだけ環境変数で渡して実行する。",
			"",
			"--policy は名前(" + displayPolicyDir() + "/<名前>.json)かファイルのパス。複数付けると順に重ねる。",
			"同じ場所に always.json があれば、必ず 1 層目に重ねる。",
			"ポリシーの中の {{key}} は --set key=value で置き換える。",
			"元の認証情報を取る config は --source-config か " + scopedConfigFileEnv + " で変えられる。",
		}, "\n"),
		Example: strings.Join([]string{
			"  awsp scoped --profile prod --policy query --reason \"investigate 5xx\" -- aws athena start-query-execution ...",
			"  awsp scoped --profile prod --policy cfn --set stack=api --reason \"deploy api\" -- task deploy",
		}, "\n"),
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() != 0 {
				return errors.New("実行するコマンドは -- の後ろに書いてください: awsp scoped ... -- <コマンド>")
			}
			if profile == "" {
				current, _, err := resolveCurrentProfile()
				if err != nil {
					return errors.New("profile が未指定で AWS_PROFILE も未設定です: --profile <profile> を付けてください")
				}
				profile = current
			}
			if !validScopedReason(reason) {
				return errors.New(
					"--reason に用途を半角の英数字と記号で 2〜256 文字書いてください(CloudTrail の発行イベントに残ります。日本語は STS が受け付けません)",
				)
			}
			if duration < time.Second || duration > awscli.ScopedMaxDuration {
				return errors.New("--duration は 1 秒から 36 時間の間にしてください")
			}

			parsedVars, err := awsp.ParseScopedVars(vars)
			if err != nil {
				return err
			}
			policies, err := awsp.LoadScopedPolicies(awsp.ScopedPolicyOptions{
				Dir:   scopedPolicyDir(),
				Names: policyNames,
				Vars:  parsedVars,
			})
			if err != nil {
				return err
			}
			documents := make([]string, 0, len(policies))
			for _, policy := range policies {
				documents = append(documents, policy.Document)
			}

			if sourceConfig == "" {
				sourceConfig = strings.TrimSpace(os.Getenv(scopedConfigFileEnv))
			}
			creds, err := awscli.NewClient().ScopedCredentials(cmd.Context(), awscli.ScopedRequest{
				Profile:     profile,
				ConfigFile:  expandHome(sourceConfig),
				Policies:    documents,
				Duration:    duration,
				Description: reason,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				cmd.ErrOrStderr(), "awsp scoped: %s (%s) ポリシー %d 層 期限 %s\n",
				profile, awscli.RoleName(creds.ARN), creds.PolicyLayers, creds.Expiration.Local().Format("15:04:05"),
			)

			//nolint:gosec // 利用者が -- の後ろに書いたコマンドをそのまま実行するのが役目
			child := exec.CommandContext(cmd.Context(), args[0], args[1:]...)
			child.Stdin = os.Stdin
			child.Stdout = cmd.OutOrStdout()
			child.Stderr = cmd.ErrOrStderr()
			child.Env = scopedEnv(os.Environ(), creds)
			if err := child.Run(); err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					return newSilentExit(exitErr.ExitCode())
				}
				return fmt.Errorf("コマンドを実行できません: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&profile, "profile", "p", "", "絞る元の profile(省略時は AWS_PROFILE)")
	cmd.Flags().StringArrayVar(&policyNames, "policy", nil, "重ねるポリシーの名前かパス(複数可。指定順に重ねる)")
	cmd.Flags().StringArrayVar(&vars, "set", nil, "ポリシーの {{key}} を置き換える key=value(複数可)")
	cmd.Flags().StringVar(&reason, "reason", "", "用途(半角の英数字と記号。CloudTrail の発行イベントに残る)")
	cmd.Flags().DurationVar(&duration, "duration", 15*time.Minute, "認証情報の有効期間")
	cmd.Flags().StringVar(&sourceConfig, "source-config", "", "元の認証情報を取る config(省略時は "+scopedConfigFileEnv+"、無ければ AWS_CONFIG_FILE)")
	_ = cmd.RegisterFlagCompletionFunc("profile", completeProfileNames)
	return cmd
}

// validScopedReason は ScopedSessionDescription に渡せる文字列かを確かめる
// STS は Latin-1 までしか受け付けないので 表示できる ASCII に絞る
func validScopedReason(reason string) bool {
	if len(reason) < 2 || len(reason) > 256 {
		return false
	}
	for _, r := range reason {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

// scopedEnv は 親の環境から profile と認証情報を取り除き 絞った認証情報を足す
// リージョンは親に無いときだけ profile のものを入れる
func scopedEnv(parent []string, creds awscli.ScopedCredentials) []string {
	env := make([]string, 0, len(parent)+6)
	hasRegion := false
	for _, entry := range parent {
		key, _, _ := strings.Cut(entry, "=")
		if containsString(scopedCredentialEnv, key) {
			continue
		}
		if key == "AWS_REGION" || key == "AWS_DEFAULT_REGION" {
			hasRegion = true
		}
		env = append(env, entry)
	}
	env = append(env,
		"AWS_ACCESS_KEY_ID="+creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY="+creds.SecretAccessKey,
		"AWS_SESSION_TOKEN="+creds.SessionToken,
	)
	if !creds.Expiration.IsZero() {
		env = append(env, "AWS_CREDENTIAL_EXPIRATION="+creds.Expiration.UTC().Format(time.RFC3339))
	}
	if !hasRegion && creds.Region != "" {
		env = append(env, "AWS_REGION="+creds.Region, "AWS_DEFAULT_REGION="+creds.Region)
	}
	return env
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// scopedPolicyDir は 名前で引くポリシーの置き場($XDG_CONFIG_HOME/awsp/policies 無ければ ~/.config/awsp/policies)
func scopedPolicyDir() string {
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, "awsp", "policies")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "awsp", "policies")
	}
	return filepath.Join(home, ".config", "awsp", "policies")
}

func displayPolicyDir() string {
	return "~/.config/awsp/policies"
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}
