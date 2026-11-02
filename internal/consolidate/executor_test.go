package consolidate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
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

				executor := NewExecutor(db, false, nil)
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

	executor := NewExecutor(nil, false, nil)
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

	executor := NewExecutor(db, false, nil)

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

func intPtr(i int) *int {
	return &i
}
