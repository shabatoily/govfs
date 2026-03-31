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

func TestVFSClient_List(t *testing.T) {
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

	c := client.New(server.URL)
	res, err := c.VFS().List("/")
	require.NoError(t, err)
	require.NotEmpty(t, res)
}

func TestVFSClient_Read(t *testing.T) {
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

	c := client.New(server.URL)
	rc, meta, err := c.VFS().Read(id)
	require.NoError(t, err)
	assert.Equal(t, expectedMeta.Name, meta.Name) // Check a field

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, rc)
	require.NoError(t, err)
	assert.Equal(t, content, buf.String())
}

func TestVFSClient_CreateDir(t *testing.T) {
	dirName := "new-dir"
	expectedMeta := types.MetaRes{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vfs", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		err := r.ParseMultipartForm(30 << 20)
		assert.NoError(t, err)
		assert.Equal(t, "true", r.FormValue("isDir"))
		assert.Equal(t, dirName, r.FormValue("name"))

		w.WriteHeader(http.StatusCreated)
		err = json.NewEncoder(w).Encode(expectedMeta)
		assert.NoError(t, err)
	}))
	defer server.Close()

	c := client.New(server.URL)
	_, err := c.VFS().CreateDir(dirName)
	assert.NoError(t, err)
}

func TestVFSClient_CreateFile(t *testing.T) {
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

		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
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

	c := client.New(server.URL)
	_, err = c.VFS().CreateFile(fileName, file)
	assert.NoError(t, err)
}

func TestVFSClient_FileOps_Write(t *testing.T) {
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

	c := client.New(server.URL)
	err := c.VFS().Write(id, content)
	assert.NoError(t, err)
}

func TestVFSClient_FileOps_Move(t *testing.T) {
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

	c := client.New(server.URL)
	err := c.VFS().Move(id, dst)
	assert.NoError(t, err)
}

func TestVFSClient_FileOps_Copy(t *testing.T) {
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

	c := client.New(server.URL)
	err := c.VFS().Copy(id, dst)
	assert.NoError(t, err)
}

func TestVFSClient_FileOps_Delete(t *testing.T) {
	id := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	c := client.New(server.URL)
	err := c.VFS().Delete(id)
	assert.NoError(t, err)
}

func TestVFSClient_Admin_Restore(t *testing.T) {
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

		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		parseErr := r.ParseMultipartForm(30 << 20)
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

	c := client.New(server.URL)
	err = c.VFS().Restore(file)
	assert.NoError(t, err)
}

func TestVFSClient_Admin_Backup(t *testing.T) {
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

	c := client.New(server.URL)
	reader, err := c.VFS().Backup()
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, reader)
	require.NoError(t, err)
	assert.Equal(t, content, buf.String())
}

func TestVFSClient_Admin_Rotate(t *testing.T) {
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

	c := client.New(server.URL)
	err := c.VFS().Rotate(newKey)
	assert.NoError(t, err)
}

func TestVFSClient_Meta_Stat(t *testing.T) {
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

	c := client.New(server.URL)
	meta, err := c.VFS().Stat(id)
	require.NoError(t, err)
	assert.Equal(t, expectedMeta.Name, meta.Name)
}

func TestVFSClient_Meta_Tree(t *testing.T) {
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

	c := client.New(server.URL)
	res, err := c.VFS().Tree("/")
	require.NoError(t, err)
	assert.Equal(t, "root", res.Meta.Name)
}

func TestVFSClient_Misc_WriteComments(t *testing.T) {
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

	c := client.New(server.URL)
	err := c.VFS().WriteComments(id, comment)
	assert.NoError(t, err)
}
