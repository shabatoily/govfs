// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"bufio"
	"time"

	"github.com/gofiber/fiber/v3/log"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/meteormin/govfs/internal/server/services"
	"github.com/meteormin/govfs/internal/types"
)

const (
	// HeartbeatInterval은 SSE 연결 유지를 위해 주기적으로 하트비트를 전송하는 간격입니다.
	HeartbeatInterval = 15 * time.Second
)

// SSEHandler는 서버 전송 이벤트(SSE) 통신을 처리하는 핸들러입니다.
type SSEHandler struct {
	broker *services.SSEBroker
}

// Subscribe handles SSE subscription requests.
// @Summary SSE Subscribe
// @Description Subscribe to Server-Sent Events
// @Tags SSE
// @Produce text/event-stream
// @Success 200 {object} types.SSEMessage
// @Failure 500 {object} any
// @Router /sse/subscribe [get]
// Subscribe는 클라이언트의 SSE 연결 요청을 처리하고 이벤트 스트림을 시작합니다.
func (h *SSEHandler) Subscribe(ctx fiber.Ctx) error {
	ctx.Set(fiber.HeaderContentType, "text/event-stream")
	ctx.Set(fiber.HeaderCacheControl, "no-cache")
	ctx.Set(fiber.HeaderConnection, "keep-alive")

	msg, clientChan := h.broker.Subscribe(ctx.Context())
	if clientChan == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "SSE Broker is not available")
	}

	// Send initial subscription success event
	return ctx.SendStreamWriter(func(w *bufio.Writer) {
		log.Debugf("SSE Stream started for client: %s", msg.ID)

		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case m, ok := <-clientChan:
				if !ok {
					log.Debug("client channel closed")
					return
				}
				log.Debugf("received message for client: %s", m.Event)
				if _, err := m.WriteTo(w); err != nil {
					log.Errorf("writeTo error: %v", err)
					return
				}
				if err := w.Flush(); err != nil {
					log.Errorf("flush error: %v", err)
					return
				}
				log.Debug("message sent to client")
			case <-ticker.C:
				// Heartbeat
				h.broker.Hearbeat(msg.ID)
				log.Debugf("heartbeat sent to client: %s", msg.ID)
			}
		}
	})
}

// Publish publishes an SSE message.
// @Summary Publish SSE Message
// @Description Publish a Server-Sent Event message
// @Tags SSE
// @Accept json
// @Produce json
// @Param metaData body types.SSEMeta true "meta data"
// @Success 204
// @Failure 400 {object} fiber.Error
// @Router /sse/:id/publish [post]
// Publish는 외부 요청을 통해 특정 클라이언트에게 SSE 메시지를 전송합니다.
func (h *SSEHandler) Publish(ctx fiber.Ctx) error {
	var metaData types.SSEMeta
	if err := ctx.Bind().Body(&metaData); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	id := ctx.Params("id")
	var targetID uuid.UUID
	if id != "" {
		parsedID, err := uuid.Parse(id)
		if err == nil {
			targetID = parsedID
		}
	}

	data := types.SSEData{
		Timestamp: time.Now(),
		Status:    true,
		Meta:      metaData,
	}

	h.broker.Publish(targetID, &data, time.Second*3)

	return ctx.SendStatus(fiber.StatusNoContent)
}

// Clients returns the list of SSE clients.
// @Summary List SSE Clients
// @Description List all SSE clients
// @Tags SSE
// @Produce json
// @Success 200 {object} types.ClientList
// @Router /sse/clients [get]
func (h *SSEHandler) Clients(ctx fiber.Ctx) error {
	return ctx.JSON(types.ClientList{
		Clients: h.broker.Clients(),
	})
}

// NewSSEHandler는 새로운 SSEHandler 인스턴스를 생성합니다.
func NewSSEHandler(broker *services.SSEBroker) *SSEHandler {
	return &SSEHandler{broker: broker}
}
