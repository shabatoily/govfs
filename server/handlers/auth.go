package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/meteormin/govfs/config"
)

type AuthHandler struct {
	cfg config.AuthConfig
}

func NewAuthHandler(cfg config.AuthConfig) *AuthHandler {
	return &AuthHandler{
		cfg: cfg,
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	if !h.cfg.Enabled {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Auth is disabled, no need to login",
		})
	}

	var req LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Username != h.cfg.Username || req.Password != h.cfg.Password {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": h.cfg.Username,
		"exp": time.Now().Add(time.Hour * 24).Unix(), // 24 hours expiry
	})

	t, err := token.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not generate token"})
	}

	return c.JSON(fiber.Map{"token": t})
}
