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
	"github.com/meteormin/govfs/config"
)

// CommonMiddlewares returns a middleware that applies common middlewares to the app.
// It returns a function that closes the access log file.
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
	app.All("/debug/*", jwtAuth)
	app.All("/expose/*", jwtAuth)
	app.All("/configs", jwtAuth)
	app.All("/routes", jwtAuth)

	// debug/vars
	app.Use(expvar.New()).Name("debug.vars")
	// debug/pprof
	app.Use(pprof.New()).Name("debug.pprof")

	// health check
	app.Get("/healthz", healthcheck.New()).Name("healthz")

	// show environment variables
	app.Use("/expose/envvars", envvar.New()).Name("envvars")

	// show configs
	app.Get("/configs", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(cfg)
	}).Name("configs")

	// show routes
	app.Get("/routes", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(app.GetRoutes())
	}).Name("routes")

	// on pre shutdown
	app.Hooks().OnPreShutdown(func() error {
		log.Info("Closing Access Log")
		return accessLogCloser()
	})
}

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
