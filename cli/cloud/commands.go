package cloud

import (
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
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}

			return h.List("")
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
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}

			return h.Upload(uploadFilePath)
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

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}

			return h.Download(driveFilePath, downloadFilePath)
		},
	}
}
