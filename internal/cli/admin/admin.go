package admin

import (
	"encoding/json"
	"uuid"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/shabatoily/govfs/internal/cli"
	"github.com/shabatoily/govfs/internal/client"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/spf13/cobra"
)

const (
	columnUserID   = "User ID"
	columnUsername = "Username"
)

// Handler는 관리자 API 호출과 결과 출력을 담당합니다.
type Handler struct {
	cmd    *cobra.Command
	client *client.AdminClient
}

// NewHandler는 기존 로그인 세션을 사용하는 관리자 핸들러를 반환합니다.
func NewHandler(cmd *cobra.Command) (*Handler, error) {
	c, err := cli.NewAuthenticatedClient(cmd)
	if err != nil {
		return nil, err
	}
	return &Handler{cmd: cmd, client: c.Admin()}, nil
}

// Status는 서버 상태 조회를 수행합니다.
func (h *Handler) Status() error {
	result, err := h.client.Status(h.cmd.Context())
	if err != nil {
		return err
	}
	w := table.NewWriter()
	w.AppendHeader(table.Row{"Users", "Open Drives", "System Items", "System Size (bytes)"})
	w.AppendRow(table.Row{result.Users, result.OpenDrives, result.System.Items, result.System.Size})
	h.cmd.Println(w.Render())

	w = table.NewWriter()
	w.SetTitle("Badger Drives")
	w.AppendHeader(table.Row{columnUserID, "LSM Size (bytes)", "Vlog Size (bytes)", "Block Cache Max Cost", "Index Cache Max Cost"})
	for _, drive := range result.BadgerDrives {
		w.AppendRow(table.Row{drive.UserID, drive.LSMSize, drive.VlogSize, drive.BlockCacheMaxCost, drive.IndexCacheMaxCost})
	}
	h.cmd.Println(w.Render())
	return nil
}

// ListUsers는 사용자 목록 조회를 수행합니다.
func (h *Handler) ListUsers() error {
	result, err := h.client.ListUsers(h.cmd.Context())
	if err != nil {
		return err
	}
	h.printUsers(result)
	return nil
}

// UserStatus는 사용자 상태 조회를 수행합니다.
func (h *Handler) UserStatus(id uuid.UUID) error {
	result, err := h.client.UserStatus(h.cmd.Context(), id)
	if err != nil {
		return err
	}
	w := table.NewWriter()
	w.AppendHeader(table.Row{columnUserID, columnUsername, "Open", "Online", "SSE Count", "Items", "Size (bytes)"})
	w.AppendRow(table.Row{result.UserID, result.Username, result.Open, result.Online, result.SSECount, result.Items, result.Size})
	h.cmd.Println(w.Render())
	return nil
}

// CreateUser는 사용자를 생성하고 결과를 출력합니다.
func (h *Handler) CreateUser(req types.CreateUserReq) error {
	result, err := h.client.CreateUser(h.cmd.Context(), req)
	if err != nil {
		return err
	}
	h.printUsers([]types.UserRes{result})
	return nil
}

// UpdateUser는 사용자를 수정하고 결과를 출력합니다.
func (h *Handler) UpdateUser(id uuid.UUID, req types.UpdateUserReq) error {
	result, err := h.client.UpdateUser(h.cmd.Context(), id, req)
	if err != nil {
		return err
	}
	h.printUsers([]types.UserRes{result})
	return nil
}

// ClearUserEvents는 사용자 이벤트 삭제를 수행합니다.
func (h *Handler) ClearUserEvents(id uuid.UUID) error {
	return h.client.ClearUserEvents(h.cmd.Context(), id)
}

// Events는 감사 이벤트 조회를 수행합니다.
func (h *Handler) Events(page, pageSize int, userID string) error {
	result, err := h.client.Events(h.cmd.Context(), page, pageSize, userID)
	if err != nil {
		return err
	}
	w := table.NewWriter()
	w.AppendHeader(table.Row{"ID", columnUserID, columnUsername, "Action", "Status", "Created"})
	for _, event := range result.Items {
		w.AppendRow(table.Row{event.ID, event.UserID, event.Username, event.Action, event.Status, event.CreatedAt})
	}
	h.cmd.Println(w.Render())
	h.printPagination(result.Page, result.PageSize, result.Total)
	return nil
}

// SystemEntries는 시스템 DB 조회를 수행합니다.
func (h *Handler) SystemEntries(page, pageSize int) error {
	result, err := h.client.SystemEntries(h.cmd.Context(), page, pageSize)
	if err != nil {
		return err
	}
	w := table.NewWriter()
	w.AppendHeader(table.Row{"Key", "Kind", "Value"})
	for _, entry := range result.Items {
		value, err := json.Marshal(entry.Value)
		if err != nil {
			return err
		}
		w.AppendRow(table.Row{entry.Key, entry.Kind, string(value)})
	}
	h.cmd.Println(w.Render())
	h.printPagination(result.Page, result.PageSize, result.Total)
	return nil
}

// printUsers는 목록과 생성·수정 응답에 같은 사용자 표를 사용합니다.
func (h *Handler) printUsers(users []types.UserRes) {
	w := table.NewWriter()
	w.AppendHeader(table.Row{"ID", columnUsername, "Role", "Disabled", "Created", "Updated"})
	for _, user := range users {
		w.AppendRow(table.Row{user.ID, user.Username, user.Role, user.Disabled, user.CreatedAt, user.UpdatedAt})
	}
	h.cmd.Println(w.Render())
}

func (h *Handler) printPagination(page, pageSize, total int) {
	h.cmd.Printf("Page: %d | Page size: %d | Total: %d\n", page, pageSize, total)
}
