package consolidate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

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
