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
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/internal/config"
	"github.com/meteormin/govfs/internal/server/middlewares/swaggo"
	"github.com/meteormin/govfs/internal/types"
)

// CommonMiddlewares는 애플리케이션 전반에 걸쳐 사용되는 공통 미들웨어들을 등록합니다.
func CommonMiddlewares(app *fiber.App, cfg *config.ServerConfig) {
	// common middlewares

	// recover middleware
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))

	// request id middleware
	app.Use(requestid.New())

	// response time middleware
	app.Use(responsetime.New(responsetime.Config{
		Next: func(c fiber.Ctx) bool {
			// skip sse
			return strings.HasPrefix(c.Path(), "/sse")
		},
	}))

	// logging access log
	accessLog, accessLogCloser := AccessLogWriter(cfg.Logger.AccessLogPath)
	app.Use(logger.New(
		logger.Config{
			Stream: accessLog,
			Next: func(c fiber.Ctx) bool {
				// skip health check
				return strings.HasPrefix(c.Path(), "/healthz")
			},
		},
	))

	// etag middleware
	app.Use(etag.New(etag.Config{
		Next: func(c fiber.Ctx) bool {
			// skip sse
			return strings.HasPrefix(c.Path(), "/sse")
		},
	}))

	// jwt auth middleware
	jwtAuth := JWTAuthMiddleware(cfg.Auth)

	// debug/vars
	if cfg.Middlewares.Expvar {
		app.All("/debug/vars", jwtAuth)
		app.Use(expvar.New()).Name("debug.vars")
	}

	// debug/pprof
	if cfg.Middlewares.Pprof {
		app.All("/debug/pprof/*", jwtAuth)
		app.Use(pprof.New()).Name("debug.pprof")
	}

	// show environment variables
	if cfg.Middlewares.Envvar {
		app.Get("/expose/envvars", jwtAuth, envvar.New()).Name("envvars")
	}

	// show config
	if cfg.Middlewares.Config {
		app.Get("/config", jwtAuth, func(c fiber.Ctx) error {
			return c.Status(fiber.StatusOK).JSON(types.ConfigRes{
				AppName:      cfg.Fiber.AppName,
				ServerConfig: *cfg,
			})
		}).Name("config")
	}

	// show routes
	if cfg.Middlewares.Route {
		app.Get("/routes", jwtAuth, func(c fiber.Ctx) error {
			return c.Status(fiber.StatusOK).JSON(app.GetRoutes())
		}).Name("routes")
	}

	// swagger ui
	if cfg.Middlewares.Swagger {
		app.Get("/swagger/*", jwtAuth, swaggo.New(swaggo.Config{
			Title: cfg.Fiber.AppName,
		}))
	}

	// health check
	app.Get("/healthz", healthcheck.New()).Name("healthz")

	// on pre shutdown
	app.Hooks().OnPreShutdown(func() error {
		log.Info("Closing Access Log")
		return accessLogCloser()
	})
}

// AccessLogWriter는 액세스 로그 파일 기록을 위한 Writer와 리소스 정리용 함수를 반환합니다.
func AccessLogWriter(path string) (io.Writer, func() error) {
	// If no path is provided, log to standard output and return a no-op cleanup function.
	if path == "" {
		return os.Stdout, func() error { return nil }
	}

	// Ensure the directory for the log file exists.
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(path), vfs.DefaultDirMode); err != nil {
			// If directory creation fails, log to standard output and return a no-op cleanup.
			return os.Stdout, func() error { return nil }
		}
	}

	// Open the log file for appending.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, vfs.DefaultFileMode)
	if err != nil {
		// If file opening fails, log to standard output and return a no-op cleanup.
		return os.Stdout, func() error { return nil }
	}

	// Return a multi-writer that writes to both stdout and the file, along with a cleanup function to close the file.
	return io.MultiWriter(os.Stdout, f), func() error {
		return f.Close()
	}
}
