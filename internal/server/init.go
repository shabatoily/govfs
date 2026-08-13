package server

import (
	"errors"
	"io"
	"os"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/static"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/cloud"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server/handlers"
	"github.com/shabatoily/govfs/internal/server/middlewares"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/pkg/drivers"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
	vfsLog "github.com/shabatoily/govfs/pkg/log"
	"github.com/shabatoily/govfs/webui"
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

	// 에러 핸들러 설정
	cfg.Fiber.ErrorHandler = middlewares.ErrorHandler
	// JSON 인코더 및 디코더 설정
	cfg.Fiber.JSONEncoder = json.Marshal
	cfg.Fiber.JSONDecoder = json.Unmarshal

	// Fiber 로거 설정
	log.SetLevel(cfg.Logger.Level)
	fiberLogFile, err := os.OpenFile(cfg.Logger.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, vfs.DefaultFileMode)
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
	registerRoutes(app, ctx)

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

func registerRoutes(app *fiber.App, ctx serverContext) {
	sseBroker := services.NewSSEBroker(services.SSEConfig{
		Context:          ctx.Config.Context,
		MaxClientBuffer:  10,
		MaxMessageBuffer: 100,
	})

	authHandler := handlers.NewAuthHandler(ctx.Config.Auth)

	cloudHandler := handlers.NewCloudHandler(ctx.Storage)

	sseHandler := handlers.NewSSEHandler(sseBroker)

	vfsService := services.NewVfsService(ctx.VFS, "/vfs")

	vfsHandler := handlers.NewVfsHandler(vfsService, sseBroker)

	jwtAuth := middlewares.JWTAuthMiddleware(ctx.Config.Auth)

	app.Route("/auth", func(router fiber.Router) {
		router.Post("/login", authHandler.Login).Name("login")
		router.Post("/logout", authHandler.Logout).Name("logout")
		router.Get("/me", jwtAuth, authHandler.IsLoggedIn).Name("me")
	}, "auth.")

	// VFS 라우트 그룹 (SSE 알림 미들웨어 포함 가능성)
	// 핸들러가 실행된 후 상태 변경을 알리기 위해 동작하도록 설계되었습니다.
	// 라우팅 경로: /vfs/*
	app.Route("/vfs", func(router fiber.Router) {
		router.Use(jwtAuth)
		router.Post("/backup", vfsHandler.Backup).Name("backup")
		router.Post("/restore", vfsHandler.Restore).Name("restore")
		router.Post("/", vfsHandler.Create).Name("create")
		router.Get("/", vfsHandler.List).Name("list")
		router.Get("/:id", vfsHandler.Read).Name("read")
		router.Get("/:id/stat", vfsHandler.Stat).Name("stat")
		router.Put("/:id", vfsHandler.Write).Name("write")
		router.Patch("/:id", vfsHandler.Move).Name("move")
		router.Delete("/:id", vfsHandler.Delete).Name("delete")
		router.Post("/:id/copy", vfsHandler.Copy).Name("copy")
		router.Patch("/:id/comments", vfsHandler.WriteComments).Name("write-comments")
	}, "vfs.")

	app.Route("/sse", func(router fiber.Router) {
		router.Use(jwtAuth)
		router.Get("/subscribe", sseHandler.Subscribe).Name("subscribe")
		router.Post("/publish/:id?", sseHandler.Publish).Name("publish")
		router.Get("/clients", sseHandler.Clients).Name("clients")
	}, "sse.")

	if bvfs, ok := ctx.VFS.(*badger.BadgerVFS); ok {
		badgerHandler := handlers.NewBadgerHandler(bvfs)
		app.Route("/badger", func(router fiber.Router) {
			router.Use(jwtAuth)
			router.Get("/keys", badgerHandler.AllKeys).Name("keys")
			router.Get("/stats", badgerHandler.Stats).Name("stats")
			router.Post("/rotate", badgerHandler.Rotate).Name("rotate")
		}, "badger.")
	}

	app.Route(cloudHandler.Prefix(), func(router fiber.Router) {
		router.Get(cloudHandler.GoogleDriveCallbackURL(), cloudHandler.GoogleDriveCallback).Name("googledrive-callback")
		router.Use(jwtAuth)
		router.Get("/googledrive/auth", cloudHandler.IsAuthorized).Name("googledrive-auth-status")
		router.Post("/googledrive/auth", cloudHandler.GoogleDriveAuthCodeURL).Name("googledrive-auth")
		router.Get("/", cloudHandler.List).Name("list")
		router.Post("/", cloudHandler.Upload).Name("upload")
		router.Post("/download", cloudHandler.Download).Name("download")
		router.Delete("/", cloudHandler.Delete).Name("delete")
	}, "cloud.")

	if ctx.Config.WebUI.Enabled {
		app.Use("/", static.New("", static.Config{
			FS:     webui.FS,
			Browse: true,
		})).Name("webui")
	}

	app.Hooks().OnPreShutdown(func() error {
		sseBroker.Shutdown()
		return nil
	})
}
