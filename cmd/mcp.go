package cmd

import (
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/kagamirror123/awsp/internal/awscli"
	mcpserver "github.com/kagamirror123/awsp/internal/mcp"
	"github.com/kagamirror123/awsp/internal/ssocache"
	"github.com/kagamirror123/awsp/internal/ssologin"
)

// newMCPCmd は awsp を MCP サーバー(stdio)として起動するコマンドを作る(D1 D2)
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "mcp",
		Short:        "エージェント向けの MCP サーバーを stdio で起動(auth_status / list_profiles / whoami / login)",
		Example:      "  awsp mcp\n  claude mcp add awsp -- awsp mcp",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// stdout は MCP プロトコル専用なのでログ・診断は必ず stderr へ出す
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

			profileStore := newProfileStore()

			ssoCacheDir, err := ssocache.DefaultCacheDir()
			if err != nil {
				return err
			}
			cliCacheDir, err := cliCacheDirOrEmpty()
			if err != nil {
				return err
			}

			deps := mcpserver.Deps{
				Profiles:    profileStore,
				SSOCacheDir: ssoCacheDir,
				CLICacheDir: cliCacheDir,
				AWS:         awscli.NewClient(),
				NewOIDCClient: func(region string) ssologin.OIDCClient {
					return ssooidc.New(ssooidc.Options{Region: region})
				},
			}

			server := mcpserver.NewServer(cmd.Context(), deps, &sdkmcp.Implementation{
				Name:    "awsp",
				Version: buildVersion,
			})

			logger.Info("awsp mcp: stdio サーバーを起動します", slog.String("configFile", profileStore.ConfigPath()))

			if err := server.Run(cmd.Context(), &sdkmcp.StdioTransport{}); err != nil {
				return err
			}

			logger.Info("awsp mcp: サーバーを終了しました")
			return nil
		},
	}

	return cmd
}
