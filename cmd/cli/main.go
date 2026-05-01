// Package main은 govfs CLI 도구의 진입점입니다.
package main

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/joho/godotenv"
	"github.com/meteormin/govfs/internal/cli"
	"github.com/meteormin/govfs/internal/cli/cloud"
	"github.com/meteormin/govfs/internal/cli/secret"
	"github.com/meteormin/govfs/internal/cli/vfs"
	"github.com/meteormin/govfs/internal/config"
)

var (
	name        = "govfs"
	version     = "0.0.1"
	buildTime   = time.Now().UTC().Format(time.RFC3339)
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
