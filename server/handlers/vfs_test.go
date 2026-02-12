package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/server/services"
	"github.com/meteormin/govfs/server/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockVFS is a mock implementation of vfs.VFS
type MockVFS struct {
	mock.Mock
}

func (m *MockVFS) List(path string) ([]vfs.Meta, error) {
	args := m.Called(path)
	return args.Get(0).([]vfs.Meta), args.Error(1)
}

func (m *MockVFS) Open(id uuid.UUID) (*vfs.File, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*vfs.File), args.Error(1)
}

func (m *MockVFS) Create(path string, r io.Reader) (vfs.Meta, error) {
	args := m.Called(path, r)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) Write(id uuid.UUID, r io.Reader) (vfs.Meta, error) {
	args := m.Called(id, r)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) WriteComments(id uuid.UUID, comment string) (vfs.Meta, error) {
	args := m.Called(id, comment)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) Delete(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockVFS) Mkdir(path string) (vfs.Meta, error) {
	args := m.Called(path)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) Stat(id uuid.UUID) (vfs.Meta, error) {
	args := m.Called(id)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) StatByPath(p string) (vfs.Meta, error) {
	args := m.Called(p)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) Move(id uuid.UUID, dst string) (vfs.Meta, error) {
	args := m.Called(id, dst)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) Copy(id uuid.UUID, dst string) (vfs.Meta, error) {
	args := m.Called(id, dst)
	return args.Get(0).(vfs.Meta), args.Error(1)
}

func (m *MockVFS) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockVFS) Backup(w io.Writer, since uint64) (uint64, error) {
	args := m.Called(w, since)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockVFS) Load(r io.Reader, maxPendingWrites int) error {
	args := m.Called(r, maxPendingWrites)
	return args.Error(0)
}

func (m *MockVFS) Tree(path string) (*vfs.TreeNode, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*vfs.TreeNode), args.Error(1)
}

func (m *MockVFS) Rotate(key []byte) error {
	args := m.Called(key)
	return args.Error(0)
}

func TestVfsHandler_List(t *testing.T) {
	// Setup
	mockVFS := new(MockVFS)
	broker := services.NewSSEBroker(services.SSEConfig{
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})
	defer broker.Shutdown()
	svc := services.NewVfsService(mockVFS, "/vfs")
	handler := NewVfsHandler(svc, broker)

	app := fiber.New()
	app.Get("/vfs", handler.List)

	// Mock Data
	mockMeta := []vfs.Meta{
		{
			ID:        uuid.New(),
			Name:      "test.txt",
			Path:      "/test.txt",
			Extension: "txt",
			Size:      100,
			IsDir:     false,
			Modified:  time.Now(),
		},
	}
	mockVFS.On("List", "/").Return(mockMeta, nil)

	// Test Request
	req, _ := http.NewRequest("GET", "/vfs?q=/", nil)
	resp, err := app.Test(req)

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var vfsRes types.VfsRes[[]types.MetaRes]
	body, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(body, &vfsRes)

	require.NoError(t, err)
	assert.Equal(t, types.ViewTypeList, vfsRes.ViewType)
	require.Len(t, vfsRes.Payload, 1)
	assert.Equal(t, "test.txt", vfsRes.Payload[0].Name)

	mockVFS.AssertExpectations(t)
}

func TestVfsHandler_Stat(t *testing.T) {
	// Setup
	mockVFS := new(MockVFS)
	broker := services.NewSSEBroker(services.SSEConfig{
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})
	defer broker.Shutdown()
	svc := services.NewVfsService(mockVFS, "/vfs")
	handler := NewVfsHandler(svc, broker)

	app := fiber.New()
	app.Get("/vfs/:id/stat", handler.Stat)

	// Mock Data
	id := uuid.New()
	mockMeta := vfs.Meta{
		ID:        id,
		Name:      "test.txt",
		Path:      "/test.txt",
		Extension: "txt",
		Size:      100,
		IsDir:     false,
		Modified:  time.Now(),
	}
	mockVFS.On("Stat", id).Return(mockMeta, nil)

	// Test Request
	req, _ := http.NewRequest("GET", "/vfs/"+id.String()+"/stat", nil)
	resp, err := app.Test(req)

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var metaRes types.MetaRes
	body, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(body, &metaRes)

	require.NoError(t, err)
	assert.Equal(t, id, metaRes.ID)
	assert.Equal(t, "test.txt", metaRes.Name)

	mockVFS.AssertExpectations(t)
}

func TestVfsHandler_Create(t *testing.T) {
	// Setup
	mockVFS := new(MockVFS)
	broker := services.NewSSEBroker(services.SSEConfig{
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})
	defer broker.Shutdown()
	svc := services.NewVfsService(mockVFS, "/vfs")
	handler := NewVfsHandler(svc, broker)

	app := fiber.New()
	app.Post("/vfs", handler.Create)

	// Mock Data
	mockMeta := vfs.Meta{
		ID:        uuid.New(),
		Name:      "new.txt",
		Path:      "/new.txt",
		Extension: "txt",
		Size:      12,
		IsDir:     false,
		Modified:  time.Now(),
	}
	mockVFS.On("Create", "new.txt", mock.Anything).Return(mockMeta, nil)

	// Create Multipart Request
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "new.txt")
	part.Write([]byte("Hello World!"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/vfs", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Client-ID", uuid.NewString())

	resp, err := app.Test(req)

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var metaRes types.MetaRes
	respBody, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(respBody, &metaRes)

	require.NoError(t, err)
	assert.Equal(t, "new.txt", metaRes.Name)

	mockVFS.AssertExpectations(t)
}

func TestVfsHandler_Delete(t *testing.T) {
	// Setup
	mockVFS := new(MockVFS)
	broker := services.NewSSEBroker(services.SSEConfig{
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})
	defer broker.Shutdown()
	svc := services.NewVfsService(mockVFS, "/vfs")
	handler := NewVfsHandler(svc, broker)

	app := fiber.New()
	app.Delete("/vfs/:id", handler.Delete)

	id := uuid.New()
	// Since Delete is async via broker, expectation might be delayed.
	// However, we are testing the Handler's response mostly.
	// The mock call happens in a goroutine.
	mockVFS.On("Delete", id).Return(nil)

	req, _ := http.NewRequest("DELETE", "/vfs/"+id.String(), nil)
	req.Header.Set("X-Client-ID", uuid.NewString())
	resp, err := app.Test(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Wait for async execution
	time.Sleep(100 * time.Millisecond)
	mockVFS.AssertExpectations(t)
}

func TestVfsHandler_Move(t *testing.T) {
	// Setup
	mockVFS := new(MockVFS)
	broker := services.NewSSEBroker(services.SSEConfig{
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})
	defer broker.Shutdown()
	svc := services.NewVfsService(mockVFS, "/vfs")
	handler := NewVfsHandler(svc, broker)

	app := fiber.New()
	app.Patch("/vfs/:id", handler.Move)

	id := uuid.New()
	mockMeta := vfs.Meta{
		ID:        id,
		Name:      "moved.txt",
		Path:      "/moved.txt",
		Extension: "txt",
		Size:      100,
		IsDir:     false,
		Modified:  time.Now(),
	}
	mockVFS.On("Move", id, "moved.txt").Return(mockMeta, nil)

	// Request Body
	dstReq := types.DstReq{Name: "moved.txt"}
	body, _ := json.Marshal(dstReq)

	req, _ := http.NewRequest("PATCH", "/vfs/"+id.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-ID", uuid.NewString())

	resp, err := app.Test(req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Wait for async execution
	time.Sleep(100 * time.Millisecond)
	mockVFS.AssertExpectations(t)
}
