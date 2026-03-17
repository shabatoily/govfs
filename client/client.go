package client

import (
	"fmt"
	"sync"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
)

type baseClient struct {
	mu sync.RWMutex // 읽기/쓰기 잠금 제어
	c  *client.Client
}

// SetToken manually sets the authorization token
func (c *baseClient) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token != "" && token != "disabled" {
		c.c.AddHeader(fiber.HeaderAuthorization, "Bearer "+token)
	}
}

type Client struct {
	*baseClient
	auth  *AuthClient
	cloud *CloudClient
	sse   *SSEClient
	vfs   *VFSClient
}

func (c *Client) Auth() *AuthClient {
	return c.auth
}

func (c *Client) Cloud() *CloudClient {
	return c.cloud
}

func (c *Client) SSE() *SSEClient {
	return c.sse
}

func (c *Client) VFS() *VFSClient {
	return c.vfs
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

func NewClient(url string) *Client {
	c := client.New()
	c.SetBaseURL(url)
	base := &baseClient{c: c}
	return &Client{
		baseClient: base,
		auth:       &AuthClient{baseClient: base},
		cloud:      &CloudClient{baseClient: base},
		sse:        &SSEClient{baseClient: base},
		vfs:        &VFSClient{baseClient: base},
	}
}
