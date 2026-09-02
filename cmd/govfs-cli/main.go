// Package main은 govfs CLI 도구의 진입점입니다.
package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/shabatoily/govfs/cmd"
	"github.com/shabatoily/govfs/internal/cli"
	"github.com/shabatoily/govfs/internal/cli/secret"
	"github.com/shabatoily/govfs/internal/cli/vfs"
)

func main() {
	_ = godotenv.Load()

	appInfo := cmd.GetAppInfo()

	root := cli.NewRootCommand(appInfo)

	// vfs commands
	vfs.RegisterCommands(root)

	// secret commands
	secret.RegisterCommands(root)

	// mcp command
	root.AddCommand(cli.NewMCPCommand(appInfo.Version))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := root.ExecuteContext(ctx)
	if err != nil {
		panic(err)
	}
}
