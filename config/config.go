package config

import (
	"runtime/debug"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/docs"
	"github.com/meteormin/govfs/drivers"
	"github.com/meteormin/govfs/drivers/badger"
	"github.com/meteormin/govfs/drivers/localstorage"
)

type AppInfo struct {
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	BuildTime   string           `json:"buildTime"`
	Description string           `json:"description"`
	BuildInfo   *debug.BuildInfo `json:"buildInfo"`
}

type ServerConfig struct {
	fiber.Config `json:"-"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	Logger       FiberLoggerConfig `json:"logger"`
	BasicAuth    BasicAuth         `json:"basicAuth"`
}
type FiberLoggerConfig struct {
	Path          string    `json:"path"`
	Level         log.Level `json:"level"`
	AccessLogPath string    `json:"accessLogPath"`
}

type BasicAuth struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}
type VfsConfig struct {
	Driver drivers.Config   `json:"driver"`
	Logger vfs.LoggerConfig `json:"logger"`
}

type CloudConfig struct {
	GoogleDrive GoogleDrive `json:"googleDrive"`
}

type GoogleDrive struct {
	ClientID       string `json:"-"`
	ClientSecret   string `json:"-"`
	ParentFolderID string `json:"parentFolderID"`
}

type Config struct {
	App    AppInfo      `json:"app"`
	Server ServerConfig `json:"server"`
	VFS    VfsConfig    `json:"vfs"`
	Cloud  CloudConfig  `json:"cloud"`
}

var DefaultConfig = &Config{
	Server: ServerConfig{
		Port: 3000,
	},
	VFS: VfsConfig{
		Driver: drivers.Config{
			Type: drivers.DriverTypeBadger,
			Badger: badger.Config{
				Path: "./data",
			},
			LocalStorage: localstorage.Config{
				Path: "./data",
			},
		},
	},
}

type ContextKeyConfig struct{}

func SetSwaggerInfo(cfg *Config) {
	docs.SwaggerInfo.Title = cfg.App.Name
	docs.SwaggerInfo.Description = cfg.App.Description
	docs.SwaggerInfo.Version = cfg.App.Version
	docs.SwaggerInfo.Host = cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
}
