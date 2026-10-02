package cli

import (
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
			c, err := NewAuthenticatedClient(cmd)
			if err != nil {
				return err
			}

			server, err := mcpserver.New(cmd.Context(), c, version)
			if err != nil {
				return err
			}
			return server.Run(cmd.Context())
		},
	}
}
