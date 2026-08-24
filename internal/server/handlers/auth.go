// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"errors"
	"time"

	jwtware "github.com/gofiber/contrib/v3/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server/middlewares"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
)

// AuthHandler는 사용자 인증 관련 요청을 처리합니다.
type AuthHandler struct {
	cfg   config.AuthConfig
	users *services.UserStore
}

// NewAuthHandler는 새로운 AuthHandler 인스턴스를 생성합니다.
func NewAuthHandler(cfg config.AuthConfig, users *services.UserStore) *AuthHandler {
	return &AuthHandler{cfg: cfg, users: users}
}

// Login은 사용자 로그인을 처리합니다.
// @Summary      로그인
// @Description  사용자 자격 증명을 확인하고 JWT 토큰을 발급합니다.
// @Tags         auth
// @Accept       json
// @Param request body types.LoginReq true "login request"
// @Success      200  {object}  types.TokenRes
// @Failure      400  {object}  fiber.Error
// @Failure      401  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /auth/login [post]
// Login은 사용자 로그인을 처리하고 JWT 토큰을 발급합니다.
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req types.LoginReq
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	user, err := h.users.Authenticate(req.Username, req.Password)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid credentials")
	}

	// 토큰 생성
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID.String(),
		"exp": float64(time.Now().Add(h.cfg.JWT.Exp).Unix()),
	})

	t, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	exp := time.Now().Add(h.cfg.JWT.Exp)
	if err := h.users.RecordEvent(user, "auth.login", fiber.StatusOK); err != nil {
		log.Errorf("failed to record login event: %v", err)
	}

	c.Cookie(&fiber.Cookie{
		Name:     types.CookieAcessToken,
		Value:    t,
		Expires:  exp,
		HTTPOnly: true,
		Secure:   true,
		SameSite: fiber.CookieSameSiteStrictMode,
	})

	return c.Status(fiber.StatusOK).JSON(types.TokenRes{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Token:     t,
		ExpiresAt: exp,
	})
}

// Logout은 사용자 로그아웃을 처리합니다.
// @Summary      로그아웃
// @Description  클라이언트의 JWT 토큰 쿠키를 삭제하여 로그아웃 처리합니다.
// @Tags         auth
// @Success      204  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /auth/logout [post]
// Logout은 사용자 로그아웃을 처리하고 클라이언트의 토큰 쿠키를 삭제합니다.
func (*AuthHandler) Logout(c fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     types.CookieAcessToken,
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
		Secure:   true,
		SameSite: fiber.CookieSameSiteStrictMode,
	})

	return c.SendStatus(fiber.StatusNoContent)
}

// IsLoggedIn은 사용자의 로그인 상태를 확인합니다.
// @Summary      로그인 상태 확인
// @Description  현재 토큰의 유효성을 검증하고 사용자 정보를 반환합니다.
// @Tags         auth
// @Success      200  {object}  types.TokenRes
// @Failure      500  {object}  fiber.Error
// @Router       /auth/me [get]
// IsLoggedIn은 현재 사용자의 로그인 상태를 확인하고 정보를 반환합니다.
func (*AuthHandler) IsLoggedIn(c fiber.Ctx) error {
	token := jwtware.FromContext(c)
	user, ok := middlewares.CurrentUser(c)
	if token == nil || !ok {
		return fiber.ErrUnauthorized
	}

	exp, err := token.Claims.GetExpirationTime()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(types.TokenRes{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Token:     token.Raw,
		ExpiresAt: exp.Time,
	})
}

// ChangePassword는 현재 사용자의 비밀번호를 변경합니다.
// @Summary 비밀번호 변경
// @Tags auth
// @Param request body types.ChangePasswordReq true "passwords"
// @Success 204
// @Router /auth/password [patch]
func (h *AuthHandler) ChangePassword(c fiber.Ctx) error {
	user, ok := middlewares.CurrentUser(c)
	if !ok {
		return fiber.ErrUnauthorized
	}
	var req types.ChangePasswordReq
	if err := c.Bind().JSON(&req); err != nil || req.NewPassword == "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid password")
	}
	if _, err := h.users.Authenticate(user.Username, req.CurrentPassword); errors.Is(err, services.ErrInvalidPassword) {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid credentials")
	} else if err != nil {
		return err
	}
	if _, err := h.users.Update(user.ID, services.UserUpdate{Password: req.NewPassword}); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
