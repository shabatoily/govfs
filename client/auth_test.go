package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goccy/go-json"
	"github.com/meteormin/govfs/client"
	"github.com/meteormin/govfs/server/types"
	"github.com/stretchr/testify/assert"
)

func TestAuthClient_Login(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		username := "admin"
		password := "password"
		token := "login-token"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/login" {
				assert.Equal(t, http.MethodPost, r.Method)
				var req types.LoginRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				assert.NoError(t, err)
				assert.Equal(t, username, req.Username)
				assert.Equal(t, password, req.Password)

				w.WriteHeader(http.StatusOK)
				resp := map[string]string{"token": token}
				err = json.NewEncoder(w).Encode(resp)
				assert.NoError(t, err)
				return
			}

			if r.URL.Path == "/sse/subscribe" {
				assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusOK)
				return
			}
		}))
		defer server.Close()

		c := client.New(server.URL)
		_, err := c.Auth().Login(username, password)
		assert.NoError(t, err)

		// Trigger a request to verify the token is set and sent
		resp, err := c.SSE().Subscribe()
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})

	t.Run("Disabled", func(t *testing.T) {
		username := "admin"
		password := "password"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/login" {
				assert.Equal(t, http.MethodPost, r.Method)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{}`))
				return
			}
		}))
		defer server.Close()

		c := client.New(server.URL)
		_, err := c.Auth().Login(username, password)
		assert.NoError(t, err)
	})

	t.Run("Failed", func(t *testing.T) {
		username := "admin"
		password := "password"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/login" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}))
		defer server.Close()

		c := client.New(server.URL)
		_, err := c.Auth().Login(username, password)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "login failed: 401")
	})
}

func TestAuthClient_Me(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		token := "login-token"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/me" {
				assert.Equal(t, http.MethodGet, r.Method)

				w.WriteHeader(http.StatusOK)
				resp := map[string]string{"token": token}
				err := json.NewEncoder(w).Encode(resp)
				assert.NoError(t, err)
				return
			}
		}))
		defer server.Close()

		c := client.New(server.URL)
		res, err := c.Auth().Me()
		assert.NoError(t, err)
		assert.Equal(t, token, res.Token)
	})

	t.Run("Failed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/me" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}))
		defer server.Close()

		c := client.New(server.URL)
		_, err := c.Auth().Me()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not logged in: 401")
	})
}
