package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/logging"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/Nomadcxx/plex2jellyfin/internal/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSeedingHandler(t *testing.T, seedDir, movieLib, tvLib string) *MediaHandler {
	t.Helper()
	handler, err := NewMediaHandler(MediaHandlerConfig{
		TVLibraries:  []string{tvLib},
		MovieLibs:    []string{movieLib},
		Watch:        config.WatchConfig{SeedingMovies: []string{seedDir}},
		DeleteSource: true,
		Backend:      transfer.BackendNative,
		Logger:       logging.Nop(),
	})
	require.NoError(t, err)
	return handler
}

// A client that preallocates its download under a temporary extension must not
// be ingested until it renames to the final video name.
func TestMediaHandlerIgnoresTemporaryExtensionsUntilFinalRename(t *testing.T) {
	root := t.TempDir()
	seedDir := filepath.Join(root, "seeding-movies")
	movieLib := filepath.Join(root, "movies")
	require.NoError(t, os.MkdirAll(seedDir, 0o755))
	require.NoError(t, os.MkdirAll(movieLib, 0o755))

	handler := newSeedingHandler(t, seedDir, movieLib, filepath.Join(root, "tv"))

	for _, name := range []string{
		"Partial.Movie.2026.1080p.WEB-DL.mkv.part",
		"Partial.Movie.2026.1080p.WEB-DL.mkv.!qB",
		"Partial.Movie.2026.1080p.WEB-DL.mkv.crdownload",
	} {
		partial := filepath.Join(seedDir, name)
		require.NoError(t, os.WriteFile(partial, []byte("incomplete"), 0o644))
		require.NoError(t, handler.HandleFileEvent(watcher.FileEvent{Path: partial, Type: watcher.EventCreate}))
		assert.False(t, handler.IsMediaFile(partial), "%s must not look like media", name)

		handler.mu.Lock()
		_, pending := handler.pending[partial]
		handler.mu.Unlock()
		assert.False(t, pending, "%s must not be queued for import", name)
		require.NoError(t, os.Remove(partial))
	}

	final := filepath.Join(seedDir, "Partial.Movie.2026.1080p.WEB-DL.mkv")
	require.NoError(t, os.WriteFile(final, []byte("complete payload"), 0o644))
	handler.processFile(final)

	assert.FileExists(t, filepath.Join(movieLib, "Partial Movie (2026)", "Partial Movie (2026).mkv"))
	assert.FileExists(t, final, "seeding source must be retained")
}

// After a hardlinked import the client owns its source: renaming or deleting it
// must leave the canonical library file intact and readable.
func TestSeedingSourceRemovalLeavesCanonicalTargetIntact(t *testing.T) {
	root := t.TempDir()
	seedDir := filepath.Join(root, "seeding-movies")
	movieLib := filepath.Join(root, "movies")
	releaseDir := filepath.Join(seedDir, "Retired.Movie.2026-GROUP")
	source := filepath.Join(releaseDir, "Retired.Movie.2026.1080p.WEB-DL.mkv")
	require.NoError(t, os.MkdirAll(releaseDir, 0o755))
	require.NoError(t, os.MkdirAll(movieLib, 0o755))
	payload := []byte("seed payload that must survive the client")
	require.NoError(t, os.WriteFile(source, payload, 0o644))

	handler := newSeedingHandler(t, seedDir, movieLib, filepath.Join(root, "tv"))
	handler.processFile(source)

	target := filepath.Join(movieLib, "Retired Movie (2026)", "Retired Movie (2026).mkv")
	require.FileExists(t, target)

	// The client relocates the torrent, then finishes seeding and deletes it.
	moved := filepath.Join(releaseDir, "renamed-by-client.mkv")
	require.NoError(t, os.Rename(source, moved))
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	require.NoError(t, os.RemoveAll(releaseDir))
	got, err = os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "canonical target must outlive the torrent source")
}

// A multi-file release keeps its non-video companions with the client; only the
// video is published, and nothing under the release directory is removed.
func TestMediaHandlerMultiFileSeedingReleaseRetainsCompanions(t *testing.T) {
	root := t.TempDir()
	seedDir := filepath.Join(root, "seeding-movies")
	movieLib := filepath.Join(root, "movies")
	releaseDir := filepath.Join(seedDir, "Companion.Movie.2026-GROUP")
	source := filepath.Join(releaseDir, "Companion.Movie.2026.1080p.WEB-DL.mkv")
	nfo := filepath.Join(releaseDir, "release.nfo")
	sample := filepath.Join(releaseDir, "Sample", "sample.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(sample), 0o755))
	require.NoError(t, os.MkdirAll(movieLib, 0o755))
	require.NoError(t, os.WriteFile(source, []byte("feature"), 0o644))
	require.NoError(t, os.WriteFile(nfo, []byte("nfo"), 0o644))
	require.NoError(t, os.WriteFile(sample, []byte("sample"), 0o644))

	handler := newSeedingHandler(t, seedDir, movieLib, filepath.Join(root, "tv"))
	handler.processFile(source)

	assert.FileExists(t, filepath.Join(movieLib, "Companion Movie (2026)", "Companion Movie (2026).mkv"))
	assert.FileExists(t, source)
	assert.FileExists(t, nfo)
	assert.FileExists(t, sample)
}
