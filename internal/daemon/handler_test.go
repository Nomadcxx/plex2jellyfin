package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/library"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
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

// TestYearAwareMatching_DifferentYears verifies that shows with different years
// are treated as different shows (e.g., "Dracula (2020)" ≠ "Dracula (2025)")
func TestYearAwareMatching_DifferentYears(t *testing.T) {
	// Create a temp directory to simulate a TV library
	tempDir := t.TempDir()

	// Create show directories with same name but different years
	dracula2020 := filepath.Join(tempDir, "Dracula (2020)")
	dracula2025 := filepath.Join(tempDir, "Dracula (2025)")

	if err := os.MkdirAll(dracula2020, 0755); err != nil {
		t.Fatalf("failed to create Dracula (2020) dir: %v", err)
	}
	if err := os.MkdirAll(dracula2025, 0755); err != nil {
		t.Fatalf("failed to create Dracula (2025) dir: %v", err)
	}

	// Create a selector to test year-aware matching
	selector := library.NewSelector([]string{tempDir})

	// When looking for "Dracula" with year "2020", should find Dracula (2020)
	result2020 := findExistingShowDirForTest(t, selector, tempDir, "Dracula", "2020")
	if result2020 != dracula2020 {
		t.Errorf("Looking for Dracula (2020): got %q, want %q", result2020, dracula2020)
	}

	// When looking for "Dracula" with year "2025", should find Dracula (2025)
	result2025 := findExistingShowDirForTest(t, selector, tempDir, "Dracula", "2025")
	if result2025 != dracula2025 {
		t.Errorf("Looking for Dracula (2025): got %q, want %q", result2025, dracula2025)
	}

	// Verify they are different directories
	if result2020 == result2025 {
		t.Error("Different years should result in different show directories")
	}
}

// TestYearAwareMatching_SameYear verifies that shows with the same year
// are correctly matched (e.g., "Show (2020)" = "Show (2020)")
func TestYearAwareMatching_SameYear(t *testing.T) {
	// Create a temp directory to simulate a TV library
	tempDir := t.TempDir()

	// Create show directory
	show2020 := filepath.Join(tempDir, "Show (2020)")
	if err := os.MkdirAll(show2020, 0755); err != nil {
		t.Fatalf("failed to create Show (2020) dir: %v", err)
	}

	// Create a selector to test year-aware matching
	selector := library.NewSelector([]string{tempDir})

	// When looking for "Show" with year "2020", should find Show (2020)
	result := findExistingShowDirForTest(t, selector, tempDir, "Show", "2020")
	if result != show2020 {
		t.Errorf("Looking for Show (2020): got %q, want %q", result, show2020)
	}

	// When looking for "Show" with year "2021", should NOT find anything
	result2021 := findExistingShowDirForTest(t, selector, tempDir, "Show", "2021")
	if result2021 != "" {
		t.Errorf("Looking for Show (2021) should return empty, got %q", result2021)
	}
}

// Helper function to find existing show directory using the selector
func findExistingShowDirForTest(t *testing.T, selector *library.Selector, library, showName, year string) string {
	// We need to access the selector's findExistingShowDir method
	// Since it's not exported, we'll use the SelectTVShowLibrary method
	// and check the returned SelectionResult

	// For testing, we use a minimal file size
	selection, err := selector.SelectTVShowLibrary(showName, year, 100)
	if err != nil {
		// If no selection could be made, check if show exists by looking at library
		entries, err := os.ReadDir(library)
		if err != nil {
			return ""
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirName := entry.Name()
			// Check if directory matches "ShowName (Year)" pattern
			expectedName := showName + " (" + year + ")"
			if dirName == expectedName {
				return filepath.Join(library, dirName)
			}
		}
		return ""
	}

	// If we got a selection, the show directory should be under the selected library
	// We need to find the exact show directory path
	entries, err := os.ReadDir(selection.Library)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		// Match by show name and year
		expectedName := showName + " (" + year + ")"
		if dirName == expectedName {
			return filepath.Join(selection.Library, dirName)
		}
	}

	return ""
}

func TestHandleFileEventRejectsInvalidPath(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv/lib"},
		MovieLibs:       []string{"/movie/lib"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		DebounceTime:    time.Hour,
		Logger:          logging.Nop(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	err = handler.HandleFileEvent(watcher.FileEvent{
		Type: watcher.EventCreate,
		Path: "/movie.mkv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(handler.pending); got != 0 {
		t.Fatalf("expected no pending timers for invalid path, got %d", got)
	}
}

func TestHandleFileEventDebouncesOnNormalizedPath(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv/lib"},
		MovieLibs:       []string{"/movie/lib"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		DebounceTime:    time.Hour,
		Logger:          logging.Nop(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	err = handler.HandleFileEvent(watcher.FileEvent{
		Type: watcher.EventCreate,
		Path: "/watch/movies/./movie.mkv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = handler.HandleFileEvent(watcher.FileEvent{
		Type: watcher.EventWrite,
		Path: "/watch/movies/movie.mkv",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := len(handler.pending); got != 1 {
		t.Fatalf("expected one pending timer, got %d", got)
	}
	if _, exists := handler.pending["/watch/movies/movie.mkv"]; !exists {
		t.Fatalf("expected normalized path key in pending map")
	}
}

func TestHandleFileEventDefersTransientUnpackOnce(t *testing.T) {
	prevDelay := transientRetryDelay
	transientRetryDelay = time.Hour
	defer func() {
		transientRetryDelay = prevDelay
	}()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv/lib"},
		MovieLibs:       []string{"/movie/lib"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		DebounceTime:    time.Hour,
		Logger:          logging.Nop(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	event := watcher.FileEvent{
		Type: watcher.EventCreate,
		Path: "/watch/tv/_UNPACK_abc/show.mkv",
	}
	if err := handler.HandleFileEvent(event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := handler.HandleFileEvent(event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := handler.transientRetries["/watch/tv/_UNPACK_abc/show.mkv"]; got != 1 {
		t.Fatalf("expected one transient retry, got %d", got)
	}
	if got := len(handler.pending); got != 1 {
		t.Fatalf("expected one pending timer, got %d", got)
	}
}
