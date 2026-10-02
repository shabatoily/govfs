package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

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

// ShutdownTimeout은 HTTP 요청과 비동기 드라이브 작업의 종료 대기 시간을 제한합니다.
const ShutdownTimeout = 5 * time.Second

const banner = `
   ____             __     
  / ___| _____   __/ _|___ 
 | |  _ / _ \ \ / / |_/ __|
 | |_| | (_) \ V /|  _\__ \
  \____|\___/ \_/ |_| |___/
`

type serverContext struct {
	Config    *config.Config
	Users     *services.UserStore
	Drives    *services.DriveManager
	VFSLogger *vfsLog.Logger
}

// Init은 전체 설정을 읽어 VFS를 초기화하고 최종적으로 Fiber 애플리케이션 인스턴스를 반환합니다.
func Init(cfg *config.Config) (*fiber.App, error) {
	var driveRoot string
	switch cfg.VFS.Driver.Type {
	case "badger":
		driveRoot = cfg.VFS.Driver.Badger.Path
	case "localstorage":
		driveRoot = cfg.VFS.Driver.LocalStorage.Path
	default:
		return nil, fmt.Errorf("unsupported VFS driver: %s", cfg.VFS.Driver.Type)
	}
	if driveRoot == "" {
		return nil, fmt.Errorf("VFS driver path is required")
	}
	userStore, err := services.OpenUserStore(filepath.Join(filepath.Dir(driveRoot), "system", "users"))
	if err != nil {
		return nil, err
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = userStore.Close()
		}
	}()
	list, err := userStore.List()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		if cfg.Server.Auth.Admin.Username == "" || cfg.Server.Auth.Admin.Password == "" {
			return nil, fmt.Errorf("initial admin credentials are required")
		}
		if _, err = userStore.Create(cfg.Server.Auth.Admin.Username, cfg.Server.Auth.Admin.Password, types.RoleAdmin); err != nil {
			return nil, err
		}
	}

	vfsLogger, err := vfsLog.NewLogger(cfg.VFS.Logger)
	if err != nil {
		return nil, err
	}
	cfg.VFS.Driver.Badger.Logger = vfsLogger
	cfg.VFS.Driver.LocalStorage.Logger = vfsLogger
	drives := services.NewDriveManager(services.DriveManagerConfig{
		Driver:      cfg.VFS.Driver,
		IdleTimeout: cfg.VFS.IdleTimeout,
	})

	server := initServer(serverContext{
		Config:    cfg,
		Users:     userStore,
		Drives:    drives,
		VFSLogger: vfsLogger,
	})

	initialized = true
	return server, nil
}

// initServer는 Fiber 애플리케이션을 생성하고 라우트, 미들웨어, 이벤트 후크를 설정합니다.
func initServer(ctx serverContext) *fiber.App {
	srvCfg := ctx.Config.Server

	// 에러 핸들러 설정
	srvCfg.Fiber.ErrorHandler = middlewares.ErrorHandler
	// JSON 인코더 및 디코더 설정
	srvCfg.Fiber.JSONEncoder = json.Marshal
	srvCfg.Fiber.JSONDecoder = json.Unmarshal

	// Fiber 로거 설정
	log.SetLevel(srvCfg.Logger.Level)
	fiberLogFile, err := os.OpenFile(srvCfg.Logger.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, vfs.DefaultFileMode)
	if err != nil {
		log.SetOutput(os.Stdout)
	} else {
		w := io.MultiWriter(os.Stdout, fiberLogFile)
		log.SetOutput(w)
	}

	app := fiber.New(srvCfg.Fiber)

	// 공통 미들웨어 등록
	middlewares.Register(app, &srvCfg, ctx.Users)

	// 웹 라우트 설정
	registerRoutes(app, ctx)

	app.Hooks().OnPreStartupMessage(func(preMsgData *fiber.PreStartupMessageData) error {
		preMsgData.BannerHeader = banner
		return nil
	})

	var shutdownDeadline time.Time
	app.Hooks().OnPreShutdown(func() error {
		shutdownDeadline = time.Now().Add(ShutdownTimeout)
		return nil
	})

	// Fiber의 직접 실행 경로는 종료 후크를 반복 호출하므로 리소스는 한 번만 정리합니다.
	var closeOnce sync.Once
	app.Hooks().OnPostShutdown(func(_ error) (err error) {
		closeOnce.Do(func() {
			// HTTP 요청과 비동기 작업은 같은 종료 대기 예산을 사용합니다.
			shutdownCtx, cancel := context.WithDeadline(context.Background(), shutdownDeadline)
			defer cancel()
			err = errors.Join(ctx.Drives.CloseWithContext(shutdownCtx), ctx.Users.Close(), ctx.VFSLogger.Close())
			if fiberLogFile != nil {
				log.SetOutput(os.Stdout)
				err = errors.Join(err, fiberLogFile.Close())
			}
		})
		return err
	})

	// 리슨 주소를 설정과 Swagger 정보에 반영합니다.
	app.Hooks().OnListen(func(listenData fiber.ListenData) error {
		srvCfg.Host = listenData.Host
		config.SetSwaggerInfo(ctx.Config)
		return nil
	})

	return app
}

func registerRoutes(app *fiber.App, ctx serverContext) {
	sseBroker := services.NewSSEBroker(services.SSEConfig{
		Context:          ctx.Config.Context(),
		MaxClientBuffer:  10,
		MaxMessageBuffer: 100,
	})

	srvCfg := ctx.Config.Server

	authHandler := handlers.NewAuthHandler(srvCfg.Auth, ctx.Users, ctx.Drives)
	adminHandler := handlers.NewAdminHandler(ctx.Users, ctx.Drives, sseBroker)

	sseHandler := handlers.NewSSEHandler(sseBroker)

	jwtAuth := middlewares.JWTAuthMiddleware(srvCfg.Auth)
	userAuth := middlewares.UserMiddleware(ctx.Users)

	app.Route("/auth", func(router fiber.Router) {
		router.Post("/login", authHandler.Login).Name("login")
		router.Post("/logout", jwtAuth, userAuth, authHandler.Logout).Name("logout")
		router.Get("/me", jwtAuth, userAuth, authHandler.IsLoggedIn).Name("me")
		router.Patch("/password", jwtAuth, userAuth, middlewares.Audit(ctx.Users), authHandler.ChangePassword).Name("password")
	}, "auth.")

	app.Route("/admin", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth, middlewares.AdminOnly, middlewares.Audit(ctx.Users))
		router.Get("/users", adminHandler.ListUsers).Name("users")
		router.Post("/users", adminHandler.CreateUser).Name("create-user")
		router.Patch("/users/:id", adminHandler.UpdateUser).Name("update-user")
		router.Get("/users/:id/status", adminHandler.UserStatus).Name("user-status")
		router.Delete("/users/:id/events", adminHandler.ClearUserEvents).Name("clear-user-events")
		router.Get("/status", adminHandler.Status).Name("status")
		router.Get("/system/entries", adminHandler.SystemEntries).Name("system-entries")
		router.Get("/events", adminHandler.Events).Name("events")
	}, "admin.")

	// VFS 라우트 그룹 (SSE 알림 미들웨어 포함 가능성)
	// 핸들러가 실행된 후 상태 변경을 알리기 위해 동작하도록 설계되었습니다.
	// 라우팅 경로: /vfs/*
	app.Route("/vfs", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth, middlewares.Audit(ctx.Users))
		router.Post("/backup", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Backup)).Name("backup")
		router.Post("/restore", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Restore)).Name("restore")
		router.Post("/", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Create)).Name("create")
		router.Get("/", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).List)).Name("list")
		router.Get("/search", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Search)).Name("search")
		router.Get("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Read)).Name("read")
		router.Get("/:id/stat", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Stat)).Name("stat")
		router.Put("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Write)).Name("write")
		router.Patch("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Move)).Name("move")
		router.Delete("/:id", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Delete)).Name("delete")
		router.Post("/:id/copy", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).Copy)).Name("copy")
		router.Patch("/:id/comments", withVFS(ctx.Drives, sseBroker, (*handlers.VfsHandler).WriteComments)).Name("write-comments")
	}, "vfs.")

	app.Route("/sse", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth, middlewares.Audit(ctx.Users))
		router.Get("/subscribe", sseHandler.Subscribe).Name("subscribe")
		router.Post("/publish/:id?", sseHandler.Publish).Name("publish")
		router.Get("/clients", sseHandler.Clients).Name("clients")
	}, "sse.")

	app.Route("/badger", func(router fiber.Router) {
		router.Use(jwtAuth, userAuth, middlewares.Audit(ctx.Users))
		router.Get("/keys", withBadger(ctx.Drives, (*handlers.BadgerHandler).AllKeys)).Name("keys")
		router.Get("/stats", withBadger(ctx.Drives, (*handlers.BadgerHandler).Stats)).Name("stats")
		router.Post("/rotate", withBadger(ctx.Drives, (*handlers.BadgerHandler).Rotate)).Name("rotate")
	}, "badger.")

	if srvCfg.WebUI.Enabled {
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
		drive, release, err := drives.Acquire(user.ID)
		if err != nil {
			return err
		}
		vfsHandler := handlers.NewVfsHandler(services.NewVfsService(drive, "/vfs"), broker, release, user.ID.String())
		defer vfsHandler.Close()
		return handler(vfsHandler, ctx)
	}
}

func withBadger(drives *services.DriveManager, handler func(*handlers.BadgerHandler, fiber.Ctx) error) fiber.Handler {
	return func(ctx fiber.Ctx) error {
		user, ok := middlewares.CurrentUser(ctx)
		if !ok {
			return fiber.ErrUnauthorized
		}
		drive, release, err := drives.Acquire(user.ID)
		if err != nil {
			return err
		}
		defer release()
		badgerDrive, ok := drive.(*badger.BadgerVFS)
		if !ok {
			return vfs.ErrNotSupported
		}
		return handler(handlers.NewBadgerHandler(badgerDrive), ctx)
	}
}
