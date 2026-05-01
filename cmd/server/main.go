// Package main은 govfs 서버의 진입점입니다.
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
	_ "github.com/meteormin/govfs/docs"
	"github.com/meteormin/govfs/internal/bootstrap"
	"github.com/meteormin/govfs/internal/config"
)

var (
	name        = "govfs"
	version     = "0.0.0"
	buildTime   = time.Now().UTC().Format(time.RFC3339)
	description = "govfs is a virtual file system server"
	configPath  = "config.toml"
)

// @title govfs
// @version 1.0.0
// @description govfs is a virtual file system server
// @contact.name meteormin
// @contact.email miniyu97@gmail.com
// @servers.url http://localhost:3000
// @servers.description Localhost
func main() {
	// signal context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// load .env
	_ = godotenv.Load()

	// parse flags
	flag.StringVar(&configPath, "config", "config.toml", "config file path")
	flag.Parse()

	// load config
	buildInfo, _ := debug.ReadBuildInfo()
	cfg, err := config.LoadWithViper(configPath, config.AppInfo{
		Name:        name,
		Version:     version,
		BuildTime:   buildTime,
		Description: description,
		BuildInfo:   buildInfo,
	})
	if err != nil {
		log.Panic(err)
	}

	// set context
	cfg.SetContext(ctx)

	// init server
	app, err := bootstrap.Init(cfg)
	if err != nil {
		log.Panic(err)
	}

	// set host to config and set swagger info on listen
	app.Hooks().OnListen(func(listenData fiber.ListenData) error {
		cfg.Server.Host = listenData.Host
		config.SetSwaggerInfo(cfg)
		return nil
	})

	// listen
	if err := app.Listen(":"+strconv.Itoa(cfg.Server.Port), fiber.ListenConfig{
		GracefulContext: ctx,
	}); err != nil {
		log.Panic(err)
	}
}
