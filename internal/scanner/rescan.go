package scanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

// RescanRoot pairs a library root path with its configured media type
// ("movie" or "episode"). FullRescan uses this to index files with the
// correct type instead of guessing from the filename, which fails for
// obfuscated TV episode names that lack the SxxExx pattern.
type RescanRoot struct {
	Path      string
	MediaType string
}

// FullRescan walks the given roots, emitting ProgressEvent values, and
// indexes each video file unless dryRun is set. It returns ctx.Err() when
// cancellation is observed at a file boundary.
func (s *FileScanner) FullRescan(ctx context.Context, roots []RescanRoot, dryRun bool, progress chan<- database.ProgressEvent) error {
	type fileEntry struct {
		path      string
		root      string
		mediaType string
	}
	var files []fileEntry
	var errs []error
	for i, root := range roots {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		progress <- database.ProgressEvent{Phase: "walking", Msg: root.Path, Current: i + 1, Total: len(roots)}
		walkErr := filepath.Walk(root.Path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				errs = append(errs, fmt.Errorf("walk %s: %w", p, err))
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !isVideoFile(p) {
				return nil
			}
			files = append(files, fileEntry{path: p, root: root.Path, mediaType: root.MediaType})
			return nil
		})
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", root.Path, walkErr)
		}
	}

	for i, fe := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		progress <- database.ProgressEvent{Phase: "indexing", Msg: fe.path, Current: i + 1, Total: len(files)}
		if dryRun {
			continue
		}
		skipped, err := s.indexOne(fe.path, fe.root, fe.mediaType)
		if skipped {
			progress <- database.ProgressEvent{Phase: "skipped", Msg: fe.path, Current: i + 1, Total: len(files)}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	progress <- database.ProgressEvent{Phase: "complete"}
	return errors.Join(errs...)
}

// indexOne filters files like the regular scanner before processing them. When
// mediaType is empty, it falls back to inferring from the filename (legacy behavior).
func (s *FileScanner) indexOne(path, root, mediaType string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if mediaType == "" {
		mediaType = "movie"
		if naming.IsTVEpisodeFilename(filepath.Base(path)) {
			mediaType = "episode"
		}
	}
	if !s.shouldIncludeFile(path, info.Size(), mediaType) {
		return true, nil
	}
	result := &ScanResult{}
	if err := s.processFile(path, info, root, mediaType, result); err != nil {
		return false, fmt.Errorf("index %s: %w", path, err)
	}
	return false, nil
}
