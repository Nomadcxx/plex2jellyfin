// Package jellywatch provides the embedded frontend filesystem.
//
// This is the canonical embed point for frontend static files.
// Used by internal/api/server.go for HTTP serving.
//
// Build: Run `make frontend` before `go build` to populate embedded/frontend/.
package jellywatch

import (
	"embed"
	"io/fs"
)

//go:embed all:embedded/frontend
var webContent embed.FS

// GetWebFS returns the embedded web filesystem with the prefix stripped.
// This is the primary entry point for serving the frontend.
func GetWebFS() fs.FS {
	webFS, err := fs.Sub(webContent, "embedded/frontend")
	if err != nil {
		// This should never happen if the embed directive is correct
		panic(err)
	}
	return webFS
}
