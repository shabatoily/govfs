package client

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
)

type CloudClient struct {
	*baseClient
}

// GoogleDriveAuthCodeURL returns the authentication URL for Google Drive.
func (c *CloudClient) GoogleDriveAuthCodeURL() (string, error) {
	resp, err := c.c.Post("/cloud/googledrive/auth")
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

// List lists files in the cloud storage.
func (c *CloudClient) List(path string) ([]string, error) {
	u := fmt.Sprintf("/cloud?path=%s", path)
	resp, err := c.c.Get(u)
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

// Upload uploads a file to the cloud storage.
func (c *CloudClient) Upload(name string, r io.Reader) error {
	f := &client.File{}
	f.SetFieldName("file")
	// the `r` parameter must be an io.ReadCloser for the fiber client.
	// If it's merely an io.Reader, we need to wrap it.
	rc, ok := r.(io.ReadCloser)
	if !ok {
		rc = io.NopCloser(r)
	}
	f.SetReader(rc)
	f.SetName(filepath.Base(name))

	resp, err := c.c.Post("/cloud", client.Config{
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

// Download downloads a file from the cloud storage.
func (c *CloudClient) Download(path string) (io.Reader, error) {
	u := fmt.Sprintf("/cloud/download?path=%s", path)
	resp, err := c.c.Post(u)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return bytes.NewReader(resp.Body()), nil
}

// Delete deletes a file from the cloud storage.
func (c *CloudClient) Delete(path string) error {
	u := fmt.Sprintf("/cloud?path=%s", path)
	resp, err := c.c.Delete(u)
	if err != nil {
		return err
	}

	if resp.StatusCode() != fiber.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}
