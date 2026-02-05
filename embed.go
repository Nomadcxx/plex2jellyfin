package jellywatch

import (
	"embed"
	"io/fs"
)

//go:embed all:embedded/web
var webContent embed.FS

// GetWebFS returns the embedded web filesystem with the prefix stripped
func GetWebFS() fs.FS {
	webFS, err := fs.Sub(webContent, "embedded/web")
	if err != nil {
		// This should never happen if the embed directive is correct
		panic(err)
	}
	return webFS
}
