// Package client는 govfs 서버와 통신하기 위한 API 클라이언트를 제공합니다.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
)

// CloudClient는 클라우드 저장소 관련 API 통신을 담당하는 클라이언트입니다.
type CloudClient struct {
	*baseClient
}

// GoogleDriveAuthCodeURL은 Google Drive 인증을 위해 필요한 URL을 서버로부터 받아옵니다.
func (c *CloudClient) GoogleDriveAuthCodeURL(ctx context.Context) (string, error) {
	resp, err := c.c.Post("/cloud/googledrive/auth", client.Config{Ctx: ctx})
	if err != nil {
		return "", err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var payload map[string]string
	if err := resp.JSON(&payload); err != nil {
		return "", err
	}

	return payload["url"], nil
}

// IsAuthorized는 현재 클라우드 스토리지가 인증된 상태인지 확인합니다.
func (c *CloudClient) IsAuthorized(ctx context.Context) error {
	resp, err := c.c.Get("/cloud/googledrive/auth", client.Config{Ctx: ctx})
	if err != nil {
		return err
	}

	if resp.StatusCode() != fiber.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}

// List는 클라우드 저장소의 특정 경로에 있는 파일 목록을 조회합니다.
func (c *CloudClient) List(ctx context.Context, path string) ([]string, error) {
	u := fmt.Sprintf("/cloud?path=%s", path)
	resp, err := c.c.Get(u, client.Config{Ctx: ctx})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var files []string
	if err := resp.JSON(&files); err != nil {
		return nil, err
	}

	return files, nil
}

// Upload는 클라우드 저장소로 로컬 파일을 업로드합니다.
func (c *CloudClient) Upload(ctx context.Context, name string, r io.Reader) error {
	f := &client.File{}
	f.SetFieldName("file")
	// Fiber 클라이언트에서는 `r` 파라미터가 반드시 io.ReadCloser여야 합니다.
	// 일반 io.Reader인 경우 NopCloser로 감싸줍니다.
	rc, ok := r.(io.ReadCloser)
	if !ok {
		rc = io.NopCloser(r)
	}
	f.SetReader(rc)
	f.SetName(filepath.Base(name))

	resp, err := c.c.Post("/cloud", client.Config{
		Ctx:  ctx,
		File: []*client.File{f},
	})
	if err != nil {
		return err
	}

	if resp.StatusCode() != fiber.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}

// Download는 클라우드 저장소로부터 파일을 다운로드하여 Reader로 반환합니다.
func (c *CloudClient) Download(ctx context.Context, path string) (io.Reader, error) {
	u := fmt.Sprintf("/cloud/download?path=%s", path)
	resp, err := c.c.Post(u, client.Config{Ctx: ctx})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return bytes.NewReader(resp.Body()), nil
}

// Delete는 클라우드 저장소의 파일을 삭제합니다.
func (c *CloudClient) Delete(ctx context.Context, path string) error {
	u := fmt.Sprintf("/cloud?path=%s", path)
	resp, err := c.c.Delete(u, client.Config{Ctx: ctx})
	if err != nil {
		return err
	}

	if resp.StatusCode() != fiber.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}
