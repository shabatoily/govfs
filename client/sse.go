// Package client는 govfs 서버와 통신하기 위한 API 클라이언트를 제공합니다.
package client

import (
	"github.com/gofiber/fiber/v3/client"
	"github.com/google/uuid"
)

type SSEClient struct {
	*baseClient
}

// Subscribe는 서버의 SSE 이벤트 스트림을 구독합니다.
func (c *SSEClient) Subscribe() (*client.Response, error) {
	return c.c.Get("/sse/subscribe")
}

// Publish는 특정 ID를 대상으로 SSE 이벤트를 발행합니다.
func (c *SSEClient) Publish(id uuid.UUID, data map[string]any) (*client.Response, error) {
	cfg, err := createJSONConfig(data)
	if err != nil {
		return nil, err
	}
	return c.c.Post("/sse/"+id.String()+"/publish", cfg)
}
