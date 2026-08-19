// Package mcp는 govfs 기능을 MCP 도구로 제공합니다.
package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shabatoily/govfs/internal/client"
	"github.com/shabatoily/govfs/internal/types"
)

const mutationTimeout = 30 * time.Second

// Server는 MCP 도구와 비동기 VFS 완료 이벤트를 연결합니다.
type Server struct {
	client     *client.Client
	events     <-chan types.SSEMessage
	errors     <-chan error
	mutationMu sync.Mutex
	sdk        *mcpsdk.Server
}

// New는 SSE 구독을 열고 stdio에서 실행할 MCP 서버를 구성합니다.
func New(ctx context.Context, c *client.Client, version string) (*Server, error) {
	subscription, err := c.SSE().SubscribeEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe to completion events: %w", err)
	}
	c.SetClientID(subscription.ID)

	s := &Server{
		client: c,
		events: subscription.Events,
		errors: subscription.Errors,
		sdk: mcpsdk.NewServer(&mcpsdk.Implementation{
			Name:    "govfs",
			Version: version,
		}, nil),
	}
	s.registerTools()
	return s, nil
}

// Run은 MCP stdio transport를 실행합니다.
func (s *Server) Run(ctx context.Context) error {
	return s.sdk.Run(ctx, &mcpsdk.StdioTransport{})
}

func (s *Server) waitForMutation(ctx context.Context, action string) (types.SSEMeta, error) {
	timer := time.NewTimer(mutationTimeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return types.SSEMeta{}, ctx.Err()
		case <-timer.C:
			return types.SSEMeta{}, fmt.Errorf("%s completion timed out", action)
		case err, ok := <-s.errors:
			if !ok {
				s.errors = nil
				continue
			}
			return types.SSEMeta{}, fmt.Errorf("completion stream: %w", err)
		case event, ok := <-s.events:
			if !ok {
				return types.SSEMeta{}, fmt.Errorf("completion stream closed")
			}
			if event.Event != types.SSEEventPublish || event.Data.Meta.Action != action {
				continue
			}
			if !event.Data.Status {
				return types.SSEMeta{}, fmt.Errorf("%s failed: %s", action, event.Data.Message)
			}
			return event.Data.Meta, nil
		}
	}
}
