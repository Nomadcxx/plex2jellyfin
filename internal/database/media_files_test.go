package database

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
)

func TestMoveMediaFileWithDoesNotBlockUnrelatedWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	otherDB, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open second database handle: %v", err)
	}
	defer otherDB.Close()
	sourcePath := filepath.Join(t.TempDir(), "source.mkv")
	targetPath := filepath.Join(t.TempDir(), "target.mkv")
	unrelatedPath := filepath.Join(t.TempDir(), "unrelated.mkv")
	file := &MediaFile{Path: sourcePath, Size: 1, MediaType: "movie", NormalizedTitle: "movie"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	file.Path = targetPath
	moveStarted := make(chan struct{})
	releaseMove := make(chan struct{})
	moveDone := make(chan error, 1)
	go func() {
		moveDone <- db.MoveMediaFileWith(file, sourcePath, "", false, func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
			if sourceIdentity == "" || sourceQuarantinePath == "" {
				return MediaFileMoveAttempt{}, errors.New("source recovery identity was not provided")
			}
			var savedIdentity, savedQuarantinePath string
			if err := db.SQL().QueryRow(`
				SELECT source_identity, source_quarantine_path FROM media_file_move_reservations WHERE file_id = ?`, file.ID).
				Scan(&savedIdentity, &savedQuarantinePath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			if savedIdentity != sourceIdentity || savedQuarantinePath != sourceQuarantinePath {
				return MediaFileMoveAttempt{}, errors.New("source recovery identity was not persisted before transfer")
			}
			close(moveStarted)
			<-releaseMove
			if err := os.Rename(sourcePath, targetPath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			identity, ok := fileidentity.FromPath(targetPath)
			if !ok {
				return MediaFileMoveAttempt{}, errors.New("no file identity")
			}
			targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
			if err != nil {
				return MediaFileMoveAttempt{}, err
			}
			if err := recordPublished(identity, targetQuarantinePath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			return MediaFileMoveAttempt{Published: true, SourceRemoved: true, PublishedIdentity: identity, DestinationQuarantinePath: targetQuarantinePath}, nil
		})
	}()
	<-moveStarted

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- otherDB.UpsertMediaFile(&MediaFile{Path: unrelatedPath, Size: 1, MediaType: "movie", NormalizedTitle: "unrelated"})
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			close(releaseMove)
			<-moveDone
			t.Fatalf("unrelated write during move: %v", err)
		}
	case <-time.After(3 * time.Second):
		close(releaseMove)
		<-moveDone
		t.Fatal("unrelated database write blocked during file transfer")
	}
	if err := otherDB.UpsertMediaFile(&MediaFile{Path: targetPath, Size: 1, MediaType: "movie", NormalizedTitle: "conflicting"}); !errors.Is(err, ErrMediaFileMoveInProgress) {
		close(releaseMove)
		<-moveDone
		t.Fatalf("write to reserved target error = %v, want ErrMediaFileMoveInProgress", err)
	}
	if err := otherDB.CheckMediaFileMoveDestination(sourcePath, targetPath); !errors.Is(err, ErrMediaFileMoveInProgress) {
		close(releaseMove)
		<-moveDone
		t.Fatalf("destination preflight error = %v, want ErrMediaFileMoveInProgress", err)
	}
	read, err := otherDB.GetMediaFile(sourcePath)
	if err != nil || read == nil || read.ID != file.ID {
		close(releaseMove)
		<-moveDone
		t.Fatalf("source row during move = %#v, error = %v; want ID %d", read, err, file.ID)
	}
	close(releaseMove)
	if err := <-moveDone; err != nil {
		t.Fatalf("MoveMediaFileWith: %v", err)
	}
	got, err := otherDB.GetMediaFile(targetPath)
	if err != nil || got == nil || got.ID != file.ID {
		t.Fatalf("target media row = %#v, error = %v; want ID %d", got, err, file.ID)
	}
}

func TestMoveMediaFileWithReplacementKeepsDestinationIDAndBestFileLink(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "incoming", "movie.mkv")
	targetPath := filepath.Join(root, "library", "Movie (2024)", "Movie (2024).mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("better movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	movie := &Movie{Title: "Movie", Year: 2024, CanonicalPath: filepath.Dir(targetPath), LibraryRoot: filepath.Dir(filepath.Dir(targetPath)), Source: "plex2jellyfin", SourcePriority: 100}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatal(err)
	}
	destination := &MediaFile{
		Path: targetPath, Size: 3, MediaType: "movie", ParentMovieID: &movie.ID,
		NormalizedTitle: "movie", Resolution: "720p", LibraryRoot: movie.LibraryRoot,
	}
	if err := db.UpsertMediaFile(destination); err != nil {
		t.Fatal(err)
	}
	incoming := &MediaFile{
		Path: sourcePath, Size: int64(len("better movie")), MediaType: "movie", ParentMovieID: &movie.ID,
		NormalizedTitle: "movie incoming", Resolution: "2160p", LibraryRoot: movie.LibraryRoot,
	}
	if err := db.UpsertMediaFile(incoming); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateMovieBestFile(movie.ID, &destination.ID); err != nil {
		t.Fatal(err)
	}

	incoming.Path = targetPath
	err = db.MoveMediaFileWithReplacement(incoming, sourcePath, filepath.Dir(targetPath), false,
		func(sourceIdentity, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
			if err := db.DeleteMediaFileByID(destination.ID); !errors.Is(err, ErrMediaFileMoveInProgress) {
				return MediaFileMoveAttempt{}, errors.New("replacement destination row was not protected by its reservation")
			}
			if err := db.UpdateMovieBestFile(movie.ID, nil); err == nil {
				return MediaFileMoveAttempt{}, errors.New("replacement best-file link was not protected by its reservation")
			}
			if err := os.Rename(sourcePath, targetPath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			targetIdentity, ok := fileidentity.FromPath(targetPath)
			if !ok {
				return MediaFileMoveAttempt{}, errors.New("published file has no identity")
			}
			targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
			if err != nil {
				return MediaFileMoveAttempt{}, err
			}
			if err := recordPublished(targetIdentity, targetQuarantinePath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			return MediaFileMoveAttempt{Published: true, SourceRemoved: true, PublishedIdentity: targetIdentity, DestinationQuarantinePath: targetQuarantinePath}, nil
		})
	if err != nil {
		t.Fatalf("MoveMediaFileWithReplacement: %v", err)
	}

	got, err := db.GetMediaFileByID(destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Path != targetPath || got.Size != incoming.Size || got.NormalizedTitle != incoming.NormalizedTitle || got.Resolution != incoming.Resolution {
		t.Fatalf("destination row after replacement = %#v, want destination ID %d with incoming metadata", got, destination.ID)
	}
	if got, err := db.GetMediaFileByID(incoming.ID); err != nil || got != nil {
		t.Fatalf("incoming row after replacement = %#v, error = %v; want it removed", got, err)
	}
	var bestFileID int64
	if err := db.SQL().QueryRow("SELECT best_file_id FROM movies WHERE id = ?", movie.ID).Scan(&bestFileID); err != nil {
		t.Fatal(err)
	}
	if bestFileID != destination.ID {
		t.Fatalf("movie best_file_id = %d, want preserved destination ID %d", bestFileID, destination.ID)
	}
	var canonicalPath string
	var radarrPathDirty bool
	if err := db.SQL().QueryRow("SELECT canonical_path, radarr_path_dirty FROM movies WHERE id = ?", movie.ID).Scan(&canonicalPath, &radarrPathDirty); err != nil {
		t.Fatal(err)
	}
	if canonicalPath != filepath.Dir(targetPath) || !radarrPathDirty {
		t.Fatalf("movie canonical path=%q radarr_path_dirty=%v", canonicalPath, radarrPathDirty)
	}
	entries, err := os.ReadDir(filepath.Dir(targetPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(targetPath) {
		t.Fatalf("destination directory entries after replacement = %v, want only published file", entries)
	}
}

func TestMoveMediaFileWithReplacementRestoresDestinationWhenTransferFails(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "incoming", "movie.mkv")
	targetPath := filepath.Join(root, "library", "movie.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("incoming"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("canonical"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := &MediaFile{Path: targetPath, Size: 9, MediaType: "movie", NormalizedTitle: "movie"}
	if err := db.UpsertMediaFile(destination); err != nil {
		t.Fatal(err)
	}
	incoming := &MediaFile{Path: sourcePath, Size: 8, MediaType: "movie", NormalizedTitle: "movie"}
	if err := db.UpsertMediaFile(incoming); err != nil {
		t.Fatal(err)
	}
	incoming.Path = targetPath
	err = db.MoveMediaFileWithReplacement(incoming, sourcePath, filepath.Dir(targetPath), false,
		func(string, string, func(string, string) error) (MediaFileMoveAttempt, error) {
			return MediaFileMoveAttempt{}, errors.New("injected transfer failure")
		})
	if err == nil {
		t.Fatal("MoveMediaFileWithReplacement succeeded after the transfer failed")
	}
	if got, err := os.ReadFile(targetPath); err != nil || string(got) != "canonical" {
		t.Fatalf("canonical destination after failed transfer = %q, error = %v", got, err)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || string(got) != "incoming" {
		t.Fatalf("incoming source after failed transfer = %q, error = %v", got, err)
	}
	got, err := db.GetMediaFileByID(destination.ID)
	if err != nil || got == nil || got.Path != targetPath {
		t.Fatalf("destination row after failed transfer = %#v, error = %v", got, err)
	}
	got, err = db.GetMediaFileByID(incoming.ID)
	if err != nil || got == nil || got.Path != sourcePath {
		t.Fatalf("incoming row after failed transfer = %#v, error = %v", got, err)
	}
	var reservations int
	if err := db.SQL().QueryRow("SELECT COUNT(*) FROM media_file_move_reservations").Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("move reservations after rollback = %d, want 0", reservations)
	}
	entries, err := os.ReadDir(filepath.Dir(targetPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(targetPath) {
		t.Fatalf("destination directory entries after rollback = %v, want only restored destination", entries)
	}
}

func TestRecoverStaleReplacementMoveKeepsDestinationIDAndBestFileLink(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "incoming", "movie.mkv")
	targetPath := filepath.Join(root, "library", "Movie (2024)", "Movie (2024).mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("better movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	movie := &Movie{Title: "Movie", Year: 2024, CanonicalPath: filepath.Dir(sourcePath), LibraryRoot: filepath.Dir(filepath.Dir(targetPath)), Source: "plex2jellyfin", SourcePriority: 100}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatal(err)
	}
	destination := &MediaFile{Path: targetPath, Size: 3, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "movie", Resolution: "720p", LibraryRoot: movie.LibraryRoot}
	if err := db.UpsertMediaFile(destination); err != nil {
		t.Fatal(err)
	}
	incoming := &MediaFile{Path: sourcePath, Size: int64(len("better movie")), MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "movie incoming", Resolution: "2160p"}
	if err := db.UpsertMediaFile(incoming); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateMovieBestFile(movie.ID, &destination.ID); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, ok := fileidentity.FromPath(sourcePath)
	if !ok {
		t.Fatal("source identity unavailable")
	}
	replaceIdentity, ok := fileidentity.FromPath(targetPath)
	if !ok {
		t.Fatal("destination identity unavailable")
	}
	replaceQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(targetPath, replaceQuarantinePath, replaceIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		t.Fatal(err)
	}
	targetIdentity, ok := fileidentity.FromPath(targetPath)
	if !ok {
		t.Fatal("published identity unavailable")
	}
	sourceQuarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, canonical_path, library_root, clear_compliance, owner_pid,
			 source_identity, source_quarantine_path, target_identity, target_quarantine_path, source_removal_pending,
			 replace_file_id, replace_identity, replace_quarantine_path)
		VALUES (?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, 1, ?, ?, ?)`,
		incoming.ID, sourcePath, targetPath, filepath.Dir(targetPath), destination.LibraryRoot,
		sourceIdentity, sourceQuarantinePath, targetIdentity, targetQuarantinePath,
		destination.ID, replaceIdentity, replaceQuarantinePath); err != nil {
		t.Fatal(err)
	}

	if err := db.RecoverStaleMediaFileMoves(); err != nil {
		t.Fatalf("recover stale replacement move: %v", err)
	}
	got, err := db.GetMediaFileByID(destination.ID)
	if err != nil || got == nil || got.Path != targetPath || got.NormalizedTitle != incoming.NormalizedTitle || got.Resolution != incoming.Resolution {
		t.Fatalf("destination row after recovery = %#v, error = %v", got, err)
	}
	if got, err := db.GetMediaFileByID(incoming.ID); err != nil || got != nil {
		t.Fatalf("incoming row after recovery = %#v, error = %v; want it removed", got, err)
	}
	var bestFileID int64
	if err := db.SQL().QueryRow("SELECT best_file_id FROM movies WHERE id = ?", movie.ID).Scan(&bestFileID); err != nil {
		t.Fatal(err)
	}
	if bestFileID != destination.ID {
		t.Fatalf("movie best_file_id after recovery = %d, want %d", bestFileID, destination.ID)
	}
	var reservations int
	if err := db.SQL().QueryRow("SELECT COUNT(*) FROM media_file_move_reservations").Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("reservations after recovery = %d, want 0", reservations)
	}
	entries, err := os.ReadDir(filepath.Dir(targetPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(targetPath) {
		t.Fatalf("destination directory after recovery = %v, want only published file", entries)
	}
}

func TestMoveMediaFileWithAllowsIndependentMovesToRunConcurrently(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	var files [2]*MediaFile
	var sources, targets [2]string
	for i := range files {
		sources[i] = filepath.Join(root, "source", string(rune('a'+i))+".mkv")
		targets[i] = filepath.Join(root, "target", string(rune('a'+i))+".mkv")
		if err := os.MkdirAll(filepath.Dir(sources[i]), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(targets[i]), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sources[i], []byte("media"), 0o644); err != nil {
			t.Fatal(err)
		}
		files[i] = &MediaFile{Path: sources[i], Size: 5, MediaType: "movie", NormalizedTitle: string(rune('a' + i))}
		if err := db.UpsertMediaFile(files[i]); err != nil {
			t.Fatal(err)
		}
	}

	started := make(chan struct{}, len(files))
	release := make(chan struct{})
	done := make(chan error, len(files))
	for i := range files {
		go func(i int) {
			file := *files[i]
			file.Path = targets[i]
			done <- db.MoveMediaFileWith(&file, sources[i], "", false,
				func(sourceIdentity, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
					started <- struct{}{}
					<-release
					if err := os.Rename(sources[i], targets[i]); err != nil {
						return MediaFileMoveAttempt{}, err
					}
					quarantinePath, err := fileidentity.NewQuarantinePath(targets[i])
					if err != nil {
						return MediaFileMoveAttempt{}, err
					}
					if err := recordPublished(sourceIdentity, quarantinePath); err != nil {
						return MediaFileMoveAttempt{}, err
					}
					return MediaFileMoveAttempt{
						Published: true, SourceRemoved: true, PublishedIdentity: sourceIdentity,
						DestinationQuarantinePath: quarantinePath,
					}, nil
				})
		}(i)
	}

	for i := 0; i < len(files); i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			for range files {
				<-done
			}
			t.Fatal("independent media moves did not enter their transfer callbacks concurrently")
		}
	}
	close(release)
	for range files {
		if err := <-done; err != nil {
			t.Fatalf("MoveMediaFileWith: %v", err)
		}
	}
}

func TestMoveMediaFileWithReconcilesAfterFinalizationFailure(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 5, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`CREATE TRIGGER fail_media_file_move_update BEFORE UPDATE OF path ON media_files BEGIN SELECT RAISE(ABORT, 'injected finalization failure'); END`); err != nil {
		t.Fatal(err)
	}

	file.Path = targetPath
	err = db.MoveMediaFileWith(file, sourcePath, "", false,
		func(sourceIdentity, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
			targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
			if err != nil {
				return MediaFileMoveAttempt{}, err
			}
			if err := recordPublished(sourceIdentity, targetQuarantinePath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			if err := os.Rename(sourcePath, targetPath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			return MediaFileMoveAttempt{
				Published: true, SourceRemoved: true, PublishedIdentity: sourceIdentity,
				DestinationQuarantinePath: targetQuarantinePath,
			}, nil
		})
	if err == nil {
		t.Fatal("MoveMediaFileWith succeeded despite injected database finalization failure")
	}
	if _, err := db.SQL().Exec("DROP TRIGGER fail_media_file_move_update"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		moved, err := db.GetMediaFile(targetPath)
		if err == nil && moved != nil && moved.ID == file.ID {
			var reservations int
			if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
				t.Fatal(err)
			}
			if reservations == 0 {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("media move was not reconciled: moved row = %#v, error = %v", moved, err)
		case <-ticker.C:
		}
	}
}

func TestMoveMediaFileWithReconcilesAfterPublishedCallbackFailure(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 5, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	file.Path = targetPath
	transferErr := errors.New("post-move verification failed")
	err = db.MoveMediaFileWith(file, sourcePath, "", false,
		func(_, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
			if err := os.Rename(sourcePath, targetPath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			identity, ok := fileidentity.FromPath(targetPath)
			if !ok {
				return MediaFileMoveAttempt{}, errors.New("published file identity is unavailable")
			}
			targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
			if err != nil {
				return MediaFileMoveAttempt{}, err
			}
			if err := recordPublished(identity, targetQuarantinePath); err != nil {
				return MediaFileMoveAttempt{}, err
			}
			return MediaFileMoveAttempt{
				Published: true, SourceRemoved: true, PublishedIdentity: identity,
				DestinationQuarantinePath: targetQuarantinePath,
			}, transferErr
		})
	if !errors.Is(err, transferErr) {
		t.Fatalf("MoveMediaFileWith error = %v, want %v", err, transferErr)
	}

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		moved, err := db.GetMediaFile(targetPath)
		var reservations int
		if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
			t.Fatal(err)
		}
		if err == nil && moved != nil && moved.ID == file.ID && reservations == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("published move was not reconciled: row = %#v, error = %v, reservations = %d", moved, err, reservations)
		case <-ticker.C:
		}
	}
}

func TestRepointMediaFilesUnderFolderPreservesIdentityAndParentLinks(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourceDir := filepath.Join(root, "old-title")
	targetDir := filepath.Join(root, "new-title")
	movie := &Movie{Title: "Film", Year: 2024, CanonicalPath: sourceDir, LibraryRoot: root, Source: "test", SourcePriority: 100}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceDir, "Film.mkv")
	file := &MediaFile{Path: sourcePath, MediaType: "movie", NormalizedTitle: "film", ParentMovieID: &movie.ID}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`UPDATE movies SET best_file_id = ? WHERE id = ?`, file.ID, movie.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.RepointMediaFilesUnderFolder(sourceDir, targetDir); err != nil {
		t.Fatalf("repoint folder paths: %v", err)
	}
	moved, err := db.GetMediaFile(filepath.Join(targetDir, "Film.mkv"))
	if err != nil || moved == nil {
		t.Fatalf("read moved media row: %#v, error = %v", moved, err)
	}
	if moved.ID != file.ID || moved.ParentMovieID == nil || *moved.ParentMovieID != movie.ID {
		t.Fatalf("moved row = %#v; want original ID %d and movie %d", moved, file.ID, movie.ID)
	}
	var canonicalPath string
	var bestFileID sql.NullInt64
	var pathDirty bool
	if err := db.SQL().QueryRow(`SELECT canonical_path, best_file_id, radarr_path_dirty FROM movies WHERE id = ?`, movie.ID).
		Scan(&canonicalPath, &bestFileID, &pathDirty); err != nil {
		t.Fatal(err)
	}
	if canonicalPath != targetDir || !bestFileID.Valid || bestFileID.Int64 != file.ID || !pathDirty {
		t.Fatalf("movie path=%q best_file_id=%v dirty=%v", canonicalPath, bestFileID, pathDirty)
	}
}

func TestMoveMediaFilePreservesMovieAndEpisodeLinks(t *testing.T) {
	for _, mediaType := range []string{"movie", "episode"} {
		t.Run(mediaType, func(t *testing.T) {
			db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			root := t.TempDir()
			sourceDir := filepath.Join(root, "old")
			targetDir := filepath.Join(root, "new")
			sourcePath := filepath.Join(sourceDir, "media.mkv")
			targetPath := filepath.Join(targetDir, "media.mkv")
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sourcePath, []byte("media"), 0o644); err != nil {
				t.Fatal(err)
			}

			file := &MediaFile{Path: sourcePath, Size: 5, MediaType: mediaType, NormalizedTitle: "media", LibraryRoot: root}
			var movieID, episodeID int64
			if mediaType == "movie" {
				movie := &Movie{Title: "Film", Year: 2024, CanonicalPath: sourceDir, LibraryRoot: root, Source: "test", SourcePriority: 100}
				if _, err := db.UpsertMovie(movie); err != nil {
					t.Fatal(err)
				}
				movieID = movie.ID
				file.ParentMovieID = &movie.ID
			} else {
				series := &Series{Title: "Show", Year: 2024, CanonicalPath: sourceDir, LibraryRoot: root, Source: "test", SourcePriority: 100}
				if _, err := db.UpsertSeries(series); err != nil {
					t.Fatal(err)
				}
				episode := &Episode{SeriesID: series.ID, Season: 1, Episode: 2, Title: "Episode"}
				if err := db.UpsertEpisode(episode); err != nil {
					t.Fatal(err)
				}
				episodeID = episode.ID
				file.ParentSeriesID = &series.ID
				file.ParentEpisodeID = &episode.ID
			}
			if err := db.UpsertMediaFile(file); err != nil {
				t.Fatal(err)
			}
			if mediaType == "movie" {
				if err := db.UpdateMovieBestFile(movieID, &file.ID); err != nil {
					t.Fatal(err)
				}
			} else if err := db.UpdateEpisodeBestFile(episodeID, &file.ID); err != nil {
				t.Fatal(err)
			}

			moving := *file
			moving.Path = targetPath
			err = db.MoveMediaFileWith(&moving, sourcePath, targetDir, false,
				func(sourceIdentity, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
					if err := os.Rename(sourcePath, targetPath); err != nil {
						return MediaFileMoveAttempt{}, err
					}
					targetIdentity, ok := fileidentity.FromPath(targetPath)
					if !ok {
						return MediaFileMoveAttempt{}, errors.New("published file has no identity")
					}
					targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
					if err != nil {
						return MediaFileMoveAttempt{}, err
					}
					if err := recordPublished(targetIdentity, targetQuarantinePath); err != nil {
						return MediaFileMoveAttempt{}, err
					}
					return MediaFileMoveAttempt{Published: true, SourceRemoved: true, PublishedIdentity: sourceIdentity, DestinationQuarantinePath: targetQuarantinePath}, nil
				})
			if err != nil {
				t.Fatalf("move media file: %v", err)
			}
			moved, err := db.GetMediaFile(targetPath)
			if err != nil || moved == nil || moved.ID != file.ID {
				t.Fatalf("moved media row = %#v, error = %v; want ID %d", moved, err, file.ID)
			}
			if mediaType == "movie" {
				var path string
				var bestFileID sql.NullInt64
				var dirty bool
				if err := db.SQL().QueryRow("SELECT canonical_path, best_file_id, radarr_path_dirty FROM movies WHERE id = ?", movieID).Scan(&path, &bestFileID, &dirty); err != nil {
					t.Fatal(err)
				}
				if path != targetDir || !bestFileID.Valid || bestFileID.Int64 != file.ID || !dirty {
					t.Fatalf("movie path=%q best_file_id=%v radarr_path_dirty=%v", path, bestFileID, dirty)
				}
			} else {
				var path string
				var bestFileID sql.NullInt64
				var dirty bool
				if err := db.SQL().QueryRow("SELECT s.canonical_path, e.best_file_id, s.sonarr_path_dirty FROM series s JOIN episodes e ON e.series_id = s.id WHERE e.id = ?", episodeID).Scan(&path, &bestFileID, &dirty); err != nil {
					t.Fatal(err)
				}
				if path != targetDir || !bestFileID.Valid || bestFileID.Int64 != file.ID || !dirty {
					t.Fatalf("series path=%q best_file_id=%v sonarr_path_dirty=%v", path, bestFileID, dirty)
				}
			}
		})
	}
}

func TestConcurrentOpenRecoversCompletedFilesystemMoveOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "old", "Film.mkv")
	targetPath := filepath.Join(root, "new", "Film.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	movie := &Movie{Title: "Film", Year: 2020, CanonicalPath: filepath.Dir(sourcePath), LibraryRoot: filepath.Dir(root), Source: "filesystem"}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatalf("insert movie: %v", err)
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "film", LibraryRoot: filepath.Dir(root)}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	if err := db.UpdateMovieBestFile(movie.ID, &file.ID); err != nil {
		t.Fatalf("link best file: %v", err)
	}
	if err := os.Rename(sourcePath, targetPath); err != nil {
		t.Fatalf("simulate completed move: %v", err)
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, canonical_path, library_root, target_identity, clear_compliance, owner_pid)
		VALUES (?, ?, ?, ?, ?, ?, 0, -1)`,
		file.ID, sourcePath, targetPath, filepath.Dir(targetPath), filepath.Dir(root), targetIdentity); err != nil {
		t.Fatalf("insert stale reservation: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close database before recovery: %v", err)
	}
	type openResult struct {
		db  *MediaDB
		err error
	}
	opened := make(chan openResult, 2)
	for range 2 {
		go func() {
			reopened, err := OpenPath(dbPath)
			opened <- openResult{db: reopened, err: err}
		}()
	}
	first, second := <-opened, <-opened
	if first.err != nil || second.err != nil {
		if first.db != nil {
			first.db.Close()
		}
		if second.db != nil {
			second.db.Close()
		}
		t.Fatalf("concurrent database opens = %v, %v", first.err, second.err)
	}
	defer first.db.Close()
	defer second.db.Close()
	reopened := first.db
	moved, err := reopened.GetMediaFile(targetPath)
	if err != nil || moved == nil {
		t.Fatalf("read moved media row: %#v, error = %v", moved, err)
	}
	if moved.ID != file.ID {
		t.Fatalf("moved media ID = %d, want preserved ID %d", moved.ID, file.ID)
	}
	var bestFileID sql.NullInt64
	if err := reopened.SQL().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, movie.ID).Scan(&bestFileID); err != nil {
		t.Fatalf("read movie best-file link: %v", err)
	}
	if !bestFileID.Valid || bestFileID.Int64 != file.ID {
		t.Fatalf("movie best-file link = %v, want %d", bestFileID, file.ID)
	}
	updatedMovie, err := reopened.GetMovieByTitle("Film", 2020)
	if err != nil {
		t.Fatalf("read movie: %v", err)
	}
	if updatedMovie.CanonicalPath != filepath.Dir(targetPath) || !updatedMovie.RadarrPathDirty {
		t.Fatalf("movie after recovery = %#v, want canonical path %q and dirty Radarr path", updatedMovie, filepath.Dir(targetPath))
	}
	var reservations int
	if err := reopened.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestRecoverStaleMoveFinishesSourceQuarantinedBeforeCrash(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	quarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(sourcePath, quarantinePath, sourceIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, target_identity, source_identity, source_quarantine_path, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, ?, ?, 1, -1)`,
		file.ID, sourcePath, targetPath, targetIdentity, sourceIdentity, quarantinePath); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("recover interrupted move: %v", err)
	}
	defer reopened.Close()
	if _, err := os.Lstat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source path after recovery: %v, want missing", err)
	}
	if _, err := os.Lstat(quarantinePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source quarantine after recovery: %v, want missing", err)
	}
	moved, err := reopened.GetMediaFile(targetPath)
	if err != nil || moved == nil || moved.ID != file.ID {
		t.Fatalf("recovered media file = %#v, error = %v; want ID %d", moved, err, file.ID)
	}
	var reservations int
	if err := reopened.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestMoveMediaFileWithReconcilesDelayedSourceRemoval(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	file.Path = targetPath
	pending := make(chan error, 1)
	pendingErr := errors.New("source removal timed out")
	err = db.MoveMediaFileWith(file, sourcePath, "", false, func(_, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
		if err := os.WriteFile(targetPath, []byte("film"), 0o644); err != nil {
			return MediaFileMoveAttempt{}, err
		}
		identity, ok := fileidentity.FromPath(targetPath)
		if !ok {
			return MediaFileMoveAttempt{}, errors.New("no file identity")
		}
		targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
		if err != nil {
			return MediaFileMoveAttempt{}, err
		}
		if err := recordPublished(identity, targetQuarantinePath); err != nil {
			return MediaFileMoveAttempt{Published: true, PublishedIdentity: identity}, err
		}
		go func() {
			time.Sleep(25 * time.Millisecond)
			pending <- os.Remove(sourcePath)
		}()
		return MediaFileMoveAttempt{Published: true, PublishedIdentity: identity, DestinationQuarantinePath: targetQuarantinePath, PendingSourceRemoval: pending}, pendingErr
	})
	if !errors.Is(err, pendingErr) {
		t.Fatalf("MoveMediaFileWith error = %v, want source removal pending", err)
	}
	var pendingReservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE source_removal_pending = 1`).Scan(&pendingReservations); err != nil {
		t.Fatal(err)
	}
	if pendingReservations != 1 {
		t.Fatalf("pending reservations = %d, want 1", pendingReservations)
	}

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		moved, err := db.GetMediaFile(targetPath)
		if err != nil {
			t.Fatalf("read finalized media file: %v", err)
		}
		if moved != nil {
			if moved.ID != file.ID {
				t.Fatalf("final media file ID = %d, want %d", moved.ID, file.ID)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("delayed source removal did not finalize the media row")
		case <-ticker.C:
		}
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestClaimStaleMediaFileMoveRequiresMatchingPaths(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	reservation := mediaFileMoveReservation{
		fileID:     1,
		sourcePath: "/old/source.mkv",
		targetPath: "/old/target.mkv",
		ownerPID:   -1,
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations (file_id, source_path, target_path, owner_pid)
		VALUES (?, ?, ?, ?)`, reservation.fileID, reservation.sourcePath, reservation.targetPath, reservation.ownerPID); err != nil {
		t.Fatalf("insert stale reservation: %v", err)
	}
	if _, err := db.SQL().Exec(`
		UPDATE media_file_move_reservations SET source_path = ?, target_path = ? WHERE file_id = ?`,
		"/new/source.mkv", "/new/target.mkv", reservation.fileID); err != nil {
		t.Fatalf("replace reservation paths: %v", err)
	}

	claimed, err := db.claimStaleMediaFileMove(&reservation)
	if err != nil {
		t.Fatalf("claim stale reservation snapshot: %v", err)
	}
	if claimed {
		t.Fatal("stale snapshot claimed a reservation with different paths")
	}
	var sourcePath, targetPath string
	var ownerPID int
	if err := db.SQL().QueryRow(`
		SELECT source_path, target_path, owner_pid FROM media_file_move_reservations WHERE file_id = ?`, reservation.fileID).
		Scan(&sourcePath, &targetPath, &ownerPID); err != nil {
		t.Fatalf("read current reservation: %v", err)
	}
	if sourcePath != "/new/source.mkv" || targetPath != "/new/target.mkv" || ownerPID != -1 {
		t.Fatalf("current reservation = %q -> %q owned by %d; want unchanged new paths and stale owner", sourcePath, targetPath, ownerPID)
	}
}

func TestRecoverStaleMediaFileMoveWithSymlinkDestination(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "old", "Film.mkv")
	targetPath := filepath.Join(root, "new", "Film.mkv")
	backingPath := filepath.Join(root, "backing", "Film.mkv")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(backingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backingPath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backingPath, targetPath); err != nil {
		t.Skipf("create destination symlink: %v", err)
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations (file_id, source_path, target_path, target_identity, owner_pid)
		VALUES (?, ?, ?, ?, -1)`, file.ID, sourcePath, targetPath, targetIdentity); err != nil {
		t.Fatalf("insert stale reservation: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database before recovery: %v", err)
	}

	reopened, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen database and recover moved symlink: %v", err)
	}
	defer reopened.Close()
	moved, err := reopened.GetMediaFile(targetPath)
	if err != nil || moved == nil {
		t.Fatalf("read recovered symlink media row: %#v, error = %v", moved, err)
	}
	if moved.ID != file.ID || moved.Size != 4 {
		t.Fatalf("recovered symlink row = %#v, want ID %d and size 4", moved, file.ID)
	}
	var reservations int
	if err := reopened.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestRecoverStaleMoveRemovesBrokenPublishedSymlinkWithoutPanicking(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source", "Film.mkv")
	targetPath := filepath.Join(root, "target", "Film.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-relative-target.mkv", targetPath); err != nil {
		t.Skipf("create dangling symlink: %v", err)
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	quarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, target_identity, source_identity, source_quarantine_path, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, ?, ?, 1, -1)`,
		file.ID, sourcePath, targetPath, targetIdentity, sourceIdentity, quarantinePath); err != nil {
		t.Fatal(err)
	}
	if err := db.RecoverStaleMediaFileMoves(); err != nil {
		t.Fatalf("recover dangling symlink destination: %v", err)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || string(got) != "film" {
		t.Fatalf("source file = %q, error = %v; want preserved source", got, err)
	}
	if _, err := os.Lstat(targetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dangling published symlink remains: %v", err)
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestRecoverStaleMoveRestoresReplacementCapturedBySourceQuarantine(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("original media"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	quarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("replacement media"), 0o644); err != nil {
		t.Fatal(err)
	}
	replacementIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if err := fileidentity.MoveToQuarantine(sourcePath, quarantinePath, replacementIdentity, true); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 14, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, source_identity, source_quarantine_path, owner_pid)
		VALUES (?, ?, ?, ?, ?, -1)`, file.ID, sourcePath, targetPath, sourceIdentity, quarantinePath); err != nil {
		t.Fatal(err)
	}
	if err := db.RecoverStaleMediaFileMoves(); err != nil {
		t.Fatalf("recover interrupted source quarantine: %v", err)
	}
	got, err := os.ReadFile(sourcePath)
	if err != nil || string(got) != "replacement media" {
		t.Fatalf("restored source replacement = %q, error = %v", got, err)
	}
	if _, err := os.Lstat(quarantinePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source quarantine remains after restoration: %v", err)
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestRecoverStaleMoveRestoresPublishedDestinationQuarantine(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("source media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("published media"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	sourceQuarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: int64(len("source media")), MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(sourcePath, sourceQuarantinePath, sourceIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(targetPath, targetQuarantinePath, targetIdentity, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, source_identity, source_quarantine_path,
			 target_identity, target_quarantine_path, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, -1)`,
		file.ID, sourcePath, targetPath, sourceIdentity, sourceQuarantinePath,
		targetIdentity, targetQuarantinePath); err != nil {
		t.Fatal(err)
	}

	if err := db.RecoverStaleMediaFileMoves(); err != nil {
		t.Fatalf("recover interrupted move: %v", err)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil || string(got) != "published media" {
		t.Fatalf("restored destination = %q, error = %v", got, err)
	}
	if _, err := os.Lstat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source remains after completed move: %v", err)
	}
	for _, path := range []string{sourceQuarantinePath, targetQuarantinePath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("quarantine %s remains: %v", path, err)
		}
	}
	moved, err := db.GetMediaFile(targetPath)
	if err != nil || moved == nil || moved.ID != file.ID {
		t.Fatalf("moved media row = %#v, error = %v; want ID %d", moved, err, file.ID)
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining reservations = %d, want 0", reservations)
	}
}

func TestRecoverStaleMoveRestoresMismatchedDestinationQuarantine(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("source media"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: int64(len("source media")), MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if err := os.WriteFile(targetPath, []byte("published media"), 0o644); err != nil {
		t.Fatal(err)
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	sourceQuarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(sourcePath, sourceQuarantinePath, sourceIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetQuarantinePath, []byte("unrelated replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, source_identity, source_quarantine_path,
			 target_identity, target_quarantine_path, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, -1)`,
		file.ID, sourcePath, targetPath, sourceIdentity, sourceQuarantinePath,
		targetIdentity, targetQuarantinePath); err != nil {
		t.Fatal(err)
	}

	if err := db.RecoverStaleMediaFileMoves(); err != nil {
		t.Fatalf("recover interrupted move: %v", err)
	}
	for path, want := range map[string]string{sourcePath: "source media", targetPath: "unrelated replacement"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("recovered %s = %q, error = %v; want %q", path, got, err, want)
		}
	}
	for _, path := range []string{sourceQuarantinePath, targetQuarantinePath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("quarantine %s remains: %v", path, err)
		}
	}
	row, err := db.GetMediaFile(sourcePath)
	if err != nil || row == nil || row.ID != file.ID {
		t.Fatalf("media row after rollback = %#v, error = %v; want ID %d at source", row, err, file.ID)
	}
}

func TestRecoverStaleMoveReturnsPublishedFileToSourceWhenDestinationWasReplaced(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	targetIdentity, ok := fileidentity.FromLstat(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	sourceQuarantinePath, err := fileidentity.NewQuarantinePath(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 5, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(sourcePath, sourceQuarantinePath, sourceIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.MoveToQuarantine(targetPath, targetQuarantinePath, targetIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := fileidentity.RemoveQuarantine(sourceQuarantinePath, sourceIdentity, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, source_identity, source_quarantine_path,
			 target_identity, target_quarantine_path, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, -1)`,
		file.ID, sourcePath, targetPath, sourceIdentity, sourceQuarantinePath,
		targetIdentity, targetQuarantinePath); err != nil {
		t.Fatal(err)
	}

	if err := db.RecoverStaleMediaFileMoves(); err != nil {
		t.Fatalf("recover interrupted move: %v", err)
	}
	if got, err := os.ReadFile(sourcePath); err != nil || string(got) != "media" {
		t.Fatalf("restored source = %q, error = %v; want original media", got, err)
	}
	if got, err := os.ReadFile(targetPath); err != nil || string(got) != "replacement" {
		t.Fatalf("replacement destination = %q, error = %v", got, err)
	}
	if _, err := os.Lstat(targetQuarantinePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published quarantine remains after restoring source: %v", err)
	}
	if got, err := db.GetMediaFile(sourcePath); err != nil || got == nil || got.ID != file.ID {
		t.Fatalf("source media row = %#v, error = %v; want ID %d", got, err, file.ID)
	}
	if got, err := db.GetMediaFile(targetPath); err != nil || got != nil {
		t.Fatalf("target media row = %#v, error = %v; want no row", got, err)
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining reservations = %d, want 0", reservations)
	}
}

func TestRecoverStalePendingMoveWithBothPathsRemovesPublishedDestination(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	for _, path := range []string{sourcePath, targetPath} {
		if err := os.WriteFile(path, []byte("film"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	targetIdentity, ok := fileidentity.FromPath(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, target_identity, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, 1, -1)`, file.ID, sourcePath, targetPath, targetIdentity); err != nil {
		t.Fatalf("insert stale pending reservation: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database before recovery: %v", err)
	}

	reopened, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen database and recover stale pending move: %v", err)
	}
	defer reopened.Close()
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source file was not preserved: %v", err)
	}
	if _, err := os.Lstat(targetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published destination remains after recovery: stat error = %v", err)
	}
	got, err := reopened.GetMediaFile(sourcePath)
	if err != nil || got == nil || got.ID != file.ID {
		t.Fatalf("source media row after recovery = %#v, error = %v; want ID %d", got, err, file.ID)
	}
	var reservations int
	if err := reopened.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestRecoverStalePendingMoveKeepsReplacementDestination(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("source media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("published media"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, ok := fileidentity.FromPath(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	file := &MediaFile{Path: sourcePath, Size: 12, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, target_identity, source_removal_pending, owner_pid)
		VALUES (?, ?, ?, ?, 1, -1)`, file.ID, sourcePath, targetPath, identity); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("unrelated replacement"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := db.RecoverStaleMediaFileMoves(); err == nil {
		t.Fatal("stale recovery succeeded with a replacement destination")
	}
	got, err := os.ReadFile(targetPath)
	if err != nil || string(got) != "unrelated replacement" {
		t.Fatalf("replacement destination = %q, error = %v", got, err)
	}
	var reservations int
	if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 1 {
		t.Fatalf("remaining reservations = %d, want 1 for manual recovery", reservations)
	}
}

func TestMoveMediaFileWithReconcilesPublishedFailureWithoutPendingChannel(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	for _, path := range []string{sourcePath, targetPath} {
		if err := os.WriteFile(path, []byte("film"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	file.Path = targetPath
	identity, ok := fileidentity.FromPath(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	moveErr := errors.New("published destination cleanup failed")
	err = db.MoveMediaFileWith(file, sourcePath, "", false, func(_, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
		targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
		if err != nil {
			return MediaFileMoveAttempt{}, err
		}
		if err := recordPublished(identity, targetQuarantinePath); err != nil {
			return MediaFileMoveAttempt{}, err
		}
		return MediaFileMoveAttempt{Published: true, PublishedIdentity: identity, DestinationQuarantinePath: targetQuarantinePath}, moveErr
	})
	if !errors.Is(err, moveErr) {
		t.Fatalf("MoveMediaFileWith error = %v, want %v", err, moveErr)
	}

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, targetErr := os.Lstat(targetPath)
		var reservations int
		if err := db.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&reservations); err != nil {
			t.Fatal(err)
		}
		if errors.Is(targetErr, os.ErrNotExist) && reservations == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("cleanup not reconciled: target error = %v, reservations = %d", targetErr, reservations)
		case <-ticker.C:
		}
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source was not preserved after cleanup reconciliation: %v", err)
	}
}

func TestMoveMediaFileWithDoesNotFinalizeReplacedDelayedDestination(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 4, MediaType: "movie", NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatal(err)
	}
	file.Path = targetPath
	identity, ok := fileidentity.FromPath(targetPath)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	pending := make(chan error, 1)
	pendingErr := errors.New("source removal timed out")
	err = db.MoveMediaFileWith(file, sourcePath, "", false, func(_, _ string, recordPublished func(string, string) error) (MediaFileMoveAttempt, error) {
		targetQuarantinePath, err := fileidentity.NewQuarantinePath(targetPath)
		if err != nil {
			return MediaFileMoveAttempt{}, err
		}
		if err := recordPublished(identity, targetQuarantinePath); err != nil {
			return MediaFileMoveAttempt{}, err
		}
		return MediaFileMoveAttempt{Published: true, PublishedIdentity: identity, DestinationQuarantinePath: targetQuarantinePath, PendingSourceRemoval: pending}, pendingErr
	})
	if !errors.Is(err, pendingErr) {
		t.Fatalf("MoveMediaFileWith error = %v, want %v", err, pendingErr)
	}
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	pending <- nil

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var ownerPID int
		err := db.SQL().QueryRow(`SELECT owner_pid FROM media_file_move_reservations WHERE file_id = ?`, file.ID).Scan(&ownerPID)
		if err == nil && ownerPID == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("replacement remained unreported or reservation was lost: owner=%d, error=%v", ownerPID, err)
		case <-ticker.C:
		}
	}
	got, err := os.ReadFile(targetPath)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("replacement destination = %q, error = %v", got, err)
	}
	if moved, err := db.GetMediaFile(targetPath); err != nil || moved != nil {
		t.Fatalf("unexpected media row at replacement target: %#v, error = %v", moved, err)
	}
}

func TestRecoverStaleMediaFileMoveKeepsExistingSource(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := &MediaFile{Path: sourcePath, Size: 6, MediaType: "movie", NormalizedTitle: "source"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations (file_id, source_path, target_path, owner_pid)
		VALUES (?, ?, ?, -1)`, file.ID, sourcePath, targetPath); err != nil {
		t.Fatalf("insert stale reservation: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database before recovery: %v", err)
	}

	reopened, err := OpenPath(dbPath)
	if err != nil {
		t.Fatalf("reopen database and recover stale move: %v", err)
	}
	defer reopened.Close()
	got, err := reopened.GetMediaFile(sourcePath)
	if err != nil || got == nil || got.ID != file.ID {
		t.Fatalf("source row after recovery = %#v, error = %v; want ID %d", got, err, file.ID)
	}
	target, err := reopened.GetMediaFile(targetPath)
	if err != nil || target != nil {
		t.Fatalf("target row after recovery = %#v, error = %v; want no row", target, err)
	}
	var reservations int
	if err := reopened.SQL().QueryRow(`SELECT COUNT(*) FROM media_file_move_reservations`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("remaining move reservations = %d, want 0", reservations)
	}
}

func TestDeletingReservedMediaFileKeepsBestFileLinks(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	movie := &Movie{Title: "Film", Year: 2020, CanonicalPath: "/movies/Film", LibraryRoot: "/movies", Source: "filesystem"}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatalf("insert movie: %v", err)
	}
	file := &MediaFile{Path: "/movies/Film/Film.mkv", Size: 4, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "film"}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	if err := db.UpdateMovieBestFile(movie.ID, &file.ID); err != nil {
		t.Fatalf("link best file: %v", err)
	}
	if _, err := db.SQL().Exec(`
		INSERT INTO media_file_move_reservations
			(file_id, source_path, target_path, owner_pid)
		VALUES (?, ?, ?, ?)`, file.ID, file.Path, "/movies/Film/Film-new.mkv", os.Getpid()); err != nil {
		t.Fatalf("insert active reservation: %v", err)
	}
	if err := db.UpsertMediaFile(&MediaFile{Path: "/movies/Film/Film-new.mkv", Size: 4, MediaType: "movie", NormalizedTitle: "film"}); !errors.Is(err, ErrMediaFileMoveInProgress) {
		t.Fatalf("upsert to reserved destination error = %v, want ErrMediaFileMoveInProgress", err)
	}

	if err := db.DeleteMediaFile(file.Path); !errors.Is(err, ErrMediaFileMoveInProgress) {
		t.Fatalf("DeleteMediaFile error = %v, want ErrMediaFileMoveInProgress", err)
	}
	var bestFileID sql.NullInt64
	if err := db.SQL().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, movie.ID).Scan(&bestFileID); err != nil {
		t.Fatal(err)
	}
	if !bestFileID.Valid || bestFileID.Int64 != file.ID {
		t.Fatalf("movie best-file link after rejected delete = %v, want %d", bestFileID, file.ID)
	}
	if err := db.DeleteMediaFileByID(file.ID); !errors.Is(err, ErrMediaFileMoveInProgress) {
		t.Fatalf("DeleteMediaFileByID error = %v, want ErrMediaFileMoveInProgress", err)
	}
	if err := db.SQL().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, movie.ID).Scan(&bestFileID); err != nil {
		t.Fatal(err)
	}
	if !bestFileID.Valid || bestFileID.Int64 != file.ID {
		t.Fatalf("movie best-file link after rejected ID delete = %v, want %d", bestFileID, file.ID)
	}
}

func TestLegacyBestFileClearIsBlockedDuringMove(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	root := t.TempDir()
	movie := &Movie{Title: "Film", Year: 2020, CanonicalPath: filepath.Join(root, "Film"), LibraryRoot: root, Source: "filesystem"}
	if _, err := db.UpsertMovie(movie); err != nil {
		t.Fatalf("insert movie: %v", err)
	}
	series := &Series{Title: "Show", Year: 2020, CanonicalPath: filepath.Join(root, "Show"), LibraryRoot: root, Source: "filesystem"}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("insert series: %v", err)
	}
	episode := &Episode{SeriesID: series.ID, Season: 1, Episode: 1, Title: "Episode 1"}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	movieFile := &MediaFile{Path: filepath.Join(root, "Film", "Film.mkv"), Size: 4, MediaType: "movie", ParentMovieID: &movie.ID, NormalizedTitle: "film"}
	episodeFile := &MediaFile{Path: filepath.Join(root, "Show", "Season 01", "Show.S01E01.mkv"), Size: 4, MediaType: "episode", ParentSeriesID: &series.ID, ParentEpisodeID: &episode.ID, NormalizedTitle: "show"}
	for _, file := range []*MediaFile{movieFile, episodeFile} {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("insert media file %s: %v", file.Path, err)
		}
	}
	if err := db.UpdateMovieBestFile(movie.ID, &movieFile.ID); err != nil {
		t.Fatalf("link movie best file: %v", err)
	}
	if err := db.UpdateEpisodeBestFile(episode.ID, &episodeFile.ID); err != nil {
		t.Fatalf("link episode best file: %v", err)
	}
	for _, file := range []*MediaFile{movieFile, episodeFile} {
		if _, err := db.SQL().Exec(`
			INSERT INTO media_file_move_reservations (file_id, source_path, target_path, owner_pid)
			VALUES (?, ?, ?, ?)`, file.ID, file.Path, file.Path+".moved", os.Getpid()); err != nil {
			t.Fatalf("reserve %s: %v", file.Path, err)
		}
	}

	if _, err := db.SQL().Exec(`UPDATE movies SET best_file_id = NULL WHERE best_file_id = ?`, movieFile.ID); err == nil {
		t.Fatal("legacy movie best-file clear succeeded during move")
	}
	if _, err := db.SQL().Exec(`UPDATE episodes SET best_file_id = NULL WHERE best_file_id = ?`, episodeFile.ID); err == nil {
		t.Fatal("legacy episode best-file clear succeeded during move")
	}
	for _, file := range []*MediaFile{movieFile, episodeFile} {
		if _, err := db.SQL().Exec(`DELETE FROM media_files WHERE id = ?`, file.ID); err == nil {
			t.Fatalf("legacy delete of reserved media file %d succeeded", file.ID)
		}
		got, err := db.GetMediaFileByID(file.ID)
		if err != nil || got == nil || got.Path != file.Path {
			t.Fatalf("media row after legacy delete = %#v, error = %v; want row at %s", got, err, file.Path)
		}
	}
	var movieBestFileID, episodeBestFileID sql.NullInt64
	if err := db.SQL().QueryRow(`SELECT best_file_id FROM movies WHERE id = ?`, movie.ID).Scan(&movieBestFileID); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL().QueryRow(`SELECT best_file_id FROM episodes WHERE id = ?`, episode.ID).Scan(&episodeBestFileID); err != nil {
		t.Fatal(err)
	}
	if !movieBestFileID.Valid || movieBestFileID.Int64 != movieFile.ID {
		t.Fatalf("movie best-file ID after legacy clear = %v, want %d", movieBestFileID, movieFile.ID)
	}
	if !episodeBestFileID.Valid || episodeBestFileID.Int64 != episodeFile.ID {
		t.Fatalf("episode best-file ID after legacy clear = %v, want %d", episodeBestFileID, episodeFile.ID)
	}
}

func TestMediaFileMigrations(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Open database (should run migrations)
	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Verify schema version
	var version int
	err = db.db.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("failed to get schema version: %v", err)
	}

	if version != currentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", currentSchemaVersion, version)
	}

	// Verify tables exist
	tables := []string{"media_files", "episodes", "consolidation_plans"}
	for _, table := range tables {
		var count int
		err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist or is not accessible: %v", table, err)
		}
	}
}

func TestMediaFileCRUD(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Test insert
	year := 2014
	file := &MediaFile{
		Path:       "/test/Interstellar (2014)/Interstellar (2014).mkv",
		Size:       3500000000, // 3.5 GB
		ModifiedAt: time.Now(),

		MediaType:       "movie",
		NormalizedTitle: "interstellar",
		Year:            &year,

		Resolution:          "1080p",
		SourceType:          "BluRay",
		Codec:               "x264",
		AudioFormat:         "DTS",
		QualityScore:        305,
		IsJellyfinCompliant: true,
		ComplianceIssues:    []string{},

		Source:         "filesystem",
		SourcePriority: 50,
		LibraryRoot:    "/test",
	}

	err = db.UpsertMediaFile(file)
	if err != nil {
		t.Fatalf("failed to insert media file: %v", err)
	}

	if file.ID == 0 {
		t.Error("expected ID to be set after insert")
	}

	// Test retrieve
	retrieved, err := db.GetMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to retrieve media file: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve media file, got nil")
	}

	if retrieved.NormalizedTitle != file.NormalizedTitle {
		t.Errorf("expected normalized title %s, got %s", file.NormalizedTitle, retrieved.NormalizedTitle)
	}

	if retrieved.Resolution != file.Resolution {
		t.Errorf("expected resolution %s, got %s", file.Resolution, retrieved.Resolution)
	}

	// Test update
	file.QualityScore = 350
	file.Resolution = "2160p"

	err = db.UpsertMediaFile(file)
	if err != nil {
		t.Fatalf("failed to update media file: %v", err)
	}

	updated, err := db.GetMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to retrieve updated media file: %v", err)
	}

	if updated.QualityScore != 350 {
		t.Errorf("expected quality score 350, got %d", updated.QualityScore)
	}

	if updated.Resolution != "2160p" {
		t.Errorf("expected resolution 2160p, got %s", updated.Resolution)
	}

	// Test delete
	err = db.DeleteMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to delete media file: %v", err)
	}

	deleted, err := db.GetMediaFile(file.Path)
	if err != nil {
		t.Fatalf("failed to check deleted media file: %v", err)
	}

	if deleted != nil {
		t.Error("expected media file to be deleted, but it still exists")
	}
}

func TestGetLowConfidenceFilesUnderPathScopesToDirectoryAndExactFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	files := []*MediaFile{
		{
			Path:            "/library/A/low-a.mkv",
			Size:            1,
			ModifiedAt:      time.Now(),
			MediaType:       "movie",
			NormalizedTitle: "low a",
			Confidence:      0.2,
			Source:          "filesystem",
			SourcePriority:  50,
			LibraryRoot:     "/library",
		},
		{
			Path:            "/library/B/low-b.mkv",
			Size:            1,
			ModifiedAt:      time.Now(),
			MediaType:       "movie",
			NormalizedTitle: "low b",
			Confidence:      0.3,
			Source:          "filesystem",
			SourcePriority:  50,
			LibraryRoot:     "/library",
		},
		{
			Path:            "/library/A/high-a.mkv",
			Size:            1,
			ModifiedAt:      time.Now(),
			MediaType:       "movie",
			NormalizedTitle: "high a",
			Confidence:      0.99,
			Source:          "filesystem",
			SourcePriority:  50,
			LibraryRoot:     "/library",
		},
	}
	for _, file := range files {
		if err := db.UpsertMediaFile(file); err != nil {
			t.Fatalf("UpsertMediaFile(%s): %v", file.Path, err)
		}
	}

	dirScoped, err := db.GetLowConfidenceFilesUnderPath(0.8, 0, "/library/A")
	if err != nil {
		t.Fatalf("GetLowConfidenceFilesUnderPath dir: %v", err)
	}
	if len(dirScoped) != 1 || dirScoped[0].Path != "/library/A/low-a.mkv" {
		t.Fatalf("dir scoped files = %#v, want only /library/A/low-a.mkv", dirScoped)
	}

	fileScoped, err := db.GetLowConfidenceFilesUnderPath(0.8, 0, "/library/B/low-b.mkv")
	if err != nil {
		t.Fatalf("GetLowConfidenceFilesUnderPath file: %v", err)
	}
	if len(fileScoped) != 1 || fileScoped[0].Path != "/library/B/low-b.mkv" {
		t.Fatalf("file scoped files = %#v, want only /library/B/low-b.mkv", fileScoped)
	}
}

func TestGetLowConfidenceFilesIncludesParseMethod(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	file := &MediaFile{
		Path:            "/library/show/hash.mkv",
		Size:            1,
		ModifiedAt:      time.Now(),
		MediaType:       "episode",
		NormalizedTitle: "show",
		Confidence:      0.1,
		ParseMethod:     "folder",
		NeedsReview:     true,
		Source:          "filesystem",
		SourcePriority:  50,
		LibraryRoot:     "/library",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("UpsertMediaFile: %v", err)
	}

	files, err := db.GetLowConfidenceFiles(0.8, 0)
	if err != nil {
		t.Fatalf("GetLowConfidenceFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0].ParseMethod != "folder" {
		t.Fatalf("ParseMethod = %q, want folder", files[0].ParseMethod)
	}
	if !files[0].NeedsReview {
		t.Fatal("NeedsReview = false, want true")
	}
}

func TestUpsertEpisodeConflictResolvesExistingID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title: "Conflict ID Test", TitleNormalized: "conflict id test", Year: 2024,
		CanonicalPath: "/library/Conflict ID Test", LibraryRoot: "/library",
		Source: "filesystem", SourcePriority: 50,
	}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("UpsertSeries: %v", err)
	}

	first := &Episode{SeriesID: series.ID, Season: 1, Episode: 1, Title: "First"}
	if err := db.UpsertEpisode(first); err != nil {
		t.Fatalf("UpsertEpisode first: %v", err)
	}
	if first.ID == 0 {
		t.Fatal("first episode ID was not set")
	}

	// Leave an unrelated row as sqlite's last insert ID before the conflict update.
	unrelated := &Episode{SeriesID: series.ID, Season: 1, Episode: 2, Title: "Unrelated"}
	if err := db.UpsertEpisode(unrelated); err != nil {
		t.Fatalf("UpsertEpisode unrelated: %v", err)
	}

	retry := &Episode{SeriesID: series.ID, Season: 1, Episode: 1, Title: "First"}
	if err := db.UpsertEpisode(retry); err != nil {
		t.Fatalf("UpsertEpisode conflict: %v", err)
	}
	if retry.ID != first.ID {
		t.Fatalf("conflict ID = %d, want existing episode ID %d", retry.ID, first.ID)
	}
}

func TestUpsertMediaFileSetsExistingIDOnUpdate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	path := "/movies/Example (2026)/Example (2026).mkv"
	year := 2026
	first := &MediaFile{
		Path:            path,
		Size:            100,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "example",
		Year:            &year,
	}
	if err := db.UpsertMediaFile(first); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if first.ID == 0 {
		t.Fatalf("first upsert did not set ID")
	}

	second := &MediaFile{
		Path:            path,
		Size:            200,
		ModifiedAt:      time.Now(),
		MediaType:       "movie",
		NormalizedTitle: "example",
		Year:            &year,
	}
	if err := db.UpsertMediaFile(second); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("updated row ID = %d, want existing ID %d", second.ID, first.ID)
	}
}

func TestDuplicateDetection(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert duplicate movies
	year := 2013
	files := []*MediaFile{
		{
			Path:            "/storage1/Her (2013)/Her (2013).mkv",
			Size:            5500000000,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year,
			Resolution:      "1080p",
			SourceType:      "WEB-DL",
			QualityScore:    305,
			LibraryRoot:     "/storage1",
		},
		{
			Path:            "/storage8/Her (2013)/Her.2013.MULTI.1080p.BluRay.x264-Goatlove.mkv",
			Size:            9800000000,
			MediaType:       "movie",
			NormalizedTitle: "her",
			Year:            &year,
			Resolution:      "1080p",
			SourceType:      "BluRay",
			QualityScore:    380,
			LibraryRoot:     "/storage8",
		},
	}

	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("failed to insert media file: %v", err)
		}
	}

	// Find duplicates
	duplicates, err := db.FindDuplicateMovies()
	if err != nil {
		t.Fatalf("failed to find duplicate movies: %v", err)
	}

	if len(duplicates) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(duplicates))
	}

	group := duplicates[0]
	if group.NormalizedTitle != "her" {
		t.Errorf("expected normalized title 'her', got %s", group.NormalizedTitle)
	}

	if len(group.Files) != 2 {
		t.Errorf("expected 2 files in group, got %d", len(group.Files))
	}

	// Verify best file (highest quality score)
	if group.BestFile.QualityScore != 380 {
		t.Errorf("expected best file quality score 380, got %d", group.BestFile.QualityScore)
	}

	// Verify space reclaimable
	expectedReclaimable := int64(5500000000)
	if group.SpaceReclaimable != expectedReclaimable {
		t.Errorf("expected space reclaimable %d, got %d", expectedReclaimable, group.SpaceReclaimable)
	}
}

func TestEpisodeCRUD(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// First create a series
	year := 2023
	series := &Series{
		Title:           "Silo",
		TitleNormalized: "silo",
		Year:            year,
		CanonicalPath:   "/test/Silo (2023)",
		LibraryRoot:     "/test",
		Source:          "filesystem",
		SourcePriority:  50,
	}

	_, err = db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("failed to insert series: %v", err)
	}

	// Insert episode
	episode := &Episode{
		SeriesID: series.ID,
		Season:   1,
		Episode:  1,
		Title:    "Freedom Day",
	}

	err = db.UpsertEpisode(episode)
	if err != nil {
		t.Fatalf("failed to insert episode: %v", err)
	}

	if episode.ID == 0 {
		t.Error("expected episode ID to be set after insert")
	}

	// Retrieve episode
	retrieved, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("failed to retrieve episode: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected to retrieve episode, got nil")
	}

	if retrieved.Title != episode.Title {
		t.Errorf("expected title %s, got %s", episode.Title, retrieved.Title)
	}

	// Update best file
	fileID := int64(123)
	err = db.UpdateEpisodeBestFile(episode.ID, &fileID)
	if err != nil {
		t.Fatalf("failed to update episode best file: %v", err)
	}

	updated, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("failed to retrieve updated episode: %v", err)
	}

	if updated.BestFileID == nil || *updated.BestFileID != fileID {
		t.Errorf("expected best file ID %d, got %v", fileID, updated.BestFileID)
	}
}

func TestConsolidationStats(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test data
	year := 2014
	files := []*MediaFile{
		{
			Path:                "/test1/movie1.mkv",
			Size:                1000000000,
			MediaType:           "movie",
			NormalizedTitle:     "movie1",
			Year:                &year,
			QualityScore:        300,
			IsJellyfinCompliant: true,
			LibraryRoot:         "/test1",
		},
		{
			Path:                "/test2/movie1.mkv",
			Size:                2000000000,
			MediaType:           "movie",
			NormalizedTitle:     "movie1",
			Year:                &year,
			QualityScore:        350,
			IsJellyfinCompliant: true,
			LibraryRoot:         "/test2",
		},
		{
			Path:                "/test3/movie2.mkv",
			Size:                3000000000,
			MediaType:           "movie",
			NormalizedTitle:     "movie2",
			Year:                &year,
			QualityScore:        300,
			IsJellyfinCompliant: false,
			LibraryRoot:         "/test3",
		},
	}

	for _, f := range files {
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("failed to insert media file: %v", err)
		}
	}

	// Get stats
	stats, err := db.GetConsolidationStats()
	if err != nil {
		t.Fatalf("failed to get consolidation stats: %v", err)
	}

	if stats.TotalFiles != 3 {
		t.Errorf("expected 3 total files, got %d", stats.TotalFiles)
	}

	if stats.DuplicateGroups != 1 {
		t.Errorf("expected 1 duplicate group, got %d", stats.DuplicateGroups)
	}

	if stats.NonCompliantFiles != 1 {
		t.Errorf("expected 1 non-compliant file, got %d", stats.NonCompliantFiles)
	}

	if stats.SpaceReclaimable != 1000000000 {
		t.Errorf("expected space reclaimable 1000000000, got %d", stats.SpaceReclaimable)
	}
}

func TestMigration7NullableSourceFileID(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	query := `
		INSERT INTO consolidation_plans 
		(action, source_path, target_path, reason) 
		VALUES (?, ?, ?, ?)
	`
	_, err = db.db.Exec(query, "move", "/src/file.mkv", "/dst/file.mkv", "test")

	if err != nil {
		t.Errorf("Expected no error inserting plan without source_file_id, got: %v", err)
	}

	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM consolidation_plans WHERE source_path = ?", "/src/file.mkv").Scan(&count)
	if err != nil {
		t.Errorf("Failed to query consolidation_plans: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 consolidation plan, got %d", count)
	}
}

func TestMigration8AddConflictID(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "plex2jellyfin-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	db, err := OpenPath(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	query := `
		INSERT INTO consolidation_plans
		(action, source_path, target_path, reason, conflict_id)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err = db.db.Exec(query, "move", "/src/file.mkv", "/dst/file.mkv", "test", 123)

	if err != nil {
		t.Errorf("Expected no error inserting plan with conflict_id, got: %v", err)
	}

	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM consolidation_plans WHERE source_path = ?", "/src/file.mkv").Scan(&count)
	if err != nil {
		t.Errorf("Failed to query consolidation_plans: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 consolidation plan, got %d", count)
	}

	var conflictID int64
	err = db.db.QueryRow("SELECT conflict_id FROM consolidation_plans WHERE source_path = ?", "/src/file.mkv").Scan(&conflictID)
	if err != nil {
		t.Errorf("Failed to query conflict_id: %v", err)
	}

	if conflictID != 123 {
		t.Errorf("Expected conflict_id = 123, got %d", conflictID)
	}
}
