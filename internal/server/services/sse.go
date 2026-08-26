// Package services는 서버의 핵심 비즈니스 로직을 제공합니다.
package services

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"

	"github.com/shabatoily/govfs/internal/types"
)

const (
	defaultBufferSize = 100
)

// SSEConfig는 SSEBroker 생성 및 초기화 시 사용할 설정 값들을 정의하는 구조체입니다.
type SSEConfig struct {
	Context          context.Context
	MaxClientBuffer  int
	MaxMessageBuffer int
}

// SSEBroker는 여러 클라이언트에게 실시간 메시지를 중계하는 역할을 합니다.
type SSEBroker struct {
	// 새 클라이언트 채널을 등록
	newClients chan *client
	// 끊긴 클라이언트 채널을 제거
	closingClients chan uuid.UUID
	// 메시지를 모든 클라이언트에게 전달
	message chan publication
	// 클라이언트 목록 요청 채널
	listClients chan listRequest
	// 현재 활성화된 클라이언트 목록
	clients map[uuid.UUID]*client
	// 브로커 컨텍스트 (서비스 종료 시 사용)
	ctx    context.Context
	cancel context.CancelFunc
	// 실행 상태
	isRunning atomic.Bool
}

type publication struct {
	user string
	msg  *types.SSEMessage
}

type listRequest struct {
	user string
	ch   chan []types.ClientInfo
}

type client struct {
	types.ClientInfo
	ch      chan *types.SSEMessage
	initial *types.SSEMessage
	ready   chan struct{}
}

// Subscribe는 새로운 클라이언트를 브로커에 등록하고 구독 성공 메시지를 반환합니다.
func (b *SSEBroker) Subscribe(req types.SubscribeReq) (*types.SSEMessage, <-chan *types.SSEMessage, error) {
	if !b.isRunning.Load() {
		log.Warn("Attempted to subscribe to stopped SSE Broker")
		return nil, nil, errors.New("SSE broker is not running")
	}

	// 구독 시 채널 생성을 브로커가 담당하여 일관성을 유지합니다.
	// 버퍼를 두어 일시적인 블로킹을 방지합니다.
	ch := make(chan *types.SSEMessage, defaultBufferSize)
	id := uuid.New()
	now := time.Now()
	subMsg := &types.SSEMessage{
		ID:    id,
		Event: types.SSEEventSubscribe,
		Data: types.SSEData{
			Timestamp: now,
			Status:    true,
			Message:   "Subscribed successfully",
		},
	}

	newClient := &client{
		ClientInfo: types.ClientInfo{
			ID:        id,
			CreatedAt: now,
			Addr:      req.Addr,
			User:      req.User,
		},
		ch:      ch,
		initial: subMsg,
		ready:   make(chan struct{}),
	}
	select {
	case b.newClients <- newClient:
	case <-b.ctx.Done():
		log.Info("SSE Broker is shutting down, cannot subscribe")
		return nil, nil, errors.New("SSE broker is shutting down")
	}

	select {
	case <-newClient.ready:
		log.Infof("SSE Broker subscribed: %s", id)
	case <-b.ctx.Done():
		return nil, nil, errors.New("SSE broker is shutting down")
	}

	// 클라이언트 컨텍스트가 취소(연결 끊김)되면 자동으로 Unsubscribe 호출
	context.AfterFunc(req.Ctx, func() {
		b.Unsubscribe(id)
	})

	log.Infof("Initial message sent: %s", subMsg.ID)
	return subMsg, ch, nil
}

// Unsubscribe는 브로커에서 클라이언트를 제거하고 관련 리소스를 정리합니다.
func (b *SSEBroker) Unsubscribe(id uuid.UUID) {
	// Broker가 실행 중이 아니면 무시
	if !b.isRunning.Load() {
		return
	}

	select {
	case b.closingClients <- id:
		log.Infof("SSE Broker unsubscribed: %s", id)
	case <-b.ctx.Done():
		log.Info("SSE Broker is shutting down, cannot unsubscribe")
	}
}

// Hearbeat는 클라이언트 연결 유지를 위해 하트비트 메시지를 전송합니다.
func (b *SSEBroker) Hearbeat(user string, id uuid.UUID) {
	msg := types.SSEMessage{
		ID:    id,
		Event: types.SSEEventHeartbeat,
		Data: types.SSEData{
			Timestamp: time.Now(),
			Status:    true,
		},
	}
	b.publish(user, &msg)
}

// Publish는 특정 클라이언트에게 일반 메시지를 발행합니다.
func (b *SSEBroker) Publish(user string, id uuid.UUID, data *types.SSEData, retry time.Duration) {
	msg := types.SSEMessage{Event: types.SSEEventPublish, ID: id, Data: *data, Retry: retry}
	b.publish(user, &msg)
}

// Broadcast는 연결된 모든 클라이언트에게 메시지를 브로드캐스트합니다.
func (b *SSEBroker) Broadcast(user string, data *types.SSEData, retry time.Duration) {
	b.Publish(user, uuid.Nil, data, retry)
}

// Error는 특정 클라이언트에게 에러 이벤트를 전송합니다.
func (b *SSEBroker) Error(user string, id uuid.UUID, data *types.SSEData, retry time.Duration) {
	msg := types.SSEMessage{Event: types.SSEEventError, ID: id, Data: *data, Retry: retry}
	b.publish(user, &msg)
}

// Shutdown은 브로커를 종료하고 모든 클라이언트 연결을 해제합니다.
func (b *SSEBroker) Shutdown() {
	b.isRunning.Store(false)
	b.cancel()
}

func (b *SSEBroker) publish(user string, msg *types.SSEMessage) {
	if !b.isRunning.Load() {
		return
	}

	select {
	case b.message <- publication{user: user, msg: msg}:
		log.Debugf("SSE Broker published message: %s %s", msg.Event, msg.ID)
	case <-b.ctx.Done():
		// 브로커가 종료되었으면 메시지 발행을 중단
		log.Infof("SSE Broker is shutting down, cannot publish message: %s", msg.ID)
	}
}

// Clients는 현재 활성화된 클라이언트 목록을 반환합니다.
func (b *SSEBroker) Clients(user string) []types.ClientInfo {
	if !b.isRunning.Load() {
		return []types.ClientInfo{}
	}

	req := make(chan []types.ClientInfo)
	select {
	case b.listClients <- listRequest{user: user, ch: req}:
		return <-req
	case <-b.ctx.Done():
		return []types.ClientInfo{}
	}
}

// NewSSEBroker는 주어진 설정을 기반으로 새로운 SSEBroker 인스턴스를 생성합니다.
func NewSSEBroker(config SSEConfig) *SSEBroker {
	if config.Context == nil {
		config.Context = context.Background()
	}

	ctx, cancel := context.WithCancel(config.Context)

	b := &SSEBroker{
		newClients:     make(chan *client, config.MaxClientBuffer),
		closingClients: make(chan uuid.UUID, config.MaxClientBuffer),
		message:        make(chan publication, config.MaxMessageBuffer),
		listClients:    make(chan listRequest),
		clients:        make(map[uuid.UUID]*client),
		ctx:            ctx,
		cancel:         cancel,
		isRunning:      atomic.Bool{},
	}

	b.isRunning.Store(true)

	go b.listen()

	return b
}

func (b *SSEBroker) listen() {
	defer func() {
		// 루프 종료 시 클린업
		for id, c := range b.clients {
			close(c.ch)
			log.Infof("Closed client channel: %s", id)
		}
		// 맵 초기화
		b.clients = make(map[uuid.UUID]*client)

		log.Info("SSE Broker stopped")
	}()

	for {
		select {
		case <-b.ctx.Done():
			log.Info("SSE Broker shutting down...")
			b.isRunning.Store(false)
			return
		case newClient := <-b.newClients:
			b.clients[newClient.ID] = newClient
			newClient.ch <- newClient.initial
			close(newClient.ready)
			log.Infof("Client added: %s. Total clients: %d", newClient.ID, len(b.clients))
		case clientID := <-b.closingClients:
			if c, ok := b.clients[clientID]; ok {
				delete(b.clients, clientID)
				close(c.ch)
				log.Infof("Client removed: %s. Total clients: %d", clientID, len(b.clients))
			}
		case req := <-b.listClients:
			clients := make([]types.ClientInfo, 0, len(b.clients))
			for _, c := range b.clients {
				if c.User == req.user {
					clients = append(clients, c.ClientInfo)
				}
			}
			req.ch <- clients
		case publication := <-b.message:
			msg := publication.msg
			if msg.ID != uuid.Nil {
				// 특정 클라이언트에게만 전송
				if c, ok := b.clients[msg.ID]; ok && c.User == publication.user {
					select {
					case c.ch <- msg:
						log.Debugf("SSE Broker published message: %s %s", msg.Event, msg.ID)
					default:
						log.Warnf("Client %s channel buffer full. Dropping message.", msg.ID)
					}
				}
			} else {
				// 모든 활성 클라이언트에게 메시지 전송 (브로드캐스트)
				for id, c := range b.clients {
					if c.User != publication.user {
						continue
					}
					select {
					case c.ch <- msg:
						log.Debugf("SSE Broker published message: %s %s", msg.Event, msg.ID)
					default:
						// 채널이 가득 참 -> 클라이언트가 느려서 메시지를 받지 못함
						// 여기서는 메시지를 드랍하고 로그를 남깁니다.
						// 필요하다면 연결을 끊을 수도 있습니다.
						log.Warnf("Client %s channel buffer full. Dropping message.", id)
					}
				}
			}
		}
	}
}
