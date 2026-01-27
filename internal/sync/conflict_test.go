package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestLibrary creates a temporary library structure for testing
func setupTestLibraryForConflict(t *testing.T) (libraryDir string, cleanup func()) {
	libraryDir = filepath.Join(os.TempDir(), "jellywatch-test-lib-"+t.Name())
	err := os.MkdirAll(libraryDir, 0755)
	require.NoError(t, err)

	cleanup = func() {
		os.RemoveAll(libraryDir)
	}

	return libraryDir, cleanup
}

// createTestShowForConflict creates a test TV show with episodes
func createTestShowForConflict(t *testing.T, libraryDir, showName string, season, episodeCount int) {
	showDir := filepath.Join(libraryDir, showName)
	seasonDir := filepath.Join(showDir, naming.FormatSeasonFolder(season))
	err := os.MkdirAll(seasonDir, 0755)
	require.NoError(t, err)

	// Extract title and year from showName (e.g., "Silo (2023)")
	title := showName
	year := ""
	if idx := len(showName) - 6; idx > 0 && showName[idx:idx+1] == "(" && showName[len(showName)-1:] == ")" {
		year = showName[idx+1 : len(showName)-1]
		title = showName[:idx-1]
	}

	for i := 1; i <= episodeCount; i++ {
		episodeFile := filepath.Join(seasonDir, naming.FormatTVEpisodeFilename(title, year, season, i, "mkv"))
		f, err := os.Create(episodeFile)
		require.NoError(t, err)
		f.Write(make([]byte, 500*1024*1024)) // 500MB
		f.Close()
	}
}

// TestSyncFromFilesystem_DetectsConflicts tests that filesystem sync detects conflicts
func TestSyncFromFilesystem_DetectsConflicts(t *testing.T) {
	// Create two library directories with unique names
	lib1 := filepath.Join(os.TempDir(), "jellywatch-test-lib1-"+t.Name())
	lib2 := filepath.Join(os.TempDir(), "jellywatch-test-lib2-"+t.Name())
	
	err := os.MkdirAll(lib1, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(lib2, 0755)
	require.NoError(t, err)
	
	defer func() {
		os.RemoveAll(lib1)
		os.RemoveAll(lib2)
	}()

	// Create same show in both libraries
	createTestShowForConflict(t, lib1, "Silo (2023)", 1, 3)
	createTestShowForConflict(t, lib2, "Silo (2023)", 1, 2)

	// Create database
	dbPath := filepath.Join(os.TempDir(), "jellywatch-test-"+t.Name()+".db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer func() {
		db.Close()
		os.Remove(dbPath)
	}()

	// Create sync service (this is what jellywatch scan uses)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Suppress info logs
	}))

	syncService := NewSyncService(SyncConfig{
		DB:             db,
		TVLibraries:    []string{lib1, lib2},
		MovieLibraries: []string{},
		Logger:         logger,
	})

	ctx := context.Background()

	// Sync from filesystem (this calls UpsertSeries which triggers conflict detection)
	// Note: SyncFromFilesystem processes all libraries in one call
	_, err = syncService.SyncFromFilesystem(ctx)
	require.NoError(t, err)
	
	// Verify both shows were processed
	series1, _ := db.GetSeriesByTitle("Silo", 2023)
	require.NotNil(t, series1, "Series should exist after sync")
	t.Logf("Series after sync: Path=%s, Source=%s, Priority=%d", series1.CanonicalPath, series1.Source, series1.SourcePriority)

	// Verify conflict recorded
	conflicts, err := db.GetUnresolvedConflicts()
	require.NoError(t, err)

	// Debug: show all conflicts
	if len(conflicts) == 0 {
		t.Logf("No conflicts found. Checking series table...")
		series, _ := db.GetSeriesByTitle("Silo", 2023)
		if series != nil {
			t.Logf("Series found: Title=%s, Path=%s, Source=%s, Priority=%d", series.Title, series.CanonicalPath, series.Source, series.SourcePriority)
		} else {
			t.Logf("No series found for Silo (2023)")
		}
	} else {
		t.Logf("Found %d conflicts:", len(conflicts))
		for _, c := range conflicts {
			t.Logf("  - %s (%d): %d locations", c.Title, *c.Year, len(c.Locations))
		}
	}

	// Find Silo conflict
	var siloConflict *database.Conflict
	for _, c := range conflicts {
		if c.Title == "Silo" && c.Year != nil && *c.Year == 2023 {
			siloConflict = &c
			break
		}
	}

	require.NotNil(t, siloConflict, "Conflict should be detected for Silo (2023). Found %d total conflicts", len(conflicts))
	assert.Contains(t, siloConflict.Locations, filepath.Join(lib1, "Silo (2023)"), "Conflict should include lib1 location")
	assert.Contains(t, siloConflict.Locations, filepath.Join(lib2, "Silo (2023)"), "Conflict should include lib2 location")
}
