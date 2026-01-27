package organizer

import (
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOrganizeWorkflow_EndToEnd tests the complete file organization workflow
func TestOrganizeWorkflow_EndToEnd(t *testing.T) {
	// Setup
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create test files
	movieFile := filepath.Join(sourceDir, "The.Matrix.1999.1080p.mkv")
	tvFile := filepath.Join(sourceDir, "Silo.2023.S01E02.1080p.mkv")
	createTestFile(t, movieFile, 2*1024*1024*1024)
	createTestFile(t, tvFile, 500*1024*1024)

	// Initialize components
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize movie
	movieResult, err := org.OrganizeMovie(movieFile, libraryDir)
	require.NoError(t, err)
	assert.True(t, movieResult.Success, "Movie organization should succeed")

	// Organize TV episode
	tvResult, err := org.OrganizeTVEpisode(tvFile, libraryDir)
	require.NoError(t, err)
	assert.True(t, tvResult.Success, "TV episode organization should succeed")

	// Verify database has both
	movie, err := db.GetMovieByTitle("The Matrix", 1999)
	require.NoError(t, err)
	assert.NotNil(t, movie, "Movie should be in database")
	assert.Equal(t, "jellywatch", movie.Source)

	series, err := db.GetSeriesByTitle("Silo", 2023)
	require.NoError(t, err)
	assert.NotNil(t, series, "Series should be in database")
	assert.Equal(t, "jellywatch", series.Source)

	// Verify files in correct locations
	moviePath := filepath.Join(libraryDir, "The Matrix (1999)", "The Matrix (1999).mkv")
	assert.FileExists(t, moviePath, "Movie should be in correct location")

	tvPath := filepath.Join(libraryDir, "Silo (2023)", "Season 01", "Silo (2023) S01E02.mkv")
	assert.FileExists(t, tvPath, "TV episode should be in correct location")
}
