package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func configuredLibraryRoots(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	roots := make([]string, 0, len(cfg.Libraries.TV)+len(cfg.Libraries.Movies))
	for _, root := range append(cfg.Libraries.TV, cfg.Libraries.Movies...) {
		root = strings.TrimSpace(root)
		if root != "" {
			roots = append(roots, filepath.Clean(root))
		}
	}
	return roots
}

func pathUnderAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return false
	}
	for _, root := range roots {
		if path == root || pathIsWithin(root, path) {
			return true
		}
	}
	return false
}

func rootBoundPathIssue(label, path string, roots []string) string {
	if len(roots) == 0 {
		return ""
	}
	if pathUnderAnyRoot(path, roots) {
		return ""
	}
	return fmt.Sprintf("%s is outside configured library roots: %s", label, path)
}
