// Package bootstrap은 서버 및 VFS, 클라우드 스토리지의 초기화를 담당합니다.
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
	"github.com/meteormin/govfs/server/routes"
)

const banner = `
   ____             __     
  / ___| _____   __/ _|___ 
 | |  _ / _ \ \ / / |_/ __|
 | |_| | (_) \ V /|  _\__ \
  \____|\___/ \_/ |_| |___/
`

// InitServer는 Fiber 애플리케이션을 생성하고 라우트, 미들웨어, 이벤트 후크를 설정합니다.
func InitServer(fs vfs.VFS, cfg *config.ServerConfig) *fiber.App {
	// 에러 핸들러 설정
	cfg.Fiber.ErrorHandler = middlewares.ErrorHandler
	// JSON 인코더 및 디코더 설정
	cfg.Fiber.JSONEncoder = json.Marshal
	cfg.Fiber.JSONDecoder = json.Unmarshal

	// Fiber 로거 설정
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

	// 웹 라우트 설정
	routes.Web(app, &routes.DepsWeb{
		Context:      cfg.Context,
		VFS:          fs,
		Auth:         cfg.Auth,
		WebUIEnabled: cfg.WebUI.Enabled,
	})

	app.Hooks().OnPreStartupMessage(func(preMsgData *fiber.PreStartupMessageData) error {
		preMsgData.BannerHeader = banner
		return nil
	})

	// 서버 종료 전 리소스 정리 정의
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

// InitVFS는 주어진 설정을 기반으로 적절한 드라이버(Badger, LocalStorage 등)를 사용하여 VFS를 초기화합니다.
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

// InitCloud는 클라우드 스토리지 인터페이스를 초기화합니다.
func InitCloud(cfg *config.CloudConfig) (cloud.Storage, error) {
	return cloud.New(&cfg.Config)
}
