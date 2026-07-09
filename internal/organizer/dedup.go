package organizer

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/video"
)

var (
	reSxxExx = regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,2})`)
	reNxNN   = regexp.MustCompile(`(\d{1,2})x(\d{2,3})`)
)

// ExtractEpisodeKey returns the season and episode number encoded in filename.
// It first delegates to naming.ParseTVShowName (which handles SxxExx, NxNN, and
// date-based formats).  If that returns a zero season/episode it falls back to
// a simple SxxExx then NxNN regex so that bare filenames without a title are
// still handled.
func ExtractEpisodeKey(filename string) (season, episode int, ok bool) {
	info, err := naming.ParseTVShowName(filename)
	if err == nil && (info.Season != 0 || info.Episode != 0) {
		return info.Season, info.Episode, true
	}

	base := filepath.Base(filename)
	if m := reSxxExx.FindStringSubmatch(base); m != nil {
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return s, e, true
	}
	if m := reNxNN.FindStringSubmatch(base); m != nil {
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		return s, e, true
	}
	return 0, 0, false
}

// FindEpisodeFile scans seasonDir for a video file whose episode key matches
// the given season and episode numbers.  Directories and non-video files
// (including subtitles) are ignored.
func FindEpisodeFile(seasonDir string, season, episode int) (string, bool) {
	entries, err := os.ReadDir(seasonDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !video.IsVideo(entry.Name()) {
			continue
		}
		s, e, ok := ExtractEpisodeKey(entry.Name())
		if ok && s == season && e == episode {
			return filepath.Join(seasonDir, entry.Name()), true
		}
	}
	return "", false
}
