package client_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/meteormin/govfs/client"
	"github.com/stretchr/testify/assert"
)

func TestCloudClient_GoogleDriveAuthCodeURL(t *testing.T) {
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

		c := client.New(server.URL)
		url, err := c.Cloud().GoogleDriveAuthCodeURL()
		assert.NoError(t, err)
		assert.Equal(t, expectedURL, url)
	})
}

func TestCloudClient_List(t *testing.T) {
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

		c := client.New(server.URL)
		files, err := c.Cloud().List("/some/path")
		assert.NoError(t, err)
		assert.Equal(t, expectedFiles, files)
	})
}

func TestCloudClient_Upload(t *testing.T) {
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

		c := client.New(server.URL)
		err := c.Cloud().Upload(fileName, strings.NewReader(fileContent))
		assert.NoError(t, err)
	})
}

func TestCloudClient_Download(t *testing.T) {
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

		c := client.New(server.URL)
		reader, err := c.Cloud().Download("/some/file.txt")
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, reader)
		assert.NoError(t, err)
		assert.Equal(t, fileContent, buf.String())
	})
}

func TestCloudClient_Delete(t *testing.T) {
	t.Run("Delete", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/cloud", r.URL.Path)
			assert.Equal(t, "/some/file.txt", r.URL.Query().Get("path"))
			assert.Equal(t, http.MethodDelete, r.Method)

			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		c := client.New(server.URL)
		err := c.Cloud().Delete("/some/file.txt")
		assert.NoError(t, err)
	})
}
