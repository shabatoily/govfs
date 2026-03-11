package main

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/joho/godotenv"
	"github.com/meteormin/govfs/cli"
	"github.com/meteormin/govfs/cli/cloud"
	"github.com/meteormin/govfs/cli/secret"
	"github.com/meteormin/govfs/cli/vfs"
	"github.com/meteormin/govfs/config"
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
