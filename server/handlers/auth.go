package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/meteormin/govfs/config"
	"github.com/meteormin/govfs/server/types"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	cfg config.AuthConfig
}

func NewAuthHandler(cfg config.AuthConfig) *AuthHandler {
	return &AuthHandler{
		cfg: cfg,
	}
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	if !h.cfg.Enabled {
		return c.SendStatus(fiber.StatusNoContent)
	}

	var req types.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if req.Username != h.cfg.Username {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(h.cfg.Password), []byte(req.Password)); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "Invalid credentials")
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": h.cfg.Username,
		"exp": time.Now().Add(h.cfg.JWT.Exp).Unix(), // 24 hours expiry
	})

	t, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	c.Cookie(&fiber.Cookie{
		Name:     "ACCESS_TOKEN",
		Value:    t,
		Expires:  time.Now().Add(h.cfg.JWT.Exp),
		HTTPOnly: true,
		Secure:   true,
		SameSite: fiber.CookieSameSiteStrictMode,
	})

	return c.SendStatus(fiber.StatusOK)
}

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

	return c.SendStatus(fiber.StatusOK)
}
