// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"time"

	jwtware "github.com/gofiber/contrib/v3/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/meteormin/govfs/internal/config"
	"github.com/meteormin/govfs/internal/types"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler는 사용자 인증 관련 요청을 처리합니다.
type AuthHandler struct {
	cfg config.AuthConfig
}

// NewAuthHandler는 새로운 AuthHandler 인스턴스를 생성합니다.
func NewAuthHandler(cfg config.AuthConfig) *AuthHandler {
	return &AuthHandler{
		cfg: cfg,
	}
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
	if !h.cfg.Enabled {
		return c.SendStatus(fiber.StatusNoContent)
	}

	var req types.LoginReq
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if req.Username != h.cfg.Username {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(h.cfg.Password), []byte(req.Password)); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid credentials")
	}

	// 토큰 생성
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": h.cfg.Username,
		"exp": float64(time.Now().Add(h.cfg.JWT.Exp).Unix()),
	})

	t, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	exp := time.Now().Add(h.cfg.JWT.Exp)

	c.Cookie(&fiber.Cookie{
		Name:     "ACCESS_TOKEN",
		Value:    t,
		Expires:  exp,
		HTTPOnly: true,
		Secure:   true,
		SameSite: fiber.CookieSameSiteStrictMode,
	})

	return c.Status(fiber.StatusOK).JSON(types.TokenRes{
		Username:  h.cfg.Username,
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
func (h *AuthHandler) Logout(c fiber.Ctx) error {
	if !h.cfg.Enabled {
		return c.SendStatus(fiber.StatusNoContent)
	}

	c.Cookie(&fiber.Cookie{
		Name:     "ACCESS_TOKEN",
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
func (h *AuthHandler) IsLoggedIn(c fiber.Ctx) error {
	if !h.cfg.Enabled {
		return c.SendStatus(fiber.StatusNoContent)
	}

	token := jwtware.FromContext(c)
	if token == nil {
		return fiber.ErrUnauthorized
	}

	sub, err := token.Claims.GetSubject()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	exp, err := token.Claims.GetExpirationTime()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(types.TokenRes{
		Username:  sub,
		Token:     token.Raw,
		ExpiresAt: exp.Time,
	})
}
