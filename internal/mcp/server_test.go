package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/shabatoily/govfs/internal/client"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloseStopsSubscription(t *testing.T) {
	disconnected := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		message := types.SSEMessage{ID: uuid.NewV4(), Event: types.SSEEventSubscribe}
		_, err := message.WriteTo(w)
		assert.NoError(t, err)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(disconnected)
	}))
	defer httpServer.Close()

	// 부모 컨텍스트가 유효한 상태에서도 서버 자체 종료로 구독이 해제되는지 확인합니다.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := New(ctx, client.New(httpServer.URL), "test")
	require.NoError(t, err)
	defer server.Close()
	server.Close()
	server.Close()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("SSE connection remained open")
	}
	select {
	case _, ok := <-server.events:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("SSE receiver did not stop")
	}
	require.NoError(t, ctx.Err())
}

func TestWaitForMutationFailures(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := (&Server{}).waitForMutation(ctx, "vfs.create")
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("closed stream", func(t *testing.T) {
		events := make(chan types.SSEMessage)
		close(events)
		_, err := (&Server{events: events}).waitForMutation(context.Background(), "vfs.create")
		require.ErrorContains(t, err, "stream closed")
	})
	t.Run("failed mutation", func(t *testing.T) {
		events := make(chan types.SSEMessage, 1)
		events <- types.SSEMessage{
			Event: types.SSEEventPublish,
			Data:  types.SSEData{Message: "file exists", Meta: types.SSEMeta{Action: "vfs.create"}},
		}
		_, err := (&Server{events: events}).waitForMutation(context.Background(), "vfs.create")
		require.ErrorContains(t, err, "file exists")
	})
}
