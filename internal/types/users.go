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
	Users      int `json:"users"`
	OpenDrives int `json:"openDrives"`
}
