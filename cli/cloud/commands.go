package cloud

import (
	"github.com/meteormin/govfs/config"
	"github.com/spf13/cobra"
)

func RegisterCommands(target *cobra.Command) {
	cloudCmd := NewCloudCmd()
	cloudCmd.AddCommand(NewListCmd(), NewUploadCmd(), NewDownloadCmd())
	target.AddCommand(cloudCmd)
}

func NewCloudCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cloud",
		Short: "Connect cloud storage",
	}
}

func NewListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := cmd.Context().Value(config.ContextKeyConfig{}).(*config.Config)
			s, err := NewStorage(cmd.Context(), &cfg.Cloud)
			if err != nil {
				return err
			}

			return List(s, "")
		},
	}
}

func NewUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upload <upload path>",
		Short: "upload file to google drive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uploadFilePath := args[0]
			cfg := cmd.Context().Value(config.ContextKeyConfig{}).(*config.Config)
			s, err := NewStorage(cmd.Context(), &cfg.Cloud)
			if err != nil {
				return err
			}

			return Upload(s, uploadFilePath)
		},
	}
}

func NewDownloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "download <path in google drive> <download path in local, default: ./>",
		Short: "download file to google drive",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			driveFilePath := args[0]

			var downloadFilePath string
			if len(args) > 1 {
				downloadFilePath = args[1]
			} else {
				downloadFilePath = "./"
			}

			cfg := cmd.Context().Value(config.ContextKeyConfig{}).(*config.Config)
			s, err := NewStorage(cmd.Context(), &cfg.Cloud)
			if err != nil {
				return err
			}

			return Download(s, driveFilePath, downloadFilePath)
		},
	}
}
