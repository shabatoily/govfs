// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"bufio"
	"time"

	"github.com/gofiber/fiber/v3/log"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/server/middlewares"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
)

const (
	// heartbeatInterval은 SSE 연결 유지를 위해 주기적으로 하트비트를 전송하는 간격입니다.
	heartbeatInterval = 15 * time.Second
)

// SSEHandler는 서버 전송 이벤트(SSE) 통신을 처리하는 핸들러입니다.
type SSEHandler struct {
	broker *services.SSEBroker
}

// Subscribe는 클라이언트의 SSE 연결 요청을 처리하고 이벤트 스트림을 시작합니다.
// @Summary      SSE 구독
// @Description  Server-Sent Events 스트림에 클라이언트를 구독시킵니다.
// @Tags SSE
// @Produce text/event-stream
// @Success 200 {object} types.SSEMessage
// @Failure 503 {string} string
// @Security BearerAuth
// @Router /sse/subscribe [get]
func (h *SSEHandler) Subscribe(ctx fiber.Ctx) error {
	user := userIDFromContext(ctx)
	ctx.Set(fiber.HeaderContentType, "text/event-stream")
	ctx.Set(fiber.HeaderCacheControl, "no-cache")
	ctx.Set(fiber.HeaderConnection, "keep-alive")

	msgID, clientChan, err := h.broker.Subscribe(types.SubscribeReq{
		Ctx:  ctx.Context(),
		Addr: ctx.IP(),
		User: user,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "SSE Broker is not available")
	}

	// Send initial subscription success event
	return ctx.SendStreamWriter(func(w *bufio.Writer) {
		log.Debugf("SSE Stream started for client: %s", msgID)

		ticker := time.NewTicker(heartbeatInterval)
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
				h.broker.Hearbeat(user, msgID)
				log.Debugf("heartbeat sent to client: %s", msgID)
			}
		}
	})
}

// Publish는 외부 요청을 통해 특정 클라이언트에게 SSE 메시지를 전송합니다.
// @Summary      SSE 메시지 발행
// @Description  지정된 클라이언트(또는 전체)에 Server-Sent Event 메시지를 발행합니다.
// @Tags SSE
// @Accept json
// @Param metaData body types.SSEMeta true "meta data"
// @Param id path string true "client id"
// @Success 204
// @Failure 400 {string} string
// @Security BearerAuth
// @Router /sse/publish/{id} [post]
func (h *SSEHandler) Publish(ctx fiber.Ctx) error {
	var metaData types.SSEMeta
	if err := ctx.Bind().Body(&metaData); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	id := ctx.Params("id")
	var targetID uuid.UUID
	if id != "" {
		parsedID, err := uuid.Parse(id)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid client id")
		}
		targetID = parsedID
	}

	data := types.SSEData{
		Timestamp: time.Now(),
		Status:    true,
		Meta:      metaData,
	}

	h.broker.Publish(userIDFromContext(ctx), targetID, &data, time.Second*3)

	return ctx.SendStatus(fiber.StatusNoContent)
}

// Clients는 현재 연결된 모든 SSE 클라이언트 목록을 반환합니다.
// @Summary      SSE 클라이언트 목록 조회
// @Description  연결된 모든 SSE 클라이언트의 정보를 반환합니다.
// @Tags SSE
// @Produce json
// @Success 200 {object} types.ClientList
// @Security BearerAuth
// @Router /sse/clients [get]
func (h *SSEHandler) Clients(ctx fiber.Ctx) error {
	return ctx.JSON(types.ClientList{
		Clients: h.broker.Clients(userIDFromContext(ctx)),
	})
}

// NewSSEHandler는 새로운 SSEHandler 인스턴스를 생성합니다.
func NewSSEHandler(broker *services.SSEBroker) *SSEHandler {
	return &SSEHandler{broker: broker}
}

func userIDFromContext(ctx fiber.Ctx) string {
	user, ok := middlewares.CurrentUser(ctx)
	if !ok {
		return ""
	}
	return user.ID.String()
}
