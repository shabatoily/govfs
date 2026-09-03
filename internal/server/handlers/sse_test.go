package handlers

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEHandlerPublishRejectsInvalidClientID(t *testing.T) {
	broker := services.NewSSEBroker(services.SSEConfig{Context: context.Background()})
	defer broker.Shutdown()

	app := fiber.New()
	app.Post("/sse/publish/:id", NewSSEHandler(broker).Publish)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/sse/publish/not-a-uuid", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
