package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/naming"
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
	for i, root := range roots {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		progress <- database.ProgressEvent{Phase: "walking", Msg: root, Current: i + 1, Total: len(roots)}
		walkErr := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
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
		s.indexOneBestEffort(fe.path, fe.root)
	}

	progress <- database.ProgressEvent{Phase: "complete"}
	return nil
}

// indexOneBestEffort calls processFile with an inferred media type. Errors
// are swallowed so a single bad file doesn't abort the whole rescan; the
// caller's progress stream is the source of truth.
func (s *FileScanner) indexOneBestEffort(path, root string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	mediaType := "movie"
	if naming.IsTVEpisodeFilename(filepath.Base(path)) {
		mediaType = "episode"
	}
	result := &ScanResult{}
	_ = s.processFile(path, info, root, mediaType, result)
}
