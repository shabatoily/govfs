package types

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleUser
}

type UserRes struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateUserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     Role   `json:"role"`
}

type UpdateUserReq struct {
	Password string `json:"password"`
	Role     *Role  `json:"role"`
	Disabled *bool  `json:"disabled"`
}

type StatusRes struct {
	Users      int            `json:"users"`
	OpenDrives int            `json:"openDrives"`
	System     StorageStatRes `json:"system"`
}

type StorageStatRes struct {
	Items int   `json:"items"`
	Size  int64 `json:"size"`
}

type UserDriveStatusRes struct {
	UserID   uuid.UUID `json:"userId"`
	Username string    `json:"username"`
	Open     bool      `json:"open"`
	Online   bool      `json:"online"`
	SSECount int       `json:"sseCount"`
	Items    int       `json:"items"`
	Size     int64     `json:"size"`
}

type UserEventRes struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"userId"`
	Username  string    `json:"username"`
	Action    string    `json:"action"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type UserEventPageRes struct {
	Items    []UserEventRes `json:"items"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Total    int            `json:"total"`
}

type SystemEntryRes struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	Value any    `json:"value"`
}

type SystemEntryPageRes struct {
	Items    []SystemEntryRes `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int              `json:"total"`
}
