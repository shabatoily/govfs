package main

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/joho/godotenv"
	"github.com/meteormin/go-vfs/cli"
	"github.com/meteormin/go-vfs/cli/cloud"
	"github.com/meteormin/go-vfs/cli/vfs"
	"github.com/meteormin/go-vfs/config"
)

var (
	name        = "go-vfs"
	version     = "0.0.1"
	buildTime   = time.Now().UTC().Format(time.RFC3339)
	description = "go-vfs is a virtual file system server"
)

func main() {
	_ = godotenv.Load()

	buildInfo, _ := debug.ReadBuildInfo()
	root := cli.NewRootCommand(config.AppInfo{
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

	ctx := context.Background()

	err := root.ExecuteContext(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
