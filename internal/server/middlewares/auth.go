// Package middlewares는 Fiber 애플리케이션에서 사용하는 공통 및 커스텀 미들웨어를 제공합니다.
package middlewares

import (
	jwtware "github.com/gofiber/contrib/v3/jwt"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/shabatoily/govfs/internal/config"
)

// JWTAuthMiddleware는 설정된 인증 방식(활성화 여부)에 따라 JWT 인증 미들웨어를 생성합니다.
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
