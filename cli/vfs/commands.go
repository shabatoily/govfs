package vfs

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/meteormin/go-vfs"
	"github.com/meteormin/go-vfs/cli"
	"github.com/meteormin/go-vfs/client"
	"github.com/meteormin/go-vfs/config"
	"github.com/spf13/cobra"
)

func RegisterCommands(target *cobra.Command) *cobra.Command {
	c := cli.NewCommands(target)
	return c.Append(NewBackupCommand(), NewRestoreCommand(), NewRotateCommand(),
		NewListCommand(), NewTreeCommand(), NewStatCommand(), NewCopyCommand(),
		NewMkdirCommand(), NewRemoveCommand())
}

func getClient(cmd *cobra.Command) *client.Client {
	cfg := cmd.Context().Value(config.ContextKeyConfig{}).(*config.Config)
	host := cfg.Server.Host
	if host == "" {
		host = "localhost"
	}
	url := fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
	return client.NewClient(url)
}

func NewBackupCommand() *cobra.Command {
	var backupFile string

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup go-vfs database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := getClient(cmd)
			return Backup(c, backupFile)
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
		Short: "Restore go-vfs database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := getClient(cmd)
			return Restore(c, restoreFile)
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

			return Rotate(getClient(cmd), string(keyContent))
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
			c := getClient(cmd)
			p := vfs.Root
			if len(args) > 0 {
				p = args[0]
			}

			metas, err := c.List(p)
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
			c := getClient(cmd)
			p := vfs.Root
			if len(args) > 0 {
				p = args[0]
			}

			tree, err := c.Tree(p)
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
			c := getClient(cmd)
			var id uuid.UUID
			var err error
			if len(args) > 0 {
				id, err = uuid.Parse(args[0])
				if err != nil {
					return err
				}
			}

			stat, err := c.Stat(id)
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
			c := getClient(cmd)
			src, dst := args[0], args[1]
			return Copy(c, src, dst, recursive)
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
			c := getClient(cmd)
			return Mkdir(c, path, mkdirParentsFlag)
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
			c := getClient(cmd)
			return Remove(c, args[0], recursive)
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "recursive mode")

	return cmd
}
