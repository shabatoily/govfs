// Package client는 govfs 서버와 통신하기 위한 API 클라이언트를 제공합니다.
package client

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/google/uuid"
	"github.com/meteormin/govfs/internal/types"
)

// VFSClient는 가상 파일 시스템(VFS) 연산 및 파일 전송 API 통신을 담당하는 클라이언트입니다.
type VFSClient struct {
	*baseClient
}

// List는 VFS의 파일 및 디렉토리 목록을 조회합니다.
func (c *VFSClient) List(q string) ([]types.MetaRes, error) {
	u := fmt.Sprintf("/vfs?q=%s", q)
	resp, err := c.c.Get(u)
	var res types.VfsRes[[]types.MetaRes]
	if checkErr := checkResponse(resp, err, fiber.StatusOK, &res); checkErr != nil {
		return nil, checkErr
	}
	return res.Payload, nil
}

// Read는 파일의 데이터와 메타데이터를 함께 조회합니다.
func (c *VFSClient) Read(id uuid.UUID) (io.Reader, types.MetaRes, error) {
	u := fmt.Sprintf("/vfs/%s", id.String())
	resp, err := c.c.Get(u)
	if err != nil {
		return nil, types.MetaRes{}, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, types.MetaRes{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	meta, err := c.Stat(id)
	if err != nil {
		return nil, types.MetaRes{}, err
	}

	return resp.BodyStream(), meta, nil
}

// Stat은 파일 또는 디렉토리의 상세 정보를 조회합니다.
func (c *VFSClient) Stat(id uuid.UUID) (types.MetaRes, error) {
	u := fmt.Sprintf("/vfs/%s/stat", id.String())
	resp, err := c.c.Get(u)
	var meta types.MetaRes
	if checkErr := checkResponse(resp, err, fiber.StatusOK, &meta); checkErr != nil {
		return types.MetaRes{}, checkErr
	}
	return meta, nil
}

// Tree는 계층적인 디렉토리 구조를 트리 형태로 조회합니다.
func (c *VFSClient) Tree(path string) (*types.TreeNodeRes, error) {
	u := fmt.Sprintf("/vfs?q=%s&viewType=tree", path)
	resp, err := c.c.Get(u)
	var res types.VfsRes[*types.TreeNodeRes]
	if checkErr := checkResponse(resp, err, fiber.StatusOK, &res); checkErr != nil {
		return nil, checkErr
	}
	return res.Payload, nil
}

// CreateDir은 새로운 디렉토리를 생성합니다.
func (c *VFSClient) CreateDir(name string) (types.MetaRes, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("isDir", "true"); err != nil {
		return types.MetaRes{}, err
	}
	if err := writer.WriteField("name", name); err != nil {
		return types.MetaRes{}, err
	}
	if err := writer.Close(); err != nil {
		return types.MetaRes{}, err
	}

	resp, err := c.c.R().
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetRawBody(body.Bytes()).
		Post("/vfs")

	var meta types.MetaRes
	if checkErr := checkResponse(resp, err, fiber.StatusCreated, &meta); checkErr != nil {
		return types.MetaRes{}, checkErr
	}
	return meta, nil
}

// CreateFile은 새로운 파일을 업로드하여 생성합니다.
func (c *VFSClient) CreateFile(name string, r io.ReadCloser) (types.MetaRes, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("isDir", "false"); err != nil {
		return types.MetaRes{}, err
	}
	if err := writer.WriteField("name", name); err != nil {
		return types.MetaRes{}, err
	}

	part, err := writer.CreateFormFile("file", filepath.Base(name))
	if err != nil {
		return types.MetaRes{}, err
	}
	if _, copyErr := io.Copy(part, r); copyErr != nil {
		return types.MetaRes{}, copyErr
	}

	if closeErr := writer.Close(); closeErr != nil {
		return types.MetaRes{}, closeErr
	}

	resp, err := c.c.R().
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetRawBody(body.Bytes()).
		Post("/vfs")

	var meta types.MetaRes
	if checkErr := checkResponse(resp, err, fiber.StatusCreated, &meta); checkErr != nil {
		return types.MetaRes{}, checkErr
	}
	return meta, nil
}

// Write는 기존 파일의 내용을 덮어씁니다. (비동기 처리)
func (c *VFSClient) Write(id uuid.UUID, content string) error {
	cfg, err := createJSONConfig(types.WriteReq{Content: content})
	if err != nil {
		return err
	}

	resp, err := c.c.Put("/vfs/"+id.String(), cfg)
	var meta types.MetaRes
	return checkResponse(resp, err, fiber.StatusAccepted, &meta)
}

// Move는 파일 또는 디렉토리의 이름을 변경하거나 다른 경로로 이동시킵니다. (비동기 처리)
func (c *VFSClient) Move(id uuid.UUID, dstName string) error {
	cfg, err := createJSONConfig(types.DstReq{Name: dstName})
	if err != nil {
		return err
	}
	// Move doesn't return a body we need to parse, passing nil
	resp, err := c.c.Patch("/vfs/"+id.String(), cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// Copy는 파일 또는 디렉토리를 지정된 경로로 복사합니다. (비동기 처리)
func (c *VFSClient) Copy(id uuid.UUID, dstName string) error {
	cfg, err := createJSONConfig(types.DstReq{Name: dstName})
	if err != nil {
		return err
	}
	resp, err := c.c.Post("/vfs/"+id.String(), cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// Delete는 파일 또는 디렉토리를 삭제합니다. (비동기 처리)
func (c *VFSClient) Delete(id uuid.UUID) error {
	resp, err := c.c.Delete("/vfs/" + id.String())
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// WriteComments는 항목에 대한 부가 설명을 추가합니다. (비동기 처리)
func (c *VFSClient) WriteComments(id uuid.UUID, comment string) error {
	cfg, err := createJSONConfig(types.WriteCommentReq{Comment: comment})
	if err != nil {
		return err
	}
	resp, err := c.c.Patch("/vfs/"+id.String()+"/comments", cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}

// Backup은 전체 VFS 데이터를 백업 파일로 받아옵니다.
func (c *VFSClient) Backup() (io.Reader, error) {
	resp, err := c.c.Post("/vfs/backup", client.Config{
		Header: map[string]string{
			"Content-Type": "application/json",
		},
	})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != fiber.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return bytes.NewReader(resp.Body()), nil
}

// Restore는 백업 파일로부터 VFS 데이터를 복구합니다.
func (c *VFSClient) Restore(file io.ReadCloser) error {
	f := &client.File{}
	f.SetFieldName("file")
	f.SetReader(file)
	f.SetName("backup")

	// We don't need to manually read the file here.
	// The client.Client will handle reading from f.Reader (which is `file`)
	// and constructing the multipart request.

	resp, err := c.c.Post("/vfs/restore", client.Config{
		File: []*client.File{f},
	})

	return checkResponse[*any](resp, err, fiber.StatusOK, nil)
}

// Rotate는 데이터 암호화 키를 교체합니다. (비동기 처리)
func (c *VFSClient) Rotate(newKey string) error {
	cfg, err := createJSONConfig(map[string]string{"key": newKey})
	if err != nil {
		return err
	}
	resp, err := c.c.Post("/vfs/rotate", cfg)
	return checkResponse[*any](resp, err, fiber.StatusAccepted, nil)
}
