package cli

import (
	"os"

	"github.com/shabatoily/govfs/internal/updater"
	"github.com/spf13/cobra"
)

func newUpdateCommand(version string) *cobra.Command {
	options := updater.Options{Version: version}
	command := &cobra.Command{
		Use:   "update",
		Short: "Update this local CLI from the official GitHub release",
		Long: "Update this local CLI. --server also updates govfs beside the CLI; " +
			"it does not update a remote server. Existing configuration and data are preserved.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := os.Executable()
			if err != nil {
				return err
			}
			options.Executable = path
			return updater.Run(cmd.Context(), options, cmd.OutOrStdout())
		},
	}
	command.Flags().BoolVar(&options.Server, "server", false, "also update the local govfs binary beside the CLI")
	command.Flags().BoolVar(&options.Check, "check", false, "show current CLI and latest release versions without changing files")
	return command
}
