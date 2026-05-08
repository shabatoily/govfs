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
	"github.com/meteormin/govfs/internal/cloud"
	"github.com/meteormin/govfs/internal/config"
	"github.com/meteormin/govfs/internal/server/middlewares"
	"github.com/meteormin/govfs/internal/server/routes"
	"github.com/meteormin/govfs/pkg/drivers"
	vfsLog "github.com/meteormin/govfs/pkg/log"
)

const banner = `
   ____             __     
  / ___| _____   __/ _|___ 
 | |  _ / _ \ \ / / |_/ __|
 | |_| | (_) \ V /|  _\__ \
  \____|\___/ \_/ |_| |___/
`

type serverContext struct {
	Config  *config.ServerConfig
	Storage cloud.Storage
	VFS     vfs.VFS
}

// Init은 전체 설정을 읽어 클라우드, VFS를 초기화하고 최종적으로 Fiber 애플리케이션 인스턴스를 반환합니다.
func Init(cfg *config.Config) (*fiber.App, error) {
	storage, err := initCloud(&cfg.Cloud)
	if err != nil {
		return nil, err
	}

	fs, err := initVFS(&cfg.VFS)
	if err != nil {
		return nil, err
	}

	server := initServer(serverContext{
		Config:  &cfg.Server,
		Storage: storage,
		VFS:     fs,
	})

	return server, nil
}

// initServer는 Fiber 애플리케이션을 생성하고 라우트, 미들웨어, 이벤트 후크를 설정합니다.
func initServer(ctx serverContext) *fiber.App {
	cfg := ctx.Config
	fs := ctx.VFS
	storage := ctx.Storage

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

	// 공통 미들웨어 등록
	middlewares.Register(app, cfg)

	// 웹 라우트 설정
	routes.Register(app, &routes.Deps{
		Context:      cfg.Context,
		VFS:          fs,
		Auth:         cfg.Auth,
		Cloud:        storage,
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

// initVFS는 주어진 설정을 기반으로 적절한 드라이버(Badger, LocalStorage 등)를 사용하여 VFS를 초기화합니다.
func initVFS(cfg *config.VfsConfig) (vfs.VFS, error) {
	vfsLogger, err := vfsLog.NewLogger(cfg.Logger)
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

// initCloud는 클라우드 스토리지 인터페이스를 초기화합니다.
func initCloud(cfg *config.CloudConfig) (cloud.Storage, error) {
	return cloud.New(&cfg.Config)
}
