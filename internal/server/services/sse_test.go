package services

import (
	"context"
	"testing"

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
