package client

import (
	"bufio"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSSEMessage(t *testing.T) {
	id := uuid.New()
	stream := "id:" + id.String() + "\n" +
		"event:publish\n" +
		`data:{"timestamp":"2026-08-19T00:00:00Z","status":true,"meta":{"action":"vfs.create"}}` + "\n\n"

	message, err := readSSEMessage(bufio.NewReader(strings.NewReader(stream)))
	require.NoError(t, err)
	assert.Equal(t, id, message.ID)
	assert.Equal(t, types.SSEEventPublish, message.Event)
	assert.True(t, message.Data.Status)
	assert.Equal(t, "vfs.create", message.Data.Meta.Action)
}
