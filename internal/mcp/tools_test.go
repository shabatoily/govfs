package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/client"

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
	id := uuid.NewV4()
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

func TestCreateToolsWaitAndReturnMetadata(t *testing.T) {
	for _, directory := range []bool{true, false} {
		t.Run(map[bool]string{true: "mkdir", false: "upload"}[directory], func(t *testing.T) {
			id := uuid.NewV4()
			events := make(chan types.SSEMessage, 1)
			meta := types.MetaRes{Meta: vfs.Meta{ID: id, Name: "resource", Path: "/resource", IsDir: directory}}
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
					if !assert.NoError(t, r.ParseMultipartForm(1<<20)) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					defer func() { assert.NoError(t, r.MultipartForm.RemoveAll()) }()
					assert.Equal(t, "/resource", r.FormValue("name"))
					assert.Equal(t, strconv.FormatBool(directory), r.FormValue("isDir"))
					events <- types.SSEMessage{
						Event: types.SSEEventPublish,
						Data:  types.SSEData{Status: true, Meta: types.SSEMeta{ID: id, Action: "vfs.create"}},
					}
					w.WriteHeader(http.StatusAccepted)
					return
				}
				assert.Equal(t, "/vfs/"+id.String()+"/stat", r.URL.Path)
				assert.NoError(t, json.NewEncoder(w).Encode(meta))
			}))
			defer httpServer.Close()
			server := &Server{client: client.New(httpServer.URL), events: events}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var result any
			var err error
			if directory {
				_, result, err = server.mkdir(ctx, nil, pathInput{Path: "/resource"})
			} else {
				_, result, err = server.upload(ctx, nil, uploadInput{
					Path: "/resource", ContentBase64: base64.StdEncoding.EncodeToString([]byte("content")),
				})
			}
			require.NoError(t, err)
			got, ok := result.(types.MetaRes)
			require.True(t, ok)
			require.Equal(t, id, got.ID)
			require.Equal(t, directory, got.IsDir)
		})
	}
}
