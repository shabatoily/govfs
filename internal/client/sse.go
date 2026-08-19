// Package client는 govfs 서버와 통신하기 위한 API 클라이언트를 제공합니다.
package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/types"
)

// SSEClient는 Server-Sent Events(SSE) 스트림 연결 및 이벤트 발행을 처리하는 클라이언트입니다.
type SSEClient struct {
	*baseClient
}

// SSESubscription은 구독 ID와 이후 수신되는 이벤트를 제공합니다.
type SSESubscription struct {
	ID     uuid.UUID
	Events <-chan types.SSEMessage
	Errors <-chan error
}

// Subscribe는 서버의 SSE 이벤트 스트림을 구독합니다.
func (c *SSEClient) Subscribe() (*client.Response, error) {
	return c.c.Get("/sse/subscribe")
}

// SubscribeEvents는 취소 가능한 SSE 연결을 열고 이벤트를 파싱합니다.
func (c *SSEClient) SubscribeEvents(ctx context.Context) (*SSESubscription, error) {
	c.mu.RLock()
	serverURL, token := c.url, c.token
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/sse/subscribe", nil)
	if err != nil {
		return nil, err
	}
	if token != "" && token != "disabled" {
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	first, err := readSSEMessage(reader)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("read subscribe event: %w", err)
	}
	if first.Event != types.SSEEventSubscribe || first.ID == uuid.Nil {
		resp.Body.Close()
		return nil, errors.New("invalid subscribe event")
	}

	events := make(chan types.SSEMessage)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		defer resp.Body.Close()
		for {
			message, readErr := readSSEMessage(reader)
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) && ctx.Err() == nil {
					errs <- readErr
				}
				return
			}
			select {
			case events <- message:
			case <-ctx.Done():
				return
			}
		}
	}()

	return &SSESubscription{ID: first.ID, Events: events, Errors: errs}, nil
}

func readSSEMessage(reader *bufio.Reader) (types.SSEMessage, error) {
	var message types.SSEMessage
	var data strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return types.SSEMessage{}, err
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if data.Len() == 0 {
				continue
			}
			if err := json.Unmarshal([]byte(data.String()), &message.Data); err != nil {
				return types.SSEMessage{}, err
			}
			return message, nil
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "id":
			message.ID, err = uuid.Parse(value)
			if err != nil {
				return types.SSEMessage{}, err
			}
		case "event":
			message.Event = types.SSEEvent(value)
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
}

// Publish는 특정 ID를 대상으로 SSE 이벤트를 발행합니다.
func (c *SSEClient) Publish(id uuid.UUID, data map[string]any) error {
	cfg, err := createJSONConfig(data)
	if err != nil {
		return err
	}
	res, err := c.c.Post("/sse/"+id.String()+"/publish", cfg)
	if err != nil {
		return err
	}

	if res.StatusCode() != fiber.StatusNoContent {
		return errors.New(string(res.Body()))
	}

	return nil
}

// Clients는 현재 활성화된 모든 SSE 클라이언트 목록을 서버로부터 조회합니다.
func (c *SSEClient) Clients() (types.ClientList, error) {
	res, err := c.c.Get("/sse/clients")
	if err != nil {
		return types.ClientList{}, err
	}

	var clientList types.ClientList
	if checkErr := checkResponse(res, fiber.StatusOK, &clientList); checkErr != nil {
		return types.ClientList{}, checkErr
	}

	return clientList, nil
}
