// Package services는 서버의 핵심 비즈니스 로직을 제공합니다.
package services

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"

	"github.com/meteormin/govfs/server/types"
)

const (
	defaultBufferSize = 100
)

type SSEConfig struct {
	Context          context.Context
	MaxClients       int
	MaxMessageBuffer int
}

// SSEBroker는 여러 클라이언트에게 실시간 메시지를 중계하는 역할을 합니다.
type SSEBroker struct {
	// 새 클라이언트 채널을 등록
	newClients chan clientInfo
	// 끊긴 클라이언트 채널을 제거
	closingClients chan uuid.UUID
	// 메시지를 모든 클라이언트에게 전달
	message chan *types.SSEMessage
	// 현재 활성화된 클라이언트 목록
	clients map[uuid.UUID]chan *types.SSEMessage
	// 브로커 컨텍스트 (서비스 종료 시 사용)
	ctx    context.Context
	cancel context.CancelFunc
	// 실행 상태
	isRunning atomic.Bool
}

type clientInfo struct {
	ID uuid.UUID
	Ch chan *types.SSEMessage
}

// Subscribe는 새로운 클라이언트를 브로커에 등록하고 구독 성공 메시지를 반환합니다.
func (b *SSEBroker) Subscribe(ctx context.Context) (*types.SSEMessage, <-chan *types.SSEMessage) {
	if !b.isRunning.Load() {
		log.Warn("Attempted to subscribe to stopped SSE Broker")
		return nil, nil
	}

	// 구독 시 채널 생성을 브로커가 담당하여 일관성을 유지합니다.
	// 버퍼를 두어 일시적인 블로킹을 방지합니다.
	ch := make(chan *types.SSEMessage, defaultBufferSize)
	id, _ := uuid.NewRandom()
	subMsg := &types.SSEMessage{
		ID:    id,
		Event: types.SSEEventSubscribe,
		Data: types.SSEData{
			Timestamp: time.Now(),
			Status:    true,
			Message:   "Subscribed successfully",
		},
	}

	// 클라이언트 컨텍스트가 취소(연결 끊김)되면 자동으로 Unsubscribe 호출
	context.AfterFunc(ctx, func() {
		b.Unsubscribe(id)
	})

	b.newClients <- clientInfo{ID: id, Ch: ch}

	log.Infof("SSE Broker subscribed: %s", id)

	ch <- subMsg

	log.Infof("Initial message sent: %s", subMsg.ID)

	select {
	case <-b.ctx.Done():
		close(ch)
		log.Info("SSE Broker is shutting down, cannot subscribe")
		return nil, ch
	default:
		return subMsg, ch
	}
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
func (b *SSEBroker) Hearbeat(id uuid.UUID) {
	msg := types.SSEMessage{
		ID:    id,
		Event: types.SSEEventHeartbeat,
		Data: types.SSEData{
			Timestamp: time.Now(),
			Status:    true,
		},
	}
	b.publish(&msg)
}

// Publish는 특정 클라이언트에게 일반 메시지를 발행합니다.
func (b *SSEBroker) Publish(id uuid.UUID, data *types.SSEData, retry time.Duration) {
	msg := types.SSEMessage{Event: types.SSEEventPublish, ID: id, Data: *data, Retry: retry}
	b.publish(&msg)
}

// Broadcast는 연결된 모든 클라이언트에게 메시지를 브로드캐스트합니다.
func (b *SSEBroker) Broadcast(data *types.SSEData, retry time.Duration) {
	b.Publish(uuid.Nil, data, retry)
}

// Error는 특정 클라이언트에게 에러 이벤트를 전송합니다.
func (b *SSEBroker) Error(id uuid.UUID, data *types.SSEData, retry time.Duration) {
	msg := types.SSEMessage{Event: types.SSEEventError, ID: id, Data: *data, Retry: retry}
	b.publish(&msg)
}

// Shutdown은 브로커를 종료하고 모든 클라이언트 연결을 해제합니다.
func (b *SSEBroker) Shutdown() {
	b.isRunning.Store(false)
	b.cancel()
}

func (b *SSEBroker) publish(msg *types.SSEMessage) {
	if !b.isRunning.Load() {
		return
	}

	select {
	case b.message <- msg:
		log.Debugf("SSE Broker published message: %s %s", msg.Event, msg.ID)
	case <-b.ctx.Done():
		// 브로커가 종료되었으면 메시지 발행을 중단
		log.Infof("SSE Broker is shutting down, cannot publish message: %s", msg.ID)
	}
}

// AsyncExcute는 함수를 비동기적으로 실행하고 결과를 SSE를 통해 클라이언트에 알립니다.
func (b *SSEBroker) AsyncExcute(id uuid.UUID, do func() (types.SSEMeta, error)) {
	go func() {
		meta, err := do()
		if err != nil {
			b.Error(id, &types.SSEData{Timestamp: time.Now(), Status: false, Meta: meta, Message: err.Error()}, 0)
			return
		}
		b.Publish(id, &types.SSEData{Timestamp: time.Now(), Status: true, Meta: meta}, 0)
	}()
}

// NewSSEBroker는 주어진 설정을 기반으로 새로운 SSEBroker 인스턴스를 생성합니다.
func NewSSEBroker(config SSEConfig) *SSEBroker {
	if config.Context == nil {
		config.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(config.Context)

	b := &SSEBroker{
		newClients:     make(chan clientInfo, config.MaxClients),
		closingClients: make(chan uuid.UUID, config.MaxClients),
		message:        make(chan *types.SSEMessage, config.MaxMessageBuffer),
		clients:        make(map[uuid.UUID]chan *types.SSEMessage),
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
		for id, ch := range b.clients {
			close(ch)
			log.Infof("Closed client channel: %s", id)
		}
		// 맵 초기화
		b.clients = make(map[uuid.UUID]chan *types.SSEMessage)

		log.Info("SSE Broker stopped")
	}()

	for {
		select {
		case <-b.ctx.Done():
			log.Info("SSE Broker shutting down...")
			b.isRunning.Store(false)
			return
		case client := <-b.newClients:
			b.clients[client.ID] = client.Ch
			log.Infof("Client added: %s. Total clients: %d", client.ID, len(b.clients))
		case clientID := <-b.closingClients:
			if ch, ok := b.clients[clientID]; ok {
				delete(b.clients, clientID)
				close(ch)
				log.Infof("Client removed: %s. Total clients: %d", clientID, len(b.clients))
			}
		case msg := <-b.message:
			if msg.ID != uuid.Nil {
				// 특정 클라이언트에게만 전송
				if ch, ok := b.clients[msg.ID]; ok {
					select {
					case ch <- msg:
					default:
						log.Warnf("Client %s channel buffer full. Dropping message.", msg.ID)
					}
				}
			} else {
				// 모든 활성 클라이언트에게 메시지 전송 (브로드캐스트)
				for id, client := range b.clients {
					select {
					case client <- msg:
						// 성공적으로 전송
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
