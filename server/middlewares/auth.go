package middlewares

import (
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
			extractors.FromCookie("ACCESS_TOKEN"),
		),
	})
}
