package organizer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/analyzer"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failAfterFirstMoveTransferer struct {
	moves int
}

type recordingTransferer struct {
	moves  int
	copies int
}

type blockingConcurrentTransferer struct {
	mu            sync.Mutex
	moves         int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
	releaseSecond chan struct{}
	inner         recordingTransferer
}

func (t *blockingConcurrentTransferer) Move(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	t.mu.Lock()
	t.moves++
	move := t.moves
	t.mu.Unlock()

	switch move {
	case 1:
		close(t.firstStarted)
		<-t.releaseFirst
	case 2:
		close(t.secondStarted)
		<-t.releaseSecond
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inner.Move(src, dst, opts)
}

func (t *blockingConcurrentTransferer) Copy(string, string, transfer.TransferOptions) (*transfer.TransferResult, error) {
	return nil, errors.New("unexpected copy")
}

func (t *blockingConcurrentTransferer) CanResume() bool { return false }
func (t *blockingConcurrentTransferer) Name() string    { return "blocking-concurrent" }

func (t *recordingTransferer) Move(src, dst string, _ transfer.TransferOptions) (*transfer.TransferResult, error) {
	t.moves++
	info, err := os.Stat(src)
	if err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	return &transfer.TransferResult{
		Success:       true,
		SourceRemoved: true,
		BytesCopied:   info.Size(),
		BytesTotal:    info.Size(),
		Attempts:      1,
	}, nil
}

func (t *recordingTransferer) Copy(src, dst string, _ transfer.TransferOptions) (*transfer.TransferResult, error) {
	t.copies++
	data, err := os.ReadFile(src)
	if err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	return &transfer.TransferResult{
		Success:     true,
		BytesCopied: int64(len(data)),
		BytesTotal:  int64(len(data)),
		Attempts:    1,
	}, nil
}

func (t *recordingTransferer) CanResume() bool { return false }
func (t *recordingTransferer) Name() string    { return "recording" }

type failAfterFirstCopyTransferer struct {
	recordingTransferer
}

func (t *failAfterFirstCopyTransferer) Copy(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	if t.copies == 1 {
		t.copies++
		err := errors.New("forced copy failure")
		return &transfer.TransferResult{Error: err}, err
	}
	return t.recordingTransferer.Copy(src, dst, opts)
}

func (t *failAfterFirstMoveTransferer) Move(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	t.moves++
	if t.moves > 1 {
		return &transfer.TransferResult{Success: false, Error: fmt.Errorf("forced move failure")}, fmt.Errorf("forced move failure")
	}
	info, err := os.Stat(src)
	if err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return &transfer.TransferResult{Error: err}, err
	}
	return &transfer.TransferResult{Success: true, SourceRemoved: true, BytesCopied: info.Size(), BytesTotal: info.Size(), Attempts: 1}, nil
}

func (t *failAfterFirstMoveTransferer) Copy(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	return nil, fmt.Errorf("copy not implemented")
}

func (t *failAfterFirstMoveTransferer) CanResume() bool { return false }
func (t *failAfterFirstMoveTransferer) Name() string    { return "fail-after-first" }

// setupTestEnv creates temporary directories for testing
func setupTestEnv(t *testing.T) (sourceDir, libraryDir string, cleanup func()) {
	sourceDir = filepath.Join(os.TempDir(), "plex2jellyfin-test-source-"+t.Name())
	libraryDir = filepath.Join(os.TempDir(), "plex2jellyfin-test-library-"+t.Name())

	err := os.MkdirAll(sourceDir, 0755)
	require.NoError(t, err)

	err = os.MkdirAll(libraryDir, 0755)
	require.NoError(t, err)

	cleanup = func() {
		os.RemoveAll(sourceDir)
		os.RemoveAll(libraryDir)
	}

	return sourceDir, libraryDir, cleanup
}

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*database.MediaDB, func()) {
	dbPath := filepath.Join(os.TempDir(), "plex2jellyfin-test-"+t.Name()+".db")
	db, err := database.OpenPath(dbPath)
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.Remove(dbPath)
	}

	return db, cleanup
}

// createTestFile creates a test video file with specified size
func createTestFile(t *testing.T, path string, size int64) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	// Write dummy data
	data := make([]byte, size)
	_, err = f.Write(data)
	require.NoError(t, err)
}

func assertTransferPolicyResult(t *testing.T, mode SourceTransferMode, transferer *recordingTransferer, sourcePath, targetPath string, expectedCalls int) {
	t.Helper()

	sourceInfo, sourceErr := os.Stat(sourcePath)
	targetInfo, targetErr := os.Stat(targetPath)
	require.NoError(t, targetErr)

	switch mode {
	case TransferMove:
		assert.ErrorIs(t, sourceErr, os.ErrNotExist)
		assert.Equal(t, expectedCalls, transferer.moves)
		assert.Zero(t, transferer.copies)
	case TransferCopy:
		require.NoError(t, sourceErr)
		assert.False(t, os.SameFile(sourceInfo, targetInfo))
		assert.Zero(t, transferer.moves)
		assert.Equal(t, expectedCalls, transferer.copies)
	case TransferSeed:
		require.NoError(t, sourceErr)
		assert.True(t, os.SameFile(sourceInfo, targetInfo))
		assert.Zero(t, transferer.moves)
		assert.Zero(t, transferer.copies, "same-filesystem seed should not invoke fallback copy")
	default:
		t.Fatalf("unhandled transfer mode %d", mode)
	}
}

func TestOrganizerMovieTransferPolicy(t *testing.T) {
	tests := []struct {
		name string
		mode SourceTransferMode
	}{
		{name: "move", mode: TransferMove},
		{name: "copy", mode: TransferCopy},
		{name: "seed", mode: TransferSeed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sourcePath := filepath.Join(root, "incoming", "Policy.Movie.2026.mkv")
			libraryDir := filepath.Join(root, "library")
			createTestFile(t, sourcePath, 64)

			db, cleanup := setupTestDB(t)
			defer cleanup()
			require.NoError(t, db.UpsertMediaFile(&database.MediaFile{
				Path:            sourcePath,
				Size:            64,
				ModifiedAt:      time.Now(),
				MediaType:       "movie",
				NormalizedTitle: "Policy Movie",
			}))

			transferer := &recordingTransferer{}
			indexCalls := 0
			org, err := NewOrganizer([]string{libraryDir},
				WithDatabase(db),
				WithTransferer(transferer),
				WithTransferModeResolver(func(string) SourceTransferMode { return tt.mode }),
				WithPathIndexer(func(_ context.Context, path, libraryRoot, mediaType string) error {
					indexCalls++
					return db.UpsertMediaFile(&database.MediaFile{
						Path:            path,
						Size:            64,
						ModifiedAt:      time.Now(),
						MediaType:       mediaType,
						NormalizedTitle: "Policy Movie",
						LibraryRoot:     libraryRoot,
					})
				}),
			)
			require.NoError(t, err)

			result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Policy Movie", Year: "2026"})
			require.NoError(t, err)
			require.True(t, result.Success, "organize error: %v", result.Error)
			assertTransferPolicyResult(t, tt.mode, transferer, sourcePath, result.TargetPath, 1)
			assert.Equal(t, 1, indexCalls)

			sourceRow, err := db.GetMediaFile(sourcePath)
			require.NoError(t, err)
			assert.Nil(t, sourceRow, "staging source must not remain canonical inventory")
			targetRow, err := db.GetMediaFile(result.TargetPath)
			require.NoError(t, err)
			require.NotNil(t, targetRow)
			assert.Equal(t, libraryDir, targetRow.LibraryRoot)
		})
	}
}

func TestOrganizerEpisodeTransferPolicy(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode SourceTransferMode
	}{
		{name: "move", mode: TransferMove},
		{name: "copy", mode: TransferCopy},
		{name: "seed", mode: TransferSeed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sourcePath := filepath.Join(root, "incoming", "Policy.Show.2026.S01E01.mkv")
			libraryDir := filepath.Join(root, "library")
			createTestFile(t, sourcePath, 64)
			transferer := &recordingTransferer{}

			org, err := NewOrganizer([]string{libraryDir},
				WithTransferer(transferer),
				WithTransferModeResolver(func(string) SourceTransferMode { return tt.mode }),
			)
			require.NoError(t, err)
			result, err := org.OrganizeTVWithParsed(sourcePath, libraryDir, naming.TVShowInfo{
				Title: "Policy Show", Year: "2026", Season: 1, Episode: 1,
			})
			require.NoError(t, err)
			require.True(t, result.Success, "organize error: %v", result.Error)
			assertTransferPolicyResult(t, tt.mode, transferer, sourcePath, result.TargetPath, 1)
		})
	}
}

func TestOrganizerSeasonPackTransferPolicy(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode SourceTransferMode
	}{
		{name: "move", mode: TransferMove},
		{name: "copy", mode: TransferCopy},
		{name: "seed", mode: TransferSeed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			packDir := filepath.Join(root, "incoming", "Policy.Show.S01.1080p-GROUP")
			libraryDir := filepath.Join(root, "library")
			ep1 := filepath.Join(packDir, "Policy.Show.S01E01.1080p-GROUP.mkv")
			ep2 := filepath.Join(packDir, "Policy.Show.S01E02.1080p-GROUP.mkv")
			createTestFile(t, ep1, 64)
			createTestFile(t, ep2, 64)
			transferer := &recordingTransferer{}

			org, err := NewOrganizer([]string{libraryDir},
				WithTransferer(transferer),
				WithTransferModeResolver(func(string) SourceTransferMode { return tt.mode }),
			)
			require.NoError(t, err)
			result, err := org.OrganizeTVSeasonPackAuto(packDir, func(path string) (int64, error) {
				info, err := os.Stat(path)
				if err != nil {
					return 0, err
				}
				return info.Size(), nil
			})
			require.NoError(t, err)
			require.True(t, result.Success, "organize error: %v", result.Error)
			require.Len(t, result.Imported, 2)

			assertTransferPolicyResult(t, tt.mode, transferer, ep1, result.Imported[0].TargetPath, 2)
			if tt.mode == TransferMove {
				assert.Equal(t, 2, transferer.moves)
			} else if tt.mode == TransferCopy {
				assert.Equal(t, 2, transferer.copies)
			} else {
				assert.Zero(t, transferer.copies)
			}
			if tt.mode != TransferMove {
				sourceInfo, statErr := os.Stat(ep2)
				require.NoError(t, statErr)
				targetInfo, statErr := os.Stat(result.Imported[1].TargetPath)
				require.NoError(t, statErr)
				assert.Equal(t, tt.mode == TransferSeed, os.SameFile(sourceInfo, targetInfo))
			}
		})
	}
}

func TestOrganizerSeasonPackCopyPolicyRollsBackTargetWithoutTouchingSources(t *testing.T) {
	root := t.TempDir()
	packDir := filepath.Join(root, "incoming", "Policy.Show.S01.1080p-GROUP")
	libraryDir := filepath.Join(root, "library")
	ep1 := filepath.Join(packDir, "Policy.Show.S01E01.1080p-GROUP.mkv")
	ep2 := filepath.Join(packDir, "Policy.Show.S01E02.1080p-GROUP.mkv")
	createTestFile(t, ep1, 64)
	createTestFile(t, ep2, 64)
	transferer := &failAfterFirstCopyTransferer{}

	org, err := NewOrganizer([]string{libraryDir},
		WithTransferer(transferer),
		WithTransferModeResolver(func(string) SourceTransferMode { return TransferCopy }),
	)
	require.NoError(t, err)
	result, err := org.OrganizeTVSeasonPackAuto(packDir, func(path string) (int64, error) {
		info, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Error(t, result.Error)
	assert.NotContains(t, result.Error.Error(), "rollback failed")
	assert.FileExists(t, ep1)
	assert.FileExists(t, ep2)
	assert.NoFileExists(t, filepath.Join(libraryDir, "Policy Show", "Season 01", "Policy Show S01E01.mkv"))
}

func TestOrganizerTransferPolicyPreservesWithKeepSource(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "incoming", "Legacy.Copy.2026.mkv")
	libraryDir := filepath.Join(root, "library")
	createTestFile(t, sourcePath, 64)
	transferer := &recordingTransferer{}

	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(transferer), WithKeepSource(true))
	require.NoError(t, err)
	result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Legacy Copy", Year: "2026"})
	require.NoError(t, err)
	require.True(t, result.Success, "organize error: %v", result.Error)
	assert.FileExists(t, sourcePath)
	assert.Equal(t, 1, transferer.copies)
	assert.Zero(t, transferer.moves)
}

// TestOrganizeMovie_Basic tests basic movie organization
func TestOrganizeMovie_Basic(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create test file
	testFile := filepath.Join(sourceDir, "The.Matrix.1999.1080p.BluRay.mkv")
	// ponytail: Exercise a real transfer without requiring a multi-gigabyte temp file.
	createTestFile(t, testFile, 2*1024*1024)

	// Create organizer
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)

	// Override transferer (since NewOrganizer creates its own)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "Organization should succeed")

	// Verify file moved
	expectedPath := filepath.Join(libraryDir, "The Matrix (1999)", "The Matrix (1999).mkv")
	assert.FileExists(t, expectedPath, "File should exist at target location")
	assert.NoFileExists(t, testFile, "Source file should be removed")

	// Verify database
	movie, err := db.GetMovieByTitle("The Matrix", 1999)
	require.NoError(t, err)
	require.NotNil(t, movie, "Movie should be in database")
	assert.Equal(t, "plex2jellyfin", movie.Source, "Source should be plex2jellyfin")
	assert.Equal(t, 100, movie.SourcePriority, "Source priority should be 100")
	assert.Equal(t, filepath.Join(libraryDir, "The Matrix (1999)"), movie.CanonicalPath)
}

// TestOrganizeMovie_DryRun tests dry-run mode
func TestOrganizeMovie_DryRun(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create test file
	testFile := filepath.Join(sourceDir, "The.Matrix.1999.1080p.BluRay.mkv")
	createTestFile(t, testFile, 2*1024*1024*1024)

	// Create organizer with dry-run
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDryRun(true),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "Dry-run should report success")

	// Verify file NOT moved
	expectedPath := filepath.Join(libraryDir, "The Matrix (1999)", "The Matrix (1999).mkv")
	assert.NoFileExists(t, expectedPath, "File should NOT exist in dry-run mode")
	assert.FileExists(t, testFile, "Source file should still exist in dry-run mode")

	// Verify target path is calculated correctly
	assert.Equal(t, expectedPath, result.TargetPath, "Target path should be calculated correctly")
}

// TestOrganizeMovie_DuplicateExists tests handling of duplicate movies
func TestOrganizeMovie_DuplicateExists(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create existing lower quality file
	movieDir := filepath.Join(libraryDir, "The Matrix (1999)")
	err := os.MkdirAll(movieDir, 0755)
	require.NoError(t, err)

	existingFile := filepath.Join(movieDir, "The.Matrix.1999.1080p.BluRay.mkv")
	createTestFile(t, existingFile, 1024) // lower quality by filename

	// Create new higher quality file
	newFile := filepath.Join(sourceDir, "The.Matrix.1999.2160p.REMUX.mkv")
	createTestFile(t, newFile, 4096) // higher quality by filename

	// Create organizer
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeMovie(newFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "Organization should succeed: %v", result.Error)

	// Verify higher quality file exists
	expectedPath := filepath.Join(movieDir, "The Matrix (1999).mkv")
	assert.FileExists(t, expectedPath, "Higher quality file should exist")

	// Verify lower quality file was removed (replaced)
	// The file should be the new one (larger size)
	info, err := os.Stat(expectedPath)
	require.NoError(t, err)
	assert.Equal(t, int64(4096), info.Size(), "File should be the replacement")
}

// TestOrganizeMovie_ExistingFilePreservedOnTransferFailure verifies that
// when a transfer fails, the existing file in the target directory is NOT
// removed. The old code removed the existing file BEFORE the transfer,
// which meant a transfer failure left no file at all.
func TestOrganizeMovie_ExistingFilePreservedOnTransferFailure(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	movieDir := filepath.Join(libraryDir, "The Matrix (1999)")
	err := os.MkdirAll(movieDir, 0755)
	require.NoError(t, err)

	existingFile := filepath.Join(movieDir, "The.Matrix.1999.1080p.BluRay.mkv")
	createTestFile(t, existingFile, 1*1024*1024*1024)

	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	nonExistentSource := filepath.Join(sourceDir, "The.Matrix.1999.2160p.REMUX.mkv")

	movie := naming.MovieInfo{Title: "The Matrix", Year: "1999"}
	result, err := org.OrganizeMovieWithParsed(nonExistentSource, libraryDir, movie)
	require.NoError(t, err)
	require.False(t, result.Success, "should fail because source doesn't exist")

	assert.FileExists(t, existingFile,
		"existing file must be preserved when transfer fails")
}

// TestOrganizeMovie_DatabaseUpdate tests database is updated after organization
func TestOrganizeMovie_DatabaseUpdate(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create test file
	testFile := filepath.Join(sourceDir, "Inception.2010.1080p.mkv")
	// ponytail: Exercise DB persistence after a real transfer with a small fixture.
	createTestFile(t, testFile, 2*1024*1024)

	// Create organizer
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify database entry
	movie, err := db.GetMovieByTitle("Inception", 2010)
	require.NoError(t, err)
	require.NotNil(t, movie)
	assert.Equal(t, "plex2jellyfin", movie.Source)
	assert.Equal(t, 100, movie.SourcePriority)
	assert.Equal(t, filepath.Join(libraryDir, "Inception (2010)"), movie.CanonicalPath)
	assert.Equal(t, libraryDir, movie.LibraryRoot)
}

// TestOrganizeTVEpisode_Basic tests basic TV episode organization
func TestOrganizeTVEpisode_Basic(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create test file (with year in filename for proper parsing)
	testFile := filepath.Join(sourceDir, "Silo.2023.S01E02.1080p.WEB-DL.mkv")
	createTestFile(t, testFile, 500*1024*1024) // 500MB

	// Create organizer
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeTVEpisode(testFile, libraryDir)
	require.NoError(t, err)
	if !result.Success {
		t.Logf("Organization failed: %v, Error: %v, SkipReason: %s", result.Success, result.Error, result.SkipReason)
	}
	require.True(t, result.Success, "Organization should succeed: %v", result.Error)

	// Verify file moved
	expectedPath := filepath.Join(libraryDir, "Silo (2023)", "Season 01", "Silo (2023) S01E02.mkv")
	if !assert.FileExists(t, expectedPath, "File should exist at target location") {
		// Debug: list what's actually in the directory
		showDir := filepath.Join(libraryDir, "Silo (2023)")
		if entries, err := os.ReadDir(showDir); err == nil {
			t.Logf("Contents of %s:", showDir)
			for _, e := range entries {
				t.Logf("  - %s (dir: %v)", e.Name(), e.IsDir())
			}
		}
		t.Logf("Expected path: %s", expectedPath)
		t.Logf("Result target: %s", result.TargetPath)
	}
	assert.NoFileExists(t, testFile, "Source file should be removed")

	// Verify database
	series, err := db.GetSeriesByTitle("Silo", 2023)
	require.NoError(t, err)
	require.NotNil(t, series, "Series should be in database")
	assert.Equal(t, "plex2jellyfin", series.Source, "Source should be plex2jellyfin")
	assert.Equal(t, 100, series.SourcePriority, "Source priority should be 100")
	assert.Equal(t, filepath.Join(libraryDir, "Silo (2023)"), series.CanonicalPath)
}

// TestOrganizeTVEpisode_DatabaseUpdate tests database is updated after TV organization
func TestOrganizeTVEpisode_DatabaseUpdate(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create test file (with year in filename)
	testFile := filepath.Join(sourceDir, "Breaking.Bad.2008.S02E05.1080p.mkv")
	createTestFile(t, testFile, 500*1024*1024)

	// Create organizer
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeTVEpisode(testFile, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "result=%+v", result)

	// Verify database entry
	series, err := db.GetSeriesByTitle("Breaking Bad", 2008)
	require.NoError(t, err)
	require.NotNil(t, series)
	assert.Equal(t, "plex2jellyfin", series.Source)
	assert.Equal(t, 100, series.SourcePriority)
	assert.Equal(t, filepath.Join(libraryDir, "Breaking Bad (2008)"), series.CanonicalPath)
}

// TestOrganizeTVEpisode_ExistingShow tests organizing episode to existing show
func TestOrganizeTVEpisode_ExistingShow(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create existing show directory
	showDir := filepath.Join(libraryDir, "Silo (2023)")
	seasonDir := filepath.Join(showDir, "Season 01")
	err := os.MkdirAll(seasonDir, 0755)
	require.NoError(t, err)

	// Create existing episode (must match the format the organizer expects)
	existingEpisode := filepath.Join(seasonDir, "Silo (2023) S01E01.mkv")
	createTestFile(t, existingEpisode, 500*1024*1024)

	// Verify it exists before organizing
	assert.FileExists(t, existingEpisode, "Existing episode should exist before organizing")

	// Create new episode (with year in filename)
	newEpisode := filepath.Join(sourceDir, "Silo.2023.S01E02.1080p.mkv")
	createTestFile(t, newEpisode, 500*1024*1024)

	// Create organizer
	transferer, err := transfer.New(transfer.BackendRsync)
	require.NoError(t, err)

	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	org.transferer = transferer

	// Organize
	result, err := org.OrganizeTVEpisode(newEpisode, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success)

	// Verify new episode was added
	expectedPath := filepath.Join(seasonDir, "Silo (2023) S01E02.mkv")
	assert.FileExists(t, expectedPath, "New episode should be added")

	// Note: The existing episode might be removed if the organizer thinks it's a duplicate
	// This is actually correct behavior - the organizer removes lower quality duplicates
	// For this test, we just verify the new episode was added correctly
}

func TestOrganizeTVEpisode_TwoEpisodeRangeUsesCanonicalName(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	source := filepath.Join(sourceDir, "Daniel.Tigers.Neighborhood.S08E03-04.Daniel.Jodi.Asks.Before.Touching.1080p.WEB-DL.mkv")
	createTestFile(t, source, 1024)
	org, err := NewOrganizer([]string{libraryDir}, WithBackend(transfer.BackendNative))
	require.NoError(t, err)

	result, err := org.OrganizeTVEpisode(source, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "result=%+v", result)
	assert.Equal(t, filepath.Join(libraryDir, "Daniel Tigers Neighborhood", "Season 08", "Daniel Tigers Neighborhood S08E03-E04 - Daniel Jodi Asks Before Touching.mkv"), result.TargetPath)
	assert.FileExists(t, result.TargetPath)
}

func TestOrganizeTVEpisode_TwoEpisodeRangeDoesNotReplaceIndividualEpisode(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	seasonDir := filepath.Join(libraryDir, "Daniel Tigers Neighborhood", "Season 08")
	require.NoError(t, os.MkdirAll(seasonDir, 0o755))
	existing := filepath.Join(seasonDir, "Daniel Tigers Neighborhood S08E04.mkv")
	createTestFile(t, existing, 2048)
	source := filepath.Join(sourceDir, "Daniel.Tigers.Neighborhood.S08E03-04.2160p.WEB-DL.mkv")
	createTestFile(t, source, 4096)
	org, err := NewOrganizer([]string{libraryDir}, WithBackend(transfer.BackendNative))
	require.NoError(t, err)

	result, err := org.OrganizeTVEpisode(source, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Skipped)
	assert.Contains(t, result.SkipReason, "overlaps existing episode E04")
	assert.FileExists(t, existing)
	assert.FileExists(t, source)
}

func recordSuccessfulTVImport(t *testing.T, db *database.MediaDB, targetPath, sourceFilename string) {
	recordSuccessfulImport(t, db, targetPath, sourceFilename, "tv")
}

func recordSuccessfulImport(t *testing.T, db *database.MediaDB, targetPath, sourceFilename, mediaType string) {
	t.Helper()
	_, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       filepath.Join(filepath.Dir(targetPath), sourceFilename),
		SourceFilename:   sourceFilename,
		EventAt:          time.Now(),
		MediaTypeGuessed: mediaType,
		TargetPath:       targetPath,
		OrganizeOutcome:  "success",
	})
	require.NoError(t, err)
}

func insertPendingQualityDecision(t *testing.T, db *database.MediaDB, sourcePath string) int64 {
	t.Helper()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:     sourcePath,
		SourceFilename: filepath.Base(sourcePath),
		EventAt:        time.Now().UTC(),
	})
	require.NoError(t, err)
	return id
}

func persistTestOrganizeResult(t *testing.T, db *database.MediaDB, id int64, result *OrganizationResult, organizeErr error) {
	t.Helper()
	update := database.OrganizeUpdate{}
	switch {
	case organizeErr != nil:
		update.OrganizeOutcome = "failed"
		update.OrganizeError = organizeErr.Error()
	case result == nil:
		update.OrganizeOutcome = "failed"
		update.OrganizeError = "nil result"
	case result.Success:
		update.OrganizeOutcome = "success"
		update.TargetPath = result.TargetPath
		now := time.Now().UTC()
		update.TargetAt = &now
	case result.Skipped:
		update.OrganizeOutcome = "skipped"
		update.TargetPath = result.TargetPath
		update.OrganizeError = result.SkipReason
	default:
		update.OrganizeOutcome = "failed"
	}
	assert.NoError(t, db.UpdateOrganize(id, update))
}

func TestOrganizeTVRangeReplacesCoveredLowerQualityEpisode(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
	existing := filepath.Join(seasonDir, "Silo (2023) S01E07.mkv")
	createTestFile(t, existing, 32)
	recordSuccessfulTVImport(t, db, existing, "Silo.2023.S01E07.720p.WEB-DL.mkv")

	source := filepath.Join(sourceDir, "Silo.2023.S01E07-E08.1080p.BluRay.mkv")
	createTestFile(t, source, 64)
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
	require.NoError(t, err)

	result, err := org.OrganizeTVWithParsed(source, libraryDir, naming.TVShowInfo{
		Title: "Silo", Year: "2023", Season: 1, Episode: 7, EpisodeEnd: 8,
	})
	require.NoError(t, err)
	require.True(t, result.Success, "result=%+v", result)
	assert.Equal(t, 1, transferer.moves)
	assert.NoFileExists(t, existing)
	assert.FileExists(t, result.TargetPath)
}

func TestOrganizeTVRangeReplacesCoveredLowerQualityRange(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
	existing := filepath.Join(seasonDir, "Silo (2023) S01E07-E08.720p.WEB-DL.mkv")
	createTestFile(t, existing, 32)
	recordSuccessfulTVImport(t, db, existing, "Silo.2023.S01E07-E08.720p.WEB-DL.mkv")

	source := filepath.Join(sourceDir, "Silo.2023.S01E07-E09.1080p.BluRay.mkv")
	createTestFile(t, source, 64)
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
	require.NoError(t, err)

	result, err := org.OrganizeTVWithParsed(source, libraryDir, naming.TVShowInfo{
		Title: "Silo", Year: "2023", Season: 1, Episode: 7, EpisodeEnd: 9,
	})
	require.NoError(t, err)
	require.True(t, result.Success, "result=%+v", result)
	assert.Equal(t, 1, transferer.moves)
	assert.NoFileExists(t, existing)
	assert.FileExists(t, result.TargetPath)
}

func TestOrganizeTVRangePreservesExtendingExistingRange(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
	existing := filepath.Join(seasonDir, "Silo (2023) S01E07-E09.720p.WEB-DL.mkv")
	createTestFile(t, existing, 32)
	source := filepath.Join(sourceDir, "Silo.2023.S01E07-E08.1080p.BluRay.mkv")
	createTestFile(t, source, 64)
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(transferer))
	require.NoError(t, err)

	result, err := org.OrganizeTVWithParsed(source, libraryDir, naming.TVShowInfo{
		Title: "Silo", Year: "2023", Season: 1, Episode: 7, EpisodeEnd: 8,
	})
	require.NoError(t, err)
	require.True(t, result.Skipped, "result=%+v", result)
	assert.Contains(t, result.SkipReason, "multi-episode range overlaps existing episode")
	assert.Zero(t, transferer.moves)
	assert.FileExists(t, existing)
	assert.FileExists(t, source)
}

func TestOrganizeTVRangeDetectsOverlapBeginningBeforeCandidate(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
	existing := filepath.Join(seasonDir, "Silo (2023) S01E06-E07.720p.WEB-DL.mkv")
	createTestFile(t, existing, 32)
	source := filepath.Join(sourceDir, "Silo.2023.S01E07-E08.1080p.BluRay.mkv")
	createTestFile(t, source, 64)
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(transferer))
	require.NoError(t, err)

	result, err := org.OrganizeTVWithParsed(source, libraryDir, naming.TVShowInfo{
		Title: "Silo", Year: "2023", Season: 1, Episode: 7, EpisodeEnd: 8,
	})
	require.NoError(t, err)
	require.True(t, result.Skipped, "result=%+v", result)
	assert.Contains(t, result.SkipReason, "multi-episode range overlaps existing episode")
	assert.Zero(t, transferer.moves)
	assert.FileExists(t, existing)
	assert.FileExists(t, source)
}

func TestOrganizeTVEpisodePreservesRangeWithUncoveredEpisodes(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
	existing := filepath.Join(seasonDir, "Silo (2023) S01E07-E08.mkv")
	createTestFile(t, existing, 32)
	source := filepath.Join(sourceDir, "Silo.2023.S01E07.2160p.BluRay.mkv")
	createTestFile(t, source, 64)
	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(transferer))
	require.NoError(t, err)

	result, err := org.OrganizeTVEpisode(source, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Skipped, "result=%+v", result)
	assert.Contains(t, result.SkipReason, "multi-episode range overlaps existing episode")
	assert.Zero(t, transferer.moves)
	assert.FileExists(t, existing)
	assert.FileExists(t, source)
}

func TestOrganizeTVRangePreservesUncertainQualityOverlaps(t *testing.T) {
	tests := []struct {
		name           string
		sourceFilename string
		evidence       string
	}{
		{name: "equal quality", sourceFilename: "Silo.2023.S01E07-E08.1080p.WEB-DL.mkv", evidence: "Silo.2023.S01E07.1080p.WEB-DL.mkv"},
		{name: "lower quality", sourceFilename: "Silo.2023.S01E07-E08.1080p.WEB-DL.mkv", evidence: "Silo.2023.S01E07.2160p.BluRay.mkv"},
		{name: "unavailable evidence", sourceFilename: "Silo.2023.S01E07-E08.1080p.BluRay.mkv"},
		{name: "incoming quality unavailable", sourceFilename: "Silo.2023.S01E07-E08.mkv", evidence: "Silo.2023.S01E07.720p.WEB-DL.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sourceDir := filepath.Join(root, "source")
			libraryDir := filepath.Join(root, "library")
			db, err := database.OpenPath(filepath.Join(root, "media.db"))
			require.NoError(t, err)
			defer db.Close()

			seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
			existing := filepath.Join(seasonDir, "Silo (2023) S01E07.mkv")
			createTestFile(t, existing, 32)
			if tt.evidence != "" {
				recordSuccessfulTVImport(t, db, existing, tt.evidence)
			}
			source := filepath.Join(sourceDir, tt.sourceFilename)
			createTestFile(t, source, 64)
			transferer := &recordingTransferer{}
			org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
			require.NoError(t, err)

			result, err := org.OrganizeTVWithParsed(source, libraryDir, naming.TVShowInfo{
				Title: "Silo", Year: "2023", Season: 1, Episode: 7, EpisodeEnd: 8,
			})
			require.NoError(t, err)
			require.True(t, result.Skipped, "result=%+v", result)
			assert.Contains(t, result.SkipReason, "multi-episode range overlaps existing episode")
			assert.Zero(t, transferer.moves)
			assert.FileExists(t, existing)
			assert.FileExists(t, source)
		})
	}
}

type failingRangeTransferer struct {
	moves int
}

func (t *failingRangeTransferer) Move(_, _ string, _ transfer.TransferOptions) (*transfer.TransferResult, error) {
	t.moves++
	err := errors.New("forced range transfer failure")
	return &transfer.TransferResult{Error: err}, err
}

func (t *failingRangeTransferer) Copy(_, _ string, _ transfer.TransferOptions) (*transfer.TransferResult, error) {
	return nil, errors.New("unexpected copy")
}

func (t *failingRangeTransferer) CanResume() bool { return false }
func (t *failingRangeTransferer) Name() string    { return "failing-range" }

func TestOrganizeTVRangeTransferFailurePreservesSourceAndOverlap(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	seasonDir := filepath.Join(libraryDir, "Silo (2023)", "Season 01")
	existing := filepath.Join(seasonDir, "Silo (2023) S01E07-E08.mkv")
	createTestFile(t, existing, 32)
	recordSuccessfulTVImport(t, db, existing, "Silo.2023.S01E07-E08.720p.WEB-DL.mkv")
	source := filepath.Join(sourceDir, "Silo.2023.S01E07-E08.1080p.BluRay.mkv")
	createTestFile(t, source, 64)
	transferer := &failingRangeTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
	require.NoError(t, err)

	result, err := org.OrganizeTVWithParsed(source, libraryDir, naming.TVShowInfo{
		Title: "Silo", Year: "2023", Season: 1, Episode: 7, EpisodeEnd: 8,
	})
	require.NoError(t, err)
	require.ErrorContains(t, result.Error, "transfer failed")
	assert.Equal(t, 1, transferer.moves)
	existingInfo, err := os.Stat(existing)
	require.NoError(t, err)
	assert.Equal(t, int64(32), existingInfo.Size(), "failed publication must leave the old target intact")
	assert.FileExists(t, source)
}

func TestOrganizeTVWithParsed_BlocksIdentityUnsafeExistingShow(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	showDir := filepath.Join(libraryDir, "Utopia (2013)")
	seasonDir := filepath.Join(showDir, "Season 02")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	sourcePath := filepath.Join(sourceDir, "Utopia.2014.S02E07.mkv")
	createTestFile(t, sourcePath, 500*1024*1024)

	org, err := NewOrganizer([]string{libraryDir}, WithBackend(transfer.BackendNative))
	require.NoError(t, err)

	result, err := org.OrganizeTVWithParsed(sourcePath, libraryDir, naming.TVShowInfo{
		Title:   "Utopia",
		Year:    "2014",
		Season:  2,
		Episode: 7,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Skipped)
	require.Contains(t, result.SkipReason, "identity safety")
	assert.FileExists(t, sourcePath, "identity-unsafe source should not be moved")
	assert.NoFileExists(t, filepath.Join(seasonDir, "Utopia (2014) S02E07.mkv"))
}

func TestPlaybackSafetyBlocksActiveStream(t *testing.T) {
	client := jellyfin.NewClient(jellyfin.Config{
		URL:    "http://jellyfin.local",
		APIKey: "k",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/Sessions" {
					return jsonResponse(http.StatusOK, `[{"Id":"s1","UserName":"alice","DeviceName":"Living Room","NowPlayingItem":{"Path":"/tmp/The.Matrix.1999.1080p.mkv"}}]`), nil
				}
				return jsonResponse(http.StatusNotFound, "not found"), nil
			}),
			Timeout: 2 * time.Second,
		},
	})
	org, err := NewOrganizer([]string{t.TempDir()},
		WithDryRun(true),
		WithJellyfinClient(client, true),
	)
	require.NoError(t, err)

	result, err := org.OrganizeMovie("/tmp/The.Matrix.1999.1080p.mkv", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Skipped)
	assert.Contains(t, result.SkipReason, "via API")
}

func TestPlaybackSafetyFailOpenWhenJellyfinUnavailable(t *testing.T) {
	client := jellyfin.NewClient(jellyfin.Config{
		URL:    "http://jellyfin.local",
		APIKey: "k",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusInternalServerError, "error"), nil
			}),
			Timeout: 2 * time.Second,
		},
	})
	libraryDir := t.TempDir()
	org, err := NewOrganizer([]string{libraryDir},
		WithDryRun(true),
		WithJellyfinClient(client, true),
	)
	require.NoError(t, err)

	result, err := org.OrganizeMovie("/tmp/The.Matrix.1999.1080p.mkv", libraryDir)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestOrganizeMovieWithParsed_Basic(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	testFile := filepath.Join(sourceDir, "unrecognizable.mkv")
	createTestFile(t, testFile, 100*1024*1024) // 100MB

	org, err := NewOrganizer([]string{libraryDir},
		WithDryRun(true),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)

	movie := naming.MovieInfo{Title: "The Matrix", Year: "1999"}
	result, err := org.OrganizeMovieWithParsed(testFile, libraryDir, movie)
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Equal(t, filepath.Join(libraryDir, "The Matrix (1999)", "The Matrix (1999).mkv"), result.TargetPath)
}

func TestOrganizeTVWithParsed_Basic(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	testFile := filepath.Join(sourceDir, "unrecognizable.s01e02.mkv")
	createTestFile(t, testFile, 100*1024*1024)

	org, err := NewOrganizer([]string{libraryDir},
		WithDryRun(true),
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)

	tv := naming.TVShowInfo{Title: "The Office", Year: "2005", Season: 1, Episode: 2}
	result, err := org.OrganizeTVWithParsed(testFile, libraryDir, tv)
	require.NoError(t, err)
	require.True(t, result.Success)
	// target should be in Season 01 folder
	assert.Contains(t, result.TargetPath, "Season 01")
	assert.Contains(t, result.TargetPath, "The Office")
}

func TestFindExistingShowDir_PunctuationVariants(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory with apostrophe
	os.MkdirAll(filepath.Join(tmpDir, "Chip 'n Dale Rescue Rangers (2022)"), 0755)
	// Create directory with colon
	os.MkdirAll(filepath.Join(tmpDir, "Law & Order: SVU (1999)"), 0755)
	// Create directory with exclamation
	os.MkdirAll(filepath.Join(tmpDir, "American Dad! (2005)"), 0755)

	tests := []struct {
		name      string
		title     string
		expectDir string
	}{
		{
			name:      "apostrophe in dir but not in title",
			title:     "Chip n Dale Rescue Rangers",
			expectDir: "Chip 'n Dale Rescue Rangers (2022)",
		},
		{
			name:      "colon and ampersand variants",
			title:     "Law & Order SVU",
			expectDir: "Law & Order: SVU (1999)",
		},
		{
			name:      "exclamation in dir but not in title",
			title:     "American Dad",
			expectDir: "American Dad! (2005)",
		},
		{
			name:      "no match returns empty",
			title:     "Nonexistent Show",
			expectDir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findExistingShowDir(tmpDir, tt.title)
			if tt.expectDir == "" {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, filepath.Join(tmpDir, tt.expectDir), result)
			}
		})
	}
}

func TestFindExistingSeasonDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create various season folder formats
	os.MkdirAll(filepath.Join(tmpDir, "Season 1"), 0755)  // non-padded
	os.MkdirAll(filepath.Join(tmpDir, "Season 02"), 0755) // correctly padded
	os.MkdirAll(filepath.Join(tmpDir, "Season 10"), 0755) // two digits

	tests := []struct {
		name      string
		showDir   string
		season    int
		expectDir string
	}{
		{
			name:      "finds non-padded Season 1 when looking for season 1",
			showDir:   tmpDir,
			season:    1,
			expectDir: filepath.Join(tmpDir, "Season 1"),
		},
		{
			name:      "finds padded Season 02 when looking for season 2",
			showDir:   tmpDir,
			season:    2,
			expectDir: filepath.Join(tmpDir, "Season 02"),
		},
		{
			name:      "finds Season 10 when looking for season 10",
			showDir:   tmpDir,
			season:    10,
			expectDir: filepath.Join(tmpDir, "Season 10"),
		},
		{
			name:      "returns empty for non-existent season",
			showDir:   tmpDir,
			season:    5,
			expectDir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findExistingSeasonDir(tt.showDir, tt.season)
			if result != tt.expectDir {
				t.Errorf("findExistingSeasonDir(%q, %d) = %q, want %q",
					tt.showDir, tt.season, result, tt.expectDir)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 2.2: canonical episode-key dedup tests
// ---------------------------------------------------------------------------

// setupTVOrganizer creates an Organizer with no database dependency.
func setupTVOrganizer(t *testing.T, opts ...func(*Organizer)) *Organizer {
	t.Helper()
	org, err := NewOrganizer([]string{t.TempDir()},
		WithBackend(transfer.BackendRsync),
	)
	require.NoError(t, err)
	for _, o := range opts {
		o(org)
	}
	return org
}

// TestOrganizeTVEpisode_Dedup_DifferentNameSameKey verifies that an existing
// file whose name does NOT share the canonical prefix (e.g. downloaded under a
// different show-name format) is still recognised as the same episode by its
// S/E key, so a lower-quality incoming file is skipped.
//
// The old prefix-based check (strings.HasPrefix) fails here because
// "Show.S01E03.BluRay.mkv" does not start with "Silo (2023) S01E03".
func TestOrganizeTVEpisode_Dedup_DifferentNameSameKey(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	showDir := filepath.Join(libraryDir, "Silo (2023)")
	seasonDir := filepath.Join(showDir, "Season 01")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	// Existing file uses a non-canonical naming convention — no show title.
	// The prefix check will NOT recognise this as the same episode.
	// Use BluRay so it outscores any incoming file with unknown/WEB source.
	existing := filepath.Join(seasonDir, "Show.S01E03.1080p.BluRay.mkv")
	createTestFile(t, existing, 1024*1024)

	// Incoming: same key, lower quality (720p WEB-DL scores below 1080p BluRay).
	incoming := filepath.Join(sourceDir, "Silo.2023.S01E03.720p.WEB-DL.mkv")
	createTestFile(t, incoming, 1024)

	org := setupTVOrganizer(t)
	result, err := org.OrganizeTVEpisode(incoming, libraryDir)
	require.NoError(t, err)
	assert.True(t, result.Skipped, "lower-quality same-key episode must be skipped; got SkipReason=%q err=%v", result.SkipReason, result.Error)
	assert.FileExists(t, existing, "existing higher-quality file must not be removed")
}

// TestOrganizeTVEpisode_Dedup_SubtitleNotCounted verifies that a .srt file
// with the same episode key is not treated as an existing video file.
func TestOrganizeTVEpisode_Dedup_SubtitleNotCounted(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	showDir := filepath.Join(libraryDir, "Silo (2023)")
	seasonDir := filepath.Join(showDir, "Season 01")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	// Only a subtitle — no video file.
	srt := filepath.Join(seasonDir, "Silo (2023) S01E04.en.srt")
	require.NoError(t, os.WriteFile(srt, []byte("1\n00:00:01 --> 00:00:02\nHello\n"), 0644))

	incoming := filepath.Join(sourceDir, "Silo.2023.S01E04.1080p.WEB-DL.mkv")
	createTestFile(t, incoming, 1024)

	org := setupTVOrganizer(t, WithDryRun(true))
	result, err := org.OrganizeTVEpisode(incoming, libraryDir)
	require.NoError(t, err)
	assert.True(t, result.Success, "organizer must succeed when only subtitle exists: %v", result.Error)
	assert.False(t, result.Skipped, "must not skip because of subtitle-only match")
}

// TestOrganizeTVWithParsed_Dedup_DifferentNameSameKey is the equivalent test
// for OrganizeTVWithParsed. Existing file name breaks the prefix check.
func TestOrganizeTVWithParsed_Dedup_DifferentNameSameKey(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	showDir := filepath.Join(libraryDir, "Breaking Bad (2008)")
	seasonDir := filepath.Join(showDir, "Season 02")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	// Non-canonical filename: prefix check won't match "Breaking Bad (2008) S02E05".
	existing := filepath.Join(seasonDir, "BB.S02E05.1080p.BluRay.mkv")
	createTestFile(t, existing, 1024*1024)

	incoming := filepath.Join(sourceDir, "Breaking.Bad.S02E05.720p.mkv")
	createTestFile(t, incoming, 1024)

	org := setupTVOrganizer(t)
	tv := naming.TVShowInfo{Title: "Breaking Bad", Year: "2008", Season: 2, Episode: 5}
	result, err := org.OrganizeTVWithParsed(incoming, libraryDir, tv)
	require.NoError(t, err)
	assert.True(t, result.Skipped, "lower-quality same-key episode must be skipped; got SkipReason=%q err=%v", result.SkipReason, result.Error)
	assert.FileExists(t, existing, "existing higher-quality file must not be removed")
}

// TestOrganizeTVEpisode_ForceOverwrite_RemovesOldFile checks that with
// forceOverwrite=true the same-episode file (even with a different name) is
// removed and the new file is organised successfully. Crucially, an unrelated
// episode in the same season must NOT be removed.
func TestOrganizeTVEpisode_ForceOverwrite_RemovesOldFile(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	showDir := filepath.Join(libraryDir, "Silo (2023)")
	seasonDir := filepath.Join(showDir, "Season 01")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	// Same episode, non-canonical name (breaks prefix check).
	oldSameEp := filepath.Join(seasonDir, "Silo.S01E06.OLD.mkv")
	createTestFile(t, oldSameEp, 1*1024*1024)

	// Unrelated episode — must survive.
	unrelated := filepath.Join(seasonDir, "Silo (2023) S01E01.mkv")
	createTestFile(t, unrelated, 1*1024*1024)

	incoming := filepath.Join(sourceDir, "Silo.2023.S01E06.2160p.UHD.mkv")
	createTestFile(t, incoming, 2*1024*1024)

	org := setupTVOrganizer(t, WithForceOverwrite(true))
	result, err := org.OrganizeTVEpisode(incoming, libraryDir)
	require.NoError(t, err)
	require.True(t, result.Success, "force-overwrite organize must succeed: %v", result.Error)
	assert.NoFileExists(t, oldSameEp, "old same-episode file must be removed on force overwrite")
	assert.FileExists(t, unrelated, "unrelated episode must not be removed")
}

// TestOrganizeTVEpisode_DateBased checks that a date-based episode is
// deduplicated when the naming parser yields a non-zero season/episode key,
// and does not accidentally remove an existing date-keyed file.
func TestOrganizeTVEpisode_DateBased(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// naming.ParseTVShowName maps "2021.03.15" → season=2021, episode=315.
	showDir := filepath.Join(libraryDir, "Late Night Show")
	seasonDir := filepath.Join(showDir, "Season 2021")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	// Existing file whose name the prefix check cannot match against an incoming
	// "Late Night Show 2021-03-15.mkv" target.
	existing := filepath.Join(seasonDir, "LateNight.2021.03.15.1080p.mkv")
	createTestFile(t, existing, 1024*1024)

	incoming := filepath.Join(sourceDir, "Late.Night.Show.2021.03.15.720p.mkv")
	createTestFile(t, incoming, 1024)

	org := setupTVOrganizer(t)
	result, err := org.OrganizeTVEpisode(incoming, libraryDir)
	require.NoError(t, err)
	assert.True(t, result.Skipped, "lower-quality date-based duplicate must be skipped; SkipReason=%q err=%v", result.SkipReason, result.Error)
	assert.FileExists(t, existing, "existing date-based episode must not be removed by dedup")
	// l2: be explicit about which file remains after dedup.  The newer,
	// lower-quality incoming source must be left in place (not moved) and the
	// older, higher-quality existing file must continue to live at its target.
	assert.FileExists(t, incoming, "skipped incoming source must not be moved or deleted")
}

func TestOrganizeTVSeasonPackAuto_ImportsEpisodeFiles(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	packDir := filepath.Join(sourceDir, "Supergirl.S03.1080p.BluRay.x264-GROUP")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	ep1 := filepath.Join(packDir, "Supergirl.S03E01.1080p.BluRay.x264-GROUP.mkv")
	ep2 := filepath.Join(packDir, "Supergirl.S03E02.1080p.BluRay.x264-GROUP.mkv")
	createTestFile(t, ep1, 1024)
	createTestFile(t, ep2, 1024)

	org, err := NewOrganizer([]string{libraryDir}, WithBackend(transfer.BackendNative))
	require.NoError(t, err)

	result, err := org.OrganizeTVSeasonPackAuto(packDir, func(p string) (int64, error) {
		info, err := os.Stat(p)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Len(t, result.Imported, 2)
	assert.Empty(t, result.Unresolved)
	assert.FileExists(t, filepath.Join(libraryDir, "Supergirl", "Season 03", "Supergirl S03E01.mkv"))
	assert.FileExists(t, filepath.Join(libraryDir, "Supergirl", "Season 03", "Supergirl S03E02.mkv"))
	assert.NoFileExists(t, ep1)
	assert.NoFileExists(t, ep2)
}

func TestOrganizeTVSeasonPackAuto_RollsBackImportedFilesOnPartialFailure(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	packDir := filepath.Join(sourceDir, "Supergirl.S03.1080p.BluRay.x264-GROUP")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	ep1 := filepath.Join(packDir, "Supergirl.S03E01.1080p.BluRay.x264-GROUP.mkv")
	ep2 := filepath.Join(packDir, "Supergirl.S03E02.1080p.BluRay.x264-GROUP.mkv")
	createTestFile(t, ep1, 1024)
	createTestFile(t, ep2, 1024)

	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(&failAfterFirstMoveTransferer{}))
	require.NoError(t, err)

	result, err := org.OrganizeTVSeasonPackAuto(packDir, func(p string) (int64, error) {
		info, err := os.Stat(p)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Error(t, result.Error)
	assert.Equal(t, "season_pack_partial_failure", result.SkipReason)
	assert.FileExists(t, ep1, "first imported file should be rolled back to source")
	assert.FileExists(t, ep2, "failed file should remain at source")
	assert.NoFileExists(t, filepath.Join(libraryDir, "Supergirl", "Season 03", "Supergirl S03E01.mkv"))
}

func TestOrganizeTVSeasonPackAuto_SkipsUnresolvedSeasonOnlyFile(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	packDir := filepath.Join(sourceDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD")
	require.NoError(t, os.MkdirAll(packDir, 0755))
	seasonOnly := filepath.Join(packDir, "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv")
	createTestFile(t, seasonOnly, 1024)

	org, err := NewOrganizer([]string{libraryDir}, WithBackend(transfer.BackendNative))
	require.NoError(t, err)

	result, err := org.OrganizeTVSeasonPackAuto(packDir, func(p string) (int64, error) {
		info, err := os.Stat(p)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.True(t, result.Skipped)
	assert.Equal(t, "season_pack_unresolved", result.SkipReason)
	assert.Len(t, result.Unresolved, 1)
	assert.Empty(t, result.Imported)
	assert.FileExists(t, seasonOnly)
}

// ---------------------------------------------------------------------------
// Task 3.3: stem-matching subtitle copy tests
// ---------------------------------------------------------------------------

// TestCopySubtitles_OnlyStemMatching verifies that copySubtitles copies only
// subtitle files whose stem matches the video file stem (after stripping one
// language/flag suffix such as .eng, .en, .forced, .sdh).
func TestCopySubtitles_OnlyStemMatching(t *testing.T) {
	sourceDir, targetDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create subtitle files in the source directory.
	subtitles := []struct {
		name   string
		copied bool
	}{
		{"Show.S01E01.srt", true},        // exact stem match
		{"Show.S01E01.eng.srt", true},    // stem match after stripping language suffix
		{"Show.S01E01.en.srt", true},     // stem match after stripping short lang suffix
		{"Show.S01E01.forced.srt", true}, // stem match after stripping flag suffix
		{"OtherShow.S01E01.srt", false},  // different show — must not be copied
		{"random.srt", false},            // no relation to video — must not be copied
	}

	var subFiles []analyzer.FileInfo
	for _, s := range subtitles {
		p := filepath.Join(sourceDir, s.name)
		require.NoError(t, os.WriteFile(p, []byte("sub"), 0644))
		subFiles = append(subFiles, analyzer.FileInfo{Path: p, Name: s.name})
	}

	// Build a minimal FolderAnalysis with the matching video file.
	analysis := &analyzer.FolderAnalysis{
		MainMediaFile: &analyzer.FileInfo{
			Path: filepath.Join(sourceDir, "Show.S01E01.mkv"),
			Name: "Show.S01E01.mkv",
		},
		SubtitleFiles: subFiles,
	}

	org, err := NewOrganizer([]string{targetDir}, WithBackend(transfer.BackendNative))
	require.NoError(t, err)

	copied := org.copySubtitles(analysis, targetDir)

	for _, s := range subtitles {
		_, err := os.Stat(filepath.Join(targetDir, s.name))
		if s.copied {
			assert.NoError(t, err, "%s should have been copied", s.name)
		} else {
			assert.True(t, os.IsNotExist(err), "%s should NOT have been copied", s.name)
		}
	}

	// Verify return value lists only the copied names.
	copiedSet := make(map[string]bool, len(copied))
	for _, n := range copied {
		copiedSet[n] = true
	}
	for _, s := range subtitles {
		if s.copied {
			assert.True(t, copiedSet[s.name], "%s should be in copied list", s.name)
		} else {
			assert.False(t, copiedSet[s.name], "%s should not be in copied list", s.name)
		}
	}
}

// alwaysFailTransferer simulates the duplicate-inotify-event race: the first
// event's transfer already moved the source, so this one fails at the backend.
type alwaysFailTransferer struct{}

func (t *alwaysFailTransferer) Move(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	return &transfer.TransferResult{}, fmt.Errorf("all backends failed: rsync: source file not found")
}

func (t *alwaysFailTransferer) Copy(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	return &transfer.TransferResult{}, fmt.Errorf("all backends failed: rsync: source file not found")
}

func (t *alwaysFailTransferer) CanResume() bool { return false }
func (t *alwaysFailTransferer) Name() string    { return "always-fail" }

func TestOrganizeTVDuplicateEventAlreadyOrganized(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Source was already moved by the first event: it does not exist on disk.
	testFile := filepath.Join(sourceDir, "Show.2020.S01E01.1080p.BluRay.mkv")

	// Target already organized by the first event.
	seasonDir := filepath.Join(libraryDir, "Show (2020)", "Season 01")
	require.NoError(t, os.MkdirAll(seasonDir, 0755))
	createTestFile(t, filepath.Join(seasonDir, "Show (2020) S01E01.mkv"), 10)

	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(&alwaysFailTransferer{}))
	require.NoError(t, err)

	result, err := org.OrganizeTVEpisode(testFile, libraryDir)
	require.NoError(t, err)
	assert.True(t, result.Skipped, "duplicate event should be a skip, not a failure")
	assert.Equal(t, "already_organized", result.SkipReason)
	assert.Nil(t, result.Error)
}

func TestOrganizeMovieDuplicateEventAlreadyOrganized(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	testFile := filepath.Join(sourceDir, "The.Matrix.1999.1080p.BluRay.mkv")

	movieDir := filepath.Join(libraryDir, "The Matrix (1999)")
	require.NoError(t, os.MkdirAll(movieDir, 0755))
	createTestFile(t, filepath.Join(movieDir, "The Matrix (1999).mkv"), 10)

	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(&alwaysFailTransferer{}))
	require.NoError(t, err)

	result, err := org.OrganizeMovie(testFile, libraryDir)
	require.NoError(t, err)
	assert.True(t, result.Skipped, "duplicate event should be a skip, not a failure")
	assert.Equal(t, "already_organized", result.SkipReason)
	assert.Nil(t, result.Error)
}

func TestOrganizeSeasonPackExtrasRoutedToShowExtrasDir(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Show already exists in the library.
	showDir := filepath.Join(libraryDir, "The Last Ship")
	require.NoError(t, os.MkdirAll(showDir, 0755))

	releaseDir := filepath.Join(sourceDir, "The.Last.Ship.S02.BONUS.2015.BluRay.1080p.AC3.x264-MTeam")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))
	videoFile := filepath.Join(releaseDir, "The.Last.Ship.S02.BONUS.2015.BluRay.1080p.AC3.x264-MTeam.mkv")
	createTestFile(t, videoFile, 1024*1024)

	org, err := NewOrganizer([]string{libraryDir})
	require.NoError(t, err)

	getFileSize := func(path string) (int64, error) {
		st, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		return st.Size(), nil
	}

	result, err := org.OrganizeTVSeasonPackAuto(releaseDir, getFileSize)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success, "extras release with matching show should organize")
	assert.False(t, result.Skipped)
	assert.Empty(t, result.Unresolved)

	expected := filepath.Join(showDir, "extras", "The Last Ship S02 BONUS.mkv")
	assert.FileExists(t, expected, "video should land in the show's extras folder")
	assert.NoFileExists(t, videoFile, "source should be moved")
}

func TestOrganizeSeasonPackExtrasUnresolvedWhenShowMissing(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	releaseDir := filepath.Join(sourceDir, "Absent.Show.S01.EXTRAS.1080p.BluRay.x264-GRP")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))
	videoFile := filepath.Join(releaseDir, "Absent.Show.S01.EXTRAS.1080p.BluRay.x264-GRP.mkv")
	createTestFile(t, videoFile, 1024*1024)

	org, err := NewOrganizer([]string{libraryDir})
	require.NoError(t, err)

	getFileSize := func(path string) (int64, error) {
		st, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		return st.Size(), nil
	}

	result, err := org.OrganizeTVSeasonPackAuto(releaseDir, getFileSize)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Skipped)
	assert.Equal(t, "extras_unresolved", result.SkipReason)
	assert.FileExists(t, videoFile, "unresolved extras must not be moved")
}

func TestOrganizeSeasonPackExtrasDoesNotOverwriteExistingTarget(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	extrasDir := filepath.Join(libraryDir, "Show", "extras")
	require.NoError(t, os.MkdirAll(extrasDir, 0755))
	target := filepath.Join(extrasDir, "Show S01 EXTRAS.mkv")
	require.NoError(t, os.WriteFile(target, []byte("keep me"), 0644))

	releaseDir := filepath.Join(sourceDir, "Show.S01.EXTRAS.1080p.WEB")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))
	source := filepath.Join(releaseDir, "Show.S01.EXTRAS.1080p.WEB.mkv")
	createTestFile(t, source, 1024)

	org, err := NewOrganizer([]string{libraryDir})
	require.NoError(t, err)
	result, err := org.OrganizeTVSeasonPackAuto(releaseDir, func(path string) (int64, error) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return 0, statErr
		}
		return info.Size(), nil
	})

	require.NoError(t, err)
	assert.True(t, result.Skipped)
	assert.Equal(t, "extras_unresolved", result.SkipReason)
	assert.FileExists(t, source)
	contents, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, "keep me", string(contents))
}

func TestOrganizeSeasonPackExtrasMatchesShowYear(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	require.NoError(t, os.MkdirAll(filepath.Join(libraryDir, "Show (1990)"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(libraryDir, "Show (2020)"), 0755))
	releaseDir := filepath.Join(sourceDir, "Show.S01.EXTRAS.2020.1080p.WEB")
	require.NoError(t, os.MkdirAll(releaseDir, 0755))
	source := filepath.Join(releaseDir, "Show.S01.EXTRAS.2020.1080p.WEB.mkv")
	createTestFile(t, source, 1024)

	org, err := NewOrganizer([]string{libraryDir})
	require.NoError(t, err)
	result, err := org.OrganizeTVSeasonPackAuto(releaseDir, func(path string) (int64, error) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return 0, statErr
		}
		return info.Size(), nil
	})

	require.NoError(t, err)
	require.True(t, result.Success)
	assert.FileExists(t, filepath.Join(libraryDir, "Show (2020)", "extras", "Show S01 EXTRAS.mkv"))
	assert.NoDirExists(t, filepath.Join(libraryDir, "Show (1990)", "extras"))
}

// A retained source (copy or seed policy) is re-seen by every periodic scan.
// Canonical naming strips quality tokens from the target, so re-parsing it
// yields a weaker quality than the source and the source looks like a perpetual
// upgrade. Guard the second scan explicitly.
func TestOrganizerRetainedSourceIsNotReimportedOnSecondScan(t *testing.T) {
	for _, tt := range []struct {
		name string
		mode SourceTransferMode
	}{
		{name: "seed", mode: TransferSeed},
		{name: "copy", mode: TransferCopy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			sourcePath := filepath.Join(root, "incoming", "Loop.Show.2026.S01E01.1080p.WEB-DL.x265-GROUP.mkv")
			libraryDir := filepath.Join(root, "library")
			createTestFile(t, sourcePath, 4096)

			transferer := &recordingTransferer{}
			options := []func(*Organizer){
				WithTransferer(transferer),
				WithTransferModeResolver(func(string) SourceTransferMode { return tt.mode }),
			}
			var db *database.MediaDB
			if tt.mode == TransferCopy {
				var cleanup func()
				db, cleanup = setupTestDB(t)
				defer cleanup()
				options = append(options, WithDatabase(db))
			}
			org, err := NewOrganizer([]string{libraryDir}, options...)
			require.NoError(t, err)

			info := naming.TVShowInfo{Title: "Loop Show", Year: "2026", Season: 1, Episode: 1}

			first, err := org.OrganizeTVWithParsed(sourcePath, libraryDir, info)
			require.NoError(t, err)
			require.True(t, first.Success, "first import failed: %v", first.Error)
			transfersAfterFirst := transferer.moves + transferer.copies
			if db != nil {
				// The daemon records the successful source after the organizer
				// returns; seed that evidence before simulating the next scan.
				sourceInfo, statErr := os.Stat(sourcePath)
				require.NoError(t, statErr)
				modifiedAt := sourceInfo.ModTime()
				_, insertErr := db.InsertDecision(database.ParseDecision{
					SourcePath: sourcePath, SourceFilename: filepath.Base(sourcePath),
					SourceSize: sourceInfo.Size(), SourceModifiedAt: &modifiedAt,
					EventAt: time.Now().UTC(), TargetPath: first.TargetPath,
					OrganizeOutcome: "success",
				})
				require.NoError(t, insertErr)
			}

			second, err := org.OrganizeTVWithParsed(sourcePath, libraryDir, info)
			require.NoError(t, err)
			assert.False(t, second.Success, "second scan re-imported a retained source")
			assert.True(t, second.Skipped, "second scan should skip, got %+v", second)
			assert.Equal(t, transfersAfterFirst, transferer.moves+transferer.copies,
				"retained source was transferred a second time")
			assert.FileExists(t, sourcePath, "source must remain retained")
		})
	}
}

func TestOrganizerImportsSameSizeQualityUpgradeFromDifferentSource(t *testing.T) {
	root := t.TempDir()
	oldSource := filepath.Join(root, "incoming-old", "Upgrade.Movie.2026.720p.WEB-DL.mkv")
	newSource := filepath.Join(root, "incoming-new", "Upgrade.Movie.2026.1080p.BluRay.mkv")
	libraryDir := filepath.Join(root, "library")
	for path, contents := range map[string]string{oldSource: "old!", newSource: "new!"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	}
	db, err := database.OpenPath(filepath.Join(root, "media.db"))
	require.NoError(t, err)
	defer db.Close()
	oldDecisionID := insertPendingQualityDecision(t, db, oldSource)
	newDecisionID := insertPendingQualityDecision(t, db, newSource)

	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir},
		WithDatabase(db),
		WithTransferer(transferer),
		WithTransferModeResolver(func(string) SourceTransferMode { return TransferCopy }),
	)
	require.NoError(t, err)
	movie := naming.MovieInfo{Title: "Upgrade Movie", Year: "2026"}

	first, err := org.OrganizeMovieWithParsedFinalized(oldSource, libraryDir, movie, func(result *OrganizationResult, err error) {
		persistTestOrganizeResult(t, db, oldDecisionID, result, err)
	})
	require.NoError(t, err)
	require.True(t, first.Success, "initial import failed: %v", first.Error)

	upgrade, err := org.OrganizeMovieWithParsedFinalized(newSource, libraryDir, movie, func(result *OrganizationResult, err error) {
		persistTestOrganizeResult(t, db, newDecisionID, result, err)
	})
	require.NoError(t, err)
	if !upgrade.Success {
		t.Fatalf("same-size quality upgrade was skipped: %+v", upgrade)
	}
	assert.Equal(t, 2, transferer.copies, "the second release must be transferred")
	contents, err := os.ReadFile(upgrade.TargetPath)
	require.NoError(t, err)
	assert.Equal(t, "new!", string(contents), "the library should contain the higher-quality release")
}

func TestOrganizerUsesSuccessfulImportQualityForCanonicalTargets(t *testing.T) {
	for _, tc := range []struct {
		name           string
		sourceFilename string
		existingSource string
		mediaType      string
	}{
		{
			name:           "movie",
			sourceFilename: "Quality.Movie.2026.720p.HDTV.mkv",
			existingSource: "Quality.Movie.2026.1080p.WEB-DL.mkv",
			mediaType:      "movie",
		},
		{
			name:           "single episode",
			sourceFilename: "Quality.Show.2026.S01E02.720p.HDTV.mkv",
			existingSource: "Quality.Show.2026.S01E02.1080p.WEB-DL.mkv",
			mediaType:      "tv",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			db, err := database.OpenPath(filepath.Join(root, "media.db"))
			require.NoError(t, err)
			defer db.Close()

			libraryDir := filepath.Join(root, "library")
			var targetPath, sourcePath string
			if tc.mediaType == "movie" {
				targetDir := filepath.Join(libraryDir, "Quality Movie (2026)")
				targetPath = filepath.Join(targetDir, "Quality Movie (2026).mkv")
				sourcePath = filepath.Join(root, "incoming", tc.sourceFilename)
			} else {
				targetDir := filepath.Join(libraryDir, "Quality Show (2026)", "Season 01")
				targetPath = filepath.Join(targetDir, "Quality Show (2026) S01E02.mkv")
				sourcePath = filepath.Join(root, "incoming", tc.sourceFilename)
			}
			createTestFile(t, targetPath, 4)
			createTestFile(t, sourcePath, 3)
			recordSuccessfulImport(t, db, targetPath, tc.existingSource, tc.mediaType)

			transferer := &recordingTransferer{}
			org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
			require.NoError(t, err)

			var result *OrganizationResult
			if tc.mediaType == "movie" {
				result, err = org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Quality Movie", Year: "2026"})
			} else {
				result, err = org.OrganizeTVWithParsed(sourcePath, libraryDir, naming.TVShowInfo{Title: "Quality Show", Year: "2026", Season: 1, Episode: 2})
			}
			require.NoError(t, err)
			assert.True(t, result.Skipped, "lower-quality import should be skipped: %+v", result)
			assert.Contains(t, result.SkipReason, "equal or better quality")
			assert.Zero(t, transferer.moves+transferer.copies)
			assert.FileExists(t, sourcePath)
			contents, err := os.ReadFile(targetPath)
			require.NoError(t, err)
			assert.Equal(t, []byte{0, 0, 0, 0}, contents)
		})
	}
}

func TestOrganizerDoesNotReplaceCanonicalTargetWithoutQualityEvidence(t *testing.T) {
	root := t.TempDir()
	libraryDir := filepath.Join(root, "library")
	targetPath := filepath.Join(libraryDir, "Unknown Movie (2026)", "Unknown Movie (2026).mkv")
	sourcePath := filepath.Join(root, "incoming", "Unknown.Movie.2026.2160p.REMUX.mkv")
	createTestFile(t, targetPath, 4)
	createTestFile(t, sourcePath, 3)

	db, err := database.OpenPath(filepath.Join(root, "media.db"))
	require.NoError(t, err)
	defer db.Close()

	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
	require.NoError(t, err)
	result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Unknown Movie", Year: "2026"})
	require.NoError(t, err)
	assert.True(t, result.Skipped, "unknown existing quality should fail closed: %+v", result)
	assert.Contains(t, result.SkipReason, "quality evidence is unavailable")
	assert.Contains(t, result.SkipReason, "organize --force")
	assert.Zero(t, transferer.moves+transferer.copies)
	assert.FileExists(t, sourcePath)
	contents, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, []byte{0, 0, 0, 0}, contents)
}

func TestTVSeasonOrganizationLockKeyCoversEpisodeRanges(t *testing.T) {
	base := naming.TVShowInfo{Title: "Quality Show", Year: "2026", Season: 1, Episode: 1}
	rangeImport := base
	rangeImport.EpisodeEnd = 2
	otherEpisode := base
	otherEpisode.Episode = 2

	key := tvSeasonOrganizationLockKey(base)
	assert.Equal(t, key, tvSeasonOrganizationLockKey(rangeImport), "range and single-episode imports in one season must serialize")
	assert.Equal(t, key, tvSeasonOrganizationLockKey(otherEpisode), "episodes in one season must serialize")
	punctuationVariant := base
	punctuationVariant.Title = "Quality-Show"
	assert.Equal(t, key, tvSeasonOrganizationLockKey(punctuationVariant), "directory-matching title variants must serialize")

	otherSeason := base
	otherSeason.Season = 2
	assert.NotEqual(t, key, tvSeasonOrganizationLockKey(otherSeason))
	otherShow := base
	otherShow.Title = "Different Show"
	assert.NotEqual(t, key, tvSeasonOrganizationLockKey(otherShow))
}

func TestConcurrentMovieImportsWaitForDecisionUpdateBeforeQualityCheck(t *testing.T) {
	root := t.TempDir()
	libraryDir := filepath.Join(root, "library")
	highSource := filepath.Join(root, "incoming-high", "Concurrent.Movie.2026.1080p.WEB-DL.mkv")
	lowSource := filepath.Join(root, "incoming-low", "Concurrent.Movie.2026.720p.HDTV.mkv")
	for path, content := range map[string]string{highSource: "high", lowSource: "low!"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	db, err := database.OpenPath(filepath.Join(root, "media.db"))
	require.NoError(t, err)
	defer db.Close()
	highID := insertPendingQualityDecision(t, db, highSource)
	lowID := insertPendingQualityDecision(t, db, lowSource)

	highOrg, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(&recordingTransferer{}),
		WithTransferModeResolver(func(string) SourceTransferMode { return TransferCopy }))
	require.NoError(t, err)
	lowOrg, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(&recordingTransferer{}),
		WithTransferModeResolver(func(string) SourceTransferMode { return TransferCopy }))
	require.NoError(t, err)
	movie := naming.MovieInfo{Title: "Concurrent Movie", Year: "2026"}
	type outcome struct {
		result *OrganizationResult
		err    error
	}
	highDone := make(chan outcome, 1)
	lowDone := make(chan outcome, 1)
	highFinalizing := make(chan struct{})
	releaseHigh := make(chan struct{})
	releasedHigh := false
	defer func() {
		if !releasedHigh {
			close(releaseHigh)
		}
	}()

	go func() {
		result, err := highOrg.OrganizeMovieWithParsedFinalized(highSource, libraryDir, movie, func(result *OrganizationResult, err error) {
			close(highFinalizing)
			<-releaseHigh
			persistTestOrganizeResult(t, db, highID, result, err)
		})
		highDone <- outcome{result: result, err: err}
	}()
	<-highFinalizing

	go func() {
		result, err := lowOrg.OrganizeMovieWithParsedFinalized(lowSource, libraryDir, movie, func(result *OrganizationResult, err error) {
			persistTestOrganizeResult(t, db, lowID, result, err)
		})
		lowDone <- outcome{result: result, err: err}
	}()

	movieDir := filepath.Join(libraryDir, "Concurrent Movie (2026)")
	absoluteMovieDir, err := filepath.Abs(movieDir)
	require.NoError(t, err)
	waitForOrganizationTargetRefs(t, absoluteMovieDir, 2)
	select {
	case <-lowDone:
		t.Fatal("second import finished before the first decision update released the target lock")
	default:
	}
	close(releaseHigh)
	releasedHigh = true

	high := <-highDone
	require.NoError(t, high.err)
	require.True(t, high.result.Success)
	low := <-lowDone
	require.NoError(t, low.err)
	assert.True(t, low.result.Skipped, "lower-quality import should observe first import: %+v", low.result)

	targetPath := filepath.Join(movieDir, "Concurrent Movie (2026).mkv")
	contents, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, "high", string(contents))
}

func waitForOrganizationTargetRefs(t *testing.T, key string, refs int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if organizationTargetLockReferenceCount(key) >= refs {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("target %q did not reach %d lock references", key, refs)
}

func organizationTargetLockReferenceCount(key string) int {
	organizationTargetLocks.Lock()
	defer organizationTargetLocks.Unlock()
	if lock := organizationTargetLocks.locks[key]; lock != nil {
		return lock.refs
	}
	return 0
}

func TestConcurrentOverlappingTVImportsSerializeBySeason(t *testing.T) {
	root := t.TempDir()
	libraryDir := filepath.Join(root, "library")
	rangeSource := filepath.Join(root, "incoming-range", "Quality.Show.2026.S01E01-E02.1080p.BluRay.mkv")
	episodeSource := filepath.Join(root, "incoming-episode", "Quality.Show.2026.S01E02.720p.HDTV.mkv")
	for path, content := range map[string]string{rangeSource: "high", episodeSource: "low!"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	db, err := database.OpenPath(filepath.Join(root, "media.db"))
	require.NoError(t, err)
	defer db.Close()
	rangeID := insertPendingQualityDecision(t, db, rangeSource)
	episodeID := insertPendingQualityDecision(t, db, episodeSource)

	transferer := &blockingConcurrentTransferer{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
		releaseSecond: make(chan struct{}),
	}
	firstReleased := false
	secondReleased := false
	defer func() {
		if !firstReleased {
			close(transferer.releaseFirst)
		}
		if !secondReleased {
			close(transferer.releaseSecond)
		}
	}()
	org, err := NewOrganizer([]string{libraryDir}, WithDatabase(db), WithTransferer(transferer))
	require.NoError(t, err)

	type outcome struct {
		result *OrganizationResult
		err    error
	}
	rangeDone := make(chan outcome, 1)
	episodeDone := make(chan outcome, 1)
	go func() {
		result, err := org.OrganizeTVWithParsedFinalized(rangeSource, libraryDir, naming.TVShowInfo{
			Title: "Quality Show", Year: "2026", Season: 1, Episode: 1, EpisodeEnd: 2,
		}, func(result *OrganizationResult, err error) {
			persistTestOrganizeResult(t, db, rangeID, result, err)
		})
		rangeDone <- outcome{result: result, err: err}
	}()
	select {
	case <-transferer.firstStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("range import did not reach transfer")
	}

	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		result, err := org.OrganizeTVWithParsedAutoFinalized(episodeSource, naming.TVShowInfo{
			Title: "Quality Show", Year: "2026", Season: 1, Episode: 2,
		}, func(path string) (int64, error) {
			info, err := os.Stat(path)
			if err != nil {
				return 0, err
			}
			return info.Size(), nil
		}, func(result *OrganizationResult, err error) {
			persistTestOrganizeResult(t, db, episodeID, result, err)
		})
		episodeDone <- outcome{result: result, err: err}
	}()
	<-secondStarted

	seasonLockKey := tvSeasonOrganizationLockKey(naming.TVShowInfo{Title: "Quality Show", Season: 1})
	lockRequested := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if organizationTargetLockReferenceCount(seasonLockKey) >= 2 {
			lockRequested = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	secondTransferredBeforeRangePublished := false
	select {
	case <-transferer.secondStarted:
		secondTransferredBeforeRangePublished = true
	default:
	}
	close(transferer.releaseFirst)
	firstReleased = true

	var rangeResult outcome
	select {
	case rangeResult = <-rangeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("range import did not finish after transfer was released")
	}
	if secondTransferredBeforeRangePublished {
		close(transferer.releaseSecond)
		secondReleased = true
	}

	var episodeResult outcome
	select {
	case episodeResult = <-episodeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("episode import did not finish")
	}
	require.NoError(t, rangeResult.err)
	require.True(t, rangeResult.result.Success, "range import failed: %+v", rangeResult.result)
	require.NoError(t, episodeResult.err)
	assert.True(t, lockRequested, "overlapping imports should request the same show-season lock")
	assert.True(t, episodeResult.result.Skipped, "overlapping lower-quality episode should observe the published range: %+v", episodeResult.result)

	seasonDir := filepath.Dir(rangeResult.result.TargetPath)
	entries, err := os.ReadDir(seasonDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "overlapping import must not leave a second episode file")
	assert.FileExists(t, rangeResult.result.TargetPath)
}

func TestConcurrentTVImportWaitsForSeasonPackDecisionUpdate(t *testing.T) {
	root := t.TempDir()
	libraryDir := filepath.Join(root, "library")
	packDir := filepath.Join(root, "incoming", "Supergirl.S03.1080p.BluRay.x264-GROUP")
	packSource := filepath.Join(packDir, "Supergirl.S03E01.1080p.BluRay.x264-GROUP.mkv")
	competingSource := filepath.Join(root, "other", "Supergirl.S03E01.720p.HDTV.mkv")
	createTestFile(t, packSource, 4096)
	createTestFile(t, competingSource, 1024)

	org, err := NewOrganizer([]string{libraryDir}, WithTransferer(&recordingTransferer{}))
	require.NoError(t, err)

	type packOutcome struct {
		result *SeasonPackResult
		err    error
	}
	seasonLockKey := tvSeasonOrganizationLockKey(naming.TVShowInfo{Title: "Supergirl", Season: 3})
	type finalizerState struct {
		result *SeasonPackResult
		err    error
		refs   int
	}
	packFinalizing := make(chan finalizerState, 1)
	releasePack := make(chan struct{})
	packReleased := false
	defer func() {
		if !packReleased {
			close(releasePack)
		}
	}()
	packDone := make(chan packOutcome, 1)
	getFileSize := func(path string) (int64, error) {
		info, err := os.Stat(path)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	}
	go func() {
		result, err := org.OrganizeTVSeasonPackAutoFinalized(packDir, getFileSize, func(finalResult *SeasonPackResult, finalizeErr error) {
			packFinalizing <- finalizerState{
				result: finalResult,
				err:    finalizeErr,
				refs:   organizationTargetLockReferenceCount(seasonLockKey),
			}
			<-releasePack
		})
		packDone <- packOutcome{result: result, err: err}
	}()
	select {
	case finalized := <-packFinalizing:
		require.NoError(t, finalized.err)
		require.NotNil(t, finalized.result)
		require.True(t, finalized.result.Success)
		require.GreaterOrEqual(t, finalized.refs, 1, "decision finalization should run while the season lock is held")
	case <-time.After(5 * time.Second):
		t.Fatal("season-pack decision finalizer was not called")
	}

	type episodeOutcome struct {
		result *OrganizationResult
		err    error
	}
	episodeDone := make(chan episodeOutcome, 1)
	go func() {
		result, err := org.OrganizeTVWithParsed(competingSource, libraryDir, naming.TVShowInfo{
			Title: "Supergirl", Season: 3, Episode: 1,
		})
		episodeDone <- episodeOutcome{result: result, err: err}
	}()
	waitForOrganizationTargetRefs(t, seasonLockKey, 2)
	select {
	case <-episodeDone:
		t.Fatal("overlapping episode import finished before season-pack decision finalization")
	default:
	}

	close(releasePack)
	packReleased = true
	pack := <-packDone
	require.NoError(t, pack.err)
	require.True(t, pack.result.Success)
	episode := <-episodeDone
	require.NoError(t, episode.err)
	assert.True(t, episode.result.Skipped, "overlapping lower-quality import should observe the completed pack: %+v", episode.result)
	assert.FileExists(t, filepath.Join(libraryDir, "Supergirl", "Season 03", "Supergirl S03E01.mkv"))
}

func TestOrganizerRetainedMovieSourceIsNotReimportedOnSecondScan(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "incoming", "Loop.Movie.2026.1080p.BluRay.x265-GROUP.mkv")
	libraryDir := filepath.Join(root, "library")
	createTestFile(t, sourcePath, 4096)

	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir},
		WithTransferer(transferer),
		WithTransferModeResolver(func(string) SourceTransferMode { return TransferSeed }),
	)
	require.NoError(t, err)

	info := naming.MovieInfo{Title: "Loop Movie", Year: "2026"}

	first, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, info)
	require.NoError(t, err)
	require.True(t, first.Success, "first import failed: %v", first.Error)
	transfersAfterFirst := transferer.moves + transferer.copies

	second, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, info)
	require.NoError(t, err)
	assert.True(t, second.Skipped, "second scan should skip, got %+v", second)
	assert.Equal(t, transfersAfterFirst, transferer.moves+transferer.copies,
		"retained movie source was transferred a second time")
}

func TestOrganizerRetainedSourceRangeIsNotReimportedOnSecondScan(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "incoming", "Loop.Show.2026.S01E07-E08.1080p.BluRay.mkv")
	libraryDir := filepath.Join(root, "library")
	createTestFile(t, sourcePath, 4096)

	transferer := &recordingTransferer{}
	org, err := NewOrganizer([]string{libraryDir},
		WithTransferer(transferer),
		WithTransferModeResolver(func(string) SourceTransferMode { return TransferSeed }),
	)
	require.NoError(t, err)
	info := naming.TVShowInfo{Title: "Loop Show", Year: "2026", Season: 1, Episode: 7, EpisodeEnd: 8}

	first, err := org.OrganizeTVWithParsed(sourcePath, libraryDir, info)
	require.NoError(t, err)
	require.True(t, first.Success, "first range import failed: %v", first.Error)
	transfersAfterFirst := transferer.moves + transferer.copies

	second, err := org.OrganizeTVWithParsed(sourcePath, libraryDir, info)
	require.NoError(t, err)
	assert.True(t, second.Skipped, "second scan should skip, got %+v", second)
	assert.Equal(t, "already imported from this source", second.SkipReason)
	assert.Equal(t, transfersAfterFirst, transferer.moves+transferer.copies,
		"retained range source was transferred a second time")
	assert.FileExists(t, sourcePath, "seeding source must remain retained")
	assert.FileExists(t, first.TargetPath)
}
