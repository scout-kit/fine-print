package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var buildFS embed.FS

// FS returns the frontend filesystem rooted at the build directory.
func FS() (fs.FS, error) {
	return fs.Sub(buildFS, "build")
}
