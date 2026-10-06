package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shabatoily/govfs/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminTools(t *testing.T) {
	const id = "018f0000-0000-7000-8000-000000000001"
	var requested string
	status := http.StatusOK
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.String()
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer admin-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			return
		}
		if r.URL.Path == "/admin/users" {
			_, _ = w.Write([]byte(`[{"username":"alice"}]`))
		} else if strings.HasPrefix(r.URL.Path, "/admin/users/") {
			_, _ = w.Write([]byte(`{"username":"alice","items":4}`))
		} else {
			_, _ = w.Write([]byte(`{"users":3,"items":[],"total":7}`))
		}
	}))
	defer httpServer.Close()
	c := client.New(httpServer.URL)
	c.SetToken("admin-token")
	server := &Server{client: c, sdk: mcpsdk.NewServer(&mcpsdk.Implementation{Name: "govfs", Version: "test"}, nil)}
	server.registerTools()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := server.sdk.Connect(ctx, st, nil)
	require.NoError(t, err)
	defer ss.Close()
	mcpClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := mcpClient.Connect(ctx, ct, nil)
	require.NoError(t, err)
	defer cs.Close()

	listed, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)
	var adminNames []string
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "admin_") {
			adminNames = append(adminNames, tool.Name)
			require.NotNil(t, tool.Annotations)
			assert.True(t, tool.Annotations.ReadOnlyHint)
		}
	}
	assert.ElementsMatch(t, []string{"admin_status", "admin_users", "admin_user_status", "admin_events", "admin_system_entries"}, adminNames)

	tests := []struct {
		name string
		args map[string]any
		path string
	}{
		{"admin_status", nil, "/admin/status"},
		{"admin_users", nil, "/admin/users"},
		{"admin_user_status", map[string]any{"id": id}, "/admin/users/" + id + "/status"},
		{"admin_events", nil, "/admin/events?page=1&pageSize=20"},
		{"admin_events", map[string]any{"page": 2, "page_size": 5, "user_id": id}, "/admin/events?page=2&pageSize=5&userId=" + id},
		{"admin_system_entries", nil, "/admin/system/entries?page=1&pageSize=20"},
		{"admin_system_entries", map[string]any{"page": 3, "page_size": 100}, "/admin/system/entries?page=3&pageSize=100"},
	}
	for _, tt := range tests {
		t.Run(tt.name+tt.path, func(t *testing.T) {
			result, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tt.name, Arguments: tt.args})
			require.NoError(t, err)
			require.False(t, result.IsError, "%+v", result.Content)
			assert.Equal(t, tt.path, requested)
			require.NotEmpty(t, result.Content)
			require.NotNil(t, result.StructuredContent)
		})
	}

	for _, tt := range []struct {
		name string
		args map[string]any
	}{
		{"admin_user_status", map[string]any{"id": "invalid"}},
		{"admin_user_status", nil},
		{"admin_events", map[string]any{"user_id": "invalid"}},
		{"admin_events", map[string]any{"page": 0}},
		{"admin_events", map[string]any{"page_size": 101}},
		{"admin_system_entries", map[string]any{"page": -1}},
		{"admin_system_entries", map[string]any{"page_size": 0}},
	} {
		requested = ""
		result, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tt.name, Arguments: tt.args})
		require.True(t, err != nil || result.IsError)
		assert.Empty(t, requested, "invalid input must not reach HTTP API")
	}

	status = http.StatusForbidden
	result, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "admin_status"})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
	assert.Contains(t, result.Content[0].(*mcpsdk.TextContent).Text, "403")
}
