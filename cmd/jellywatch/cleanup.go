package main

import (
	"path/filepath"
	"strings"
)

// videoExtensions defines video file extensions
var videoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".m4v":  true,
	".ts":   true,
	".wmv":  true,
	".mov":  true,
	".m2ts": true,
	".webm": true,
}

// isVideoFile checks if a path is a video file by extension
func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return videoExtensions[ext]
}
