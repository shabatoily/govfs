package cmd

import (
	"runtime/debug"

	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/pkg/version"
)

var (
	name        = "govfs"
	description = "govfs is a virtual file system server"
)

func GetAppInfo() config.AppInfo {
	buildInfo, _ := debug.ReadBuildInfo()
	return config.AppInfo{
		Name:        name,
		Description: description,
		Version:     version.Version(),
		BuildTime:   version.BuildTime(),
		BuildInfo:   buildInfo,
	}
}
