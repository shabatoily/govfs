package client

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/google/uuid"
	"github.com/meteormin/govfs/server/types"
)

type Client struct {
	c *client.Client
}

// Helper function to create JSON config
func createJSONConfig(data any) (client.Config, error) {
	jsonb, err := json.Marshal(data)
	if err != nil {
		return client.Config{}, err
	}
	return client.Config{
		Header: map[string]string{"Content-Type": "application/json"},
		Body:   jsonb,
	}, nil
}

// Helper function to check response status and unmarshal body
func checkResponse[T any](resp *client.Response, err error, expectedStatus int, out *T) error {
	if err != nil {
		return err
	}
	if resp.StatusCode() != expectedStatus {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}
	if out != nil {
		if err := resp.JSON(out); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Subscribe() (*client.Response, error) {
	return c.c.Get("/sse/subscribe")
}

func (c *Client) Publish(id uuid.UUID, data map[string]any) (*client.Response, error) {
	cfg, err := createJSONConfig(data)
	if err != nil {
		return nil, err
	}
	return c.c.Post("/sse/"+id.String()+"/publish", cfg)
}

// List lists files and directories
func (c *Client) List(q string) ([]types.MetaRes, error) {
	u := fmt.Sprintf("/vfs?q=%s", q)
	resp, err := c.c.Get(u)
	var res types.VfsRes[[]types.MetaRes]
	if checkErr := checkResponse(resp, err, fiber.StatusOK, &res); checkErr != nil {
		return nil, checkErr
	}
	return res.Payload, nil
}

// Read reads a file
func (c *Client) Read(id uuid.UUID) (io.Reader, types.MetaRes, error) {
	u := fmt.Sprintf("/vfs/%s", id.String())
	resp, err := c.c.Get(u)
	if err != nil {
		return nil, types.MetaRes{}, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, types.MetaRes{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	meta, err := c.Stat(id)
	if err != nil {
		return nil, types.MetaRes{}, err
	}

	return resp.BodyStream(), meta, nil
}

// Stat stats a file or directory
func (c *Client) Stat(id uuid.UUID) (types.MetaRes, error) {
	u := fmt.Sprintf("/vfs/%s/stat", id.String())
	resp, err := c.c.Get(u)
	var meta types.MetaRes
	if checkErr := checkResponse(resp, err, fiber.StatusOK, &meta); checkErr != nil {
		return types.MetaRes{}, checkErr
	}
	return meta, nil
}

// Tree returns the file tree
func (c *Client) Tree(path string) (*types.TreeNodeRes, error) {
	u := fmt.Sprintf("/vfs?q=%s&viewType=tree", path)
	resp, err := c.c.Get(u)
	var res types.VfsRes[*types.TreeNodeRes]
	if checkErr := checkResponse(resp, err, fiber.StatusOK, &res); checkErr != nil {
		return nil, checkErr
	}
	return res.Payload, nil
}

// CreateDir creates a new directory
func (c *Client) CreateDir(name string) (types.MetaRes, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("isDir", "true"); err != nil {
		return types.MetaRes{}, err
	}
	if err := writer.WriteField("name", name); err != nil {
		return types.MetaRes{}, err
	}
	if err := writer.Close(); err != nil {
		return types.MetaRes{}, err
	}

	resp, err := c.c.R().
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetRawBody(body.Bytes()).
		Post("/vfs")

	var meta types.MetaRes
	if checkErr := checkResponse(resp, err, fiber.StatusCreated, &meta); checkErr != nil {
		return types.MetaRes{}, checkErr
	}
	return meta, nil
}

// CreateFile creates a new file
func (c *Client) CreateFile(name string, r io.ReadCloser) (types.MetaRes, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("isDir", "false"); err != nil {
		return types.MetaRes{}, err
	}
	if err := writer.WriteField("name", name); err != nil {
		return types.MetaRes{}, err
	}

	part, err := writer.CreateFormFile("file", filepath.Base(name))
	if err != nil {
		return types.MetaRes{}, err
	}
	if _, copyErr := io.Copy(part, r); copyErr != nil {
		return types.MetaRes{}, copyErr
	}

	if closeErr := writer.Close(); closeErr != nil {
		return types.MetaRes{}, closeErr
	}

	resp, err := c.c.R().
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetRawBody(body.Bytes()).
		Post("/vfs")

	var meta types.MetaRes
	if checkErr := checkResponse(resp, err, fiber.StatusCreated, &meta); checkErr != nil {
		return types.MetaRes{}, checkErr
	}
	return meta, nil
}

// Write writes content to a file
func (c *Client) Write(id uuid.UUID, content string) error {
	cfg, err := createJSONConfig(types.WriteReq{Content: content})
	if err != nil {
		return err
	}

	resp, err := c.c.Put("/vfs/"+id.String(), cfg)
	var meta types.MetaRes
	return checkResponse(resp, err, fiber.StatusAccepted, &meta)
}

// Move renames or moves a file or directory
func (c *Client) Move(id uuid.UUID, dstName string) error {
	cfg, err := createJSONConfig(types.DstReq{Name: dstName})
	if err != nil {
		return err
	}
	// Move doesn't return a body we need to parse, passing nil
	resp, err := c.c.Patch("/vfs/"+id.String(), cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// Copy copies a file or directory
func (c *Client) Copy(id uuid.UUID, dstName string) error {
	cfg, err := createJSONConfig(types.DstReq{Name: dstName})
	if err != nil {
		return err
	}
	resp, err := c.c.Post("/vfs/"+id.String(), cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// Delete deletes a file or directory
func (c *Client) Delete(id uuid.UUID) error {
	resp, err := c.c.Delete("/vfs/" + id.String())
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// WriteComments writes comments to a file
func (c *Client) WriteComments(id uuid.UUID, comment string) error {
	cfg, err := createJSONConfig(types.WriteCommentReq{Comment: comment})
	if err != nil {
		return err
	}
	resp, err := c.c.Patch("/vfs/"+id.String()+"/comments", cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// Backup initiates a backup and returns the content
func (c *Client) Backup() (io.Reader, error) {
	resp, err := c.c.Post("/vfs/backup", client.Config{
		Header: map[string]string{
			"Content-Type": "application/json",
		},
	})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return bytes.NewReader(resp.Body()), nil
}

// CloudGoogleDriveAuthCodeURL returns the authentication URL for Google Drive.
func (c *Client) CloudGoogleDriveAuthCodeURL() (string, error) {
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

// CloudList lists files in the cloud storage.
func (c *Client) CloudList(path string) ([]string, error) {
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

// CloudUpload uploads a file to the cloud storage.
func (c *Client) CloudUpload(name string, r io.Reader) error {
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

// CloudDownload downloads a file from the cloud storage.
func (c *Client) CloudDownload(path string) (io.Reader, error) {
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

// CloudDelete deletes a file from the cloud storage.
func (c *Client) CloudDelete(path string) error {
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

// Restore restores the VFS from a backup file
func (c *Client) Restore(file io.ReadCloser) error {
	f := &client.File{}
	f.SetFieldName("file")
	f.SetReader(file)
	f.SetName("backup")

	// We don't need to manually read the file here.
	// The client.Client will handle reading from f.Reader (which is `file`)
	// and constructing the multipart request.

	resp, err := c.c.Post("/vfs/restore", client.Config{
		File: []*client.File{f},
	})

	return checkResponse[*any](resp, err, fiber.StatusOK, nil)
}

// Rotate rotates the encryption key
func (c *Client) Rotate(newKey string) error {
	cfg, err := createJSONConfig(map[string]string{"key": newKey})
	if err != nil {
		return err
	}
	resp, err := c.c.Post("/vfs/rotate", cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

func NewClient(url string) *Client {
	c := client.New()
	c.SetBaseURL(url)
	return &Client{c: c}
}

// SetToken manually sets the authorization token
func (c *Client) SetToken(token string) {
	if token != "" && token != "disabled" {
		c.c.AddHeader(fiber.HeaderAuthorization, "Bearer "+token)
	}
}

// Login authenticates with the server and configures the client to use the received token
func (c *Client) Login(username, password string) (types.TokenResponse, error) {
	var res types.TokenResponse

	req := types.LoginRequest{
		Username: username,
		Password: password,
	}

	cfg, err := createJSONConfig(req)
	if err != nil {
		return res, err
	}

	resp, err := c.c.Post("/auth/login", cfg)
	if err != nil {
		return res, err
	}

	status := resp.StatusCode()
	if status != fiber.StatusOK && status != fiber.StatusNoContent {
		return res, fmt.Errorf("login failed: %v", resp.StatusCode())
	} else if status == fiber.StatusNoContent {
		return res, nil
	}

	if err := resp.JSON(&res); err != nil {
		return res, err
	}

	c.SetToken(res.Token)
	// If no token but OK, it could be "Auth is disabled"
	return res, nil
}

func (c *Client) Me() (types.TokenResponse, error) {
	var res types.TokenResponse

	resp, err := c.c.Get("/auth/me")
	if err != nil {
		return res, err
	}

	status := resp.StatusCode()
	if status != fiber.StatusOK && status != fiber.StatusNoContent {
		return res, fmt.Errorf("not logged in: %v", resp.StatusCode())
	} else if status == fiber.StatusNoContent {
		return res, nil
	}

	if err := resp.JSON(&res); err != nil {
		return res, err
	}

	c.SetToken(res.Token)

	return res, nil
}
