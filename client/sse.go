package client

import (
	"github.com/gofiber/fiber/v3/client"
	"github.com/google/uuid"
)

type SSEClient struct {
	*baseClient
}

func (c *SSEClient) Subscribe() (*client.Response, error) {
	return c.c.Get("/sse/subscribe")
}

func (c *SSEClient) Publish(id uuid.UUID, data map[string]any) (*client.Response, error) {
	cfg, err := createJSONConfig(data)
	if err != nil {
		return nil, err
	}
	return c.c.Post("/sse/"+id.String()+"/publish", cfg)
}
