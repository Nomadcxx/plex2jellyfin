package main

import (
	"os"
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

// cleanupSourceDir removes cruft files and empty directories after a move.
// It only deletes cruft if NO video files remain in the directory.
// It walks up to directory tree, deleting empty dirs until hitting a library root.
func cleanupSourceDir(dir string, libraryRoots []string) error {
	currentDir := filepath.Clean(dir)

	for {
		// Safety: must be inside a known library
		if !isInsideLibrary(currentDir, libraryRoots) {
			return nil
		}

		// Safety: never delete a library root
		if isLibraryRoot(currentDir, libraryRoots) {
			return nil
		}

		// Read directory contents
		entries, err := os.ReadDir(currentDir)
		if err != nil {
			return nil // Can't read, stop safely
		}

		// Check if any video files remain
		hasVideo := false
		for _, entry := range entries {
			if !entry.IsDir() && isVideoFile(entry.Name()) {
				hasVideo = true
				break
			}
		}

		// If video files exist, stop - don't delete anything
		if hasVideo {
			return nil
		}

		// No video files - delete all cruft files
		for _, entry := range entries {
			entryPath := filepath.Join(currentDir, entry.Name())
			if entry.IsDir() {
				// Recursively clean subdirectories first
				cleanupSourceDir(entryPath, libraryRoots)
			}
			// Remove the entry (file or now-empty dir)
			os.RemoveAll(entryPath)
		}

		// Now directory should be empty, remove it
		// os.Remove fails if not empty - that's our safety net
		if err := os.Remove(currentDir); err != nil {
			return nil // Not empty or error, stop
		}
	}

	return nil
}
