package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// TVPaths returns all ordinary and seeding TV intake roots.
func (w WatchConfig) TVPaths() []string {
	paths := make([]string, 0, len(w.TV)+len(w.SeedingTV))
	paths = append(paths, w.TV...)
	return append(paths, w.SeedingTV...)
}

// MoviePaths returns all ordinary and seeding movie intake roots.
func (w WatchConfig) MoviePaths() []string {
	paths := make([]string, 0, len(w.Movies)+len(w.SeedingMovies))
	paths = append(paths, w.Movies...)
	return append(paths, w.SeedingMovies...)
}

// AllPaths returns every configured intake root in daemon scan order.
func (w WatchConfig) AllPaths() []string {
	paths := w.MoviePaths()
	return append(paths, w.TVPaths()...)
}

// PathContains reports whether candidate is root itself or a descendant.
func PathContains(root, candidate string) bool {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(candidate) == "" {
		return false
	}
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

type watchRoot struct {
	path   string
	media  string
	policy string
}

// ValidateWatchLayout rejects roots whose overlapping roles make ingestion
// or source-retention behavior ambiguous, and all watch/library overlap.
func ValidateWatchLayout(w WatchConfig, libraries LibrariesConfig) error {
	roots := []watchRoot{}
	appendRoots := func(paths []string, media, policy string) error {
		for _, path := range paths {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("%s %s watch path is empty", policy, media)
			}
			roots = append(roots, watchRoot{path: filepath.Clean(path), media: media, policy: policy})
		}
		return nil
	}
	if err := appendRoots(w.TV, "tv", "ordinary"); err != nil {
		return err
	}
	if err := appendRoots(w.Movies, "movies", "ordinary"); err != nil {
		return err
	}
	if err := appendRoots(w.SeedingTV, "tv", "seeding"); err != nil {
		return err
	}
	if err := appendRoots(w.SeedingMovies, "movies", "seeding"); err != nil {
		return err
	}

	for i := range roots {
		for j := i + 1; j < len(roots); j++ {
			a, b := roots[i], roots[j]
			if !pathsOverlap(a.path, b.path) || (a.media == b.media && a.policy == b.policy) {
				continue
			}
			return fmt.Errorf("%s %s watch path %q overlaps %s %s watch path %q",
				a.policy, a.media, a.path, b.policy, b.media, b.path)
		}
	}

	for _, root := range roots {
		for _, library := range append(append([]string{}, libraries.Movies...), libraries.TV...) {
			if strings.TrimSpace(library) == "" {
				return fmt.Errorf("library path is empty")
			}
			if pathsOverlap(root.path, library) {
				return fmt.Errorf("%s %s watch path %q overlaps library path %q",
					root.policy, root.media, root.path, filepath.Clean(library))
			}
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	return PathContains(a, b) || PathContains(b, a)
}
