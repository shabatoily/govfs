// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

import (
	"time"
	"uuid"
)

const (
	CookieAcessToken = "ACCESS_TOKEN"
)

// LoginReq는 인증 요청을 위한 사용자 계정 정보를 담고 있는 구조체입니다.
type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChangePasswordReq struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// TokenRes는 인증 성공 시 발급되는 토큰 및 만료 정보를 담고 있는 구조체입니다.
type TokenRes struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}
