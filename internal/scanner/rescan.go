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

// FullRescan walks the given roots, emitting ProgressEvent values, and
// indexes each video file unless dryRun is set. It returns ctx.Err() when
// cancellation is observed at a file boundary.
func (s *FileScanner) FullRescan(ctx context.Context, roots []string, dryRun bool, progress chan<- database.ProgressEvent) error {
	type fileEntry struct {
		path string
		root string
	}
	var files []fileEntry
	var errs []error
	for i, root := range roots {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		progress <- database.ProgressEvent{Phase: "walking", Msg: root, Current: i + 1, Total: len(roots)}
		walkErr := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
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
			files = append(files, fileEntry{path: p, root: root})
			return nil
		})
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", root, walkErr)
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
		if err := s.indexOne(fe.path, fe.root); err != nil {
			errs = append(errs, err)
		}
	}

	progress <- database.ProgressEvent{Phase: "complete"}
	return errors.Join(errs...)
}

// indexOne calls processFile with an inferred media type.
func (s *FileScanner) indexOne(path, root string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	mediaType := "movie"
	if naming.IsTVEpisodeFilename(filepath.Base(path)) {
		mediaType = "episode"
	}
	result := &ScanResult{}
	if err := s.processFile(path, info, root, mediaType, result); err != nil {
		return fmt.Errorf("index %s: %w", path, err)
	}
	return nil
}
