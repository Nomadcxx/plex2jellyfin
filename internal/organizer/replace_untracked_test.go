package organizer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrganizerReplacesUntrackedUpgradeOnExistingFile(t *testing.T) {
	t.Run("database", func(t *testing.T) {
		root := t.TempDir()
		libraryDir := filepath.Join(root, "library")
		targetPath := filepath.Join(libraryDir, "Upgrade Movie (2026)", "Upgrade Movie (2026).mkv")
		sourcePath := filepath.Join(root, "incoming", "Upgrade.Movie.2026.1080p.BluRay.mkv")
		require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
		require.NoError(t, os.WriteFile(sourcePath, []byte("new-release"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Dir(targetPath), 0o755))
		require.NoError(t, os.WriteFile(targetPath, []byte("old-release"), 0o644))

		db, err := database.OpenPath(filepath.Join(root, "media.db"))
		require.NoError(t, err)
		defer db.Close()
		recordSuccessfulImport(t, db, targetPath, "Upgrade.Movie.2026.720p.WEB-DL.mkv", "movie")
		assertNoMediaFile(t, db, sourcePath)
		assertNoMediaFile(t, db, targetPath)

		org, err := NewOrganizer([]string{libraryDir},
			WithDatabase(db),
			WithTransferer(transfer.NewNativeTransferer(1024)),
		)
		require.NoError(t, err)
		result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Upgrade Movie", Year: "2026"})
		require.NoError(t, err)
		require.True(t, result.Success, "untracked upgrade failed: %+v", result)
		require.NoFileExists(t, sourcePath)
		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, "new-release", string(got))
		assertOnlyNames(t, filepath.Dir(targetPath), "Upgrade Movie (2026).mkv")
		assertNoReservations(t, db)

		moved, err := db.GetMediaFile(targetPath)
		require.NoError(t, err)
		require.NotNil(t, moved)
		assert.Equal(t, int64(len("new-release")), moved.Size)
		assertNoMediaFile(t, db, sourcePath)
	})

	t.Run("keeps destination identity", func(t *testing.T) {
		root := t.TempDir()
		libraryDir := filepath.Join(root, "library")
		canonicalPath := filepath.Join(libraryDir, "Upgrade Movie (2026)")
		targetPath := filepath.Join(canonicalPath, "Upgrade Movie (2026).mkv")
		sourcePath := filepath.Join(root, "incoming", "Upgrade.Movie.2026.2160p.BluRay.mkv")
		require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
		require.NoError(t, os.WriteFile(sourcePath, []byte("better-release"), 0o644))
		require.NoError(t, os.MkdirAll(canonicalPath, 0o755))
		require.NoError(t, os.WriteFile(targetPath, []byte("old-release"), 0o644))

		db, err := database.OpenPath(filepath.Join(root, "media.db"))
		require.NoError(t, err)
		defer db.Close()
		movie := &database.Movie{Title: "Upgrade Movie", Year: 2026, CanonicalPath: canonicalPath, LibraryRoot: libraryDir, Source: "test", SourcePriority: 100}
		_, err = db.UpsertMovie(movie)
		require.NoError(t, err)
		destination := &database.MediaFile{
			Path: targetPath, Size: int64(len("old-release")), MediaType: "movie", ParentMovieID: &movie.ID,
			NormalizedTitle: "upgrademovie", Resolution: "720p", LibraryRoot: libraryDir,
		}
		require.NoError(t, db.UpsertMediaFile(destination))
		require.NoError(t, db.UpdateMovieBestFile(movie.ID, &destination.ID))
		recordSuccessfulImport(t, db, targetPath, "Upgrade.Movie.2026.720p.WEB-DL.mkv", "movie")
		assertNoMediaFile(t, db, sourcePath)

		org, err := NewOrganizer([]string{libraryDir},
			WithDatabase(db),
			WithTransferer(transfer.NewNativeTransferer(1024)),
		)
		require.NoError(t, err)
		result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Upgrade Movie", Year: "2026"})
		require.NoError(t, err)
		require.True(t, result.Success, "untracked upgrade failed: %+v", result)

		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, "better-release", string(got))
		assertOnlyNames(t, canonicalPath, "Upgrade Movie (2026).mkv")
		require.NoFileExists(t, sourcePath)
		assertNoMediaFile(t, db, sourcePath)
		assertNoReservations(t, db)

		kept, err := db.GetMediaFileByID(destination.ID)
		require.NoError(t, err)
		require.NotNil(t, kept)
		assert.Equal(t, targetPath, kept.Path)
		assert.Equal(t, int64(len("better-release")), kept.Size)
		var bestFileID int64
		require.NoError(t, db.SQL().QueryRow("SELECT best_file_id FROM movies WHERE id = ?", movie.ID).Scan(&bestFileID))
		assert.Equal(t, destination.ID, bestFileID)
		var rows int
		require.NoError(t, db.SQL().QueryRow("SELECT COUNT(*) FROM media_files").Scan(&rows))
		assert.Equal(t, 1, rows)
	})

	t.Run("without database", func(t *testing.T) {
		root := t.TempDir()
		libraryDir := filepath.Join(root, "library")
		targetPath := filepath.Join(libraryDir, "Upgrade Movie (2026)", "Upgrade Movie (2026).mkv")
		sourcePath := filepath.Join(root, "incoming", "Upgrade.Movie.2026.1080p.BluRay.mkv")
		require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
		require.NoError(t, os.WriteFile(sourcePath, []byte("new-release"), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Dir(targetPath), 0o755))
		require.NoError(t, os.WriteFile(targetPath, []byte("old-release"), 0o644))

		org, err := NewOrganizer([]string{libraryDir},
			WithForceOverwrite(true),
			WithTransferer(transfer.NewNativeTransferer(1024)),
		)
		require.NoError(t, err)
		result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Upgrade Movie", Year: "2026"})
		require.NoError(t, err)
		require.True(t, result.Success, "untracked upgrade failed: %+v", result)
		require.NoFileExists(t, sourcePath)
		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, "new-release", string(got))
		assertOnlyNames(t, filepath.Dir(targetPath), "Upgrade Movie (2026).mkv")
	})
}

func TestOrganizerRestoresUntrackedDestinationWhenUpgradeTransferFails(t *testing.T) {
	t.Run("database", func(t *testing.T) {
		assertFailedUpgradeRestoresDestination(t, true)
	})
	t.Run("without database", func(t *testing.T) {
		assertFailedUpgradeRestoresDestination(t, false)
	})
}

func assertFailedUpgradeRestoresDestination(t *testing.T, withDB bool) {
	t.Helper()
	root := t.TempDir()
	libraryDir := filepath.Join(root, "library")
	targetPath := filepath.Join(libraryDir, "Upgrade Movie (2026)", "Upgrade Movie (2026).mkv")
	sourcePath := filepath.Join(root, "incoming", "Upgrade.Movie.2026.1080p.BluRay.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(sourcePath), 0o755))
	require.NoError(t, os.WriteFile(sourcePath, []byte("new-release"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(targetPath), 0o755))
	require.NoError(t, os.WriteFile(targetPath, []byte("old-release"), 0o644))

	observer := &destinationAbsentTransferer{}
	options := []func(*Organizer){WithTransferer(observer)}
	var db *database.MediaDB
	if withDB {
		var err error
		db, err = database.OpenPath(filepath.Join(root, "media.db"))
		require.NoError(t, err)
		defer db.Close()
		recordSuccessfulImport(t, db, targetPath, "Upgrade.Movie.2026.720p.WEB-DL.mkv", "movie")
		options = append(options, WithDatabase(db))
	} else {
		options = append(options, WithForceOverwrite(true))
	}
	org, err := NewOrganizer([]string{libraryDir}, options...)
	require.NoError(t, err)
	result, err := org.OrganizeMovieWithParsed(sourcePath, libraryDir, naming.MovieInfo{Title: "Upgrade Movie", Year: "2026"})
	require.NoError(t, err)
	require.False(t, result.Success, "failed upgrade reported success: %+v", result)
	require.True(t, observer.destWasAbsent, "replacement must quarantine the existing file before transfer")

	got, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, "old-release", string(got))
	source, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	assert.Equal(t, "new-release", string(source))
	assertOnlyNames(t, filepath.Dir(targetPath), "Upgrade Movie (2026).mkv")
	if db != nil {
		assertNoReservations(t, db)
	}
}

type destinationAbsentTransferer struct {
	destWasAbsent bool
}

func (t *destinationAbsentTransferer) Move(_, dst string, _ transfer.TransferOptions) (*transfer.TransferResult, error) {
	_, err := os.Lstat(dst)
	t.destWasAbsent = errors.Is(err, os.ErrNotExist)
	fail := errors.New("injected transfer failure")
	return &transfer.TransferResult{Error: fail}, fail
}

func (t *destinationAbsentTransferer) Copy(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	return t.Move(src, dst, opts)
}

func (*destinationAbsentTransferer) CanResume() bool { return false }
func (*destinationAbsentTransferer) Name() string    { return "destination-absent" }

func assertNoMediaFile(t *testing.T, db *database.MediaDB, path string) {
	t.Helper()
	file, err := db.GetMediaFile(path)
	require.NoError(t, err)
	assert.Nil(t, file)
}

func assertNoReservations(t *testing.T, db *database.MediaDB) {
	t.Helper()
	var reservations int
	require.NoError(t, db.SQL().QueryRow("SELECT COUNT(*) FROM media_file_move_reservations").Scan(&reservations))
	assert.Zero(t, reservations)
}

func assertOnlyNames(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	assert.ElementsMatch(t, names, got)
}
