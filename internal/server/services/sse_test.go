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

	clients := b.Clients("admin")
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
	b.Publish("", msg.ID, &types.SSEData{Status: true, Meta: meta}, 0)

	select {
	case got := <-ch:
		if got.Data.Meta != meta {
			t.Fatalf("meta = %+v, want %+v", got.Data.Meta, meta)
		}
	case <-time.After(time.Second):
		t.Fatal("subscribed client did not receive message")
	}
}

func TestSSEBrokerSeparatesUsers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := NewSSEBroker(SSEConfig{Context: ctx, MaxClientBuffer: 2, MaxMessageBuffer: 1})
	defer b.Shutdown()
	_, first := b.Subscribe(types.SubscribeReq{Ctx: ctx, User: "first"})
	_, second := b.Subscribe(types.SubscribeReq{Ctx: ctx, User: "second"})
	<-first
	<-second

	b.Broadcast("first", &types.SSEData{Status: true}, 0)
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("첫 번째 사용자가 이벤트를 받지 못했습니다")
	}
	select {
	case <-second:
		t.Fatal("다른 사용자의 이벤트가 전달되었습니다")
	case <-time.After(20 * time.Millisecond):
	}
}
