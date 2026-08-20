package mcp

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanPath(t *testing.T) {
	cleaned, err := cleanPath("/images/../photo.png")
	require.NoError(t, err)
	assert.Equal(t, "/photo.png", cleaned)

	_, err = cleanPath("relative/path")
	require.Error(t, err)
}

func TestDecodeContent(t *testing.T) {
	content, err := decodeContent(base64.StdEncoding.EncodeToString([]byte("image")))
	require.NoError(t, err)
	assert.Equal(t, []byte("image"), content)

	_, err = decodeContent("not-base64")
	require.Error(t, err)

	tooLarge := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", maxUploadSize+1)))
	_, err = decodeContent(tooLarge)
	require.ErrorContains(t, err, "exceeds 10 MiB")
}

func TestWaitForMutation(t *testing.T) {
	events := make(chan types.SSEMessage, 1)
	errors := make(chan error)
	id := uuid.New()
	events <- types.SSEMessage{
		Event: types.SSEEventPublish,
		Data: types.SSEData{
			Status: true,
			Meta:   types.SSEMeta{ID: id, Action: "vfs.create"},
		},
	}

	server := &Server{events: events, errors: errors}
	meta, err := server.waitForMutation(context.Background(), "vfs.create")
	require.NoError(t, err)
	assert.Equal(t, id, meta.ID)
}
