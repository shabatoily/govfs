package handlers

import (
	"errors"

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
// @Router /admin/users [post]
func (h *AdminHandler) CreateUser(c fiber.Ctx) error {
	var req types.CreateUserReq
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
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
// @Router /admin/users/{id} [patch]
func (h *AdminHandler) UpdateUser(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	var req types.UpdateUserReq
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
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
	return types.UserRes{ID: user.ID, Username: user.Username, Role: user.Role, Disabled: user.Disabled, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
}

// Status는 사용자 시스템의 집계 상태를 반환합니다.
// @Summary 사용자 시스템 상태
// @Tags admin
// @Success 200 {object} types.StatusRes
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
	drives := make([]types.UserDriveStatusRes, len(list))
	for i, user := range list {
		stats, wasOpen, err := h.drives.Stats(user.ID)
		if err != nil {
			return err
		}
		sseCount := len(h.broker.Clients(user.ID.String()))
		drives[i] = types.UserDriveStatusRes{
			UserID: user.ID, Username: user.Username, Open: wasOpen,
			Online: sseCount > 0, SSECount: sseCount, Items: stats.Items, Size: stats.Size,
		}
	}
	return c.JSON(types.StatusRes{Users: len(list), OpenDrives: h.drives.OpenCount(), System: system, Drives: drives})
}

// Events는 최근 사용자 이벤트를 반환합니다.
// @Summary 최근 사용자 이벤트
// @Tags admin
// @Param userId query string false "user id"
// @Success 200 {array} types.UserEventRes
// @Router /admin/events [get]
func (h *AdminHandler) Events(c fiber.Ctx) error {
	var userID *uuid.UUID
	if value := c.Query("userId"); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
		}
		userID = &id
	}
	events, err := h.users.ListEvents(100, userID)
	if err != nil {
		return err
	}
	res := make([]types.UserEventRes, len(events))
	for i, event := range events {
		res[i] = types.UserEventRes(event)
	}
	return c.JSON(res)
}
