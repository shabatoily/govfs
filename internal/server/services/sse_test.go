package services

import (
	"context"
	"testing"
	"time"
	"uuid"

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

	id, _, err := b.Subscribe(types.SubscribeReq{
		Ctx:  ctx,
		Addr: "127.0.0.1",
		User: "admin",
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	clients := b.Clients("admin")
	if len(clients) != 1 {
		t.Fatalf("clients length = %d, want 1", len(clients))
	}

	got := clients[0]
	if got.ID != id || got.CreatedAt.IsZero() || got.Addr != "127.0.0.1" || got.User != "admin" {
		t.Fatalf("client info = %+v, client id = %s", got, id)
	}
}

func TestSSEBrokerSubscribeStopped(t *testing.T) {
	b := NewSSEBroker(SSEConfig{})
	b.Shutdown()

	id, ch, err := b.Subscribe(types.SubscribeReq{Ctx: context.Background()})
	if err == nil || id != uuid.Nil() || ch != nil {
		t.Fatalf("subscribe stopped broker = (%v, %v, %v)", id, ch, err)
	}
}

func TestSSEBrokerClientCount(t *testing.T) {
	b := NewSSEBroker(SSEConfig{})
	defer b.Shutdown()
	var firstID uuid.UUID
	var first <-chan *types.SSEMessage
	for _, user := range []string{"first", "first", "second"} {
		id, ch, err := b.Subscribe(types.SubscribeReq{Ctx: context.Background(), User: user})
		if err != nil {
			t.Fatal(err)
		}
		<-ch
		if first == nil {
			firstID, first = id, ch
		}
	}
	for user, want := range map[string]int{"first": 2, "second": 1, "missing": 0} {
		if got := b.ClientCount(user); got != want || got != len(b.Clients(user)) {
			t.Fatalf("사용자 %s 연결 수 = %d, 예상 = %d", user, got, want)
		}
	}
	b.Unsubscribe(firstID)
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("연결 해제 처리가 완료되지 않았습니다")
	}
	if got := b.ClientCount("first"); got != 1 {
		t.Fatalf("연결 해제 후 연결 수 = %d", got)
	}
	b.Shutdown()
	if got := b.ClientCount("first"); got != 0 {
		t.Fatalf("종료 후 연결 수 = %d", got)
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

	id, ch, err := b.Subscribe(types.SubscribeReq{Ctx: ctx})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	<-ch // 구독 완료 이벤트를 제거합니다.

	meta := types.SSEMeta{ID: uuid.NewV4(), Action: "vfs.create"}
	b.Publish("", id, &types.SSEData{Status: true, Meta: meta}, 0)

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
	_, first, err := b.Subscribe(types.SubscribeReq{Ctx: ctx, User: "first"})
	if err != nil {
		t.Fatalf("subscribe first: %v", err)
	}
	_, second, err := b.Subscribe(types.SubscribeReq{Ctx: ctx, User: "second"})
	if err != nil {
		t.Fatalf("subscribe second: %v", err)
	}
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
