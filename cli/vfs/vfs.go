package vfs

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/jedib0t/go-pretty/v6/table"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/cli"
	"github.com/meteormin/govfs/client"
	"github.com/meteormin/govfs/server/types"
	"github.com/spf13/cobra"
)

type Handler struct {
	cmd    *cobra.Command
	client *client.Client
}

func NewHandler(cmd *cobra.Command) (*Handler, error) {
	u, ok := cmd.Context().Value(cli.ContextKeyUserConfig{}).(*cli.UserConfig)
	if !ok {
		return nil, errors.New("config not found")
	}

	c := client.New(u.ServerURL)
	if _, err := c.Auth().Me(); err != nil {
		if !u.TokenInfo.IsExpired() {
			c.SetToken(u.TokenInfo.Token)
		}

		t, err := c.Auth().Login(u.Username, u.Password)
		if err != nil {
			return nil, err
		}

		c.SetToken(t.Token)

		u.TokenInfo = cli.TokenInfo{TokenResponse: t}
		err = cli.SetUserConfig(u)
		if err != nil {
			return nil, err
		}
	}

	return &Handler{
		cmd:    cmd,
		client: c,
	}, nil
}

func (h *Handler) Backup(backupFile string) error {
	backupFileName := fmt.Sprintf(backupFile, time.Now().Format("2006-01-02_15-04-05"))
	f, err := os.Create(backupFileName)
	if err != nil {
		return err
	}
	defer f.Close()

	r, err := h.client.VFS().Backup()
	if err != nil {
		return err
	}

	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}

	h.cmd.Printf("Backup saved to %s\n", backupFileName)

	return nil
}

func (h *Handler) Restore(restoreFile string) error {
	f, err := os.Open(restoreFile)
	if err != nil {
		return err
	}
	defer f.Close()

	err = h.client.VFS().Restore(f)
	if err != nil {
		return err
	}

	h.cmd.Printf("Restore from %s\n", restoreFile)

	return nil
}

func (h *Handler) Rotate(newKey string) error {
	return h.client.VFS().Rotate(newKey)
}

func (h *Handler) handleUpload(srcLocal, dstVfs string) error {
	info, err := os.Stat(srcLocal)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("'%s' is a directory (use -r to copy directories)", srcLocal)
	}

	f, err := os.Open(srcLocal)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}
	defer f.Close()

	if strings.HasSuffix(dstVfs, "/") {
		dstVfs = filepath.Join(dstVfs, info.Name())
	}

	// Check if dstVfs is a directory on VFS
	meta, err := h.findMetaByPath(dstVfs)
	if err == nil && meta.IsDir {
		dstVfs = dstVfs + "/" + info.Name()
	}

	res, err := h.client.VFS().CreateFile(dstVfs, f)
	if err != nil {
		return err
	}
	h.cmd.Printf("Upload: %s -> %s (%d bytes)\n", srcLocal, dstVfs, res.Size)
	return nil
}

func (h *Handler) handleRecursiveUpload(srcLocal, dstVfs string) error {
	var count int
	var totalBytes int64
	startTime := time.Now()

	err := filepath.Walk(srcLocal, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcLocal, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			// Base directory itself
			// Create dstVfs directory?
			// If srcLocal is "foo", and dstVfs is "/bar", we want "bar/foo/..."
			// Or if dstVfs is "/bar/", we want "/bar/foo/..."
			// If dstVfs is "/bar" (exists), we want "/bar/foo/..."
			// If dstVfs is "/bar" (does not exist), we want "/bar/..." (rename behavior)

			// Let's adopt cp -r behavior:
			// If dstVfs exists and is dir: Upload into it (create dir `basename(srcLocal)`)
			// If dstVfs does not exist: Create it (copy `srcLocal` to `dstVfs`)

			// But filepath.Walk starts at root.
			// Checking dstVfs once outside loop is better.
			return nil
		}

		// Simplified logic: If dstVfs ends with /, append RelPath directly.
		// If dstVfs does not exist (or is target name), verify interaction.

		// To replicate `cp -r src dst` semantics fully is complex.
		// Let's assume simpler: `cp -r src_dir /vfs/path` -> creates `/vfs/path/src_dir/file` IF /vfs/path exists
		// OR creates `/vfs/path/file` if `/vfs/path` is the target name.

		// Let's implement simpler semantic:
		// Always require user to specify full target path or directory.
		// Actually, let's look at `findMetaByPath`.

		targetPath := filepath.Join(dstVfs, relPath)
		// On windows filepath.Join uses backslash. We must ensure VFS paths are slashed.
		targetPath = filepath.ToSlash(targetPath)

		if info.IsDir() {
			_, err = h.client.VFS().CreateDir(targetPath)
			if err != nil {
				// Ignore "already exists" error if possible, or client should handle
				// Client CreateDir returns error on existing.
				// We can check existence first or ignore error.
				// For now, let's catch error and ignore if "file exists" string?
				// Better: check existence.
				if _, err = h.findMetaByPath(targetPath); err != nil {
					return err // Real error
				}
				// If exists, continue
			}
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		res, err := h.client.VFS().CreateFile(targetPath, f)
		if err != nil {
			return err
		}
		h.cmd.Printf("Upload: %s -> %s (%d bytes)\n", path, targetPath, res.Size)
		count++
		totalBytes += res.Size
		return nil
	})
	if err == nil {
		h.cmd.Printf("\nSummary: Uploaded %d files (%s) in %v\n", count, formatBytes(totalBytes), time.Since(startTime).Round(time.Millisecond))
	}
	return err
}

func (h *Handler) handleDownload(srcVfs, dstLocal string) error {
	if strings.HasSuffix(srcVfs, "/") {
		return fmt.Errorf("'%s' is a directory (use -r to copy directories)", srcVfs)
	}

	// ParseID logic? Client Read takes UUID using 'id'.
	// We need to resolve Path to ID first.
	meta, err := h.findMetaByPath(srcVfs)
	if err != nil {
		return err
	}

	if meta.IsDir {
		return fmt.Errorf("'%s' is a directory (use -r to copy directories)", srcVfs)
	}

	reader, _, err := h.client.VFS().Read(meta.ID)
	if err != nil {
		return err
	}

	// Handle destination
	destPath := dstLocal
	info, err := os.Stat(dstLocal)
	if err == nil && info.IsDir() {
		destPath = filepath.Join(dstLocal, meta.Name)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return err
	}

	h.cmd.Printf("Download: %s -> %s (%d bytes)\n", srcVfs, destPath, meta.Size)

	metaFilePath := destPath + ".json"
	metaFileJson, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	return os.WriteFile(metaFilePath, metaFileJson, vfs.DefaultFileMode)
}

func (h *Handler) handleRecursiveDownload(srcVfs, dstLocal string) error {
	var count int
	var totalBytes int64
	startTime := time.Now()

	// Use Tree API
	tree, err := h.client.VFS().Tree(srcVfs)
	if err != nil {
		return err
	}

	// Determine target root path
	targetRoot := dstLocal
	info, err := os.Stat(dstLocal)
	if err == nil && info.IsDir() {
		// If dstLocal exists and is directory, download INTO it using source name
		targetRoot = filepath.Join(dstLocal, tree.Meta.Name)
	}

	var walker func(node *types.TreeNodeRes, currentLocalPath string) error
	walker = func(node *types.TreeNodeRes, currentLocalPath string) error {
		fullLocalPath := currentLocalPath
		// If this is not the root of recursion (or if we changed logic),
		// actually walker is called with full path for THIS node.

		if node.Meta.IsDir {
			if mkdirErr := os.MkdirAll(fullLocalPath, vfs.DefaultDirMode); mkdirErr != nil {
				return mkdirErr
			}
			for _, child := range node.Children {
				childPath := filepath.Join(fullLocalPath, child.Meta.Name)
				if walkErr := walker(child, childPath); walkErr != nil {
					log.Printf("error downloading %s: %s", child.Meta.ID, walkErr.Error())
				}
			}
		} else {
			reader, _, readErr := h.client.VFS().Read(node.Meta.ID)
			if readErr != nil {
				return readErr
			}

			f, createErr := os.Create(fullLocalPath)
			if createErr != nil {
				return createErr
			}
			defer f.Close()

			_, copyErr := io.Copy(f, reader)
			if copyErr != nil {
				return copyErr
			}
			h.cmd.Printf("Download: %s -> %s (%d bytes)\n", node.Meta.Path, fullLocalPath, node.Meta.Size)
			count++
			totalBytes += node.Meta.Size
		}

		metaFilePath := fullLocalPath + ".json"
		metaFileJson, marshalErr := json.Marshal(node.Meta)
		if marshalErr != nil {
			return marshalErr
		}

		return os.WriteFile(metaFilePath, metaFileJson, vfs.DefaultFileMode)
	}

	err = walker(tree, targetRoot)
	if err == nil {
		h.cmd.Printf("\nSummary: Downloaded %d files (%s) in %v\n", count, formatBytes(totalBytes), time.Since(startTime).Round(time.Millisecond))
	}
	return err
}

func (h *Handler) findMetaByPath(path string) (types.MetaRes, error) {
	cleanPath := strings.Trim(path, "/")
	if cleanPath == "" {
		cleanPath = vfs.Root
	}

	var parentPath string
	var targetName string

	lastSlash := strings.LastIndex(cleanPath, "/")
	if lastSlash == -1 {
		parentPath = "/"
		targetName = cleanPath
	} else {
		parentPath = "/" + cleanPath[:lastSlash]
		targetName = cleanPath[lastSlash+1:]
	}

	metas, err := h.client.VFS().List(parentPath)
	if err != nil {
		return types.MetaRes{}, err
	}

	for i := range metas {
		fullName := metas[i].Name + metas[i].Extension
		if fullName == targetName || metas[i].Name == targetName {
			return metas[i], nil
		}
	}

	return types.MetaRes{}, fmt.Errorf("cannot find '%s' in path", targetName)
}

func appendTableHeaderForMeta(w table.Writer) {
	w.AppendHeader(table.Row{"ID", "Path", "Name", "Extension", "Size", "IsDir", "Modified"})
}

func appendTableRowFromMeta(w table.Writer, meta *types.MetaRes) {
	w.AppendRow(table.Row{meta.ID, meta.Path, meta.Name, meta.Extension, meta.Size, meta.IsDir, meta.Modified})
}

// buildList adds tree nodes to list.Writer
func buildList(l list.Writer, node *types.TreeNodeRes) {
	if node == nil {
		return
	}

	item := node.Meta.Name
	if node.Meta.Path != vfs.Root && node.Meta.IsDir {
		item += "/"
	}
	l.AppendItem(item)

	if len(node.Children) > 0 {
		l.Indent()
		for _, child := range node.Children {
			buildList(l, child)
		}
		l.UnIndent()
	}
}

func (h *Handler) Mkdir(path string, parents bool) error {
	if parents {
		var paths []string
		if strings.HasPrefix(path, "/") {
			paths = strings.Split(path, "/")[1:]
		} else {
			paths = strings.Split(path, "/")
		}

		parent := "/"
		lastIdx := len(paths) - 1
		for i, p := range paths {
			if p != "" {
				target := filepath.Join(parent, p)
				_, err := h.client.VFS().CreateDir(target)
				parent = target
				if err != nil {
					// Client CreateDir returns error if exists or failed.
					// If parents is true, we tolerate "exists" errors implicitly by continuing?
					// Or strictly check? The command implementation ignored error if it wasn't the last one?
					// Actually command impl: `if err != nil { if i == lastIdx { return err } }`
					// This implies ignoring intermediate errors (assuming they might be "already exists").
					if i == lastIdx {
						return err
					}
				} else {
					h.cmd.Printf("Created directory: %s\n", target)
				}
			}
		}
		return nil
	}
	_, err := h.client.VFS().CreateDir(path)
	if err == nil {
		h.cmd.Printf("Created directory: %s\n", path)
	}
	return err
}

func (h *Handler) Remove(path string, recursive bool) error {
	meta, err := h.findMetaByPath(path)
	if err != nil {
		return err
	}

	if !recursive {
		delErr := h.client.VFS().Delete(meta.ID)
		if delErr == nil {
			h.cmd.Printf("Removed: %s\n", path)
		}
		return delErr
	}

	if !meta.IsDir {
		delErr := h.client.VFS().Delete(meta.ID)
		if delErr == nil {
			h.cmd.Printf("Removed: %s\n", path)
		}
		return delErr
	}

	treeRes, treeErr := h.client.VFS().Tree(path)
	if treeErr != nil {
		return treeErr
	}

	var walker func(node *types.TreeNodeRes) error
	walker = func(node *types.TreeNodeRes) error {
		for _, child := range node.Children {
			if walkErr := walker(child); walkErr != nil {
				return walkErr
			}
		}
		if delErr := h.client.VFS().Delete(node.Meta.ID); delErr != nil {
			return delErr
		}
		h.cmd.Printf("Removed: %s\n", node.Meta.Path)
		return nil
	}

	return walker(treeRes)
}

func (h *Handler) Copy(src, dst string, recursive bool) error {
	srcIsVfs := strings.HasPrefix(src, "vfs:")
	dstIsVfs := strings.HasPrefix(dst, "vfs:")

	srcRaw := strings.TrimPrefix(src, "vfs:")
	dstRaw := strings.TrimPrefix(dst, "vfs:")

	switch {
	// 1. Local -> VFS (Upload)
	case !srcIsVfs && dstIsVfs:
		if recursive {
			// Check if src is dir
			info, err := os.Stat(srcRaw)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				// Recursive flag on file? Just upload file.
				return h.handleUpload(srcRaw, dstRaw)
			}

			// Determine actual destination root
			// If dstVfs exists and is dir, append basename(srcRaw)
			meta, err := h.findMetaByPath(dstRaw)
			if err == nil && meta.IsDir {
				dstRaw = strings.TrimSuffix(dstRaw, "/") + "/" + filepath.Base(srcRaw)
			}
			_, _ = h.client.VFS().CreateDir(dstRaw)

			return h.handleRecursiveUpload(srcRaw, dstRaw)
		}
		return h.handleUpload(srcRaw, dstRaw)

	// 2. VFS -> Local (Download)
	case srcIsVfs && !dstIsVfs:
		if recursive {
			return h.handleRecursiveDownload(srcRaw, dstRaw)
		}
		return h.handleDownload(srcRaw, dstRaw)

	// 3. VFS -> VFS (Internal Copy)
	case srcIsVfs && dstIsVfs:
		srcMeta, err := h.findMetaByPath(srcRaw)
		if err != nil {
			return err
		}
		return h.client.VFS().Copy(srcMeta.ID, dstRaw)
	default:
		return fmt.Errorf("local to local copy is not supported by this tool")
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
