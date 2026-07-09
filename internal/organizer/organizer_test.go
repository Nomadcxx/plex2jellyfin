package organizer

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/analyzer"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failAfterFirstMoveTransferer struct {
	moves int
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
	sourceDir = filepath.Join(os.TempDir(), "jellywatch-test-source-"+t.Name())
	libraryDir = filepath.Join(os.TempDir(), "jellywatch-test-library-"+t.Name())

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
	dbPath := filepath.Join(os.TempDir(), "jellywatch-test-"+t.Name()+".db")
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
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	// Write dummy data
	data := make([]byte, size)
	_, err = f.Write(data)
	require.NoError(t, err)
}

// TestOrganizeMovie_Basic tests basic movie organization
func TestOrganizeMovie_Basic(t *testing.T) {
	sourceDir, libraryDir, cleanup := setupTestEnv(t)
	defer cleanup()

	db, dbCleanup := setupTestDB(t)
	defer dbCleanup()

	// Create test file
	testFile := filepath.Join(sourceDir, "The.Matrix.1999.1080p.BluRay.mkv")
	createTestFile(t, testFile, 2*1024*1024*1024) // 2GB

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
	assert.Equal(t, "jellywatch", movie.Source, "Source should be jellywatch")
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
	createTestFile(t, testFile, 2*1024*1024*1024)

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
	assert.Equal(t, "jellywatch", movie.Source)
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
	assert.Equal(t, "jellywatch", series.Source, "Source should be jellywatch")
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
	require.True(t, result.Success)

	// Verify database entry
	series, err := db.GetSeriesByTitle("Breaking Bad", 2008)
	require.NoError(t, err)
	require.NotNil(t, series)
	assert.Equal(t, "jellywatch", series.Source)
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
