package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxUploadSize = 10 << 20

type pathInput struct {
	Path string `json:"path" jsonschema:"absolute VFS path"`
}

type idInput struct {
	ID string `json:"id" jsonschema:"VFS resource UUID"`
}

type uploadInput struct {
	Path          string `json:"path" jsonschema:"absolute destination VFS path"`
	ContentBase64 string `json:"content_base64" jsonschema:"base64-encoded file content"`
}

func (s *Server) registerTools() {
	readOnly := mcpsdk.ToolAnnotations{ReadOnlyHint: true}
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "vfs_tree",
		Description: "List the VFS tree below an absolute path.",
		Annotations: &readOnly,
	}, s.tree)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "vfs_stat",
		Description: "Get metadata for a VFS resource UUID.",
		Annotations: &readOnly,
	}, s.stat)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "vfs_mkdir",
		Description: "Create a directory at an absolute VFS path and wait for completion.",
	}, s.mkdir)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "vfs_upload",
		Description: "Upload base64-encoded content and wait for completion. Maximum decoded size is 10 MiB.",
	}, s.upload)
	destructive := true
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "vfs_delete",
		Description: "Delete a VFS resource UUID and wait for completion.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: true},
	}, s.delete)
}

func (s *Server) tree(ctx context.Context, _ *mcpsdk.CallToolRequest, input pathInput) (*mcpsdk.CallToolResult, any, error) {
	cleaned, err := cleanPath(input.Path)
	if err != nil {
		return nil, nil, err
	}
	tree, err := s.client.VFS().Tree(ctx, cleaned)
	return nil, tree, err
}

func (s *Server) stat(ctx context.Context, _ *mcpsdk.CallToolRequest, input idInput) (*mcpsdk.CallToolResult, any, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid resource ID: %w", err)
	}
	meta, err := s.client.VFS().Stat(ctx, id)
	return nil, meta, err
}

func (s *Server) mkdir(ctx context.Context, _ *mcpsdk.CallToolRequest, input pathInput) (*mcpsdk.CallToolResult, any, error) {
	cleaned, err := cleanPath(input.Path)
	if err != nil {
		return nil, nil, err
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.client.VFS().CreateDir(ctx, cleaned); err != nil {
		return nil, nil, err
	}
	meta, err := s.waitForMutation(ctx, "vfs.create")
	if err != nil {
		return nil, nil, err
	}
	created, err := s.client.VFS().Stat(ctx, meta.ID)
	return nil, created, err
}

func (s *Server) upload(ctx context.Context, _ *mcpsdk.CallToolRequest, input uploadInput) (*mcpsdk.CallToolResult, any, error) {
	cleaned, err := cleanPath(input.Path)
	if err != nil {
		return nil, nil, err
	}
	content, err := decodeContent(input.ContentBase64)
	if err != nil {
		return nil, nil, err
	}

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.client.VFS().CreateFile(ctx, cleaned, io.NopCloser(bytes.NewReader(content))); err != nil {
		return nil, nil, err
	}
	meta, err := s.waitForMutation(ctx, "vfs.create")
	if err != nil {
		return nil, nil, err
	}
	created, err := s.client.VFS().Stat(ctx, meta.ID)
	return nil, created, err
}

func (s *Server) delete(ctx context.Context, _ *mcpsdk.CallToolRequest, input idInput) (*mcpsdk.CallToolResult, any, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid resource ID: %w", err)
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if err := s.client.VFS().Delete(ctx, id); err != nil {
		return nil, nil, err
	}
	meta, err := s.waitForMutation(ctx, "vfs.delete")
	return nil, meta, err
}

func cleanPath(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, "/") {
		return "", errors.New("path must be absolute")
	}
	return path.Clean(value), nil
}

func decodeContent(value string) ([]byte, error) {
	if base64.StdEncoding.DecodedLen(len(value)) > maxUploadSize {
		return nil, errors.New("decoded upload exceeds 10 MiB")
	}
	content, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 content: %w", err)
	}
	if len(content) > maxUploadSize {
		return nil, errors.New("decoded upload exceeds 10 MiB")
	}
	return content, nil
}
