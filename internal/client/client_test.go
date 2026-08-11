package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meteormin/govfs/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_SetToken(t *testing.T) {
	t.Run("SetToken", func(t *testing.T) {
		token := "my-auth-token" //nolint:gosec // This is a test token
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.New(server.URL)
		c.SetToken(token)

		// Trigger a request to verify the token is sent
		resp, err := c.SSE().Subscribe()
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})
}
