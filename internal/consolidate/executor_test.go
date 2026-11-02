package consolidate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
)

type fixedResultTransferer struct {
	result *transfer.TransferResult
}

func (f fixedResultTransferer) Move(string, string, transfer.TransferOptions) (*transfer.TransferResult, error) {
	return f.result, nil
}

func (f fixedResultTransferer) Copy(string, string, transfer.TransferOptions) (*transfer.TransferResult, error) {
	return f.result, nil
}

func (fixedResultTransferer) CanResume() bool { return false }
func (fixedResultTransferer) Name() string    { return "fixed-result" }

type racingDestinationTransferer struct {
	db             *database.MediaDB
	destinationRow *database.MediaFile
	writeDone      chan error
	writeErr       error
	moves          int
}

func (r *racingDestinationTransferer) Move(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	r.moves++
	if r.moves == 1 {
		go func() {
			r.writeDone <- r.db.UpsertMediaFile(r.destinationRow)
		}()
		r.writeErr = <-r.writeDone
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return nil, err
	}
	identity, ok := fileidentity.FromLstat(dst)
	if !ok {
		return nil, errors.New("no file identity")
	}
	destinationQuarantinePath, err := fileidentity.NewQuarantinePath(dst)
	if err != nil {
		return nil, err
	}
	if opts.OnPublished != nil {
		if err := opts.OnPublished(identity, destinationQuarantinePath); err != nil {
			return nil, err
		}
	}
	if err := fileidentity.MoveToQuarantine(src, opts.MoveSourceQuarantinePath, opts.MoveSourceIdentity, true); err != nil {
		return nil, err
	}
	if err := fileidentity.RemoveQuarantine(opts.MoveSourceQuarantinePath, opts.MoveSourceIdentity, true); err != nil {
		return nil, err
	}
	return &transfer.TransferResult{Success: true, SourceRemoved: true, Published: true, PublishedIdentity: identity, DestinationQuarantinePath: destinationQuarantinePath}, nil
}

func (r *racingDestinationTransferer) Copy(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	return r.Move(src, dst, opts)
}

func (*racingDestinationTransferer) CanResume() bool { return false }
func (*racingDestinationTransferer) Name() string    { return "racing-destination" }

func TestExecutorRejectsIncompleteTransferResultsBeforeDatabaseUpdates(t *testing.T) {
	responses := []struct {
		name   string
		result *transfer.TransferResult
	}{
		{name: "unsuccessful without error", result: &transfer.TransferResult{}},
		{name: "nil result"},
	}
	operations := []string{"move", "rename"}

	for _, response := range responses {
		for _, operation := range operations {
			t.Run(response.name+"/"+operation, func(t *testing.T) {
				db := setupTestDB(t)
				defer db.Close()

				root := t.TempDir()
				source := filepath.Join(root, "source.mkv")
				target := filepath.Join(root, "target.mkv")
				if err := os.WriteFile(source, []byte("source media"), 0o644); err != nil {
					t.Fatalf("create source: %v", err)
				}
				file := &database.MediaFile{
					Path:            source,
					Size:            int64(len("source media")),
					MediaType:       "movie",
					NormalizedTitle: "source media",
				}
				if err := db.UpsertMediaFile(file); err != nil {
					t.Fatalf("insert source media row: %v", err)
				}

				executor := NewExecutor(db, false, nil, nil)
				executor.transferer = fixedResultTransferer{result: response.result}
				plan := &ConsolidationPlan{SourcePath: source, TargetPath: target}
				var err error
				if operation == "move" {
					err = executor.executeMove(context.Background(), plan)
				} else {
					err = executor.executeRename(context.Background(), plan)
				}
				if err == nil {
					t.Fatal("executor accepted an unsuccessful transfer result")
				}
				if _, err := os.Stat(source); err != nil {
					t.Errorf("source file changed: %v", err)
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Errorf("target file exists, stat error = %v", err)
				}
				if got, err := db.GetMediaFile(source); err != nil || got == nil {
					t.Errorf("source database row = %+v, %v; want original row", got, err)
				}
				if got, err := db.GetMediaFile(target); err != nil || got != nil {
					t.Errorf("target database row = %+v, %v; want no row", got, err)
				}
			})
		}
	}
}

func TestNewExecutorFallsBackWhenRsyncIsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	executor := NewExecutor(nil, false, nil, nil)
	if executor.transferer == nil {
		t.Fatal("executor has no transferer when rsync is unavailable")
	}
	if got := executor.transferer.Name(); got != "native" {
		t.Fatalf("transferer = %q, want native fallback", got)
	}
}

func TestExecuteRename_CrossDevice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db, false, nil, nil)

	testDir := t.TempDir()
	srcPath := filepath.Join(testDir, "show.s01e01.mkv")
	dstPath := filepath.Join(testDir, "TV Shows/Test Show (2024)/Season 01/Test Show (2024) - S01E01.mkv")

	os.MkdirAll(filepath.Dir(srcPath), 0755)
	os.WriteFile(srcPath, []byte("test content"), 0644)

	file := &database.MediaFile{
		Path:            srcPath,
		NormalizedTitle: "show",
		Year:            intPtr(2024),
		MediaType:       "episode",
		Season:          intPtr(1),
		Episode:         intPtr(1),
	}
	db.UpsertMediaFile(file)

	plan := &ConsolidationPlan{
		SourcePath: srcPath,
		TargetPath: dstPath,
	}

	ctx := context.Background()
	err := executor.executeRename(ctx, plan)

	if err != nil {
		t.Errorf("executeRename failed: %v", err)
	}

	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("Destination file does not exist")
	}
	if _, err := os.Stat(srcPath); err == nil {
		t.Error("Source file still exists")
	}
}

func TestExecuteMoveRejectsTrackedDestinationBeforeTransfer(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
		t.Fatalf("create source: %v", err)
	}
	source := &database.MediaFile{Path: sourcePath, Size: 6, MediaType: "movie", NormalizedTitle: "source"}
	if err := db.UpsertMediaFile(source); err != nil {
		t.Fatalf("insert source row: %v", err)
	}
	target := &database.MediaFile{Path: targetPath, Size: 6, MediaType: "movie", NormalizedTitle: "existing target"}
	if err := db.UpsertMediaFile(target); err != nil {
		t.Fatalf("insert target row: %v", err)
	}

	executor := NewExecutor(db, false, nil, nil)
	err := executor.executeMove(context.Background(), &ConsolidationPlan{SourcePath: sourcePath, TargetPath: targetPath})
	if err == nil || !strings.Contains(err.Error(), "already tracked") {
		t.Fatalf("executeMove error = %v, want tracked destination rejection", err)
	}
	if _, err := os.Stat(sourcePath); err != nil {
		t.Fatalf("source file moved before database conflict was rejected: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("destination file exists after conflict, stat error = %v", err)
	}
	gotSource, err := db.GetMediaFile(sourcePath)
	if err != nil || gotSource == nil || gotSource.ID != source.ID {
		t.Fatalf("source row = %#v, error = %v; want preserved ID %d", gotSource, err, source.ID)
	}
	gotTarget, err := db.GetMediaFile(targetPath)
	if err != nil || gotTarget == nil || gotTarget.ID != target.ID || gotTarget.NormalizedTitle != "existing target" {
		t.Fatalf("target row = %#v, error = %v; want unchanged ID %d", gotTarget, err, target.ID)
	}
}

func TestExecutorDoesNotOverwriteExistingUntrackedDestination(t *testing.T) {
	for _, operation := range []string{"move", "rename"} {
		t.Run(operation, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			root := t.TempDir()
			sourcePath := filepath.Join(root, "source.mkv")
			targetPath := filepath.Join(root, "target.mkv")
			if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
				t.Fatalf("create source: %v", err)
			}
			if err := os.WriteFile(targetPath, []byte("keep target"), 0o644); err != nil {
				t.Fatalf("create existing target: %v", err)
			}
			if err := db.UpsertMediaFile(&database.MediaFile{Path: sourcePath, MediaType: "movie", NormalizedTitle: "source"}); err != nil {
				t.Fatalf("insert source row: %v", err)
			}

			executor := NewExecutor(db, false, nil, nil)
			var err error
			plan := &ConsolidationPlan{SourcePath: sourcePath, TargetPath: targetPath}
			if operation == "move" {
				err = executor.executeMove(context.Background(), plan)
			} else {
				err = executor.executeRename(context.Background(), plan)
			}
			if err == nil || !strings.Contains(err.Error(), "destination") {
				t.Fatalf("%s error = %v, want existing destination rejection", operation, err)
			}
			if _, err := os.Stat(sourcePath); err != nil {
				t.Fatalf("source changed: %v", err)
			}
			got, err := os.ReadFile(targetPath)
			if err != nil || string(got) != "keep target" {
				t.Fatalf("existing target = %q, error = %v; want original contents", got, err)
			}
		})
	}
}

func TestExecuteMoveRejectsConcurrentDestinationRegistration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	concurrentDB, err := database.OpenPath(db.Path())
	if err != nil {
		t.Fatalf("open concurrent database handle: %v", err)
	}
	defer concurrentDB.Close()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.mkv")
	targetPath := filepath.Join(root, "target.mkv")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
		t.Fatalf("create source: %v", err)
	}
	source := &database.MediaFile{Path: sourcePath, MediaType: "movie", NormalizedTitle: "source"}
	if err := db.UpsertMediaFile(source); err != nil {
		t.Fatalf("insert source row: %v", err)
	}

	executor := NewExecutor(db, false, nil, nil)
	transferer := &racingDestinationTransferer{
		db:             concurrentDB,
		destinationRow: &database.MediaFile{Path: targetPath, MediaType: "movie", NormalizedTitle: "concurrent target"},
		writeDone:      make(chan error, 1),
	}
	executor.transferer = transferer
	err = executor.executeMove(context.Background(), &ConsolidationPlan{SourcePath: sourcePath, TargetPath: targetPath})
	if err != nil {
		t.Fatalf("executeMove: %v", err)
	}
	if !errors.Is(transferer.writeErr, database.ErrMediaFileMoveInProgress) {
		t.Fatalf("concurrent destination upsert error = %v, want ErrMediaFileMoveInProgress", transferer.writeErr)
	}
	if _, err := os.Lstat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source file remains after move, stat error = %v", err)
	}
	if got, err := os.ReadFile(targetPath); err != nil || string(got) != "source" {
		t.Fatalf("target file = %q, error = %v; want moved source file", got, err)
	}
	got, err := db.GetMediaFile(targetPath)
	if err != nil || got == nil || got.ID != source.ID {
		t.Fatalf("target row = %#v, error = %v; want original ID %d", got, err, source.ID)
	}
	if got, err := db.GetMediaFile(sourcePath); err != nil || got != nil {
		t.Fatalf("source row = %#v, error = %v; want no stale source row", got, err)
	}
}

func TestExecuteMoveUpdatesEpisodeIdentityAndCanonicalPath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "storage2", "TV")
	targetRoot := filepath.Join(root, "storage1", "TV")
	sourceSeriesPath := filepath.Join(sourceRoot, "Show (2024)")
	targetSeriesPath := filepath.Join(targetRoot, "Show (2024)")
	sourcePath := filepath.Join(sourceSeriesPath, "Season 01", "Show.S01E01.mkv")
	targetPath := filepath.Join(targetSeriesPath, "Season 01", "Show.S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("episode"), 0o644); err != nil {
		t.Fatalf("create source file: %v", err)
	}

	series := &database.Series{
		Title: "Show", Year: 2024, CanonicalPath: sourceSeriesPath,
		LibraryRoot: sourceRoot, Source: "plex2jellyfin", SourcePriority: 100,
	}
	if _, err := db.UpsertSeries(series); err != nil {
		t.Fatalf("insert series: %v", err)
	}
	episode := &database.Episode{SeriesID: series.ID, Season: 1, Episode: 1, Title: "Episode 1"}
	if err := db.UpsertEpisode(episode); err != nil {
		t.Fatalf("insert episode: %v", err)
	}
	file := &database.MediaFile{
		Path: sourcePath, Size: 7, MediaType: "episode", ParentSeriesID: &series.ID,
		ParentEpisodeID: &episode.ID, NormalizedTitle: "show", LibraryRoot: sourceRoot,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("insert media file: %v", err)
	}
	mediaFileID := file.ID
	if err := db.UpdateEpisodeBestFile(episode.ID, &mediaFileID); err != nil {
		t.Fatalf("set episode best file: %v", err)
	}

	executor := NewExecutor(db, false, nil, &config.Config{Libraries: config.LibrariesConfig{TV: []string{targetRoot}}})
	if err := executor.executeMove(context.Background(), &ConsolidationPlan{SourcePath: sourcePath, TargetPath: targetPath}); err != nil {
		t.Fatalf("executeMove: %v", err)
	}

	movedFile, err := db.GetMediaFile(targetPath)
	if err != nil || movedFile == nil || movedFile.ID != mediaFileID || movedFile.LibraryRoot != targetRoot {
		t.Fatalf("moved media file = %#v, error = %v; want ID %d and root %q", movedFile, err, mediaFileID, targetRoot)
	}
	updatedSeries, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("read moved series: %v", err)
	}
	if updatedSeries.CanonicalPath != targetSeriesPath || updatedSeries.LibraryRoot != targetRoot || !updatedSeries.SonarrPathDirty {
		t.Fatalf("series path/root/dirty = %q/%q/%v; want %q/%q/true", updatedSeries.CanonicalPath, updatedSeries.LibraryRoot, updatedSeries.SonarrPathDirty, targetSeriesPath, targetRoot)
	}
	updatedEpisode, err := db.GetEpisode(series.ID, 1, 1)
	if err != nil {
		t.Fatalf("read moved episode: %v", err)
	}
	if updatedEpisode.BestFileID == nil || *updatedEpisode.BestFileID != mediaFileID {
		t.Fatalf("episode best file ID = %v, want preserved ID %d", updatedEpisode.BestFileID, mediaFileID)
	}
}

func intPtr(i int) *int {
	return &i
}
