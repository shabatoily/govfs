package vfs

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/jedib0t/go-pretty/v6/table"
	vfs "github.com/meteormin/govfs"
	"github.com/spf13/cobra"
)

func RegisterCommands(target *cobra.Command) {
	target.AddCommand(NewBackupCommand(), NewRestoreCommand(), NewRotateCommand(),
		NewListCommand(), NewTreeCommand(), NewStatCommand(), NewCopyCommand(),
		NewMkdirCommand(), NewRemoveCommand())
}

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

			metas, err := h.client.VFS().List(p)
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

			tree, err := h.client.VFS().Tree(p)
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

			stat, err := h.client.VFS().Stat(id)
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
