// Package routes는 서버의 엔드포인트를 정의하고 라우트를 등록합니다.
package routes

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/config"
	"github.com/meteormin/govfs/drivers/badger"
	"github.com/meteormin/govfs/server/handlers"
	"github.com/meteormin/govfs/server/middlewares"
	"github.com/meteormin/govfs/server/services"
	"github.com/meteormin/govfs/webui"
)

var (
	// PrefixVFS는 가상 파일 시스템 관련 API의 URL 접두사입니다.
	PrefixVFS = "/vfs"
	// PrefixSSE는 서버 전송 이벤트(SSE) 관련 API의 URL 접두사입니다.
	PrefixSSE = "/sse"
	// PrefixWebui는 내장 웹 UI 서비스의 기본 URL 접두사입니다.
	PrefixWebui = "/"
)

// DepsWeb은 웹 라우트 등록 시 필요한 의존성 객체들을 묶어 전달하기 위한 구조체입니다.
type DepsWeb struct {
	Context context.Context

	Auth         config.AuthConfig
	Cloud        cloud.Storage
	VFS          vfs.VFS
	WebUIEnabled bool
}

// Web은 Fiber 애플리케이션에 모든 웹 서비스 라우트를 등록합니다.
func Web(app *fiber.App, deps *DepsWeb) {
	sseBroker := services.NewSSEBroker(services.SSEConfig{
		Context:          deps.Context,
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})

	authHandler := handlers.NewAuthHandler(deps.Auth)

	cloudHandler := handlers.NewCloudHandler(deps.Cloud)

	sseHandler := handlers.NewSSEHandler(sseBroker)

	vfsService := services.NewVfsService(deps.VFS, PrefixVFS)

	vfsHandler := handlers.NewVfsHandler(vfsService, sseBroker)

	jwtAuth := middlewares.JWTAuthMiddleware(deps.Auth)

	app.Route("/auth", func(router fiber.Router) {
		router.Post("/login", authHandler.Login).Name("login")
		router.Post("/logout", authHandler.Logout).Name("logout")
		router.Get("/me", jwtAuth, authHandler.IsLoggedIn).Name("me")
	}, "auth.")

	// VFS Group with SSE Notification Middleware
	// We want to notify AFTER the handler executes, and the middleware logic does exactly that (Next() called first).
	// Routing /vfs/*
	app.Route("/vfs", func(router fiber.Router) {
		router.Use(jwtAuth)
		router.Post("/backup", vfsHandler.Backup).Name("backup")
		router.Post("/restore", vfsHandler.Restore).Name("restore")
		router.Post("/rotate", vfsHandler.Rotate).Name("rotate")
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
	}, "sse.")

	if bvfs, ok := deps.VFS.(*badger.BadgerVFS); ok {
		badgerHandler := handlers.NewBadgerHandler(bvfs)
		app.Route("/badger", func(router fiber.Router) {
			router.Use(jwtAuth)
			router.Get("/keys", badgerHandler.AllKeys).Name("keys")
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

	if deps.WebUIEnabled {
		app.Use(PrefixWebui, static.New("", static.Config{
			FS:     webui.FS,
			Browse: true,
		})).Name("webui")
	}

	app.Hooks().OnPreShutdown(func() error {
		sseBroker.Shutdown()
		return nil
	})
}
