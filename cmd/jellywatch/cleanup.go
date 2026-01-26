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

// isLibraryRoot checks if path is exactly a library root
func isLibraryRoot(path string, libraryRoots []string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range libraryRoots {
		if filepath.Clean(root) == cleanPath {
			return true
		}
	}
	return false
}

// isInsideLibrary checks if path is inside any library root
func isInsideLibrary(path string, libraryRoots []string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range libraryRoots {
		cleanRoot := filepath.Clean(root)
		if strings.HasPrefix(cleanPath, cleanRoot) {
			return true
		}
	}
	return false
}
