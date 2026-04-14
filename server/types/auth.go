// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

import (
	"time"

	"github.com/goccy/go-json"
)

// LoginRequest는 인증 요청을 위한 사용자 계정 정보를 담고 있는 구조체입니다.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenResponse는 인증 성공 시 발급되는 토큰 및 만료 정보를 담고 있는 구조체입니다.
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
