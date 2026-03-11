package client_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/client"
	"github.com/meteormin/govfs/server/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRandomMetaRes(isDir bool) types.MetaRes {
	id, _ := uuid.NewRandom()
	randPath, _ := uuid.NewRandom()
	return types.MetaRes{
		Meta: vfs.Meta{
			ID:       id,
			Name:     id.String(),
			Path:     "/" + randPath.String(),
			Size:     0,
			IsDir:    isDir,
			Modified: time.Now(),
		},
		URL: "/vfs/" + id.String(),
	}
}

func TestClient_List(t *testing.T) {
	meta := newRandomMetaRes(true)
	expectedRes := types.VfsRes[[]types.MetaRes]{
		ViewType: types.ViewTypeList,
		Path:     "/",
		Payload: []types.MetaRes{
			meta,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vfs", r.URL.Path)
		assert.Equal(t, "/", r.URL.Query().Get("q"))
		encErr := json.NewEncoder(w).Encode(expectedRes)
		assert.NoError(t, encErr)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	res, err := c.List("/")
	require.NoError(t, err)
	require.NotEmpty(t, res)
}

func TestClient_Read(t *testing.T) {
	id := uuid.New()
	content := "hello world"
	expectedMeta := types.MetaRes{
		// Fill minimal fields
		Meta: vfs.Meta{
			ID:       id,
			Path:     "/test.txt",
			Name:     "test.txt",
			Size:     0,
			IsDir:    false,
			Modified: time.Now(),
		},
		URL: "/vfs/" + id.String(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stat") {
			err := json.NewEncoder(w).Encode(expectedMeta)
			assert.NoError(t, err)
			return
		}
		assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
		_, err := w.Write([]byte(content))
		assert.NoError(t, err)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	rc, meta, err := c.Read(id)
	require.NoError(t, err)
	assert.Equal(t, expectedMeta.Name, meta.Name) // Check a field

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, rc)
	require.NoError(t, err)
	assert.Equal(t, content, buf.String())
}

func TestClient_Create(t *testing.T) {
	t.Run("CreateDir", func(t *testing.T) {
		dirName := "new-dir"
		expectedMeta := types.MetaRes{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			err := r.ParseMultipartForm(32 << 20)
			assert.NoError(t, err)
			assert.Equal(t, "true", r.FormValue("isDir"))
			assert.Equal(t, dirName, r.FormValue("name"))

			w.WriteHeader(http.StatusCreated)
			err = json.NewEncoder(w).Encode(expectedMeta)
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		_, err := c.CreateDir(dirName)
		assert.NoError(t, err)
	})

	t.Run("CreateFile", func(t *testing.T) {
		fileName := "new-file.txt"
		fileContent := "file content"

		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, fileName)
		err := os.WriteFile(filePath, []byte(fileContent), 0o600)
		require.NoError(t, err)
		defer os.Remove(filePath)

		file, err := os.Open(filePath)
		require.NoError(t, err)
		defer file.Close()

		expectedMeta := types.MetaRes{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			parseErr := r.ParseMultipartForm(32 << 20)
			assert.NoError(t, parseErr)
			assert.Equal(t, "false", r.FormValue("isDir"))
			assert.Equal(t, fileName, r.FormValue("name"))

			file, header, fileErr := r.FormFile("file")
			assert.NoError(t, fileErr)
			assert.Equal(t, fileName, header.Filename)

			buf := new(bytes.Buffer)
			_, copyErr := io.Copy(buf, file)
			assert.NoError(t, copyErr)
			assert.Equal(t, fileContent, buf.String())

			w.WriteHeader(http.StatusCreated)
			encErr := json.NewEncoder(w).Encode(expectedMeta)
			assert.NoError(t, encErr)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		_, err = c.CreateFile(fileName, file)
		assert.NoError(t, err)
	})
}

func TestClient_FileOps(t *testing.T) {
	t.Run("Write", func(t *testing.T) {
		id := uuid.New()
		content := "updated content"
		expectedMeta := types.MetaRes{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
			assert.Equal(t, http.MethodPut, r.Method)

			var req types.WriteReq
			decErr := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, decErr)
			assert.Equal(t, content, req.Content)

			w.WriteHeader(http.StatusAccepted)
			encErr := json.NewEncoder(w).Encode(expectedMeta)
			assert.NoError(t, encErr)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.Write(id, content)
		assert.NoError(t, err)
	})

	t.Run("Move", func(t *testing.T) {
		id := uuid.New()
		dst := "moved-loc"
		expectedMeta := types.MetaRes{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
			assert.Equal(t, http.MethodPatch, r.Method)

			var req types.DstReq
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, dst, req.Name)

			w.WriteHeader(http.StatusAccepted)
			encErr := json.NewEncoder(w).Encode(expectedMeta)
			assert.NoError(t, encErr)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.Move(id, dst)
		assert.NoError(t, err)
	})

	t.Run("Copy", func(t *testing.T) {
		id := uuid.New()
		dst := "copy-loc"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			var req types.DstReq
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, dst, req.Name)

			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.Copy(id, dst)
		assert.NoError(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		id := uuid.New()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
			assert.Equal(t, http.MethodDelete, r.Method)
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.Delete(id)
		assert.NoError(t, err)
	})
}

func TestClient_Admin(t *testing.T) {
	t.Run("Restore", func(t *testing.T) {
		fileContent := "backup content"
		fileName := "backup.tar.gz"

		// Create a dummy backup file
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, fileName)
		err := os.WriteFile(filePath, []byte(fileContent), 0o600)
		require.NoError(t, err)

		file, err := os.Open(filePath)
		require.NoError(t, err)
		defer file.Close()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/restore", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			parseErr := r.ParseMultipartForm(32 << 20)
			assert.NoError(t, parseErr)

			file, header, fileErr := r.FormFile("file")
			assert.NoError(t, fileErr)
			assert.Equal(t, "backup", header.Filename)

			buf := new(bytes.Buffer)
			_, copyErr := io.Copy(buf, file)
			assert.NoError(t, copyErr)
			assert.Equal(t, fileContent, buf.String())

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err = c.Restore(file)
		assert.NoError(t, err)
	})

	t.Run("Backup", func(t *testing.T) {
		content := "backup content"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/backup", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(content))
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		reader, err := c.Backup()
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, reader)
		assert.NoError(t, err)
		assert.Equal(t, content, buf.String())
	})

	t.Run("Rotate", func(t *testing.T) {
		newKey := "new-encryption-key"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/rotate", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			var req map[string]string
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, newKey, req["key"])

			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.Rotate(newKey)
		assert.NoError(t, err)
	})
}

func TestClient_SSE(t *testing.T) {
	t.Run("Subscribe", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/sse/subscribe", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		resp, err := c.Subscribe()
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})

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

		c := client.NewClient(server.URL)
		resp, err := c.Publish(id, data)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})
}

func TestClient_Meta(t *testing.T) {
	t.Run("Stat", func(t *testing.T) {
		id := uuid.New()
		expectedMeta := types.MetaRes{
			Meta: vfs.Meta{
				ID:   id,
				Name: "test.txt",
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String()+"/stat", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)

			err := json.NewEncoder(w).Encode(expectedMeta)
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		meta, err := c.Stat(id)
		assert.NoError(t, err)
		assert.Equal(t, expectedMeta.Name, meta.Name)
	})

	t.Run("Tree", func(t *testing.T) {
		expectedRes := types.VfsRes[*types.TreeNodeRes]{
			Payload: &types.TreeNodeRes{
				Meta: types.MetaRes{
					Meta: vfs.Meta{
						Name: "root",
					},
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs", r.URL.Path)
			assert.Equal(t, "/", r.URL.Query().Get("q"))
			assert.Equal(t, "tree", r.URL.Query().Get("viewType"))
			assert.Equal(t, http.MethodGet, r.Method)

			err := json.NewEncoder(w).Encode(expectedRes)
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		res, err := c.Tree("/")
		assert.NoError(t, err)
		assert.Equal(t, "root", res.Meta.Name)
	})
}

func TestClient_Misc(t *testing.T) {
	t.Run("Copy", func(t *testing.T) {
		id := uuid.New()
		dst := "copy-loc"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			var req types.DstReq
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, dst, req.Name)

			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.Copy(id, dst)
		assert.NoError(t, err)
	})

	t.Run("WriteComments", func(t *testing.T) {
		id := uuid.New()
		comment := "this is a comment"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/vfs/"+id.String()+"/comments", r.URL.Path)
			assert.Equal(t, http.MethodPatch, r.Method)

			var req types.WriteCommentReq
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, comment, req.Comment)

			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.WriteComments(id, comment)
		assert.NoError(t, err)
	})

	t.Run("SetToken", func(t *testing.T) {
		token := "my-auth-token"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer "+token, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		c.SetToken(token)

		// Trigger a request to verify the token is sent
		resp, err := c.Subscribe()
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode())
	})
}

func TestClient_Login(t *testing.T) {
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

		c := client.NewClient(server.URL)
		_, err := c.Login(username, password)
		assert.NoError(t, err)

		// Trigger a request to verify the token is set and sent
		resp, err := c.Subscribe()
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

		c := client.NewClient(server.URL)
		_, err := c.Login(username, password)
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

		c := client.NewClient(server.URL)
		_, err := c.Login(username, password)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "login failed: 401")
	})
}

func TestClient_Cloud(t *testing.T) {
	t.Run("GoogleDriveAuthCodeURL", func(t *testing.T) {
		expectedURL := "https://accounts.google.com/o/oauth2/auth"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/cloud/googledrive/auth", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			w.WriteHeader(http.StatusOK)
			err := json.NewEncoder(w).Encode(map[string]string{"url": expectedURL})
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		url, err := c.CloudGoogleDriveAuthCodeURL()
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, url)
	})

	t.Run("List", func(t *testing.T) {
		expectedFiles := []string{"file1.txt", "file2.txt"}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/cloud", r.URL.Path)
			assert.Equal(t, "/some/path", r.URL.Query().Get("path"))
			assert.Equal(t, http.MethodGet, r.Method)

			w.WriteHeader(http.StatusOK)
			err := json.NewEncoder(w).Encode(expectedFiles)
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		files, err := c.CloudList("/some/path")
		assert.NoError(t, err)
		assert.Equal(t, expectedFiles, files)
	})

	t.Run("Upload", func(t *testing.T) {
		fileName := "upload.txt"
		fileContent := "hello cloud"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/cloud", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)

			err := r.ParseMultipartForm(32 << 20)
			assert.NoError(t, err)

			file, header, err := r.FormFile("file")
			assert.NoError(t, err)
			assert.Equal(t, fileName, header.Filename)

			buf := new(bytes.Buffer)
			_, err = io.Copy(buf, file)
			assert.NoError(t, err)
			assert.Equal(t, fileContent, buf.String())

			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.CloudUpload(fileName, strings.NewReader(fileContent))
		assert.NoError(t, err)
	})

	t.Run("Download", func(t *testing.T) {
		fileContent := "downloaded content"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/cloud/download", r.URL.Path)
			assert.Equal(t, "/some/file.txt", r.URL.Query().Get("path"))
			assert.Equal(t, http.MethodPost, r.Method)

			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(fileContent))
			assert.NoError(t, err)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		r, err := c.CloudDownload("/some/file.txt")
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, r)
		assert.NoError(t, err)
		assert.Equal(t, fileContent, buf.String())
	})

	t.Run("Delete", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/cloud", r.URL.Path)
			assert.Equal(t, "/some/file.txt", r.URL.Query().Get("path"))
			assert.Equal(t, http.MethodDelete, r.Method)

			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		c := client.NewClient(server.URL)
		err := c.CloudDelete("/some/file.txt")
		assert.NoError(t, err)
	})
}
