// Package middlewares는 Fiber 애플리케이션에서 사용하는 공통 및 커스텀 미들웨어를 제공합니다.
package middlewares

import (
	jwtware "github.com/gofiber/contrib/v3/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/log"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
)

const currentUserKey = "current-user"

// JWTAuthMiddleware는 JWT 서명과 만료 시간을 검증합니다.
func JWTAuthMiddleware(cfg config.AuthConfig) fiber.Handler {
	return jwtware.New(jwtware.Config{
		SigningKey: jwtware.SigningKey{Key: []byte(cfg.JWT.Secret)},
		Extractor: extractors.Chain(
			extractors.FromAuthHeader("Bearer"),
			extractors.FromCookie(types.CookieAcessToken),
		),
	})
}

func UserMiddleware(store *services.UserStore) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		token := jwtware.FromContext(ctx)
		if token == nil {
			return fiber.ErrUnauthorized
		}
		subject, err := token.Claims.GetSubject()
		if err != nil {
			return fiber.ErrUnauthorized
		}
		id, err := uuid.Parse(subject)
		if err != nil {
			return fiber.ErrUnauthorized
		}
		user, err := store.ByID(id)
		if err != nil || user.Disabled {
			return fiber.ErrUnauthorized
		}
		ctx.Locals(currentUserKey, user)
		return ctx.Next()
	}
}

func CurrentUser(ctx fiber.Ctx) (services.User, bool) {
	user, ok := ctx.Locals(currentUserKey).(services.User)
	return user, ok
}

func AdminOnly(ctx fiber.Ctx) error {
	user, ok := CurrentUser(ctx)
	if !ok {
		return fiber.ErrUnauthorized
	}
	if user.Role != types.RoleAdmin {
		return fiber.ErrForbidden
	}
	return ctx.Next()
}

func Audit(store *services.UserStore) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		err := ctx.Next()
		if ctx.Method() == fiber.MethodGet {
			return err
		}
		user, ok := CurrentUser(ctx)
		if !ok {
			return err
		}
		status := ctx.Response().StatusCode()
		if fiberErr, ok := err.(*fiber.Error); ok {
			status = fiberErr.Code
		}
		if recordErr := store.RecordEvent(user, ctx.Route().Name, status); recordErr != nil {
			log.Errorf("failed to record user event: %v", recordErr)
		}
		return err
	}
}
