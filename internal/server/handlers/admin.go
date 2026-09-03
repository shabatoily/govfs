package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
)

type AdminHandler struct {
	users  *services.UserStore
	drives *services.DriveManager
	broker *services.SSEBroker
}

func NewAdminHandler(users *services.UserStore, drives *services.DriveManager, broker *services.SSEBroker) *AdminHandler {
	return &AdminHandler{users: users, drives: drives, broker: broker}
}

// ListUsers는 사용자 목록을 반환합니다.
// @Summary 사용자 목록
// @Tags admin
// @Success 200 {array} types.UserRes
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/users [get]
func (h *AdminHandler) ListUsers(c fiber.Ctx) error {
	list, err := h.users.List()
	if err != nil {
		return err
	}
	res := make([]types.UserRes, len(list))
	for i, user := range list {
		res[i] = userResponse(user)
	}
	return c.JSON(res)
}

// CreateUser는 사용자를 생성합니다.
// @Summary 사용자 생성
// @Tags admin
// @Param request body types.CreateUserReq true "user"
// @Success 201 {object} types.UserRes
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/users [post]
func (h *AdminHandler) CreateUser(c fiber.Ctx) error {
	var req types.CreateUserReq
	if bindErr := c.Bind().JSON(&req); bindErr != nil {
		return fiber.NewError(fiber.StatusBadRequest, bindErr.Error())
	}
	user, err := h.users.Create(req.Username, req.Password, req.Role)
	if errors.Is(err, services.ErrAlreadyExists) || errors.Is(err, services.ErrInvalidRole) || errors.Is(err, services.ErrInvalidPassword) {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(userResponse(user))
}

// UpdateUser는 역할, 비밀번호 또는 활성 상태를 변경합니다.
// @Summary 사용자 수정
// @Tags admin
// @Param id path string true "user id"
// @Param request body types.UpdateUserReq true "changes"
// @Success 200 {object} types.UserRes
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 404 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/users/{id} [patch]
func (h *AdminHandler) UpdateUser(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	var req types.UpdateUserReq
	if bindErr := c.Bind().JSON(&req); bindErr != nil {
		return fiber.NewError(fiber.StatusBadRequest, bindErr.Error())
	}
	user, err := h.users.Update(id, services.UserUpdate{Role: req.Role, Disabled: req.Disabled, Password: req.Password})
	if errors.Is(err, services.ErrNotFound) {
		return fiber.ErrNotFound
	}
	if errors.Is(err, services.ErrInvalidRole) || errors.Is(err, services.ErrLastAdmin) {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err != nil {
		return err
	}
	return c.JSON(userResponse(user))
}

func userResponse(user services.User) types.UserRes {
	return types.UserRes{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Disabled:  user.Disabled,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

// Status는 사용자 시스템의 집계 상태를 반환합니다.
// @Summary 사용자 시스템 상태
// @Tags admin
// @Success 200 {object} types.StatusRes
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/status [get]
func (h *AdminHandler) Status(c fiber.Ctx) error {
	list, err := h.users.List()
	if err != nil {
		return err
	}
	system, err := h.users.Stats()
	if err != nil {
		return err
	}
	badgerDrives, err := h.drives.BadgerResources()
	if err != nil {
		return err
	}
	return c.JSON(types.StatusRes{
		Users: len(list), OpenDrives: h.drives.OpenCount(), System: system, BadgerDrives: badgerDrives,
	})
}

// UserStatus는 한 사용자의 드라이브와 연결 상태를 반환합니다.
// @Summary 사용자 드라이브 상태
// @Tags admin
// @Param id path string true "user id"
// @Success 200 {object} types.UserDriveStatusRes
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 404 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/users/{id}/status [get]
func (h *AdminHandler) UserStatus(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	user, err := h.users.ByID(id)
	if errors.Is(err, services.ErrNotFound) {
		return fiber.ErrNotFound
	}
	if err != nil {
		return err
	}
	stats, wasOpen, err := h.drives.Stats(user.ID)
	if err != nil {
		return err
	}
	sseCount := len(h.broker.Clients(user.ID.String()))
	return c.JSON(types.UserDriveStatusRes{
		UserID: user.ID, Username: user.Username, Open: wasOpen,
		Online: sseCount > 0, SSECount: sseCount, Items: stats.Items, Size: stats.Size,
	})
}

// Events는 사용자 이벤트를 페이지 단위로 반환합니다.
// @Summary 사용자 이벤트
// @Tags admin
// @Param userId query string false "user id"
// @Param page query int false "page" default(1)
// @Param pageSize query int false "page size" default(20) maximum(100)
// @Success 200 {object} types.UserEventPageRes
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/events [get]
func (h *AdminHandler) Events(c fiber.Ctx) error {
	page, pageSize, err := pagination(c)
	if err != nil {
		return err
	}
	var userID *uuid.UUID
	if value := c.Query("userId"); value != "" {
		id, parseErr := uuid.Parse(value)
		if parseErr != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
		}
		userID = &id
	}
	events, total, err := h.users.ListEvents(page, pageSize, userID)
	if err != nil {
		return err
	}
	res := make([]types.UserEventRes, len(events))
	for i, event := range events {
		res[i] = types.UserEventRes(event)
	}
	return c.JSON(types.UserEventPageRes{Items: res, Page: page, PageSize: pageSize, Total: total})
}

// SystemEntries는 시스템 DB key와 안전하게 변환한 value를 반환합니다.
// @Summary 시스템 DB 상세
// @Tags admin
// @Param page query int false "page" default(1)
// @Param pageSize query int false "page size" default(20) maximum(100)
// @Success 200 {object} types.SystemEntryPageRes
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/system/entries [get]
func (h *AdminHandler) SystemEntries(c fiber.Ctx) error {
	page, pageSize, err := pagination(c)
	if err != nil {
		return err
	}
	entries, total, err := h.users.ListSystemEntries(page, pageSize)
	if err != nil {
		return err
	}
	return c.JSON(types.SystemEntryPageRes{Items: entries, Page: page, PageSize: pageSize, Total: total})
}

func pagination(c fiber.Ctx) (int, int, error) {
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "invalid page")
	}
	pageSize, err := strconv.Atoi(c.Query("pageSize", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "invalid page size")
	}
	return page, pageSize, nil
}

// ClearUserEvents는 한 사용자의 이벤트를 모두 삭제합니다.
// @Summary 사용자 이벤트 전체 삭제
// @Tags admin
// @Param id path string true "user id"
// @Success 204
// @Failure 400 {string} string
// @Failure 401 {string} string
// @Failure 403 {string} string
// @Failure 404 {string} string
// @Failure 500 {string} string
// @Security BearerAuth
// @Router /admin/users/{id}/events [delete]
func (h *AdminHandler) ClearUserEvents(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	if _, err := h.users.ByID(id); errors.Is(err, services.ErrNotFound) {
		return fiber.ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := h.users.ClearEvents(id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
