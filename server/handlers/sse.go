package handlers

import (
	"bufio"
	"time"

	"github.com/gofiber/fiber/v3/log"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/meteormin/govfs/server/services"
	"github.com/meteormin/govfs/server/types"
)

const (
	HeartbeatInterval = 15 * time.Second
)

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
				log.Debug("heartbeat sent to client: %s", msg.ID)
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

func NewSSEHandler(broker *services.SSEBroker) *SSEHandler {
	return &SSEHandler{broker: broker}
}
