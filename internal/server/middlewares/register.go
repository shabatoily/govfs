// Package middlewares는 Fiber 애플리케이션에서 사용하는 공통 및 커스텀 미들웨어를 제공합니다.
package middlewares

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/envvar"
	"github.com/gofiber/fiber/v3/middleware/etag"
	"github.com/gofiber/fiber/v3/middleware/expvar"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/pprof"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/responsetime"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server/middlewares/swaggo"
	"github.com/shabatoily/govfs/internal/types"
)

// Register는 애플리케이션 전반에 걸쳐 사용되는 공통 미들웨어들을 등록합니다.
func Register(app *fiber.App, cfg *config.ServerConfig) {
	// 복구(recover) 미들웨어 추가
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))

	// 요청 ID 할당 미들웨어
	app.Use(requestid.New())

	// 응답 시간 측정 미들웨어
	app.Use(responsetime.New(responsetime.Config{
		Next: func(c fiber.Ctx) bool {
			// SSE 연결인 경우 측정 생략
			return strings.HasPrefix(c.Path(), "/sse")
		},
	}))

	// 액세스 로그 미들웨어
	accessLog, accessLogCloser := accessLogWriter(cfg.Logger.AccessLogPath)
	app.Use(logger.New(
		logger.Config{
			Stream: accessLog,
			Next: func(c fiber.Ctx) bool {
				// 헬스체크 경로는 로깅 생략
				return strings.HasPrefix(c.Path(), "/healthz")
			},
		},
	))

	// ETag 미들웨어
	app.Use(etag.New(etag.Config{
		Next: func(c fiber.Ctx) bool {
			// SSE 응답에 대해서는 ETag 생성을 생략
			return strings.HasPrefix(c.Path(), "/sse")
		},
	}))

	// JWT 인증 미들웨어 초기화
	jwtAuth := JWTAuthMiddleware(cfg.Auth)

	// debug/vars 라우트 활성화 여부 확인
	if cfg.Middlewares.Expvar {
		app.All("/debug/vars", jwtAuth)
		app.Use(expvar.New()).Name("debug.vars")
	}

	// debug/pprof 라우트 활성화 여부 확인
	if cfg.Middlewares.Pprof {
		app.All("/debug/pprof/*", jwtAuth)
		app.Use(pprof.New()).Name("debug.pprof")
	}

	// 환경 변수 노출 라우트 활성화 여부 확인
	if cfg.Middlewares.Envvar {
		app.Get("/expose/envvars", jwtAuth, envvar.New()).Name("envvars")
	}

	// 설정 정보 노출 라우트 활성화 여부 확인
	if cfg.Middlewares.Config {
		app.Get("/config", jwtAuth, func(c fiber.Ctx) error {
			res := types.ConfigRes{
				AppName:      cfg.Fiber.AppName,
				ServerConfig: *cfg,
			}
			data, err := res.MarshalJSON()
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
			}
			return c.Status(fiber.StatusOK).Send(data)
		}).Name("config")
	}

	// 라우트 정보 노출 활성화 여부 확인
	if cfg.Middlewares.Route {
		app.Get("/routes", jwtAuth, func(c fiber.Ctx) error {
			return c.Status(fiber.StatusOK).JSON(app.GetRoutes())
		}).Name("routes")
	}

	// Swagger UI 활성화 여부 확인
	if cfg.Middlewares.Swagger {
		app.Get("/swagger/*", jwtAuth, swaggo.New(swaggo.Config{
			Title: cfg.Fiber.AppName,
		}))
	}

	// 헬스체크 엔드포인트
	app.Get("/healthz", healthcheck.New()).Name("healthz")

	// 서버 종료 전 처리 (Access Log 닫기)
	app.Hooks().OnPreShutdown(func() error {
		log.Info("Closing Access Log")
		return accessLogCloser()
	})
}

// accessLogWriter는 액세스 로그 파일 기록을 위한 Writer와 리소스 정리용 함수를 반환합니다.
func accessLogWriter(path string) (io.Writer, func() error) {
	// 파일 경로가 비어있으면 표준 출력(stdout)과 아무 작업도 하지 않는 클린업 함수를 반환합니다.
	if path == "" {
		return os.Stdout, func() error { return nil }
	}

	// 로그 파일을 위한 상위 디렉터리가 존재하는지 확인하고, 없다면 생성합니다.
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(path), vfs.DefaultDirMode); err != nil {
			// 디렉터리 생성 실패 시 표준 출력과 빈 클린업 함수를 반환합니다.
			return os.Stdout, func() error { return nil }
		}
	}

	// 추가(Append) 모드로 로그 파일을 엽니다.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, vfs.DefaultFileMode)
	if err != nil {
		// 파일 열기 실패 시 표준 출력과 빈 클린업 함수를 반환합니다.
		return os.Stdout, func() error { return nil }
	}

	// 표준 출력과 파일 양쪽에 동시에 기록하는 MultiWriter 및 파일 닫기 함수를 반환합니다.
	return io.MultiWriter(os.Stdout, f), func() error {
		return f.Close()
	}
}
