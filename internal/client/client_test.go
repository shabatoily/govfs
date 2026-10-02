package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/shabatoily/govfs/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_SetToken(t *testing.T) {
	t.Run("SetToken", func(t *testing.T) {
		token := "my-auth-token"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.New(server.URL)
		c.SetToken(token)

		// Trigger a request to verify the token is sent
		resp, err := c.SSE().Subscribe(context.Background())
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})
}

// TestClientReplacesHeaders는 갱신·해제 시 이전 인증 정보가 남지 않는지 확인합니다.
func TestClientReplacesHeaders(t *testing.T) {
	var authorization []string
	var clientIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Values("Authorization")
		clientIDs = r.Header.Values("X-Client-ID")
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	c := client.New(server.URL)
	first, second := uuid.NewV4(), uuid.NewV4()
	c.SetClientID(first)
	c.SetClientID(second)
	for _, token := range []string{"first", "second", "", "disabled"} {
		c.SetToken(token)
		_, err := c.Config(context.Background())
		require.NoError(t, err)
		want := ""
		if token != "" && token != "disabled" {
			want = "Bearer " + token
		}
		require.LessOrEqual(t, len(authorization), 1)
		require.Equal(t, want, strings.Join(authorization, ""))
		require.Equal(t, []string{second.String()}, clientIDs)
	}
}

func TestConfigRejectsErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	_, err := client.New(server.URL).Config(context.Background())
	require.ErrorContains(t, err, "403")
}
