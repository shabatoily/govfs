// Package vfs는 CLI에서 VFS 기능을 제어하기 위한 핸들러와 커맨드를 제공합니다.
package vfs

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/jedib0t/go-pretty/v6/table"
	vfs "github.com/shabatoily/govfs"
	"github.com/spf13/cobra"
)

// RegisterCommands는 VFS와 관련된 모든 CLI 명령을 등록합니다.
func RegisterCommands(target *cobra.Command) {
	target.AddCommand(NewBackupCommand(), NewRestoreCommand(), NewRotateCommand(),
		NewListCommand(), NewTreeCommand(), NewStatCommand(), NewCopyCommand(),
		NewMkdirCommand(), NewRemoveCommand())
}

// NewBackupCommand는 VFS 데이터를 지정된 로컬 파일로 백업하는 커맨드를 반환합니다.
func NewBackupCommand() *cobra.Command {
	var backupFile string

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup govfs database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Backup(backupFile)
		},
	}

	backupCmd.Flags().StringVarP(&backupFile, "backup-file", "f",
		"data_%s.bak", "File to backup to")

	return backupCmd
}

// NewRestoreCommand는 지정된 로컬 백업 파일을 이용해 VFS 데이터를 복원하는 커맨드를 반환합니다.
func NewRestoreCommand() *cobra.Command {
	var restoreFile string
	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore govfs database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Restore(restoreFile)
		},
	}

	restoreCmd.Flags().StringVarP(&restoreFile, "backup-file", "f",
		"badger.bak", "File to restore from")

	return restoreCmd
}

// NewRotateCommand는 VFS 데이터의 암호화 키를 교체(Rotate)하는 커맨드를 반환합니다.
func NewRotateCommand() *cobra.Command {
	var newKeyPath string

	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate encryption key.",
		Long:  "Rotate will rotate the encryption key.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var keyContent []byte

			if newKeyPath != "" {
				content, err := os.ReadFile(newKeyPath)
				if err != nil {
					return err
				}
				keyContent = content
			} else {
				return fmt.Errorf("new key path is required")
			}

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Rotate(string(keyContent))
		},
	}

	cmd.Flags().StringVarP(&newKeyPath, "new-key-path", "n",
		"", "Path of the new key")
	return cmd
}

// NewListCommand는 VFS 상의 파일 및 디렉토리 목록을 표 형식으로 조회하는 커맨드를 반환합니다.
func NewListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ls <path>",
		Short: "Show file list",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			p := vfs.Root
			if len(args) > 0 {
				p = args[0]
			}

			metas, err := h.client.VFS().List(cmd.Context(), p)
			if err != nil {
				return err
			}

			w := table.NewWriter()
			appendTableHeaderForMeta(w)
			for i := range metas {
				appendTableRowFromMeta(w, &metas[i])
			}

			cmd.Println(w.Render())

			return nil
		},
	}
}

// NewTreeCommand는 특정 경로 내부의 파일 및 디렉토리 구조를 트리 형태로 출력하는 커맨드를 반환합니다.
func NewTreeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tree <path>",
		Short: "Show file tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			p := vfs.Root
			if len(args) > 0 {
				p = args[0]
			}

			tree, err := h.client.VFS().Tree(cmd.Context(), p)
			if err != nil {
				return err
			}

			w := list.NewWriter()
			w.SetStyle(list.StyleConnectedRounded)
			buildList(w, tree)
			cmd.Println(w.Render())
			return nil
		},
	}
}

// NewStatCommand는 지정한 메타데이터 ID(UUID)를 기반으로 파일 상태 정보를 조회하는 커맨드를 반환합니다.
func NewStatCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stat <id>",
		Short: "Show file metadata",
		Args:  cobra.MatchAll(cobra.RangeArgs(1, 1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}

			var id uuid.UUID
			if len(args) > 0 {
				id, err = uuid.Parse(args[0])
				if err != nil {
					return err
				}
			}

			stat, err := h.client.VFS().Stat(cmd.Context(), id)
			if err != nil {
				return err
			}

			w := table.NewWriter()
			appendTableHeaderForMeta(w)
			appendTableRowFromMeta(w, &stat)
			cmd.Println(w.Render())

			return nil
		},
	}
}

// NewCopyCommand는 로컬과 VFS 간, 혹은 VFS 내부의 파일 및 디렉토리를 복사하는 커맨드를 반환합니다.
// vfs: prefix를 사용하여 대상이 로컬인지 VFS환경인지 구분합니다.
func NewCopyCommand() *cobra.Command {
	var recursive bool

	cpCmd := &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "Copy files/directories between local and VFS",
		Long:  `Use 'vfs:' prefix for VFS paths (e.g., vfs:/etc/config.yaml)`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			src, dst := args[0], args[1]
			return h.Copy(src, dst, recursive)
		},
	}

	cpCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "copy directories recursively")

	return cpCmd
}

// NewMkdirCommand는 VFS 환경에 새로운 디렉토리를 생성하는 커맨드를 반환합니다.
func NewMkdirCommand() *cobra.Command {
	var mkdirParentsFlag bool
	cmd := &cobra.Command{
		Use:   "mkdir <path>",
		Short: "make directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := strings.TrimSpace(args[0])
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Mkdir(path, mkdirParentsFlag)
		},
	}

	cmd.Flags().BoolVarP(&mkdirParentsFlag, "parents", "p", false, "make parent directories")

	return cmd
}

// NewRemoveCommand는 VFS 환경의 특정 파일 또는 디렉토리를 삭제하는 커맨드를 반환합니다.
func NewRemoveCommand() *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "rm <path>",
		Short: "Remove a file or directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Remove(args[0], recursive)
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "recursive mode")

	return cmd
}
