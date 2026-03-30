package hookwatch

import (
	"embed"
	"io/fs"
)

// frontendBuild contains the compiled SvelteKit SPA output, including `_app`.
//
//go:embed all:frontend/build
var frontendBuild embed.FS

// FrontendFS returns the embedded frontend build directory.
func FrontendFS() (fs.FS, error) {
	return fs.Sub(frontendBuild, "frontend/build")
}
