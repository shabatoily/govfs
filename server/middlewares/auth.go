package middlewares

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/basicauth"
	"github.com/meteormin/govfs/config"
)

// BasicAuthMiddleware creates a new basic auth middleware
func BasicAuthMiddleware(cfg config.BasicAuth) fiber.Handler {
	// Auto-enable if credentials are provided but Enabled flag is false
	enabled := cfg.Enabled
	if cfg.Username == "" && cfg.Password == "" {
		enabled = false
	}

	if !enabled {
		return func(ctx fiber.Ctx) error {
			return ctx.Next()
		}
	}

	return basicauth.New(basicauth.Config{
		Users: map[string]string{
			cfg.Username: cfg.Password,
		},
		Realm: "VFS Private Server",
	})
}
