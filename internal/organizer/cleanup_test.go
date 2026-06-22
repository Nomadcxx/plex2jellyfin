package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPurgeNonAllowed_VideoSurvives(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"movie.mkv", "episode.mp4", "clip.avi"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644))
	}
	require.NoError(t, PurgeNonAllowed(dir))
	for _, name := range []string{"movie.mkv", "episode.mp4", "clip.avi"} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.NoError(t, err, "%s should survive purge", name)
	}
}

func TestPurgeNonAllowed_SubtitleSurvives(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"movie.srt", "movie.en.srt", "movie.ass", "movie.vtt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("sub"), 0644))
	}
	require.NoError(t, PurgeNonAllowed(dir))
	for _, name := range []string{"movie.srt", "movie.en.srt", "movie.ass", "movie.vtt"} {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.NoError(t, err, "%s should survive purge", name)
	}
}

func TestPurgeNonAllowed_JunkRemoved(t *testing.T) {
	dir := t.TempDir()
	junkFiles := []string{"readme.txt", "info.nfo", "cover.jpg", "notes.exe", ".part"}
	for _, name := range junkFiles {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("junk"), 0644))
	}
	require.NoError(t, PurgeNonAllowed(dir))
	for _, name := range junkFiles {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.True(t, os.IsNotExist(err), "%s should be removed", name)
	}
}

func TestPurgeNonAllowed_NestedEmptyDirsRemoved(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "Subs")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "info.nfo"), []byte("x"), 0644))

	require.NoError(t, PurgeNonAllowed(dir))

	_, err := os.Stat(sub)
	assert.True(t, os.IsNotExist(err), "empty nested dir should be removed after junk deleted")
}

func TestPurgeNonAllowed_NestedDirWithAllowedFilePreserved(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "Subs")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "movie.srt"), []byte("sub"), 0644))

	require.NoError(t, PurgeNonAllowed(dir))

	_, err := os.Stat(filepath.Join(sub, "movie.srt"))
	assert.NoError(t, err, "subtitle in nested dir should survive")
}

func TestPurgeNonAllowed_ReportsWalkErrors(t *testing.T) {
	err := PurgeNonAllowed(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}
