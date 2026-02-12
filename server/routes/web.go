package routes

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/server/handlers"
	"github.com/meteormin/govfs/server/services"
	"github.com/meteormin/govfs/webui"
)

var (
	// endpoints
	PrefixVFS   = "/vfs"
	PrefixSSE   = "/sse"
	PrefixWebui = "/"
)

type DepsWeb struct {
	VFS vfs.VFS
}

func Web(app *fiber.App, deps DepsWeb) {
	sseBroker := services.NewSSEBroker(services.SSEConfig{
		MaxClients:       10,
		MaxMessageBuffer: 100,
	})

	sseHandler := handlers.NewSSEHandler(sseBroker)

	vfsService := services.NewVfsService(deps.VFS, PrefixVFS)

	vfsHandler := handlers.NewVfsHandler(vfsService, sseBroker)

	// VFS Group with SSE Notification Middleware
	// We want to notify AFTER the handler executes, and the middleware logic does exactly that (Next() called first).
	vfsGroup := app.Group(vfsHandler.Prefix()).Name("vfs.")

	// Routing /vfs/*
	vfsGroup.Post("/backup", vfsHandler.Backup).Name("backup")
	vfsGroup.Post("/restore", vfsHandler.Restore).Name("restore")
	vfsGroup.Post("/rotate", vfsHandler.Rotate).Name("rotate")

	vfsGroup.Post("/", vfsHandler.Create).Name("create")
	vfsGroup.Get("/", vfsHandler.List).Name("list")
	vfsGroup.Get("/:id", vfsHandler.Read).Name("read")
	vfsGroup.Get("/:id/stat", vfsHandler.Stat).Name("stat")
	vfsGroup.Put("/:id", vfsHandler.Write).Name("write")
	vfsGroup.Patch("/:id", vfsHandler.Move).Name("move")
	vfsGroup.Delete("/:id", vfsHandler.Delete).Name("delete")
	vfsGroup.Post("/:id/copy", vfsHandler.Copy).Name("copy")
	vfsGroup.Patch("/:id/comments", vfsHandler.WriteComments).Name("write-comments")

	app.Group(PrefixSSE).Name("sse.").
		Get("/subscribe", sseHandler.Subscribe).Name("subscribe").
		Post("/publish/:id?", sseHandler.Publish).Name("publish")

	app.Use(PrefixWebui, static.New("", static.Config{
		FS:     webui.FS,
		Browse: true,
	})).Name("webui")

	app.Hooks().OnPreShutdown(func() error {
		sseBroker.Shutdown()
		return nil
	})
}
