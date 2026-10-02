package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/server/middlewares"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
	"github.com/stretchr/testify/require"
)

func TestTransferWaitConflictAndReplace(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			fs, err := badger.New(&badger.Config{InMemory: true})
			require.NoError(t, err)
			defer fs.Close()
			src, err := fs.Create("/source.txt", strings.NewReader("source"))
			require.NoError(t, err)
			dst, err := fs.Create("/target.txt", strings.NewReader("target"))
			require.NoError(t, err)
			handler := NewVfsHandler(services.NewVfsService(fs, "/vfs"), nil, nil)
			app := fiber.New(fiber.Config{ErrorHandler: middlewares.ErrorHandler})
			app.Post("/vfs/:id/copy", handler.Copy)
			app.Patch("/vfs/:id", handler.Move)
			url := "/vfs/" + src.ID.String()
			if method == http.MethodPost {
				url += "/copy"
			}
			url += "?wait=true"
			for _, replace := range []bool{false, true} {
				body := types.DstReq{Name: dst.Path, CheckConflict: true}
				expected := http.StatusConflict
				if replace {
					body.ReplaceID = dst.ID
					expected = http.StatusOK
				}
				data, err := json.Marshal(body)
				require.NoError(t, err)
				req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(data))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				res, err := app.Test(req)
				require.NoError(t, err)
				require.Equal(t, expected, res.StatusCode)
				res.Body.Close()
				if !replace {
					_, err = fs.Stat(dst.ID)
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestMoveWaitOverwritesByDefault(t *testing.T) {
	fs, err := badger.New(&badger.Config{InMemory: true})
	require.NoError(t, err)
	defer fs.Close()
	src, err := fs.Create("/source.txt", strings.NewReader("source"))
	require.NoError(t, err)
	dst, err := fs.Create("/target.txt", strings.NewReader("target"))
	require.NoError(t, err)
	handler := NewVfsHandler(services.NewVfsService(fs, "/vfs"), nil, nil)
	app := fiber.New(fiber.Config{ErrorHandler: middlewares.ErrorHandler})
	app.Patch("/vfs/:id", handler.Move)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch,
		"/vfs/"+src.ID.String()+"?wait=true", strings.NewReader(`{"name":"/target.txt"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	moved, err := fs.Stat(src.ID)
	require.NoError(t, err)
	require.Equal(t, dst.Path, moved.Path)
	_, err = fs.Stat(dst.ID)
	require.ErrorIs(t, err, vfs.ErrNotFound)
}
