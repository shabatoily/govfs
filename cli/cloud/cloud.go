package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jedib0t/go-pretty/v6/list"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/bootstrap"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/config"
)

func NewStorage(ctx context.Context, cfg *config.CloudConfig) (cloud.Storage, error) {
	return bootstrap.InitCloud(ctx, cfg)
}

func List(s cloud.Storage, p string) error {
	files, err := s.List(p)
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
}

func Upload(s cloud.Storage, uploadFilePath string) error {
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
}

func Download(s cloud.Storage, src, dst string) error {
	p, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	r, err := s.Download(src)
	defer func() {
		_ = r.Close()
	}()

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

	fmt.Printf("file downloaded: %s\n", p)

	return nil
}
