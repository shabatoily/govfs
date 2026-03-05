package bootstrap

import (
	"errors"
	"io"
	"os"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/config"
	"github.com/meteormin/govfs/drivers"
	"github.com/meteormin/govfs/server/middlewares"
	"github.com/meteormin/govfs/server/middlewares/swaggo"
	"github.com/meteormin/govfs/server/routes"
)

const banner = `
   ____             __     
  / ___| _____   __/ _|___ 
 | |  _ / _ \ \ / / |_/ __|
 | |_| | (_) \ V /|  _\__ \
  \____|\___/ \_/ |_| |___/
`

func InitServer(fs vfs.VFS, cfg *config.ServerConfig) *fiber.App {
	// Set error handler
	cfg.Fiber.ErrorHandler = middlewares.ErrorHandler
	// Set JSON encoder and decoder
	cfg.Fiber.JSONEncoder = json.Marshal
	cfg.Fiber.JSONDecoder = json.Unmarshal

	// Set fiber logger
	log.SetLevel(cfg.Logger.Level)
	fiberLogFile, err := os.Open(cfg.Logger.Path)
	if err != nil {
		log.SetOutput(os.Stdout)
	} else {
		w := io.MultiWriter(os.Stdout, fiberLogFile)
		log.SetOutput(w)
	}

	app := fiber.New(cfg.Fiber)

	middlewares.CommonMiddlewares(app, cfg)

	app.Get("/swagger/*", swaggo.New(swaggo.Config{
		Title: "Go VFS - Swagger UI",
	}))

	// web routes
	routes.Web(app, &routes.DepsWeb{
		Context: cfg.Context,
		VFS:     fs,
		Auth:    cfg.Auth,
	})

	app.Hooks().OnPreStartupMessage(func(preMsgData *fiber.PreStartupMessageData) error {
		preMsgData.BannerHeader = banner
		return nil
	})

	// on pre shutdown
	app.Hooks().OnPreShutdown(func() error {
		var err error
		if fs != nil {
			err = fs.Close()
		}
		if fiberLogFile != nil {
			err = errors.Join(err, fiberLogFile.Close())
		}
		return err
	})

	return app
}

func InitVFS(cfg *config.VfsConfig) (vfs.VFS, error) {
	vfsLogger, err := vfs.NewLogger(cfg.Logger)
	if err != nil {
		return nil, err
	}

	switch cfg.Driver.Type {
	case drivers.DriverTypeBadger:
		cfg.Driver.Badger.Logger = vfsLogger
	case drivers.DriverTypeLocalStorage:
		cfg.Driver.LocalStorage.Logger = vfsLogger
	}

	return drivers.New(&cfg.Driver)
}

func InitCloud(cfg *config.CloudConfig) (cloud.Storage, error) {
	return cloud.New(&cfg.Config)
}
