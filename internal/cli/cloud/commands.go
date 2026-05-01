// Package cloud는 CLI에서 클라우드 저장소 기능을 사용하기 위한 핸들러와 커맨드를 제공합니다.
package cloud

import (
	"github.com/spf13/cobra"
)

// RegisterCommands는 `cloud`와 관련된 모든 하위 명령을 등록합니다.
func RegisterCommands(target *cobra.Command) {
	cloudCmd := NewCloudCmd()
	cloudCmd.AddCommand(NewListCmd(), NewUploadCmd(), NewDownloadCmd())
	target.AddCommand(cloudCmd)
}

// NewCloudCmd는 `cloud` 명령 그룹을 생성합니다.
func NewCloudCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cloud",
		Short: "Connect cloud storage",
	}
}

// NewListCmd는 클라우드 저장소의 최상위 또는 지정된 경로의 파일 목록을 조회하는 커맨드를 반환합니다.
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

// NewUploadCmd는 로컬에 존재하는 파일을 클라우드 저장소 특정 경로로 업로드하는 커맨드를 반환합니다.
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

// NewDownloadCmd는 클라우드 저장소에 있는 파일을 로컬 지정 경로로 다운로드하는 커맨드를 반환합니다.
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
