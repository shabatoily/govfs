package mcp

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type adminUserInput struct {
	ID string `json:"id" jsonschema:"user UUID"`
}

type adminPageInput struct {
	Page     *int `json:"page,omitempty" jsonschema:"page number (default 1)"`
	PageSize *int `json:"page_size,omitempty" jsonschema:"items per page from 1 to 100 (default 20)"`
}

type adminEventsInput struct {
	adminPageInput
	UserID string `json:"user_id,omitempty" jsonschema:"optional user UUID filter"`
}

// registerAdminTools는 조회 전용 관리자 도구만 등록합니다.
func (s *Server) registerAdminTools() {
	readOnly := &mcpsdk.ToolAnnotations{ReadOnlyHint: true}
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "admin_status",
		Description: "Get server system status and Badger drive resources. Requires admin privileges.",
		Annotations: readOnly,
	}, s.adminStatus)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "admin_users",
		Description: "List users. Requires admin privileges.",
		Annotations: readOnly,
	}, s.adminUsers)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "admin_user_status",
		Description: "Get a user's drive and connection status. Requires admin privileges.",
		Annotations: readOnly,
	}, s.adminUserStatus)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "admin_events",
		Description: "List paginated audit events with an optional user filter. Requires admin privileges.",
		Annotations: readOnly,
	}, s.adminEvents)
	mcpsdk.AddTool(s.sdk, &mcpsdk.Tool{
		Name:        "admin_system_entries",
		Description: "List paginated sanitized system database entries. Requires admin privileges.",
		Annotations: readOnly,
	}, s.adminSystemEntries)
}

func (s *Server) adminStatus(ctx context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, any, error) {
	result, err := s.client.Admin().Status(ctx)
	return nil, result, err
}

func (s *Server) adminUsers(ctx context.Context, _ *mcpsdk.CallToolRequest, _ struct{}) (*mcpsdk.CallToolResult, any, error) {
	result, err := s.client.Admin().ListUsers(ctx)
	return nil, result, err
}

func (s *Server) adminUserStatus(
	ctx context.Context, _ *mcpsdk.CallToolRequest, input adminUserInput,
) (*mcpsdk.CallToolResult, any, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid user ID: %w", err)
	}
	result, err := s.client.Admin().UserStatus(ctx, id)
	return nil, result, err
}

func (s *Server) adminEvents(ctx context.Context, _ *mcpsdk.CallToolRequest, input adminEventsInput) (*mcpsdk.CallToolResult, any, error) {
	page, pageSize, err := input.pagination()
	if err != nil {
		return nil, nil, err
	}
	if input.UserID != "" {
		if _, err := uuid.Parse(input.UserID); err != nil {
			return nil, nil, fmt.Errorf("invalid user ID: %w", err)
		}
	}
	result, err := s.client.Admin().Events(ctx, page, pageSize, input.UserID)
	return nil, result, err
}

func (s *Server) adminSystemEntries(
	ctx context.Context, _ *mcpsdk.CallToolRequest, input adminPageInput,
) (*mcpsdk.CallToolResult, any, error) {
	page, pageSize, err := input.pagination()
	if err != nil {
		return nil, nil, err
	}
	result, err := s.client.Admin().SystemEntries(ctx, page, pageSize)
	return nil, result, err
}

// pagination은 생략한 값에만 API 기본값을 적용합니다.
func (input adminPageInput) pagination() (int, int, error) {
	page, pageSize := 1, 20
	if input.Page != nil {
		page = *input.Page
	}
	if input.PageSize != nil {
		pageSize = *input.PageSize
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return 0, 0, errors.New("page must be positive and page size must be between 1 and 100")
	}
	return page, pageSize, nil
}
