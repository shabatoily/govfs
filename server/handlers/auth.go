package handlers

import (
	"time"

	jwtware "github.com/gofiber/contrib/v3/jwt"
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

// Login logs in the user
// @Summary      Login
// @Description  login
// @Tags         auth
// @Accept       json
// @Param request body types.LoginRequest true "login request"
// @Success      200  {object}  types.TokenResponse
// @Failure      400  {object}  fiber.Error
// @Failure      401  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /auth/login [post]
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

	return c.Status(fiber.StatusOK).JSON(types.TokenResponse{
		Username:  h.cfg.Username,
		Token:     t,
		ExpiresAt: exp,
	})
}

// Logout logs out the user
// @Summary      Logout
// @Description  logout
// @Tags         auth
// @Success      204  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /auth/logout [post]
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

// IsLoggedIn checks if the user is logged in
// @Summary      IsLoggedIn
// @Description  is logged in
// @Tags         auth
// @Success      200  {object}  types.TokenResponse
// @Failure      500  {object}  fiber.Error
// @Router       /auth/me [get]
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

	return c.Status(fiber.StatusOK).JSON(types.TokenResponse{
		Username:  sub,
		Token:     token.Raw,
		ExpiresAt: exp.Time,
	})
}
