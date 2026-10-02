// Package vfs는 CLI에서 VFS 기능을 제어하기 위한 핸들러와 커맨드를 제공합니다.
package vfs

import (
	"fmt"
	"io"
	"os"
	vfsPath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/jedib0t/go-pretty/v6/table"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/cli"
	"github.com/shabatoily/govfs/internal/client"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/spf13/cobra"
)

// Handler는 CLI의 VFS 관련 명령 처리를 담당하는 구조체입니다.
type Handler struct {
	cmd    *cobra.Command
	client *client.Client
}

// NewHandler는 로그인 세션을 사용하는 VFS 핸들러를 반환합니다.
func NewHandler(cmd *cobra.Command) (*Handler, error) {
	c, err := cli.NewAuthenticatedClient(cmd)
	if err != nil {
		return nil, err
	}

	return &Handler{
		cmd:    cmd,
		client: c,
	}, nil
}

// Backup은 서버의 전체 VFS 데이터를 로컬 파일로 백업합니다.
func (h *Handler) Backup(backupFile string) error {
	backupFileName := strings.ReplaceAll(backupFile, "%s", time.Now().Format("2006-01-02_15-04-05"))
	r, err := h.client.VFS().Backup(h.cmd.Context())
	if err != nil {
		return err
	}
	f, err := os.Create(backupFileName)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}

	h.cmd.Printf("Backup saved to %s\n", backupFileName)

	return nil
}

// Restore는 로컬 백업 파일을 서버로 전송하여 VFS를 복구합니다.
func (h *Handler) Restore(restoreFile string) error {
	f, err := os.Open(restoreFile)
	if err != nil {
		return err
	}
	defer f.Close()

	err = h.client.VFS().Restore(h.cmd.Context(), f)
	if err != nil {
		return err
	}

	h.cmd.Printf("Restore from %s\n", restoreFile)

	return nil
}

func (h *Handler) Rotate(newKey string) error {
	return h.client.VFS().Rotate(h.cmd.Context(), newKey)
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
		dstVfs = vfsPath.Join(dstVfs, info.Name())
	}

	// 대상이 VFS 디렉터리이면 원본 파일 이름을 덧붙입니다.
	meta, err := h.findMetaByPath(dstVfs)
	if err == nil && meta.IsDir {
		dstVfs = dstVfs + "/" + info.Name()
	}

	err = h.client.VFS().CreateFile(h.cmd.Context(), dstVfs, f)
	if err != nil {
		return err
	}
	h.cmd.Printf("Upload accepted: %s -> %s\n", srcLocal, dstVfs)
	return nil
}

func (h *Handler) handleRecursiveUpload(srcLocal, dstVfs string) error {
	var count int
	var totalBytes int64

	startTime := time.Now()

	root, err := os.OpenRoot(srcLocal)
	if err != nil {
		return err
	}
	defer root.Close()

	err = filepath.Walk(srcLocal, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcLocal, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		targetPath := vfsPath.Join(dstVfs, filepath.ToSlash(relPath))

		if info.IsDir() {
			err = h.client.VFS().CreateDir(h.cmd.Context(), targetPath)
			if err != nil {
				meta, lookupErr := h.findMetaByPath(targetPath)
				if lookupErr != nil || !meta.IsDir {
					return err
				}
			}
			return nil
		}

		f, err := root.Open(relPath)
		if err != nil {
			return err
		}
		defer f.Close()

		err = h.client.VFS().CreateFile(h.cmd.Context(), targetPath, f)
		if err != nil {
			return err
		}
		h.cmd.Printf("Upload accepted: %s -> %s\n", path, targetPath)
		count++
		totalBytes += info.Size()
		return nil
	})
	if err == nil {
		h.cmd.Printf("\nSummary: Accepted %d file uploads (%s) in %v\n",
			count, formatBytes(totalBytes), time.Since(startTime).Round(time.Millisecond))
	}
	return err
}

func (h *Handler) handleDownload(srcVfs, dstLocal string) error {
	if strings.HasSuffix(srcVfs, "/") {
		return fmt.Errorf("'%s' is a directory (use -r to copy directories)", srcVfs)
	}

	meta, err := h.findMetaByPath(srcVfs)
	if err != nil {
		return err
	}

	if meta.IsDir {
		return fmt.Errorf("'%s' is a directory (use -r to copy directories)", srcVfs)
	}

	// 목적지(대상) 경로 처리
	destPath := dstLocal
	info, err := os.Stat(dstLocal)
	if err == nil && info.IsDir() {
		destPath = filepath.Join(dstLocal, meta.Name)
	}

	if err := h.downloadFile(meta, destPath); err != nil {
		return err
	}
	return writeMeta(destPath, meta)
}

// downloadFile은 단일·재귀 다운로드에서 파일 쓰기와 종료 오류를 함께 처리합니다.
func (h *Handler) downloadFile(meta types.MetaRes, destPath string) error {
	reader, _, err := h.client.VFS().Read(h.cmd.Context(), meta.ID)
	if err != nil {
		return err
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	h.cmd.Printf("Download: %s -> %s (%d bytes)\n", meta.Path, destPath, meta.Size)
	return nil
}

func writeMeta(localPath string, meta types.MetaRes) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(localPath+".json", data, vfs.DefaultFileMode)
}

func (h *Handler) handleRecursiveDownload(srcVfs, dstLocal string) error {
	var count int
	var totalBytes int64
	startTime := time.Now()

	// 트리(Tree) API 사용
	tree, err := h.client.VFS().Tree(h.cmd.Context(), srcVfs)
	if err != nil {
		return err
	}

	// 목적지 최상위 경로 결정
	targetRoot := dstLocal
	info, err := os.Stat(dstLocal)
	if err == nil && info.IsDir() {
		// dstLocal이 이미 존재하고 디렉토리인 경우, 원본 이름을 사용하여 해당 폴더 내부로 다운로드
		targetRoot = filepath.Join(dstLocal, tree.Meta.Name)
	}

	var walker func(node *types.TreeNodeRes, currentLocalPath string) error
	walker = func(node *types.TreeNodeRes, currentLocalPath string) error {
		if node.Meta.IsDir {
			if mkdirErr := os.MkdirAll(currentLocalPath, vfs.DefaultDirMode); mkdirErr != nil {
				return mkdirErr
			}
			for _, child := range node.Children {
				childPath := filepath.Join(currentLocalPath, child.Meta.Name)
				if walkErr := walker(child, childPath); walkErr != nil {
					return fmt.Errorf("download %s: %w", child.Meta.Path, walkErr)
				}
			}
		} else {
			if err := h.downloadFile(node.Meta, currentLocalPath); err != nil {
				return err
			}
			count++
			totalBytes += node.Meta.Size
		}

		return writeMeta(currentLocalPath, node.Meta)
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

	metas, err := h.client.VFS().List(h.cmd.Context(), parentPath)
	if err != nil {
		return types.MetaRes{}, err
	}

	for i := range metas {
		if metas[i].Name == targetName {
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

// buildList는 트리 노드를 목록 출력에 추가합니다.
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

// Mkdir은 VFS 상에 새로운 디렉토리를 생성합니다.
func (h *Handler) Mkdir(path string, parents bool) error {
	if parents {
		parent := "/"
		for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
			if part == "" {
				continue
			}
			parent = vfsPath.Join(parent, part)
			if err := h.client.VFS().CreateDir(h.cmd.Context(), parent); err != nil {
				meta, lookupErr := h.findMetaByPath(parent)
				if lookupErr != nil || !meta.IsDir {
					return err
				}
				continue
			}
			h.cmd.Printf("Directory creation accepted: %s\n", parent)
		}
		return nil
	}
	err := h.client.VFS().CreateDir(h.cmd.Context(), path)
	if err == nil {
		h.cmd.Printf("Directory creation accepted: %s\n", path)
	}
	return err
}

// Remove는 VFS 상의 파일 또는 디렉토리를 삭제합니다.
func (h *Handler) Remove(path string, recursive bool) error {
	meta, err := h.findMetaByPath(path)
	if err != nil {
		return err
	}

	if meta.IsDir && !recursive {
		return fmt.Errorf("'%s' is a directory (use -r to remove directories)", path)
	}
	// 서버가 하위 항목을 함께 삭제하므로 디렉터리도 한 번만 요청합니다.
	if err := h.client.VFS().Delete(h.cmd.Context(), meta.ID); err != nil {
		return err
	}
	h.cmd.Printf("Removal accepted: %s\n", path)
	return nil
}

// Copy는 로컬과 VFS 간, 또는 VFS 내부에서 파일/디렉토리를 복사합니다.
func (h *Handler) Copy(src, dst string, recursive bool) error {
	srcIsVfs := strings.HasPrefix(src, "vfs:")
	dstIsVfs := strings.HasPrefix(dst, "vfs:")

	srcRaw := strings.TrimPrefix(src, "vfs:")
	dstRaw := strings.TrimPrefix(dst, "vfs:")

	switch {
	// 1. Local -> VFS (Upload)
	case !srcIsVfs && dstIsVfs:
		if recursive {
			// 원본(src)이 디렉토리인지 확인
			info, err := os.Stat(srcRaw)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				// 파일에 대해서 재귀(recursive) 플래그가 주어졌다면, 단순 파일 업로드로 처리
				return h.handleUpload(srcRaw, dstRaw)
			}

			// 실제 대상 위치의 최상위 경로를 결정
			// 대상(dstVfs)이 존재하고 디렉토리라면, 원본 이름(basename)을 덧붙임
			meta, err := h.findMetaByPath(dstRaw)
			if err == nil && meta.IsDir {
				dstRaw = strings.TrimSuffix(dstRaw, "/") + "/" + filepath.Base(srcRaw)
			}
			if err := h.client.VFS().CreateDir(h.cmd.Context(), dstRaw); err != nil {
				meta, lookupErr := h.findMetaByPath(dstRaw)
				if lookupErr != nil || !meta.IsDir {
					return err
				}
			}

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
		return h.client.VFS().Copy(h.cmd.Context(), srcMeta.ID, dstRaw)
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
