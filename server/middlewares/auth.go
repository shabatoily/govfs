package middlewares

import (
	"strings"

	jwtware "github.com/gofiber/contrib/v3/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/meteormin/govfs/config"
)

// JWTAuthMiddleware creates a new JWT auth middleware
func JWTAuthMiddleware(cfg config.AuthConfig) fiber.Handler {
	if !cfg.Enabled {
		return func(ctx fiber.Ctx) error {
			return ctx.Next()
		}
	}

	return jwtware.New(jwtware.Config{
		SigningKey: jwtware.SigningKey{Key: []byte(cfg.JWT.Secret)},
		Extractor: extractors.Chain(
			extractors.FromAuthHeader("Bearer"),
			extractors.FromQuery("token"),
		),
		ErrorHandler: func(ctx fiber.Ctx, err error) error {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Unauthorized: " + err.Error(),
			})
		},
		Next: func(ctx fiber.Ctx) bool {
			path := ctx.Path()

			// Always skip login
			if path == "/auth/login" {
				return true
			}

			// Require auth for these specific paths/prefixes
			if strings.HasPrefix(path, "/vfs") ||
				strings.HasPrefix(path, "/sse") ||
				strings.HasPrefix(path, "/debug") ||
				strings.HasPrefix(path, "/expose") ||
				path == "/configs" ||
				path == "/routes" {
				return false // don't skip auth
			}

			// Skip auth for everything else (healthz, static UI files, etc.)
			return true
		},
	})
}
