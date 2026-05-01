// Package cloud는 CLI에서 클라우드 저장소 기능을 사용하기 위한 핸들러와 커맨드를 제공합니다.
package cloud

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jedib0t/go-pretty/v6/list"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/internal/cli"
	"github.com/meteormin/govfs/internal/client"
	"github.com/spf13/cobra"
)

// Handler는 CLI의 클라우드 관련 명령을 처리하는 핸들러 구조체입니다.
type Handler struct {
	cmd    *cobra.Command
	client *client.Client
}

// NewHandler는 컨텍스트의 설정을 기반으로 새로운 클라우드 핸들러를 생성합니다.
func NewHandler(cmd *cobra.Command) (*Handler, error) {
	u, ok := cmd.Context().Value(cli.ContextKeyUserConfig{}).(*cli.UserConfig)
	if !ok {
		return nil, fmt.Errorf("not found user config in context")
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

	// Check cloud authorization
	if err := c.Cloud().IsAuthorized(); err != nil {
		code, err := c.Cloud().GoogleDriveAuthCodeURL()
		if err != nil {
			return nil, fmt.Errorf("failed to get auth code URL: %w", err)
		}

		cmd.Printf("Please authorize by visiting the following URL: %s\n", code)
		cmd.Println("After authorization, please press Enter to continue.")

		_, _ = os.Stdin.Read(make([]byte, 1))

		// Check again
		if err := c.Cloud().IsAuthorized(); err != nil {
			return nil, fmt.Errorf("still not authorized after user action: %w", err)
		}
	}

	return &Handler{
		cmd:    cmd,
		client: c,
	}, nil
}

// List는 클라우드 저장소의 파일 목록을 출력합니다.
func (h *Handler) List(p string) error {
	files, err := h.client.Cloud().List(p)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		h.cmd.Println("empty directory")
		return nil
	}

	w := list.NewWriter()
	w.SetStyle(list.StyleConnectedRounded)

	items := make([]any, len(files))
	for i, f := range files {
		items[i] = f
	}

	w.AppendItems(items)

	h.cmd.Println(w.Render())

	return nil
}

// Upload는 로컬 파일을 클라우드 저장소로 업로드합니다.
func (h *Handler) Upload(uploadFilePath string) error {
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
	err = h.client.Cloud().Upload(p, f)
	if err != nil {
		return err
	}

	h.cmd.Printf("file uploaded: %s\n", uploadFilePath)

	return nil
}

// Download는 클라우드 저장소의 파일을 로컬로 다운로드합니다.
func (h *Handler) Download(src, dst string) error {
	p, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	r, err := h.client.Cloud().Download(src)
	if err != nil {
		return err
	}

	if stat, osErr := os.Stat(p); osErr == nil {
		if stat.IsDir() {
			p = filepath.Join(p, filepath.Base(src))
		}
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	err = os.WriteFile(p, data, vfs.DefaultFileMode)
	if err != nil {
		return err
	}

	h.cmd.Printf("file downloaded: %s\n", p)

	return nil
}
