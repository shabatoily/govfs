package config

import (
	"runtime/debug"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/docs"
	"github.com/meteormin/govfs/drivers"
	"github.com/meteormin/govfs/drivers/badger"
	"github.com/meteormin/govfs/drivers/localstorage"
)

type AppInfo struct {
	Name        string           `json:"name" toml:"name" yaml:"name"`
	Version     string           `json:"version" toml:"version" yaml:"version"`
	BuildTime   string           `json:"buildTime" toml:"buildTime" yaml:"buildTime"`
	Description string           `json:"description" toml:"description" yaml:"description"`
	BuildInfo   *debug.BuildInfo `json:"buildInfo" toml:"buildInfo" yaml:"buildInfo"`
}

type ServerConfig struct {
	fiber.Config `json:"-" toml:"-" yaml:"-"`
	Host         string            `json:"host" toml:"host" yaml:"host"`
	Port         int               `json:"port" toml:"port" yaml:"port"`
	Logger       FiberLoggerConfig `json:"logger" toml:"logger" yaml:"logger"`
	BasicAuth    BasicAuth         `json:"basicAuth" toml:"basicAuth" yaml:"basicAuth"`
}
type FiberLoggerConfig struct {
	Path          string    `json:"path" toml:"path" yaml:"path"`
	Level         log.Level `json:"level" toml:"level" yaml:"level"`
	AccessLogPath string    `json:"accessLogPath" toml:"accessLogPath" yaml:"accessLogPath"`
}

type BasicAuth struct {
	Enabled  bool   `json:"enabled" toml:"enabled" yaml:"enabled"`
	Username string `json:"username" toml:"username" yaml:"username"`
	Password string `json:"-" toml:"-" yaml:"-"`
}

type VfsConfig struct {
	Driver drivers.Config   `json:"driver" toml:"driver" yaml:"driver"`
	Logger vfs.LoggerConfig `json:"logger" toml:"logger" yaml:"logger"`
}

type CloudConfig struct {
	GoogleDrive GoogleDrive `json:"googleDrive" toml:"googleDrive" yaml:"googleDrive"`
}

type GoogleDrive struct {
	ClientID       string `json:"-" toml:"-" yaml:"-"`
	ClientSecret   string `json:"-" toml:"-" yaml:"-"`
	ParentFolderID string `json:"parentFolderID" toml:"parentFolderID" yaml:"parentFolderID"`
}

type Config struct {
	App    AppInfo      `json:"app" toml:"app" yaml:"app"`
	Server ServerConfig `json:"server" toml:"server" yaml:"server"`
	VFS    VfsConfig    `json:"vfs" toml:"vfs" yaml:"vfs"`
	Cloud  CloudConfig  `json:"cloud" toml:"cloud" yaml:"cloud"`
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
	docs.SwaggerInfo.Host = cfg.Server.Host
}
