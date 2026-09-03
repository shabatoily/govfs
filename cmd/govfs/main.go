// Package main은 govfs 서버의 진입점입니다.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
	"github.com/shabatoily/govfs/cmd"
	_ "github.com/shabatoily/govfs/docs"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server"
)

var configPath = "config.toml"

// @title govfs
// @version 1.0.0
// @description govfs is a virtual file system server
// @contact.name meteormin
// @contact.email miniyu97@gmail.com
// @servers.url http://localhost:3000
// @servers.description Localhost
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer 토큰을 "Bearer {token}" 형식으로 입력합니다.
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
			panic(err)
		}
		configPath = filepath.Join(baseDir, ".govfs", "config.toml")
	}

	// load config
	appInfo := cmd.GetAppInfo()
	cfg, err := config.LoadWithViper(configPath, appInfo)
	if err != nil {
		panic(err)
	}

	// set context
	cfg.SetContext(ctx)

	// init server
	app, err := server.Init(cfg)
	if err != nil {
		panic(err)
	}

	// listen
	if err := app.Listen(":"+strconv.Itoa(cfg.Server.Port), fiber.ListenConfig{
		GracefulContext: ctx,
	}); err != nil {
		panic(err)
	}
}
