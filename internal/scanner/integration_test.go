package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestLibrary creates a temporary library structure for testing
func setupTestLibrary(t *testing.T) (libraryDir string, cleanup func()) {
	libraryDir = filepath.Join(os.TempDir(), "plex2jellyfin-test-library-"+t.Name())
	err := os.MkdirAll(libraryDir, 0755)
	require.NoError(t, err)

	cleanup = func() {
		os.RemoveAll(libraryDir)
	}

	return libraryDir, cleanup
}

// createTestShow creates a test TV show with episodes
func createTestShow(t *testing.T, libraryDir, showName string, season, episodeCount int) {
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
		require.NoError(t, f.Truncate(500*1024*1024)) // 500MB
		require.NoError(t, f.Close())
	}
}

// createTestMovie creates a test movie
func createTestMovie(t *testing.T, libraryDir, movieName string) {
	movieDir := filepath.Join(libraryDir, movieName)
	err := os.MkdirAll(movieDir, 0755)
	require.NoError(t, err)

	movieFile := filepath.Join(movieDir, movieName+".mkv")
	f, err := os.Create(movieFile)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(2*1024*1024*1024)) // 2GB
	require.NoError(t, f.Close())
}

// TestScanner_PopulatesDatabase tests that scanner populates the database correctly
func TestScanner_PopulatesDatabase(t *testing.T) {
	// Create test library structure
	libraryDir, cleanup := setupTestLibrary(t)
	defer cleanup()

	// Create test files
	createTestShow(t, libraryDir, "Silo (2023)", 1, 5) // 5 episodes
	createTestMovie(t, libraryDir, "The Matrix (1999)")

	// Scan
	dbPath := filepath.Join(os.TempDir(), "plex2jellyfin-test-"+t.Name()+".db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer func() {
		db.Close()
		os.Remove(dbPath)
	}()

	scanner := NewFileScanner(db)
	result, err := scanner.ScanLibraries(context.Background(), []string{libraryDir}, []string{libraryDir})
	require.NoError(t, err)

	// Verify database populated
	assert.Greater(t, result.FilesAdded, 0, "Files should be added to database")

	// Verify series
	series, err := db.GetSeriesByTitle("Silo", 2023)
	require.NoError(t, err)
	if series != nil {
		assert.Equal(t, "filesystem", series.Source, "Source should be filesystem")
	}

	// Verify movie
	movie, err := db.GetMovieByTitle("The Matrix", 1999)
	require.NoError(t, err)
	if movie != nil {
		assert.Equal(t, "filesystem", movie.Source, "Source should be filesystem")
	}
}

func TestScanner_UsesParentFolderForObfuscatedEpisodeFilename(t *testing.T) {
	libraryDir, cleanup := setupTestLibrary(t)
	defer cleanup()

	seasonDir := filepath.Join(
		libraryDir,
		"Euphoria.US.S02E02.1080p.HMAX.WEB-DL.DD5.1.x264-NTb-AsRequested",
	)
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	sourcePath := filepath.Join(seasonDir, "r2Wy7PaRxoEn0Q5WHNLI (1_0).mkv")
	f, err := os.Create(sourcePath)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(60*1024*1024))
	require.NoError(t, f.Close())

	dbPath := filepath.Join(os.TempDir(), "plex2jellyfin-test-"+t.Name()+".db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)
	defer func() {
		db.Close()
		os.Remove(dbPath)
	}()

	scanner := NewFileScanner(db)
	result, err := scanner.ScanLibraries(context.Background(), []string{libraryDir}, nil)
	require.NoError(t, err)
	require.Empty(t, result.Errors)

	file, err := db.GetMediaFile(sourcePath)
	require.NoError(t, err)
	require.NotNil(t, file)
	require.NotNil(t, file.Season)
	require.NotNil(t, file.Episode)
	require.NotNil(t, file.ParentSeriesID)
	require.NotNil(t, file.ParentEpisodeID)
	assert.Equal(t, "episode", file.MediaType)
	assert.Equal(t, "euphoriaus", file.NormalizedTitle)
	assert.Equal(t, 2, *file.Season)
	assert.Equal(t, 2, *file.Episode)
	assert.Equal(t, "folder", file.ParseMethod)

	episode, err := db.GetEpisode(*file.ParentSeriesID, 2, 2)
	require.NoError(t, err)
	require.NotNil(t, episode)
	assert.Equal(t, *file.ParentEpisodeID, episode.ID)
	require.NotNil(t, episode.BestFileID)
	assert.Equal(t, file.ID, *episode.BestFileID)
}
