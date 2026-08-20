package server

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/goccy/go-json"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/static"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server/handlers"
	"github.com/shabatoily/govfs/internal/server/middlewares"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
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
	Config *config.ServerConfig
	Users  *services.UserStore
	Drives *services.DriveManager
}

// Init은 전체 설정을 읽어 VFS를 초기화하고 최종적으로 Fiber 애플리케이션 인스턴스를 반환합니다.
func Init(cfg *config.Config) (*fiber.App, error) {
	if cfg.VFS.Driver.Type != "badger" {
		return nil, fmt.Errorf("user drives require badger driver")
	}
	userStore, err := services.OpenUserStore(filepath.Join(filepath.Dir(cfg.VFS.Driver.Badger.Path), "system", "users"))
	if err != nil {
		return nil, err
	}
	list, err := userStore.List()
	if err != nil {
		_ = userStore.Close()
		return nil, err
	}
	if len(list) == 0 {
		if cfg.Server.Auth.Admin.Username == "" || cfg.Server.Auth.Admin.Password == "" {
			_ = userStore.Close()
			return nil, fmt.Errorf("initial admin credentials are required")
		}
		if _, err = userStore.Create(cfg.Server.Auth.Admin.Username, cfg.Server.Auth.Admin.Password, types.RoleAdmin); err != nil {
			_ = userStore.Close()
			return nil, err
		}
	}

	vfsLogger, err := vfsLog.NewLogger(cfg.VFS.Logger)
	if err != nil {
		_ = userStore.Close()
		return nil, err
	}
	cfg.VFS.Driver.Badger.Logger = vfsLogger
	drives := services.NewDriveManager(cfg.VFS.Driver.Badger)

	server := initServer(serverContext{
		Config: &cfg.Server,
		Users:  userStore,
		Drives: drives,
	})

	return server, nil
}

// initServer는 Fiber 애플리케이션을 생성하고 라우트, 미들웨어, 이벤트 후크를 설정합니다.
func initServer(ctx serverContext) *fiber.App {
	cfg := ctx.Config

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
	middlewares.Register(app, cfg, ctx.Users)

	// 웹 라우트 설정
	registerRoutes(app, ctx)

	app.Hooks().OnPreStartupMessage(func(preMsgData *fiber.PreStartupMessageData) error {
		preMsgData.BannerHeader = banner
		return nil
	})

	// 서버 종료 전 리소스 정리 정의
	app.Hooks().OnPreShutdown(func() error {
		var err error
		err = errors.Join(ctx.Drives.Close(), ctx.Users.Close())
		if fiberLogFile != nil {
			err = errors.Join(err, fiberLogFile.Close())
		}
		return err
	})

	return app
}

func registerRoutes(app *fiber.App, ctx serverContext) {
	sseBroker := services.NewSSEBroker(services.SSEConfig{
		Context:          ctx.Config.Context,
		MaxClientBuffer:  10,
		MaxMessageBuffer: 100,
	})

	authHandler := handlers.NewAuthHandler(ctx.Config.Auth, ctx.Users)
	adminHandler := handlers.NewAdminHandler(ctx.Users, ctx.Drives)

	sseHandler := handlers.NewSSEHandler(sseBroker)

	jwtAuth := middlewares.JWTAuthMiddleware(ctx.Config.Auth)
	userAuth := middlewares.UserMiddleware(ctx.Users)

	app.Route("/auth", func(router fiber.Router) {
		router.Post("/login", authHandler.Login).Name("login")
		router.Post("/logout", authHandler.Logout).Name("logout")
		router.Get("/me", jwtAuth, userAuth, authHandler.IsLoggedIn).Name("me")
	}, "auth.")

	app.Route("/admin", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth, middlewares.AdminOnly)
		router.Get("/users", adminHandler.ListUsers).Name("users")
		router.Post("/users", adminHandler.CreateUser).Name("create-user")
		router.Patch("/users/:id", adminHandler.UpdateUser).Name("update-user")
		router.Get("/status", adminHandler.Status).Name("status")
	}, "admin.")

	// VFS 라우트 그룹 (SSE 알림 미들웨어 포함 가능성)
	// 핸들러가 실행된 후 상태 변경을 알리기 위해 동작하도록 설계되었습니다.
	// 라우팅 경로: /vfs/*
	app.Route("/vfs", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth)
		router.Post("/backup", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Backup)).Name("backup")
		router.Post("/restore", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Restore)).Name("restore")
		router.Post("/", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Create)).Name("create")
		router.Get("/", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).List)).Name("list")
		router.Get("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Read)).Name("read")
		router.Get("/:id/stat", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Stat)).Name("stat")
		router.Put("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Write)).Name("write")
		router.Patch("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Move)).Name("move")
		router.Delete("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Delete)).Name("delete")
		router.Post("/:id/copy", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Copy)).Name("copy")
		router.Patch("/:id/comments", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).WriteComments)).Name("write-comments")
	}, "vfs.")

	app.Route("/sse", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth)
		router.Get("/subscribe", sseHandler.Subscribe).Name("subscribe")
		router.Post("/publish/:id?", sseHandler.Publish).Name("publish")
		router.Get("/clients", sseHandler.Clients).Name("clients")
	}, "sse.")

	app.Route("/badger", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth)
		router.Get("/keys", withBadger(ctx.Drives, (*handlers.BadgerHandler).AllKeys)).Name("keys")
		router.Get("/stats", withBadger(ctx.Drives, (*handlers.BadgerHandler).Stats)).Name("stats")
		router.Post("/rotate", withBadger(ctx.Drives, (*handlers.BadgerHandler).Rotate)).Name("rotate")
	}, "badger.")

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

func withVFS(drives *services.DriveManager, broker *services.SSEBroker, handler func(*handlers.VfsHandler, fiber.Ctx) error) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		user, ok := middlewares.CurrentUser(ctx)
		if !ok {
			return fiber.ErrUnauthorized
		}
		drive, err := drives.Drive(user.ID)
		if err != nil {
			return err
		}
		vfsHandler := handlers.NewVfsHandler(services.NewVfsService(drive, "/vfs"), broker, user.ID.String())
		return handler(vfsHandler, ctx)
	}
}

func withBadger(drives *services.DriveManager, handler func(*handlers.BadgerHandler, fiber.Ctx) error) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		user, ok := middlewares.CurrentUser(ctx)
		if !ok {
			return fiber.ErrUnauthorized
		}
		drive, err := drives.Drive(user.ID)
		if err != nil {
			return err
		}
		badgerDrive, ok := drive.(*badger.BadgerVFS)
		if !ok {
			return vfs.ErrNotSupported
		}
		return handler(handlers.NewBadgerHandler(badgerDrive), ctx)
	}
}
