// Package main은 govfs 서버의 진입점입니다.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/joho/godotenv"
	_ "github.com/shabatoily/govfs/docs"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server"
)

var (
	name        = "govfs"
	version     = "dev"
	buildTime   = "unknown"
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
	flag.StringVar(&configPath, "config", "", "config file path")
	flag.Parse()

	if configPath == "" {
		baseDir, err := os.UserHomeDir()
		if err != nil {
			log.Panic(err)
		}
		configPath = baseDir
	}

	configPath = filepath.Join(configPath, ".govfs")

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
	app, err := server.Init(cfg)
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
