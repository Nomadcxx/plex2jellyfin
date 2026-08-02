package organizer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/scanner"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createSparseTestFile(t *testing.T, path string, size int64) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))
	require.NoError(t, f.Close())
}

func TestOrganizeMovie_IndexesDestinationAndRemovesStaleRows(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	testFile := filepath.Join(sourceDir, "Inception.2010.1080p.mkv")
	createSparseTestFile(t, testFile, 600*1024*1024)

	require.NoError(t, db.UpsertMediaFile(&database.MediaFile{
		Path:            testFile,
		Size:            600 * 1024 * 1024,
		MediaType:       "movie",
		NormalizedTitle: "inception",
		Year:            intPtr(2010),
		LibraryRoot:     sourceDir,
		QualityScore:    100,
	}))

	existingDir := filepath.Join(libraryDir, "Inception (2010)")
	require.NoError(t, os.MkdirAll(existingDir, 0755))
	existingFile := filepath.Join(existingDir, "Inception.2010.720p.mkv")
	createSparseTestFile(t, existingFile, 550*1024*1024)
	require.NoError(t, db.UpsertMediaFile(&database.MediaFile{
		Path:            existingFile,
		Size:            550 * 1024 * 1024,
		MediaType:       "movie",
		NormalizedTitle: "inception",
		Year:            intPtr(2010),
		LibraryRoot:     libraryDir,
		QualityScore:    50,
	}))

	fileScanner := scanner.NewFileScanner(db)
	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
		WithForceOverwrite(true),
		WithPathIndexer(func(ctx context.Context, path, libraryRoot, mediaType string) error {
			_, err := fileScanner.ScanPath(ctx, path, libraryRoot, mediaType)
			return err
		}),
	)
	require.NoError(t, err)

	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "organize failed: %v", result.Error)

	stale, err := db.GetMediaFile(testFile)
	require.NoError(t, err)
	assert.Nil(t, stale, "source path row should be removed")

	staleExisting, err := db.GetMediaFile(existingFile)
	require.NoError(t, err)
	assert.Nil(t, staleExisting, "replaced path row should be removed")

	indexed, err := db.GetMediaFile(result.TargetPath)
	require.NoError(t, err)
	require.NotNil(t, indexed, "destination must be indexed via ScanPath")
	assert.Equal(t, "movie", indexed.MediaType)
	assert.Equal(t, libraryDir, indexed.LibraryRoot)
}

// A post-move indexing failure must be reported without demoting the organize:
// the bytes are on disk, so the decision has to keep target_path or the file
// becomes invisible to the sweep, verification, and duplicate analysis.
func TestOrganizeMovie_IndexFailureReportsConsistencyErrorButKeepsSuccess(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	testFile := filepath.Join(sourceDir, "Inception.2010.1080p.mkv")
	createTestFile(t, testFile, 2*1024*1024)
	require.NoError(t, db.UpsertMediaFile(&database.MediaFile{
		Path:            testFile,
		Size:            2 * 1024 * 1024,
		MediaType:       "movie",
		NormalizedTitle: "inception",
		Year:            intPtr(2010),
		LibraryRoot:     sourceDir,
		QualityScore:    100,
	}))

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
		WithPathIndexer(func(ctx context.Context, path, libraryRoot, mediaType string) error {
			return errors.New("scan boom")
		}),
	)
	require.NoError(t, err)

	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "the move completed; only indexing failed")
	require.NoError(t, result.Error)
	require.Error(t, result.InventoryError)
	assert.Contains(t, result.InventoryError.Error(), "scan boom")
	assert.NotEmpty(t, result.TargetPath, "target path must survive an indexing failure")
	assert.FileExists(t, result.TargetPath, "file move should still have completed")

	// The stale source row is the only remaining record of the file, so it
	// must not be deleted when the replacement index never landed.
	stale, err := db.GetMediaFile(testFile)
	require.NoError(t, err)
	assert.NotNil(t, stale, "source row must be retained when indexing failed")
}

func TestOrganizeMovie_DryRunSkipsInventoryRefresh(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	testFile := filepath.Join(sourceDir, "Inception.2010.1080p.mkv")
	createTestFile(t, testFile, 1024)

	var scanned int
	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithDryRun(true),
		WithPathIndexer(func(ctx context.Context, path, libraryRoot, mediaType string) error {
			scanned++
			return nil
		}),
	)
	require.NoError(t, err)

	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Equal(t, 0, scanned)
	assert.FileExists(t, testFile)
}

func TestOrganizeTVEpisode_IndexesDestination(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	testFile := filepath.Join(sourceDir, "Silo.2023.S01E02.1080p.WEB-DL.mkv")
	createSparseTestFile(t, testFile, 60*1024*1024)

	require.NoError(t, db.UpsertMediaFile(&database.MediaFile{
		Path:            testFile,
		Size:            60 * 1024 * 1024,
		MediaType:       "episode",
		NormalizedTitle: "silo",
		Year:            intPtr(2023),
		Season:          intPtr(1),
		Episode:         intPtr(2),
		LibraryRoot:     sourceDir,
	}))

	fileScanner := scanner.NewFileScanner(db)
	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
		WithPathIndexer(func(ctx context.Context, path, libraryRoot, mediaType string) error {
			_, err := fileScanner.ScanPath(ctx, path, libraryRoot, mediaType)
			return err
		}),
	)
	require.NoError(t, err)

	result, err := org.OrganizeTVEpisode(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "organize failed: %v", result.Error)

	stale, err := db.GetMediaFile(testFile)
	require.NoError(t, err)
	assert.Nil(t, stale)

	indexed, err := db.GetMediaFile(result.TargetPath)
	require.NoError(t, err)
	require.NotNil(t, indexed)
	assert.Equal(t, "episode", indexed.MediaType)
}

func intPtr(v int) *int { return &v }
