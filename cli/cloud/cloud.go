package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jedib0t/go-pretty/v6/list"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/bootstrap"
	"github.com/meteormin/govfs/cli"
	"github.com/meteormin/govfs/client"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/config"
	"github.com/spf13/cobra"
)

type Handler struct {
	cmd    *cobra.Command
	client *client.Client
}

func NewHandler(cmd *cobra.Command) (*Handler, error) {
	u, ok := cmd.Context().Value(cli.ContextKeyUserConfig{}).(cli.UserConfig)
	if !ok {
		return nil, fmt.Errorf("not found user config in context")
	}

	c := client.NewClient(u.ServerURL)
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
		err = cli.SetUserConfig(&u)
		if err != nil {
			return nil, err
		}
	}

	return &Handler{
		cmd:    cmd,
		client: c,
	}, nil
}

func NewStorage(cfg *config.CloudConfig) (cloud.Storage, error) {
	return bootstrap.InitCloud(cfg)
}

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

func (h *Handler) Download(src, dst string) error {
	p, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	r, err := h.client.Cloud().Download(src)
	if err != nil {
		return err
	}

	if stat, osErr := os.Stat(p); errors.Is(osErr, os.ErrNotExist) {
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
