package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var dist embed.FS

var FS, _ = fs.Sub(dist, "dist")
