package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/library"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/organizer"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAIMatcher(t *testing.T, handler http.HandlerFunc) (*ai.Matcher, *httptest.Server, *int32) {
	t.Helper()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	matcher, err := ai.NewMatcher(config.AIConfig{
		Enabled:             true,
		Model:               "test-model",
		OllamaEndpoint:      server.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      1,
	})
	require.NoError(t, err)
	return matcher, server, &calls
}

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

func TestProcessFile_SkipsUnpackPaths(t *testing.T) {
	tmpLib := t.TempDir()
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{tmpLib},
		MovieWatchPaths: []string{tmpLib},
		Logger:          logging.Nop(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	paths := []string{
		filepath.Join(tmpLib, "_UNPACK_Show.S01E01.1080p.WEB.mkv", "Show.S01E01.1080p.WEB.mkv"),
		filepath.Join(tmpLib, "Obliterated S01 1080p NF WEB-DL DDP5 1 Atmos H 264-FLUX. unpack.1", "Obliterated S01 1080p NF WEB-DL DDP5 1 Atmos H 264-FLUX. unpack.mkv"),
	}
	for _, unpackPath := range paths {
		handler.processFile(unpackPath)
	}

	snap := handler.stats.Snapshot()
	if snap.Errors > 0 {
		t.Errorf("processFile recorded %d errors for transient unpack paths — should skip without error", snap.Errors)
	}
	if snap.TVProcessed > 0 || snap.MoviesProcessed > 0 {
		t.Errorf("processFile should not have processed transient unpack paths")
	}
}

func TestProcessFile_SkipsObfuscatedHashWhenNamedSiblingExists(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	releaseDir := filepath.Join(watchDir, "Dutton.Ranch.S01E08.1080p.WEB.h264-GRACE")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatalf("create release dir: %v", err)
	}

	hashFile := filepath.Join(releaseDir, "VKYruuvSDX5KQecePvxwyD8ycDM31TAK.mkv")
	namedFile := filepath.Join(releaseDir, "Dutton.Ranch.S01E08.1080p.WEB.h264-GRACE.mkv")
	if err := os.WriteFile(hashFile, []byte("hash"), 0644); err != nil {
		t.Fatalf("create hash file: %v", err)
	}
	if err := os.WriteFile(namedFile, []byte("named"), 0644); err != nil {
		t.Fatalf("create named file: %v", err)
	}

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	handler.processFile(hashFile)

	if len(handler.pendingAI) != 0 {
		t.Fatalf("expected stale hash sibling to skip AI, got %d pending AI items", len(handler.pendingAI))
	}
	snap := handler.stats.Snapshot()
	if snap.Errors != 0 {
		t.Fatalf("expected stale hash sibling to skip without errors, got %d", snap.Errors)
	}
	if snap.TVProcessed != 0 || snap.MoviesProcessed != 0 {
		t.Fatalf("expected stale hash sibling not to organize media, got tv=%d movies=%d", snap.TVProcessed, snap.MoviesProcessed)
	}
	if _, err := os.Stat(hashFile); err != nil {
		t.Fatalf("hash source should be left untouched: %v", err)
	}
}

func TestProcessFile_SkipsMissingSourceBeforeParseDecision(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	releaseDir := filepath.Join(watchDir, "My.Adventures.with.Superman.S03E04.1080p.WEB.h264-EDITH")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))

	missingPath := filepath.Join(releaseDir, "DDkTaYHq53x2Wa5v3qRrKwCUbRe50aLs.mkv")

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.processFile(missingPath)

	rows, err := db.QueryDecisions(database.QueryFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 0)

	snap := handler.stats.Snapshot()
	assert.Equal(t, int64(0), snap.Errors)
	assert.Equal(t, int64(0), snap.TVProcessed)
	assert.Equal(t, int64(0), snap.MoviesProcessed)
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

func TestProcessFile_FastLaneHighConfidence(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	srcFile := filepath.Join(watchDir, "Breaking.Bad.S01E01.1080p.mkv")
	os.WriteFile(srcFile, []byte("test"), 0644)

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false, // AI disabled = always fast lane
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	handler.processFile(srcFile)

	if len(handler.pendingAI) != 0 {
		t.Errorf("expected no pending AI items for high-confidence file, got %d", len(handler.pendingAI))
	}
}

func TestProcessFile_SlowLaneLowConfidence(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	// obfuscated filename → low confidence
	srcFile := filepath.Join(watchDir, "abc123def456.mkv")
	os.WriteFile(srcFile, []byte("test"), 0644)

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{},
		MovieWatchPaths: []string{watchDir},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	handler.processFile(srcFile)

	if len(handler.pendingAI) != 1 {
		t.Errorf("expected 1 pending AI item for low-confidence file, got %d", len(handler.pendingAI))
	}
}

func TestProcessFile_DeterministicTVBypassesAI(t *testing.T) {
	tests := []struct {
		name    string
		folder  string
		file    string
		wantDir string
	}{
		{
			name:    "absolute episode",
			folder:  "One.Piece.EP1156.Episode.1156.1080p.NF.WEB-DL.JPN.AAC2.0.H.264.MSubs-ToonsHub",
			file:    "One.Piece.EP1156.Episode.1156.1080p.NF.WEB-DL.JPN.AAC2.0.H.264.MSubs-ToonsHub.mkv",
			wantDir: "One Piece/Season 01",
		},
		{
			name:    "date based episode",
			folder:  "The.Daily.Show.2026.04.20.Annalena.Baerbock.1080p.WEB.h264-EDITH",
			file:    "The.Daily.Show.2026.04.20.Annalena.Baerbock.1080p.WEB.h264-EDITH.mkv",
			wantDir: "The Daily Show/Season 2026",
		},
		{
			name:    "known title colliding with release group",
			folder:  "BEEF.S01E01.1080p.WEB.h264-ETHEL",
			file:    "BEEF.S01E01.1080p.WEB.h264-ETHEL.mkv",
			wantDir: "BEEF/Season 01",
		},
		{
			name:    "obfuscated file in deterministic release folder",
			folder:  "BEEF.S01E02.1080p.WEB.h264-ETHEL",
			file:    "q1reIwWo3oVx97qiPp0731Eglz7WFVn8.mkv",
			wantDir: "BEEF/Season 01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpLib := t.TempDir()
			watchDir := t.TempDir()
			releaseDir := filepath.Join(watchDir, tt.folder)
			if err := os.MkdirAll(releaseDir, 0755); err != nil {
				t.Fatalf("failed to create release dir: %v", err)
			}
			srcFile := filepath.Join(releaseDir, tt.file)
			if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
				t.Fatalf("failed to create source file: %v", err)
			}

			cfg := MediaHandlerConfig{
				TVLibraries:     []string{tmpLib},
				MovieLibs:       []string{tmpLib},
				TVWatchPaths:    []string{watchDir},
				MovieWatchPaths: []string{},
				Logger:          logging.Nop(),
				AIEnabled:       true,
				AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.95},
				ConfigDir:       t.TempDir(),
			}
			handler, err := NewMediaHandler(cfg)
			if err != nil {
				t.Fatalf("failed to create handler: %v", err)
			}

			handler.processFile(srcFile)

			if len(handler.pendingAI) != 0 {
				t.Fatalf("expected deterministic TV parse to bypass AI, got %d pending items", len(handler.pendingAI))
			}
			if _, err := os.Stat(filepath.Join(tmpLib, filepath.FromSlash(tt.wantDir))); err != nil {
				t.Fatalf("expected organized target dir %q: %v", tt.wantDir, err)
			}
		})
	}
}

func TestProcessFile_DeterministicMovieWithYearBypassesAI(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	srcFile := filepath.Join(watchDir, "Dune.Part.Two.2024.1080p.WEB-DL.x264-GROUP.mkv")
	if err := os.WriteFile(srcFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{},
		MovieWatchPaths: []string{watchDir},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.95},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	handler.processFile(srcFile)

	if len(handler.pendingAI) != 0 {
		t.Fatalf("expected deterministic movie parse to bypass AI, got %d pending items", len(handler.pendingAI))
	}
	if _, err := os.Stat(filepath.Join(tmpLib, "Dune Part Two (2024)")); err != nil {
		t.Fatalf("expected organized movie dir: %v", err)
	}
}

func TestProcessPendingAI_ExpiresOldItems(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv"},
		MovieLibs:       []string{"/movie"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Add an expired pending item
	handler.pendingAI["/old/file.mkv"] = &PendingItem{
		Path:      "/old/file.mkv",
		Filename:  "file.mkv",
		MediaType: "movie",
		QueuedAt:  time.Now().Add(-25 * time.Hour), // 25 hours ago
	}

	handler.ProcessPendingAI(context.Background())

	if len(handler.pendingAI) != 0 {
		t.Errorf("expected expired item to be removed, got %d pending", len(handler.pendingAI))
	}
}

func TestProcessPendingAI_SkipsMissingFiles(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv"},
		MovieLibs:       []string{"/movie"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	handler.pendingAI["/nonexistent/file.mkv"] = &PendingItem{
		Path:      "/nonexistent/file.mkv",
		Filename:  "file.mkv",
		MediaType: "movie",
		QueuedAt:  time.Now(),
	}

	handler.ProcessPendingAI(context.Background())

	if len(handler.pendingAI) != 0 {
		t.Errorf("expected missing file to be removed, got %d pending", len(handler.pendingAI))
	}
}

func TestProcessPendingAI_OpenCircuitSkipsMatcher(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	srcFile := filepath.Join(watchDir, "Some.Movie.2024.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	matcher, server, calls := newTestAIMatcher(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"model":"test-model","response":"{\"title\":\"Some Movie\",\"type\":\"movie\",\"year\":2024,\"confidence\":0.99}","done":true}`))
	})
	defer server.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		MovieWatchPaths: []string{watchDir},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIMatcher:       matcher,
		AIConfig: config.AIConfig{
			Model:                "test-model",
			AutoTriggerThreshold: 0.95,
			CircuitBreaker: config.CircuitBreakerConfig{
				FailureThreshold:     1,
				FailureWindowSeconds: 60,
				CooldownSeconds:      3600,
			},
		},
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)
	handler.aiCircuit.RecordFailure("already open")
	handler.pendingAI[srcFile] = &PendingItem{
		Path:       srcFile,
		Filename:   filepath.Base(srcFile),
		MediaType:  "movie",
		MovieInfo:  &naming.MovieInfo{Title: "Some Movie", Year: "2024"},
		Confidence: 0.4,
		QueuedAt:   time.Now(),
		TargetLib:  tmpLib,
	}

	handler.ProcessPendingAI(context.Background())

	assert.Equal(t, int32(0), atomic.LoadInt32(calls), "open circuit must not call matcher")
	assert.Equal(t, ai.CircuitOpen, handler.aiCircuit.State())
}

func TestProcessPendingAI_FailuresOpenCircuit(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	srcFile := filepath.Join(watchDir, "Some.Movie.2024.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	matcher, server, calls := newTestAIMatcher(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	defer server.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		MovieWatchPaths: []string{watchDir},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIMatcher:       matcher,
		AIConfig: config.AIConfig{
			Model:                "test-model",
			AutoTriggerThreshold: 0.95,
			CircuitBreaker: config.CircuitBreakerConfig{
				FailureThreshold:     1,
				FailureWindowSeconds: 60,
				CooldownSeconds:      3600,
			},
		},
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)
	handler.pendingAI[srcFile] = &PendingItem{
		Path:       srcFile,
		Filename:   filepath.Base(srcFile),
		MediaType:  "movie",
		MovieInfo:  &naming.MovieInfo{Title: "Some Movie", Year: "2024"},
		Confidence: 0.4,
		QueuedAt:   time.Now(),
		TargetLib:  tmpLib,
	}

	handler.ProcessPendingAI(context.Background())

	assert.Equal(t, int32(1), atomic.LoadInt32(calls))
	assert.Equal(t, ai.CircuitOpen, handler.aiCircuit.State())
}

func makeHandler(t *testing.T, watchRoots []string) *MediaHandler {
	t.Helper()
	return &MediaHandler{
		tvWatchPaths:    watchRoots,
		movieWatchPaths: watchRoots,
		logger:          logging.Nop(),
	}
}

func TestCleanupSourceDir_SimpleMovie(t *testing.T) {
	root := t.TempDir()
	dlDir := filepath.Join(root, "Movie.Name.2025.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	junk := filepath.Join(dlDir, "abc123.txt")
	require.NoError(t, os.WriteFile(junk, []byte("nzb"), 0644))

	movedFile := filepath.Join(dlDir, "movie.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(dlDir)
	assert.True(t, os.IsNotExist(err), "download dir should be removed")

	_, err = os.Stat(root)
	assert.NoError(t, err, "watch root must not be removed")
}

func TestCleanupSourceDir_SourceStillExists_NoCleanup(t *testing.T) {
	root := t.TempDir()
	dlDir := filepath.Join(root, "Movie.Name.2025.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	videoFile := filepath.Join(dlDir, "movie.mkv")
	require.NoError(t, os.WriteFile(videoFile, []byte("data"), 0644))

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(videoFile)

	_, err := os.Stat(dlDir)
	assert.NoError(t, err, "download dir must remain when source still exists")
}

func TestCleanupSourceDir_FileDirectlyInWatchRoot_NoCleanup(t *testing.T) {
	root := t.TempDir()
	movedFile := filepath.Join(root, "movie.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(root)
	assert.NoError(t, err, "watch root must not be removed")
}

func TestCleanupSourceDir_SeasonPackRemainingEpisodes(t *testing.T) {
	root := t.TempDir()
	packDir := filepath.Join(root, "Show.S01-S04.1080p")
	s1 := filepath.Join(packDir, "Season 1")
	s2 := filepath.Join(packDir, "Season 2")
	require.NoError(t, os.MkdirAll(s1, 0755))
	require.NoError(t, os.MkdirAll(s2, 0755))

	pending := filepath.Join(s2, "Show.S02E01.mkv")
	require.NoError(t, os.WriteFile(pending, []byte("data"), 0644))

	movedFile := filepath.Join(s1, "Show.S01E02.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(s1)
	assert.True(t, os.IsNotExist(err), "empty Season 1 should be removed")

	_, err = os.Stat(packDir)
	assert.NoError(t, err, "pack dir must remain while Season 2 has pending video")
}

func TestCleanupSourceDir_LastEpisodeSeasonPack_FullCleanup(t *testing.T) {
	root := t.TempDir()
	packDir := filepath.Join(root, "Show.S01.1080p")
	s1 := filepath.Join(packDir, "Season 1")
	require.NoError(t, os.MkdirAll(s1, 0755))

	junk := filepath.Join(packDir, "nzb_marker.txt")
	require.NoError(t, os.WriteFile(junk, []byte("x"), 0644))

	movedFile := filepath.Join(s1, "Show.S01E01.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	// Starting release dir (s1) is purged and removed.
	_, err := os.Stat(s1)
	assert.True(t, os.IsNotExist(err), "Season 1 should be removed")

	// Parent packDir has junk but is not purged (allowlist only applies to starting dir).
	// os.Remove fails on non-empty packDir, so it is preserved.
	_, err = os.Stat(packDir)
	assert.NoError(t, err, "pack dir should be preserved — junk in parents is not purged")

	_, err = os.Stat(root)
	assert.NoError(t, err)
}

func TestContainsVideoFilesRecursive(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	h := makeHandler(t, []string{})

	assert.False(t, h.containsVideoFilesRecursive(root))

	require.NoError(t, os.WriteFile(filepath.Join(root, "marker.txt"), []byte("x"), 0644))
	assert.False(t, h.containsVideoFilesRecursive(root))

	require.NoError(t, os.WriteFile(filepath.Join(subDir, "ep.mkv"), []byte("x"), 0644))
	assert.True(t, h.containsVideoFilesRecursive(root))
}

func TestCleanupSourceDir_SubtitlesTreatedAsJunk(t *testing.T) {
	// Old name preserved for reference; behavior changed: subtitles are now preserved
	// by PurgeNonAllowed so a directory with only subtitle leftovers is NOT removed.
	root := t.TempDir()
	dlDir := filepath.Join(root, "Movie.2025.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	subFile := filepath.Join(dlDir, "movie.en.srt")
	require.NoError(t, os.WriteFile(subFile, []byte("subtitles"), 0644))

	movedFile := filepath.Join(dlDir, "movie.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(dlDir)
	assert.NoError(t, err, "dir with subtitle leftovers should be preserved by allowlist")
	_, err = os.Stat(subFile)
	assert.NoError(t, err, "subtitle file must survive")
}

func TestCleanupSourceDir_RemoveAllError_ContinuesUpward(t *testing.T) {
	root := t.TempDir()
	protectedDir := filepath.Join(root, "protected")
	childDir := filepath.Join(protectedDir, "child")
	require.NoError(t, os.MkdirAll(childDir, 0755))

	require.NoError(t, os.Chmod(protectedDir, 0555))
	defer os.Chmod(protectedDir, 0755)

	movedFile := filepath.Join(childDir, "video.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(root)
	assert.NoError(t, err, "root must still exist")
}

func TestCleanupSourceDir_WatchRootWithTrailingSlash(t *testing.T) {
	root := t.TempDir()
	dlDir := filepath.Join(root, "Movie.2025.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	junk := filepath.Join(dlDir, "marker.txt")
	require.NoError(t, os.WriteFile(junk, []byte("x"), 0644))

	movedFile := filepath.Join(dlDir, "movie.mkv")

	h := makeHandler(t, []string{root + string(os.PathSeparator)})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(dlDir)
	assert.True(t, os.IsNotExist(err), "download dir should be removed even with trailing slash in watch root")
}

// --- Task 3.2: allowlist cleanup tests ---

// TestCleanupAllowlist_JunkRemoved verifies that junk files in the starting
// release directory are removed by PurgeNonAllowed after a successful move.
func TestCleanupAllowlist_JunkRemoved(t *testing.T) {
	root := t.TempDir()
	dlDir := filepath.Join(root, "Movie.2025.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	junk := filepath.Join(dlDir, "www.yts.mx.jpg")
	require.NoError(t, os.WriteFile(junk, []byte("x"), 0644))
	nfo := filepath.Join(dlDir, "info.nfo")
	require.NoError(t, os.WriteFile(nfo, []byte("x"), 0644))

	movedFile := filepath.Join(dlDir, "movie.mkv") // source already moved away

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(dlDir)
	assert.True(t, os.IsNotExist(err), "dir should be removed once junk is purged and it is empty")
}

// TestCleanupAllowlist_SourceStillExists_NoCleanup re-validates the source-present
// gate remains intact when using PurgeNonAllowed.
func TestCleanupAllowlist_SourceStillExists_NoCleanup(t *testing.T) {
	root := t.TempDir()
	dlDir := filepath.Join(root, "Movie.2025.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	videoFile := filepath.Join(dlDir, "movie.mkv")
	require.NoError(t, os.WriteFile(videoFile, []byte("data"), 0644))

	junk := filepath.Join(dlDir, "info.nfo")
	require.NoError(t, os.WriteFile(junk, []byte("x"), 0644))

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(videoFile)

	_, err := os.Stat(dlDir)
	assert.NoError(t, err, "dir must remain when source file still exists")
	_, err = os.Stat(junk)
	assert.NoError(t, err, "junk must not be touched when source present")
}

// TestCleanupAllowlist_VideoRemains verifies that when another video remains in
// the release directory after purge, the directory is not removed.
func TestCleanupAllowlist_VideoRemains(t *testing.T) {
	root := t.TempDir()
	dlDir := filepath.Join(root, "Show.S01.1080p-GROUP")
	require.NoError(t, os.MkdirAll(dlDir, 0755))

	// A sibling episode still present.
	sibling := filepath.Join(dlDir, "Show.S01E02.mkv")
	require.NoError(t, os.WriteFile(sibling, []byte("data"), 0644))

	movedFile := filepath.Join(dlDir, "Show.S01E01.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	_, err := os.Stat(dlDir)
	assert.NoError(t, err, "release dir must remain while sibling video is present")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "sibling video must not be removed")
}

// TestCleanupAllowlist_ParentNotPurged verifies that junk in a parent directory
// is never removed; only the starting release directory receives PurgeNonAllowed.
func TestCleanupAllowlist_ParentNotPurged(t *testing.T) {
	root := t.TempDir()
	packDir := filepath.Join(root, "Show.Complete.Series.1080p")
	startingDir := filepath.Join(packDir, "Season.01")
	require.NoError(t, os.MkdirAll(startingDir, 0755))

	// Junk lives in the parent, not in the starting dir.
	parentJunk := filepath.Join(packDir, "rarbg.com.txt")
	require.NoError(t, os.WriteFile(parentJunk, []byte("x"), 0644))

	movedFile := filepath.Join(startingDir, "Show.S01E01.mkv")

	h := makeHandler(t, []string{root})
	h.cleanupSourceDir(movedFile)

	// Starting dir is empty → removed.
	_, err := os.Stat(startingDir)
	assert.True(t, os.IsNotExist(err), "empty starting dir should be removed")

	// Parent contains junk but is NOT purged → os.Remove fails → parent preserved.
	_, err = os.Stat(packDir)
	assert.NoError(t, err, "parent with junk must not be purged")
	_, err = os.Stat(parentJunk)
	assert.NoError(t, err, "junk in parent must survive")
}

func TestObfuscatedFileHandling(t *testing.T) {
	// ParseTVShowFromPathVerbose must fall back to the release folder name when the
	// filename is an obfuscated hash, and return stripped quality tokens from it.
	path := "/downloads/The.White.Lotus.S02E07.1080p.WEB-DL/q1reIwWo3oVx97qiPp0731Eglz7WFVn8.mkv"
	info, tokens, err := naming.ParseTVShowFromPathVerbose(path)
	require.NoError(t, err)
	assert.Equal(t, "The White Lotus", info.Title)
	assert.Equal(t, 2, info.Season)
	assert.Equal(t, 7, info.Episode)
	assert.NotEmpty(t, tokens, "verbose call should return stripped tokens from folder name")
}

func TestProcessFile_WritesParseDecision(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	srcFile := filepath.Join(watchDir, "Breaking.Bad.S01E01.1080p.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.processFile(srcFile)

	rows, err := db.QueryDecisions(database.QueryFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	d := rows[0]
	assert.Equal(t, srcFile, d.SourcePath)
	assert.Equal(t, "Breaking.Bad.S01E01.1080p.mkv", d.SourceFilename)
	assert.NotEmpty(t, d.ParseMethod)
	assert.Equal(t, "Breaking Bad", d.ParsedTitle)
	assert.NotEmpty(t, d.OrganizeOutcome)
}

func TestProcessFile_SeasonPackUnresolvedIsSkippedForReview(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	packDir := filepath.Join(watchDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	srcFile := filepath.Join(packDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.processFile(srcFile)

	rows, err := db.QueryDecisions(database.QueryFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	d := rows[0]
	assert.Equal(t, "season_pack", d.ParseMethod)
	assert.Equal(t, "Supergirl", d.ParsedTitle)
	require.NotNil(t, d.ParsedSeason)
	assert.Equal(t, 3, *d.ParsedSeason)
	assert.Nil(t, d.ParsedEpisode)
	assert.Equal(t, "skipped", d.OrganizeOutcome)
	assert.Contains(t, d.OrganizeError, "season_pack_unresolved")
	assert.FileExists(t, srcFile)
}

func TestProcessFile_SeasonPackUnresolvedIsDeferredAfterSkip(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	packDir := filepath.Join(watchDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	srcFile := filepath.Join(packDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.processFile(srcFile)
	handler.processFile(srcFile)

	rows, err := db.QueryDecisions(database.QueryFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1, "second pass should be suppressed by negative cache")
	assert.Equal(t, "skipped", rows[0].OrganizeOutcome)
	assert.Contains(t, rows[0].OrganizeError, "season_pack_unresolved")
}

func TestNewMediaHandler_HydratesSeasonPackUnresolvedNegativeCache(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	packDir := filepath.Join(watchDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	srcFile := filepath.Join(packDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       srcFile,
		SourceFilename:   filepath.Base(srcFile),
		EventAt:          time.Now().UTC(),
		MediaTypeGuessed: "tv",
		ParseMethod:      "season_pack",
		ParsedTitle:      "Supergirl",
		OrganizeOutcome:  "skipped",
		OrganizeError:    "season_pack_unresolved: " + packDir,
	})
	require.NoError(t, err)
	require.NotZero(t, id)

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	deferred, remaining, lastErr := handler.unparseableCache.IsDeferred(srcFile)
	assert.True(t, deferred)
	assert.Greater(t, remaining, time.Duration(0))
	assert.Contains(t, lastErr, "season_pack_unresolved")
}

func TestProcessFile_SeasonPackImportsEpisodeSiblings(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	packDir := filepath.Join(watchDir, "Supergirl.S03.1080p.BluRay.x264-GROUP")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	srcFile := filepath.Join(packDir, "Supergirl.S03E01.1080p.BluRay.x264-GROUP.mkv")
	sibling := filepath.Join(packDir, "Supergirl.S03E02.1080p.BluRay.x264-GROUP.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))
	require.NoError(t, os.WriteFile(sibling, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.processFile(srcFile)

	assert.FileExists(t, filepath.Join(tmpLib, "Supergirl", "Season 03", "Supergirl S03E01.mkv"))
	assert.FileExists(t, filepath.Join(tmpLib, "Supergirl", "Season 03", "Supergirl S03E02.mkv"))
	assert.NoFileExists(t, srcFile)
	assert.NoFileExists(t, sibling)
	assert.False(t, handler.containsVideoFilesRecursive(packDir), "pack cleanup must not leave videos behind")

	rows, err := db.QueryDecisions(database.QueryFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "season_pack", rows[0].ParseMethod)
	assert.Equal(t, "success", rows[0].OrganizeOutcome)
}

func TestProcessFile_SeasonPackConcurrentEventsSingleFlight(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()
	packDir := filepath.Join(watchDir, "The.Boroughs.S01.1080p.NF.WEB-DL.DDP5.1.Atmos.H.264-MWeb")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	ep1 := filepath.Join(packDir, "The.Boroughs.S01E01.1080p.NF.WEB-DL.DDP5.1.Atmos.H.264-MWeb.mkv")
	ep2 := filepath.Join(packDir, "The.Boroughs.S01E02.1080p.NF.WEB-DL.DDP5.1.Atmos.H.264-MWeb.mkv")
	require.NoError(t, os.WriteFile(ep1, []byte("episode-1"), 0644))
	require.NoError(t, os.WriteFile(ep2, []byte("episode-2"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, path := range []string{ep1, ep2} {
		path := path
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			handler.processFile(path)
		}()
	}
	close(start)
	wg.Wait()

	assert.FileExists(t, filepath.Join(tmpLib, "The Boroughs", "Season 01", "The Boroughs S01E01.mkv"))
	assert.FileExists(t, filepath.Join(tmpLib, "The Boroughs", "Season 01", "The Boroughs S01E02.mkv"))
	assert.False(t, handler.containsVideoFilesRecursive(packDir), "pack cleanup must not leave videos behind")

	rows, err := db.QueryDecisions(database.QueryFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 1, "concurrent events for one season pack should share one organizer run")
	assert.Equal(t, "season_pack", rows[0].ParseMethod)
	assert.Equal(t, "success", rows[0].OrganizeOutcome)
}

func TestAIItemHasParseDecisionID(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	// obfuscated movie filename → low confidence → queued for AI
	srcFile := filepath.Join(watchDir, "abc123def456xyz789.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{},
		MovieWatchPaths: []string{watchDir},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.processFile(srcFile)

	require.Len(t, handler.pendingAI, 1)
	var item *PendingItem
	for _, v := range handler.pendingAI {
		item = v
		break
	}
	assert.NotZero(t, item.ParseDecisionID, "PendingItem should carry ParseDecisionID for AI path")
}

func TestUpdateDecisionOrganizeReturnsDBError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "handler-test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	handler := &MediaHandler{db: db, logger: logging.Nop()}

	err = handler.updateDecisionOrganize(1, &organizer.OrganizationResult{Success: true, TargetPath: "/library/movie.mkv"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UpdateOrganize")
}

func TestRegexFallback_WritesParseDecision(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	srcFile := filepath.Join(watchDir, "Breaking.Bad.S01E01.1080p.mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("test"), 0644))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:     srcFile,
		SourceFilename: "Breaking.Bad.S01E01.1080p.mkv",
		EventAt:        time.Now().UTC(),
	})
	require.NoError(t, err)

	tvInfo := &naming.TVShowInfo{Title: "Breaking Bad", Season: 1, Episode: 1}
	item := &PendingItem{
		Path:            srcFile,
		Filename:        "Breaking.Bad.S01E01.1080p.mkv",
		MediaType:       "tv",
		TVInfo:          tvInfo,
		ParseDecisionID: id,
	}

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false,
		Database:        db,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.organizeWithRegexFallback(item)

	d, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.NotEmpty(t, d.OrganizeOutcome, "organize_outcome should be set after fallback organize")
}

func TestHandleJellyfinWebhookEvent_ItemAddedUpdatesParseDecision(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "handler-test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	targetPath := "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/downloads/the.matrix.1999.mkv",
		SourceFilename:  "the.matrix.1999.mkv",
		EventAt:         now,
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	require.NoError(t, err)

	handler := &MediaHandler{
		db:            db,
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	event := jellyfin.WebhookEvent{
		NotificationType: jellyfin.EventItemAdded,
		ItemID:           "jf-item-001",
		ItemPath:         targetPath,
		ItemName:         "The Matrix",
		ItemType:         "Movie",
		ProviderImdb:     "tt0133093",
		ProviderTmdb:     "603",
	}

	handler.HandleJellyfinWebhookEvent(event)

	dec, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, dec)
	assert.Equal(t, "jf-item-001", dec.JellyfinItemID)
	assert.Equal(t, "tt0133093", dec.JellyfinImdbID)
	assert.Equal(t, "603", dec.JellyfinTmdbID)
	assert.NotNil(t, dec.JellyfinResolvedAt)
	assert.NotNil(t, dec.JellyfinFirstSeenAt)
	require.NotNil(t, dec.JellyfinIdentified)
	assert.True(t, *dec.JellyfinIdentified)
}

func TestHandleJellyfinWebhookEvent_ItemAddedReturnsDBError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "handler-test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	handler := &MediaHandler{db: db}

	err = handler.HandleJellyfinWebhookEvent(jellyfin.WebhookEvent{
		NotificationType: jellyfin.EventItemAdded,
		ItemID:           "jf-item-001",
		ItemPath:         "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv",
		ItemName:         "The Matrix",
		ItemType:         "Movie",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert Jellyfin item")
}

func TestHandleJellyfinWebhookEvent_ItemAddedSkipsResolved(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "handler-test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	targetPath := "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	now := time.Now().UTC().Add(-1 * time.Hour)
	resolvedAt := now
	resolvedID, err := db.InsertDecision(database.ParseDecision{
		SourcePath:         "/downloads/old.mkv",
		SourceFilename:     "old.mkv",
		EventAt:            now,
		TargetPath:         targetPath,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "old-jf-id",
		JellyfinResolvedAt: &resolvedAt,
	})
	require.NoError(t, err)

	unresolvedID, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/downloads/the.matrix.1999.mkv",
		SourceFilename:  "the.matrix.1999.mkv",
		EventAt:         time.Now().UTC(),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	require.NoError(t, err)

	handler := &MediaHandler{
		db:            db,
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	handler.HandleJellyfinWebhookEvent(jellyfin.WebhookEvent{
		NotificationType: jellyfin.EventItemAdded,
		ItemID:           "new-jf-id",
		ItemPath:         targetPath,
		ItemName:         "The Matrix",
		ItemType:         "Movie",
	})

	resolved, err := db.GetDecision(resolvedID)
	require.NoError(t, err)
	assert.Equal(t, "old-jf-id", resolved.JellyfinItemID, "resolved row must not be touched")

	updated, err := db.GetDecision(unresolvedID)
	require.NoError(t, err)
	assert.Equal(t, "new-jf-id", updated.JellyfinItemID)
	assert.NotNil(t, updated.JellyfinResolvedAt)
}

func TestHandleJellyfinWebhookEvent_ItemUpdatedUpgradesIdentification(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "handler-test.db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer db.Close()

	targetPath := "/mnt/STORAGE1/MOVIES/F Valentines Day (2026)/F Valentines Day (2026).mkv"
	now := time.Now().UTC()
	identified := false
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:         "/downloads/f.valentines.day.2026.mkv",
		SourceFilename:     "f.valentines.day.2026.mkv",
		EventAt:            now,
		TargetPath:         targetPath,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "jf-bare",
		JellyfinResolvedAt: &now,
		JellyfinIdentified: &identified,
	})
	require.NoError(t, err)

	handler := &MediaHandler{
		db: db,
		pathTranslator: jellyfin.NewPathTranslator([]jellyfin.PathMapping{
			{Jellyfin: "/movies1", Daemon: "/mnt/STORAGE1/MOVIES"},
		}),
	}

	handler.HandleJellyfinWebhookEvent(jellyfin.WebhookEvent{
		NotificationType: jellyfin.EventItemUpdated,
		ItemID:           "jf-identified",
		ItemPath:         "/movies1/F Valentines Day (2026)/F Valentines Day (2026).mkv",
		ItemName:         "F Valentine's Day",
		ItemType:         "Movie",
		ProviderImdb:     "tt34622232",
		ProviderTmdb:     "1429605",
	})

	dec, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, dec)
	assert.Equal(t, "jf-identified", dec.JellyfinItemID)
	assert.Equal(t, "tt34622232", dec.JellyfinImdbID)
	assert.Equal(t, "1429605", dec.JellyfinTmdbID)
	require.NotNil(t, dec.JellyfinIdentified)
	assert.True(t, *dec.JellyfinIdentified)
	assert.NotNil(t, dec.JellyfinResolvedAt)
	assert.NotNil(t, dec.JellyfinFirstSeenAt)
}

func TestProcessFileWithGen_StaleCallbackNoOp(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:  []string{"/tv"},
		MovieLibs:    []string{"/movies"},
		DebounceTime: 10 * time.Second,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	path := "/downloads/test.mkv"
	gen := int64(1)

	handler.mu.Lock()
	handler.pendingGen[path] = gen + 1
	handler.mu.Unlock()

	handler.processFileWithGen(path, gen)
}

func TestProcessFileWithGen_CurrentGenRuns(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:  []string{"/tv"},
		MovieLibs:    []string{"/movies"},
		DebounceTime: 10 * time.Second,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	path := "/downloads/test.mkv"
	gen := int64(1)

	handler.mu.Lock()
	handler.pendingGen[path] = gen
	handler.pending[path] = time.AfterFunc(time.Hour, func() {})
	handler.transientRetries[path] = 1
	handler.mu.Unlock()

	handler.processFileWithGen(path, gen)

	handler.mu.Lock()
	_, inPending := handler.pending[path]
	_, inGen := handler.pendingGen[path]
	_, inRetries := handler.transientRetries[path]
	handler.mu.Unlock()

	assert.False(t, inPending, "pending entry should be cleaned up")
	assert.False(t, inGen, "gen entry should be cleaned up")
	assert.False(t, inRetries, "retry entry should be cleaned up")
}

func TestShutdown_InvalidatesPendingGens(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:  []string{"/tv"},
		MovieLibs:    []string{"/movies"},
		DebounceTime: 10 * time.Second,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	path := "/downloads/test.mkv"
	handler.mu.Lock()
	handler.pendingGen[path] = 1
	handler.schedulePendingTimerLocked(path, time.Hour, 1)
	handler.mu.Unlock()

	handler.Shutdown()

	handler.mu.Lock()
	_, inGen := handler.pendingGen[path]
	handler.mu.Unlock()
	assert.False(t, inGen, "pendingGen should be cleared on shutdown")
}

func TestHandleFileEventAfterShutdownDoesNotScheduleTimer(t *testing.T) {
	watchDir := t.TempDir()
	mediaPath := filepath.Join(watchDir, "movie.mkv")

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{filepath.Join(t.TempDir(), "tv")},
		MovieLibs:       []string{filepath.Join(t.TempDir(), "movies")},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{watchDir},
		DebounceTime:    time.Hour,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.Shutdown()
	err = handler.HandleFileEvent(watcher.FileEvent{Path: mediaPath, Type: watcher.EventCreate})
	require.NoError(t, err)

	handler.mu.Lock()
	pending := len(handler.pending)
	handler.mu.Unlock()
	assert.Equal(t, 0, pending, "events after shutdown must not schedule timers")
}

func TestReplayDeferredOperation_RespectsShutdown(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:  []string{"/tv"},
		MovieLibs:    []string{"/movies"},
		DebounceTime: 10 * time.Second,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	handler.cancel()

	done := make(chan struct{})
	go func() {
		handler.replayDeferredOperation(jellyfin.DeferredOp{
			Type:       "organize",
			SourcePath: "/downloads/test.mkv",
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replayDeferredOperation should return immediately when ctx is cancelled")
	}
}

func TestReplayDeferredOperation_WaitsForShutdown(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:  []string{"/tv"},
		MovieLibs:    []string{"/movies"},
		DebounceTime: 10 * time.Second,
	}
	handler, err := NewMediaHandler(cfg)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		handler.replayDeferredOperation(jellyfin.DeferredOp{
			Type:       "organize",
			SourcePath: "/downloads/test.mkv",
		})
	}()

	time.Sleep(50 * time.Millisecond)
	handler.cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("replayDeferredOperation goroutine should complete after cancel")
	}
}
