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
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Auth is disabled, no need to login",
		})
	}

	var req types.LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Username != h.cfg.Username {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(h.cfg.Password), []byte(req.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": h.cfg.Username,
		"exp": time.Now().Add(h.cfg.JWT.Exp).Unix(), // 24 hours expiry
	})

	t, err := token.SignedString([]byte(h.cfg.JWT.Secret))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not generate token"})
	}

	return c.JSON(fiber.Map{"token": t})
}
