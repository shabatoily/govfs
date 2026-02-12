package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
	"github.com/meteormin/govfs/bootstrap"
	"github.com/meteormin/govfs/config"
	_ "github.com/meteormin/govfs/docs"
)

var (
	name        = "govfs"
	version     = "0.0.1"
	buildTime   = time.Now().UTC().Format(time.RFC3339)
	description = "govfs is a virtual file system server"
)

// @title govfs
// @version 1.0.0
// @description govfs is a virtual file system server
// @contact.name meteormin
// @contact.email miniyu97@gmail.com
// @servers.url localhost:3000
// @servers.description Localhost
func main() {
	var configPath string

	_ = godotenv.Load()

	flag.StringVar(&configPath, "config", "config.toml", "config file path")
	flag.Parse()

	buildInfo, _ := debug.ReadBuildInfo()
	cfg, err := config.LoadWithViper(configPath, config.AppInfo{
		Name:        name,
		Version:     version,
		BuildTime:   buildTime,
		Description: description,
		BuildInfo:   buildInfo,
	})
	if err != nil {
		log.Fatal(err)
	}

	vfs, err := bootstrap.InitVFS(&cfg.VFS)
	if err != nil {
		log.Fatal(err)
	}

	app := bootstrap.InitServer(vfs, &cfg.Server)
	app.Hooks().OnListen(func(listenData fiber.ListenData) error {
		cfg.Server.Host = listenData.Host
		config.SetSwaggerInfo(cfg)
		return nil
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Listen(":"+strconv.Itoa(cfg.Server.Port), fiber.ListenConfig{
		GracefulContext: ctx,
	}); err != nil {
		log.Panic(err)
	}
}
