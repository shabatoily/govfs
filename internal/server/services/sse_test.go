package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/types"
)

func TestSSEBrokerClientsReturnsClientInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := NewSSEBroker(SSEConfig{
		Context:          ctx,
		MaxClientBuffer:  0,
		MaxMessageBuffer: 1,
	})
	defer b.Shutdown()

	msg, ch := b.Subscribe(types.SubscribeReq{
		Ctx:  ctx,
		Addr: "127.0.0.1",
		User: "admin",
	})
	if msg == nil || ch == nil {
		t.Fatal("subscribe failed")
	}

	clients := b.Clients()
	if len(clients) != 1 {
		t.Fatalf("clients length = %d, want 1", len(clients))
	}

	got := clients[0]
	if got.ID != msg.ID || got.CreatedAt.IsZero() || got.Addr != "127.0.0.1" || got.User != "admin" {
		t.Fatalf("client info = %+v, msg id = %s", got, msg.ID)
	}
}

func TestSSEBrokerSubscribeRegistersBeforePublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := NewSSEBroker(SSEConfig{
		Context:          ctx,
		MaxClientBuffer:  1,
		MaxMessageBuffer: 1,
	})
	defer b.Shutdown()

	msg, ch := b.Subscribe(types.SubscribeReq{Ctx: ctx})
	<-ch // 구독 완료 이벤트를 제거합니다.

	meta := types.SSEMeta{ID: uuid.New(), Action: "vfs.create"}
	b.Publish(msg.ID, &types.SSEData{Status: true, Meta: meta}, 0)

	select {
	case got := <-ch:
		if got.Data.Meta != meta {
			t.Fatalf("meta = %+v, want %+v", got.Data.Meta, meta)
		}
	case <-time.After(time.Second):
		t.Fatal("subscribed client did not receive message")
	}
}
