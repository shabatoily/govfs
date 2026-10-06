package client

import (
	"context"
	"net/url"
	"strconv"
	"uuid"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/shabatoily/govfs/internal/types"
)

// AdminClient는 관리자 API 통신을 담당합니다.
type AdminClient struct{ *baseClient }

// Status는 해당 관리자 API를 호출합니다.
func (c *AdminClient) Status(ctx context.Context) (types.StatusRes, error) {
	var out types.StatusRes
	resp, err := c.c.Get("/admin/status", client.Config{Ctx: ctx})
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusOK, &out)
	return out, err
}

// ListUsers는 해당 관리자 API를 호출합니다.
func (c *AdminClient) ListUsers(ctx context.Context) ([]types.UserRes, error) {
	var out []types.UserRes
	resp, err := c.c.Get("/admin/users", client.Config{Ctx: ctx})
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusOK, &out)
	return out, err
}

// CreateUser는 해당 관리자 API를 호출합니다.
func (c *AdminClient) CreateUser(ctx context.Context, req types.CreateUserReq) (types.UserRes, error) {
	var out types.UserRes
	cfg, err := createJSONConfig(ctx, req)
	if err != nil {
		return out, err
	}
	resp, err := c.c.Post("/admin/users", cfg)
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusCreated, &out)
	return out, err
}

// UpdateUser는 해당 관리자 API를 호출합니다.
func (c *AdminClient) UpdateUser(ctx context.Context, id uuid.UUID, req types.UpdateUserReq) (types.UserRes, error) {
	var out types.UserRes
	cfg, err := createJSONConfig(ctx, req)
	if err != nil {
		return out, err
	}
	resp, err := c.c.Patch("/admin/users/"+id.String(), cfg)
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusOK, &out)
	return out, err
}

// UserStatus는 해당 관리자 API를 호출합니다.
func (c *AdminClient) UserStatus(ctx context.Context, id uuid.UUID) (types.UserDriveStatusRes, error) {
	var out types.UserDriveStatusRes
	resp, err := c.c.Get("/admin/users/"+id.String()+"/status", client.Config{Ctx: ctx})
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusOK, &out)
	return out, err
}

// Events는 해당 관리자 API를 호출합니다.
func (c *AdminClient) Events(ctx context.Context, page, pageSize int, userID string) (types.UserEventPageRes, error) {
	var out types.UserEventPageRes
	query := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(pageSize)}}
	if userID != "" {
		query.Set("userId", userID)
	}
	resp, err := c.c.Get("/admin/events?"+query.Encode(), client.Config{Ctx: ctx})
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusOK, &out)
	return out, err
}

// SystemEntries는 해당 관리자 API를 호출합니다.
func (c *AdminClient) SystemEntries(ctx context.Context, page, pageSize int) (types.SystemEntryPageRes, error) {
	var out types.SystemEntryPageRes
	query := url.Values{"page": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(pageSize)}}
	resp, err := c.c.Get("/admin/system/entries?"+query.Encode(), client.Config{Ctx: ctx})
	if err != nil {
		return out, err
	}
	err = checkResponse(resp, fiber.StatusOK, &out)
	return out, err
}

// ClearUserEvents는 사용자 이벤트를 모두 삭제합니다.
func (c *AdminClient) ClearUserEvents(ctx context.Context, id uuid.UUID) error {
	resp, err := c.c.Delete("/admin/users/"+id.String()+"/events", client.Config{Ctx: ctx})
	if err != nil {
		return err
	}
	return checkResponse[any](resp, fiber.StatusNoContent, nil)
}
