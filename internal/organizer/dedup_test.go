package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractEpisodeKey(t *testing.T) {
	tests := []struct {
		filename    string
		wantSeason  int
		wantEpisode int
		wantOK      bool
	}{
		{"Show.Name.S02E19.1080p.mkv", 2, 19, true},
		{"show.name.s01e05.720p.mp4", 1, 5, true},
		{"Show.Name.2x07.BluRay.mkv", 2, 7, true},
		// date-based: naming.ParseTVShowName maps year→season, month*100+day→episode
		{"Late Night Show.2021.03.15.mkv", 2021, 315, true},
		// no episode marker → false
		{"The.Movie.2021.1080p.mkv", 0, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			s, e, ok := ExtractEpisodeKey(tc.filename)
			assert.Equal(t, tc.wantOK, ok, "ok mismatch")
			if tc.wantOK {
				assert.Equal(t, tc.wantSeason, s, "season mismatch")
				assert.Equal(t, tc.wantEpisode, e, "episode mismatch")
			}
		})
	}
}

func TestFindEpisodeFile(t *testing.T) {
	t.Run("subtitle only is ignored", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Show.S01E05.srt"), []byte("sub"), 0644))

		path, found := FindEpisodeFile(dir, 1, 5)
		assert.False(t, found, "subtitle-only should not match")
		assert.Empty(t, path)
	})

	t.Run("mkv selected over srt when both exist", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Show.S02E03.srt"), []byte("sub"), 0644))
		mkvPath := filepath.Join(dir, "Show.S02E03.1080p.mkv")
		require.NoError(t, os.WriteFile(mkvPath, []byte("video"), 0644))

		path, found := FindEpisodeFile(dir, 2, 3)
		assert.True(t, found)
		assert.Equal(t, mkvPath, path)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Show.S01E01.mkv"), []byte("v"), 0644))

		path, found := FindEpisodeFile(dir, 1, 9)
		assert.False(t, found)
		assert.Empty(t, path)
	})

	t.Run("directory entries are skipped", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "Show.S01E05.mkv"), 0755))

		path, found := FindEpisodeFile(dir, 1, 5)
		assert.False(t, found)
		assert.Empty(t, path)
	})
}
