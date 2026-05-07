// Package vfs는 CLI에서 VFS 기능을 제어하기 위한 핸들러와 커맨드를 제공합니다.
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
	"github.com/meteormin/govfs/internal/cli"
	"github.com/meteormin/govfs/internal/client"
	"github.com/meteormin/govfs/internal/types"
	"github.com/spf13/cobra"
)

// Handler는 CLI의 VFS 관련 명령 처리를 담당하는 구조체입니다.
type Handler struct {
	cmd    *cobra.Command
	client *client.Client
}

// NewHandler는 컨텍스트 기반으로 새로운 VFS 핸들러를 반환하며, 필요시 자동 로그인을 수행합니다.
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

		u.TokenInfo = cli.TokenInfo{TokenRes: t}
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

// Backup은 서버의 전체 VFS 데이터를 로컬 파일로 백업합니다.
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

// Restore는 로컬 백업 파일을 서버로 전송하여 VFS를 복구합니다.
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
			// 베이스 디렉토리 자체
			// dstVfs 디렉토리 생성 여부 판별:
			// srcLocal이 "foo"이고 dstVfs가 "/bar"일 때, "bar/foo/..." 구조를 원함.
			// 또는 dstVfs가 "/bar/"인 경우, "/bar/foo/..." 구조가 됨.
			// 만약 dstVfs가 "/bar" (이미 존재)인 경우, "/bar/foo/..." 구조가 됨.
			// 만약 dstVfs가 "/bar" (존재하지 않음)인 경우, "/bar/..." 구조 (이름 변경 방식)가 됨.

			// cp -r 명령어의 동작 방식을 채택:
			// dstVfs가 존재하고 디렉토리인 경우: 그 안으로 업로드 (`basename(srcLocal)` 폴더 생성)
			// dstVfs가 존재하지 않는 경우: 새로 생성 (`srcLocal`을 `dstVfs`로 복사)

			// 하지만 filepath.Walk는 루트부터 탐색을 시작합니다.
			// 루프 외부에서 dstVfs를 한 번 확인하는 것이 더 효율적입니다.
			return nil
		}

		// 단순화된 로직: dstVfs가 /로 끝나면 RelPath를 바로 이어붙임.
		// dstVfs가 존재하지 않으면 (또는 타겟 이름인 경우), 동작 방식을 확인.

		// `cp -r src dst`의 동작을 완벽하게 구현하는 것은 복잡함.
		// 더 단순한 방식으로 가정: `cp -r src_dir /vfs/path` 호출 시,
		// /vfs/path가 존재한다면 `/vfs/path/src_dir/file`을 생성하고,
		// 존재하지 않는 타겟 이름이라면 `/vfs/path/file`을 생성함.

		// 보다 간단한 의미론(semantic) 적용:

		targetPath := filepath.Join(dstVfs, relPath)
		// Windows 환경에서 filepath.Join은 백슬래시(\)를 사용하지만, VFS 경로는 항상 슬래시(/)를 사용해야 함.
		targetPath = filepath.ToSlash(targetPath)

		if info.IsDir() {
			_, err = h.client.VFS().CreateDir(targetPath)
			if err != nil {
				// Client의 CreateDir은 디렉토리가 이미 존재하면 에러를 반환함.
				// 여기서는 미리 존재 여부를 확인하거나 에러를 무시하는 방식으로 처리.
				// 현재는 에러를 잡아서 "파일이 존재함" 텍스트가 포함된 경우 무시할 수 있으나,
				// 명시적으로 존재 여부를 체크하는 것이 더 바람직함.
				if _, err = h.findMetaByPath(targetPath); err != nil {
					return err // 실제 에러 발생
				}
				// 이미 존재한다면 계속 진행
			}
			return nil
		}

		f, err := root.Open(relPath)
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

	// ID 파싱 처리? Client의 Read는 'id'로 UUID를 받음.
	// 따라서 경로(Path)를 먼저 ID로 변환해야 함.
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

	// 목적지(대상) 경로 처리
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

	// 트리(Tree) API 사용
	tree, err := h.client.VFS().Tree(srcVfs)
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
		fullLocalPath := currentLocalPath
		// 이곳이 재귀 탐색의 루트가 아니거나, 현재 노드에 대한 완전한 로컬 경로로 호출된 경우

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

// Mkdir은 VFS 상에 새로운 디렉토리를 생성합니다.
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
					// Client의 CreateDir은 디렉토리가 이미 존재하거나 실패하면 에러를 반환함.
					// 부모 폴더 생성을 옵션으로 둔 경우, "이미 존재함" 에러는 묵시적으로 무시하고 진행할 수 있음.
					// 커맨드 구현체에서는 루프의 마지막 요소가 아니면 에러를 무시하는 로직이었음:
					// `if err != nil { if i == lastIdx { return err } }`
					// 이는 중간 경로에 대해서는 "이미 존재함"을 전제로 에러를 넘긴다는 뜻임.
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

// Remove는 VFS 상의 파일 또는 디렉토리를 삭제합니다.
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
