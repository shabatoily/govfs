package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/meteormin/govfs/cli"
	"github.com/meteormin/govfs/config"
	"github.com/spf13/cobra"
)

func RegisterCommands(target *cobra.Command) *cobra.Command {
	cloudCmd := NewCloudCmd()
	subCmd := cli.NewCommands(cloudCmd)
	subCmd.Append(NewListCmd(), NewUploadCmd(), NewDownloadCmd())
	cmd := cli.NewCommands(target)
	return cmd.Append(cloudCmd)
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

			files, err := s.List("")
			if err != nil {
				return err
			}

			if len(files) == 0 {
				fmt.Println("empty directory")
				return nil
			}

			w := list.NewWriter()
			w.SetStyle(list.StyleConnectedRounded)

			items := make([]any, len(files))
			for i, f := range files {
				items[i] = f
			}

			w.AppendItems(items)

			fmt.Println(w.Render())

			return nil
		},
	}
}

func NewUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upload [upload path]",
		Short: "upload file to google drive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uploadFilePath := args[0]
			cfg := cmd.Context().Value(config.ContextKeyConfig{}).(*config.Config)
			s, err := NewStorage(cmd.Context(), &cfg.Cloud)
			if err != nil {
				return err
			}

			abs, err := filepath.Abs(uploadFilePath)
			if err != nil {
				return err
			}

			f, err := os.Open(abs)
			if err != nil {
				return err
			}
			defer f.Close()

			p := filepath.Base(uploadFilePath)
			err = s.Upload(p, f)
			if err != nil {
				return err
			}

			fmt.Printf("file uploaded: %s\n", uploadFilePath)
			return nil
		},
	}
}

func NewDownloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "download [path in google drive] [download path in local, default: ./]",
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

			p, err := filepath.Abs(downloadFilePath)
			if err != nil {
				return err
			}

			r, err := s.Download(driveFilePath)
			defer func() {
				_ = r.Close()
			}()

			if err != nil {
				return err
			}

			if stat, osErr := os.Stat(p); errors.Is(osErr, os.ErrNotExist) {
				if stat.IsDir() {
					downloadFilePath = filepath.Join(downloadFilePath, filepath.Base(driveFilePath))
				}
			}

			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}

			err = os.WriteFile(downloadFilePath, data, 0o600)
			if err != nil {
				return err
			}

			fmt.Printf("file downloaded: %s\n", downloadFilePath)
			return nil
		},
	}
}
