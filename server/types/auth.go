package types

import (
	"time"

	"github.com/goccy/go-json"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (lr *LoginResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"username":  lr.Username,
		"expiresAt": lr.ExpiresAt.Format(time.RFC3339),
	})
}
