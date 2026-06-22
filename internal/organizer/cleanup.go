package organizer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// allowedExtensions lists extensions that PurgeNonAllowed will not remove.
var allowedExtensions = map[string]bool{
	// video
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
	".vob": true, ".divx": true, ".xvid": true,
	// subtitle
	".srt": true, ".sub": true, ".idx": true, ".ass": true,
	".ssa": true, ".vtt": true, ".smi": true,
}

// PurgeNonAllowed removes every file whose extension is not in the video/subtitle
// allowlist from dir and its descendants. Directories are removed only when empty.
// Unreadable entries are skipped rather than aborting the walk.
func PurgeNonAllowed(dir string) error {
	type entry struct {
		path  string
		isDir bool
	}
	var all []entry
	var errs []error

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if path == dir {
			return nil
		}
		all = append(all, entry{path: path, isDir: d.IsDir()})
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}

	// Process deepest paths first so directories are empty when we attempt Remove.
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if e.isDir {
			if err := os.Remove(e.path); err != nil && !ignoreCleanupRemoveError(err) {
				errs = append(errs, err)
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.path))
		if !allowedExtensions[ext] {
			if err := os.Remove(e.path); err != nil && !ignoreCleanupRemoveError(err) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func ignoreCleanupRemoveError(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST)
}
