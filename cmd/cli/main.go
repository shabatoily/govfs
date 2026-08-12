// Package main은 govfs CLI 도구의 진입점입니다.
package main

import (
	"context"
	"log"
	"runtime/debug"

	"github.com/joho/godotenv"
	"github.com/shabatoily/govfs/internal/cli"
	"github.com/shabatoily/govfs/internal/cli/cloud"
	"github.com/shabatoily/govfs/internal/cli/secret"
	"github.com/shabatoily/govfs/internal/cli/vfs"
	"github.com/shabatoily/govfs/internal/config"
)

var (
	name        = "govfs"
	version     = "dev"
	buildTime   = "unknown"
	description = "govfs is a virtual file system server"
)

func main() {
	_ = godotenv.Load()

	buildInfo, _ := debug.ReadBuildInfo()
	root := cli.NewRootCommand(&config.AppInfo{
		Name:        name,
		Version:     version,
		BuildTime:   buildTime,
		Description: description,
		BuildInfo:   buildInfo,
	})

	// cloud commands
	cloud.RegisterCommands(root)

	// vfs commands
	vfs.RegisterCommands(root)

	// secret commands
	secret.RegisterCommands(root)

	ctx := context.Background()

	err := root.ExecuteContext(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
