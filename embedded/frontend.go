// Package embedded provides helper functions for embedded content.
//
// Note: The canonical embed point is root embed.go (jellywatch.GetWebFS).
// This package provides HasFrontend() for build-time validation tests.
package embedded

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend
var frontendFS embed.FS

// Frontend returns the embedded frontend filesystem.
// Prefer using jellywatch.GetWebFS() for serving.
func Frontend() (fs.FS, error) {
	return fs.Sub(frontendFS, "frontend")
}

// HasFrontend checks if frontend files are embedded (not just placeholder).
// Used by embed_test.go to validate build correctness.
func HasFrontend() bool {
	entries, err := frontendFS.ReadDir("frontend")
	return err == nil && len(entries) > 0
}
