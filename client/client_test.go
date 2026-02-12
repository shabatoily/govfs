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
	"github.com/meteormin/go-vfs"
	"github.com/meteormin/go-vfs/client"
	"github.com/meteormin/go-vfs/server/types"
	"github.com/stretchr/testify/assert"
)

func newRandomMetaRes() types.MetaRes {
	id, _ := uuid.NewRandom()
	return types.MetaRes{
		Meta: vfs.Meta{
			ID:       id,
			Name:     id.String(),
			Path:     "/test",
			Size:     0,
			IsDir:    true,
			Modified: time.Now(),
		},
		URL: "/test",
	}
}

func TestClient_List(t *testing.T) {
	meta := newRandomMetaRes()
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
		json.NewEncoder(w).Encode(expectedRes)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	res, err := c.List("/")
	assert.NoError(t, err)
	assert.NotEmpty(t, res)
}

func TestClient_Read(t *testing.T) {
	id := uuid.New()
	content := "hello world"
	expectedMeta := types.MetaRes{
		// Fill minimal fields
	}
	expectedMeta.Name = "test.txt"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stat") {
			json.NewEncoder(w).Encode(expectedMeta)
			return
		}
		assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
		w.Write([]byte(content))
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	rc, meta, err := c.Read(id)
	assert.NoError(t, err)
	assert.Equal(t, expectedMeta.Name, meta.Name) // Check a field

	buf := new(bytes.Buffer)
	io.Copy(buf, rc)
	assert.Equal(t, content, buf.String())
}

func TestClient_CreateDir(t *testing.T) {
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
		json.NewEncoder(w).Encode(expectedMeta)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	_, err := c.CreateDir(dirName)
	assert.NoError(t, err)
}

func TestClient_CreateFile(t *testing.T) {
	fileName := "new-file.txt"
	fileContent := "file content"

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, fileName)
	err := os.WriteFile(filePath, []byte(fileContent), 0o644)
	assert.NoError(t, err)
	defer os.Remove(filePath)

	file, err := os.Open(filePath)
	assert.NoError(t, err)
	defer file.Close()

	expectedMeta := types.MetaRes{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vfs", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		err := r.ParseMultipartForm(32 << 20)
		assert.NoError(t, err)
		assert.Equal(t, "false", r.FormValue("isDir"))
		assert.Equal(t, fileName, r.FormValue("name"))

		file, header, err := r.FormFile("file")
		assert.NoError(t, err)
		assert.Equal(t, fileName, header.Filename)

		buf := new(bytes.Buffer)
		io.Copy(buf, file)
		assert.Equal(t, fileContent, buf.String())

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(expectedMeta)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	_, err = c.CreateFile(fileName, file)
	assert.NoError(t, err)
}

func TestClient_Write(t *testing.T) {
	id := uuid.New()
	content := "updated content"
	expectedMeta := types.MetaRes{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vfs/"+id.String(), r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)

		var req types.WriteReq
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, content, req.Content)

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(expectedMeta)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	err := c.Write(id, content)
	assert.NoError(t, err)
}

func TestClient_Move(t *testing.T) {
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
		json.NewEncoder(w).Encode(expectedMeta)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	err := c.Move(id, dst)
	assert.NoError(t, err)
}

func TestClient_Delete(t *testing.T) {
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
}

func TestClient_Restore(t *testing.T) {
	fileContent := "backup content"
	fileName := "backup.tar.gz"

	// Create a dummy backup file
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, fileName)
	err := os.WriteFile(filePath, []byte(fileContent), 0o644)
	assert.NoError(t, err)

	file, err := os.Open(filePath)
	assert.NoError(t, err)
	defer file.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/vfs/restore", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		err := r.ParseMultipartForm(32 << 20)
		assert.NoError(t, err)

		file, header, err := r.FormFile("file")
		assert.NoError(t, err)
		assert.Equal(t, "backup", header.Filename)

		buf := new(bytes.Buffer)
		io.Copy(buf, file)
		assert.Equal(t, fileContent, buf.String())

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := client.NewClient(server.URL)
	err = c.Restore(file)
	assert.NoError(t, err)
}
