package cli

import (
	"errors"
	"fmt"

	"github.com/shabatoily/govfs/internal/client"
	mcpserver "github.com/shabatoily/govfs/internal/mcp"
	"github.com/spf13/cobra"
)

// NewMCPCommand는 기존 서버 설정을 사용하는 stdio MCP 서버 명령을 반환합니다.
func NewMCPCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:          "mcp",
		Short:        "Run the MCP server over stdio",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			userConfig, ok := cmd.Context().Value(ContextKeyUserConfig{}).(*UserConfig)
			if !ok {
				return errors.New("config not found")
			}

			c := client.New(userConfig.ServerURL)
			if !userConfig.TokenInfo.IsExpired() {
				c.SetToken(userConfig.TokenInfo.Token)
			}
			if _, err := c.Auth().Me(); err != nil {
				token, loginErr := c.Auth().Login(userConfig.Username, userConfig.Password)
				if loginErr != nil {
					return fmt.Errorf("authenticate MCP client: %w", loginErr)
				}
				c.SetToken(token.Token)
				userConfig.TokenInfo.TokenRes = token
				if err := SetUserConfig(userConfig); err != nil {
					return err
				}
			}

			server, err := mcpserver.New(cmd.Context(), c, version)
			if err != nil {
				return err
			}
			return server.Run(cmd.Context())
		},
	}
}
