package embedded

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend
var frontendFS embed.FS

// Frontend returns the embedded frontend filesystem
func Frontend() (fs.FS, error) {
	return fs.Sub(frontendFS, "frontend")
}

// HasFrontend checks if frontend files are embedded
func HasFrontend() bool {
	entries, err := frontendFS.ReadDir("frontend")
	return err == nil && len(entries) > 0
}
