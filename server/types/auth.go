package types

import (
	"time"

	"github.com/goccy/go-json"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenResponse struct {
	Username  string    `json:"username"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (tr *TokenResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"username":  tr.Username,
		"token":     tr.Token,
		"expiresAt": tr.ExpiresAt.Format(time.RFC3339),
	})
}
