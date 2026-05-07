package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/meteormin/govfs/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEClient_Subscribe(t *testing.T) {
	t.Run("Subscribe", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/sse/subscribe", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.New(server.URL)
		resp, err := c.SSE().Subscribe()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})
}

func TestSSEClient_Publish(t *testing.T) {
	t.Run("Publish", func(t *testing.T) {
		id := uuid.New()
		data := map[string]any{"key": "value"}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/sse/"+id.String()+"/publish", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req map[string]any
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, "value", req["key"])

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.New(server.URL)
		err := c.SSE().Publish(id, data)
		require.NoError(t, err)
	})
}
