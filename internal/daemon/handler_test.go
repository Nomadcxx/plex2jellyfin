package daemon

import (
	"testing"
)

func TestMediaHandler_SeparateLibraries(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv/lib1"},
		MovieLibs:       []string{"/movies/lib1"},
		TVWatchPaths:    []string{"/downloads/tv"},
		MovieWatchPaths: []string{"/downloads/movies"},
	}

	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Verify TV libraries are separate from Movie libraries
	if len(handler.tvLibraries) != 1 || handler.tvLibraries[0] != "/tv/lib1" {
		t.Error("TV libraries not set correctly")
	}
	if len(handler.movieLibs) != 1 || handler.movieLibs[0] != "/movies/lib1" {
		t.Error("Movie libraries not set correctly")
	}
}
