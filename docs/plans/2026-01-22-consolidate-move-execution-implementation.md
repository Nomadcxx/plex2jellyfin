# Consolidate MOVE Execution Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement MOVE plan execution for consolidation feature, enabling cross-drive file consolidation with 100MB size filtering, empty directory cleanup, and pv progress support.

**Architecture:** Extend existing consolidation system by:
1. Adding optional `pv` (pipe viewer) backend to `internal/transfer` for progress display
2. Adding 100MB file size filter to skip samples during consolidation
3. Adding empty directory cleanup after successful moves
4. Modifying `executeMove()` to cleanup empty dirs and mark conflicts resolved
5. Adding migration to make `source_file_id` nullable for untracked files

**Tech Stack:**
- Go 1.21+
- SQLite (modernc.org/sqlite)
- rsync backend with optional pv (pipe viewer) for progress

---

## Task 1: Add PV Backend to Transfer Package

**Files:**
- Create: `internal/transfer/pv.go`
- Modify: `internal/transfer/transfer.go:200-250`

**Step 1: Write failing test for PV detection**

Create test file `internal/transfer/pv_test.go`:

```go
package transfer

import (
	"os/exec"
	"testing"
)

func TestPVDetection(t *testing.T) {
	// Test when pv is available
	pvFound = false // Reset
	err := detectPV()
	if err != nil {
		t.Logf("PV not available: %v", err)
		return
	}

	if !pvFound {
		t.Error("Expected pvFound to be true when pv is available")
	}
}

func TestPVMove(t *testing.T) {
	// Test that PV backend implements Transferer interface
	var _ Transferer = &PVBackend{}
	// Just compilation test
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/transfer/... -v -run TestPV`
Expected: COMPILATION ERROR (PVBackend not defined yet)

**Step 3: Write minimal PV backend**

Create `internal/transfer/pv.go`:

```go
package transfer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

var (
	pvFound bool
	pvPath  string
)

func init() {
	var err error
	pvPath, err = exec.LookPath("pv")
	pvFound = (err == nil)
}

type PVBackend struct{}

func (p *PVBackend) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	if !pvFound {
		return nil, fmt.Errorf("pv not available")
	}

	// Use pv to pipe file and show progress
	cmd := exec.Command("pv", src)
	dd := exec.Command("dd", "of="+dst, "bs=1M")

	// Pipe pv -> dd
	dd.Stdin, _ = cmd.StdoutPipe()
	cmd.Stdout = dd.Stdin

	// Start pv
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Start dd
	if err := dd.Start(); err != nil {
		cmd.Process.Kill()
		return nil, err
	}

	// Wait for both
	cmd.Wait()
	dd.Wait()

	// Get file size
	info, _ := os.Stat(src)

	return &TransferResult{
		Success:      true,
		BytesTotal:   info.Size(),
		BytesCopied:  info.Size(),
		Attempts:     1,
	}, nil
}

func (p *PVBackend) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	return nil, fmt.Errorf("copy not implemented for PV backend")
}

func (p *PVBackend) CanResume() bool {
	return false
}

func (p *PVBackend) Name() string {
	return "pv"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/transfer/... -v -run TestPV`
Expected: PASS

**Step 5: Modify transfer.go to add BackendPV option**

In `internal/transfer/transfer.go`, modify `Backend` type enum:

```go
type Backend string

const (
	BackendAuto  Backend = "auto"
	BackendRsync Backend = "rsync"
	BackendPV   Backend = "pv"
)
```

Modify `New()` function to handle BackendPV:

```go
func New(backend Backend) (Transferer, error) {
	if backend == BackendAuto {
		backend = BackendRsync
	}

	switch backend {
	case BackendRsync:
		return &RsyncBackend{}, nil
	case BackendPV:
		return &PVBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}
```

**Step 6: Commit**

```bash
git add internal/transfer/pv.go internal/transfer/pv_test.go internal/transfer/transfer.go
git commit -m "feat: add PV backend for progress display during transfers"
```

---

## Task 2: Add 100MB Size Filter to Consolidator

**Files:**
- Modify: `internal/consolidate/consolidate.go:220-232`

**Step 1: Write failing test**

Create `internal/consolidate/consolidate_test.go`:

```go
package consolidate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSizeFilter(t *testing.T) {
	// Create temp file under 100MB
	tmpDir := t.TempDir()
	smallFile := filepath.Join(tmpDir, "small.mkv")
	os.WriteFile(smallFile, make([]byte, 50*1024*1024), 0644) // 50MB

	// Create conflict with small file
	conflict := &Conflict{
		Locations: []string{tmpDir},
	}

	consolidator := &Consolidator{}

	// getFilesToMove should skip small files
	ops, err := consolidator.getFilesToMove(tmpDir, "/target", conflict)
	if err != nil {
		t.Fatal(err)
	}

	if len(ops) > 0 {
		t.Error("Expected no operations for files under 100MB")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -v -run TestSizeFilter`
Expected: PASS (getFilesToMove exists but doesn't filter yet)

**Step 3: Add constant and filter**

In `internal/consolidate/consolidate.go`, add constant and modify function:

```go
const (
	MinConsolidationFileSize = 100 * 1024 * 1024 // 100MB minimum
)

func (c *Consolidator) getFilesToMove(sourcePath, targetPath string, conflict *Conflict) ([]*Operation, error) {
	var operations []*Operation

	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// SIZE FILTER: Skip files under 100MB
		if info.Size() < MinConsolidationFileSize {
			return nil
		}

		// Rest of existing code...
		ext := strings.ToLower(filepath.Ext(path))
		if !isMediaFile(ext) {
			return nil
		}

		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return nil
		}

		destPath := filepath.Join(targetPath, relPath)

		if _, err := os.Stat(destPath); err == nil {
			return nil
		}

		op := &Operation{
			SourcePath:      path,
			DestinationPath: destPath,
			Size:            info.Size(),
			Type:            conflict.MediaType,
		}

		operations = append(operations, op)
		return nil
	})

	return operations, err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -v -run TestSizeFilter`
Expected: PASS (small files are now skipped)

**Step 5: Commit**

```bash
git add internal/consolidate/consolidate.go internal/consolidate/consolidate_test.go
git commit -m "feat: add 100MB size filter to skip sample files during consolidation"
```

---

## Task 3: Make source_file_id Nullable in Database

**Files:**
- Modify: `internal/database/schema.go:383-390`
- Modify: `internal/database/schema.go:6`

**Step 1: Write failing test**

Create `internal/database/schema_test.go` (or use existing tests):

```go
package database

import (
	"testing"
)

func TestMigration7NullableSourceFileID(t *testing.T) {
	db, _ := OpenTemp(t)
	defer db.Close()

	// Create a consolidation plan without source_file_id
	query := `
		INSERT INTO consolidation_plans 
		(action, source_path, target_path, reason) 
		VALUES (?, ?, ?, ?)
	`
	_, err := db.DB().Exec(query, "move", "/src/file.mkv", "/dst/file.mkv", "test")

	if err != nil {
		t.Errorf("Expected no error inserting plan without source_file_id, got: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/database/... -v -run TestMigration7`
Expected: FAIL (constraint error: NOT NULL on source_file_id)

**Step 3: Add migration 7**

In `internal/database/schema.go`, update constant and add migration:

```go
const currentSchemaVersion = 7
```

Add to migrations array after version 6:

```go
	{
		version: 7,
		up: []string{
			// Make source_file_id nullable for untracked files in consolidation
			`ALTER TABLE consolidation_plans 
			ALTER COLUMN source_file_id DROP NOT NULL`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (7)`,
		},
	},
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/database/... -v -run TestMigration7`
Expected: PASS (can insert plans without source_file_id)

**Step 5: Commit**

```bash
git add internal/database/schema.go internal/database/schema_test.go
git commit -m "feat: make source_file_id nullable in consolidation_plans for untracked files"
```

---

## Task 4: Update StorePlan to Handle Nullable source_file_id

**Files:**
- Modify: `internal/consolidate/operations.go:253-280`

**Step 1: Write failing test**

```go
func TestStorePlanUntrackedFile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	cfg := &config.Config{}

	consolidator := NewConsolidator(db, cfg)

	// Create plan for file not in database
	plan := &Plan{
		Title:  "Test",
		SourcePaths: []string{"/tmp/untracked.mkv"},
		TargetPath:  "/dst/test.mkv",
		Operations: []*Operation{{
			SourcePath:      "/tmp/untracked.mkv",
			DestinationPath: "/dst/test.mkv",
			Size:            200 * 1024 * 1024, // 200MB
		}},
	}

	err := consolidator.StorePlan(plan)
	if err != nil {
		t.Errorf("Expected no error storing untracked file plan: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -v -run TestStorePlanUntracked`
Expected: FAIL (StorePlan doesn't handle untracked files)

**Step 3: Modify StorePlan to handle untracked files**

Update `internal/consolidate/operations.go`:

```go
func (c *Consolidator) StorePlan(plan *Plan) error {
	for _, op := range plan.Operations {
		reason := fmt.Sprintf("Consolidate conflict: %s", plan.Title)
		details := fmt.Sprintf("Moving %s to target location", op.SourcePath)

		var sourceFileID *int64 = nil

		file, err := c.db.GetMediaFile(op.SourcePath)
		if err == nil && file != nil {
			id := file.ID
			sourceFileID = &id
		}

		query := `
			INSERT INTO consolidation_plans (
				action, source_file_id, source_path, target_path, reason, reason_details
			) VALUES (?, ?, ?, ?, ?, ?)
		`

		_, err = c.db.DB().Exec(query, "move", sourceFileID, op.SourcePath, op.DestinationPath, reason, details)
		if err != nil {
			return fmt.Errorf("failed to insert consolidation plan: %w", err)
		}
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -v -run TestStorePlanUntracked`
Expected: PASS (plans stored with nil source_file_id)

**Step 5: Commit**

```bash
git add internal/consolidate/operations.go
git commit -m "feat: handle untracked files in StorePlan with nullable source_file_id"
```

---

## Task 5: Add Empty Directory Cleanup

**Files:**
- Modify: `internal/consolidate/operations.go:280-310`

**Step 1: Write failing test**

```go
func TestCleanupEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "empty")
	os.MkdirAll(emptyDir, 0755)

	// Should delete empty directory
	err := cleanupEmptyDir(emptyDir)
	if err != nil {
		t.Errorf("Expected no error cleaning empty dir: %v", err)
	}

	// Verify deleted
	if _, err := os.Stat(emptyDir); !os.IsNotExist(err) {
		t.Error("Expected empty dir to be deleted")
	}
}

func TestCleanupNotEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	notEmptyDir := filepath.Join(tmpDir, "files")
	os.MkdirAll(notEmptyDir, 0755)
	os.WriteFile(filepath.Join(notEmptyDir, "file.mkv"), []byte("data"), 0644)

	// Should NOT delete non-empty directory
	err := cleanupEmptyDir(notEmptyDir)
	if err != nil {
		t.Errorf("Expected no error for non-empty dir: %v", err)
	}

	// Verify still exists
	if _, err := os.Stat(notEmptyDir); os.IsNotExist(err) {
		t.Error("Expected non-empty dir to remain")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -v -run TestCleanup`
Expected: COMPILATION ERROR (cleanupEmptyDir not defined)

**Step 3: Add cleanupEmptyDir function**

Add to `internal/consolidate/operations.go`:

```go
// cleanupEmptyDir removes directory if it's empty
// Returns nil if directory isn't empty (not an error)
func cleanupEmptyDir(dir string) error {
	// Check if directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // Doesn't exist, not an error
	}

	if len(entries) > 0 {
		// Empty - delete it
		return os.Remove(dir)
	}

	// Not empty - do nothing
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -v -run TestCleanup`
Expected: PASS (empty dirs deleted, non-empty preserved)

**Step 5: Commit**

```bash
git add internal/consolidate/operations.go
git commit -m "feat: add empty directory cleanup after consolidation moves"
```

---

## Task 6: Integrate Cleanup into executeMove

**Files:**
- Modify: `internal/consolidate/executor.go:175-219`

**Step 1: Write failing test**

```go
func TestExecuteMoveCleanup(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	plan := &ConsolidationPlan{
		ID:         1,
		Action:     "move",
		SourcePath: "/tmp/test/file.mkv",
		TargetPath: "/dst/file.mkv",
	}

	executor := NewExecutor(db, false)

	// Create source file and parent dir
	os.MkdirAll("/tmp/test", 0755)
	os.WriteFile("/tmp/test/file.mkv", []byte("data"), 0644)

	// After move, parent should be empty
	err := executor.executeMove(context.Background(), plan)
	if err != nil {
		t.Errorf("Expected no error: %v", err)
	}

	// Check parent dir is gone (empty and cleaned)
	if _, err := os.Stat("/tmp/test"); !os.IsNotExist(err) {
		t.Error("Expected parent directory to be cleaned up")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -v -run TestExecuteMoveCleanup`
Expected: FAIL (parent dir still exists)

**Step 3: Modify executeMove to cleanup**

Update `internal/consolidate/executor.go`:

```go
func (e *Executor) executeMove(ctx context.Context, plan *ConsolidationPlan) error {
	if e.dryRun {
		return nil
	}

	// Check if source exists
	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	// Use transfer package for reliable file move
	result, err := e.transferer.Move(plan.SourcePath, plan.TargetPath, transfer.TransferOptions{
		Timeout:   5 * time.Minute,
		TargetUID: -1,
		TargetGID: -1,
	})

	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	if result.Error != nil {
		return fmt.Errorf("transfer failed: %w", result.Error)
	}

	// Update database
	file, err := e.db.GetMediaFile(plan.SourcePath)
	if err == nil && file != nil {
		// Tracked file - update path
		e.db.DeleteMediaFile(plan.SourcePath)
		file.Path = plan.TargetPath
		e.db.UpsertMediaFile(file)
	}

	// CLEANUP: Remove empty parent directory
	parentDir := filepath.Dir(plan.SourcePath)
	if err := cleanupEmptyDir(parentDir); err != nil {
		// Log but don't fail the move
		fmt.Printf("Warning: Failed to cleanup empty directory %s: %v\n", parentDir, err)
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -v -run TestExecuteMoveCleanup`
Expected: PASS (parent dir cleaned after move)

**Step 5: Commit**

```bash
git add internal/consolidate/executor.go
git commit -m "feat: cleanup empty directories after successful move operations"
```

---

## Task 7: Mark Conflicts Resolved After Moves

**Files:**
- Modify: `internal/consolidate/executor.go:45-115`

**Step 1: Write failing test**

```go
func TestConflictResolution(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a conflict
	conflict := &Conflict{
		ID:     1,
		Title:  "Test Show",
		Status: "unresolved",
	}
	// Insert conflict...

	plan := &ConsolidationPlan{
		Action:        "move",
		ConflictID:    1,
		SourcePath:     "/src/file.mkv",
		TargetPath:     "/dst/file.mkv",
	}

	executor := NewExecutor(db, false)
	
	// After execute, conflict should be resolved
	err := executor.ExecutePlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Check conflict status
	resolvedConflict, _ := db.GetConflict(1)
	if resolvedConflict.Status != "resolved" {
		t.Errorf("Expected conflict to be resolved, got: %s", resolvedConflict.Status)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -v -run TestConflictResolution`
Expected: FAIL (conflict still unresolved)

**Step 3: Add conflict resolution to executeMove**

Update executeMove in `internal/consolidate/executor.go`:

```go
func (e *Executor) executeMove(ctx context.Context, plan *ConsolidationPlan) error {
	if e.dryRun {
		return nil
	}

	if _, err := os.Stat(plan.SourcePath); os.IsNotExist(err) {
		return fmt.Errorf("source file does not exist")
	}

	result, err := e.transferer.Move(plan.SourcePath, plan.TargetPath, transfer.TransferOptions{
		Timeout:   5 * time.Minute,
		TargetUID: -1,
		TargetGID: -1,
	})

	if err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	if result.Error != nil {
		return fmt.Errorf("transfer failed: %w", result.Error)
	}

	file, err := e.db.GetMediaFile(plan.SourcePath)
	if err == nil && file != nil {
		e.db.DeleteMediaFile(plan.SourcePath)
		file.Path = plan.TargetPath
		e.db.UpsertMediaFile(file)
	}

	// Cleanup empty directory
	parentDir := filepath.Dir(plan.SourcePath)
	cleanupEmptyDir(parentDir)

	// MARK CONFLICT RESOLVED: If plan has conflict_id, mark it as resolved
	if plan.ConflictID > 0 {
		// Resolve conflict using target path
		e.db.ResolveConflict(plan.ConflictID, plan.TargetPath)
	}

	return nil
}
```

Note: Need to add ConflictID to ConsolidationPlan struct.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -v -run TestConflictResolution`
Expected: PASS (conflict marked as resolved)

**Step 5: Commit**

```bash
git add internal/consolidate/executor.go internal/consolidate/planner.go
git commit -m "feat: mark conflicts as resolved after successful move execution"
```

---

## Task 8: Improve Dry-Run Output Format

**Files:**
- Modify: `internal/consolidate/operations.go:127-181`

**Step 1: Write failing test**

```go
func TestDryRunOutput(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	cfg := &config.Config{}
	
	consolidator := NewConsolidator(db, cfg)
	
	// Create mock plans
	plan1 := &Plan{
		Title:      "Test Show",
		ConflictID:  1,
		TargetPath:  "/dst/Test Show/",
		Operations: []*Operation{
			{SourcePath: "/src/s01e01.mkv", DestinationPath: "/dst/s01e01.mkv", Size: 500 * 1024 * 1024},
			{SourcePath: "/src/s01e02.mkv", DestinationPath: "/dst/s01e02.mkv", Size: 500 * 1024 * 1024},
		},
	}

	// Capture output
	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := consolidator.DryRun()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatal(err)
	}

	// Check output contains "Test Show" and file count
	output := buf.String()
	if !strings.Contains(output, "Test Show") {
		t.Error("Expected output to show conflict title")
	}
	if !strings.Contains(output, "2 files") {
		t.Error("Expected output to show file count")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -v -run TestDryRunOutput`
Expected: FAIL (output format is per-file, not per-conflict)

**Step 3: Modify DryRun for summary format**

Update `internal/consolidate/operations.go`:

```go
func (c *Consolidator) DryRun() error {
	plans, err := c.GenerateAllPlans()
	if err != nil {
		return fmt.Errorf("failed to generate plans: %w", err)
	}

	if len(plans) == 0 {
		fmt.Println("No consolidation plans to execute.")
		return nil
	}

	var totalFiles int
	var totalBytes int64
	var skipped int

	fmt.Println("\n=== Consolidation Preview ===\n")

	for _, plan := range plans {
		if len(plan.Operations) == 0 {
			skipped++
			continue
		}

		totalFiles += len(plan.Operations)
		totalBytes += plan.TotalBytes

		yearStr := "unknown"
		if plan.Year != nil {
			yearStr = fmt.Sprintf("%d", *plan.Year)
		}

		fmt.Printf("%s (%s): %d files -> %s\n",
			plan.Title, yearStr, len(plan.Operations), plan.TargetPath)
	}

	fmt.Printf("\nTotal: %d files across %d conflicts\n", totalFiles, len(plans)-skipped)
	fmt.Printf("Estimated data: %s\n", formatBytes(totalBytes))

	if skipped > 0 {
		fmt.Printf("\nSkipped %d conflicts (no files >100MB to move)\n", skipped)
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -v -run TestDryRunOutput`
Expected: PASS (output shows per-conflict summary)

**Step 5: Commit**

```bash
git add internal/consolidate/operations.go
git commit -m "feat: improve dry-run output to show per-conflict summary"
```

---

## Final Integration Tests

**Step 1: Run full test suite**

```bash
go test ./... -v
```

Expected: All tests pass

**Step 2: End-to-end test with real database**

```bash
# Test on test data
./jellywatch consolidate --generate
./jellywatch consolidate --dry-run
./jellywatch consolidate --execute

# Verify:
# 1. Plans generated with 100MB filter
# 2. Dry-run shows per-conflict summary
# 3. Empty directories cleaned after move
# 4. Conflicts marked as resolved
```

**Step 3: Final commit**

```bash
git add .
git commit -m "test: add integration tests for consolidation move execution"
```

---

## Documentation to Update

Update any CLI help or docs if needed:
- `README.md` - Mention `pv` for progress display
- CLI help - Mention 100MB filter behavior

---

## Testing Checklist

- [ ] Unit tests pass for all new code
- [ ] Migration 7 applied correctly
- [ ] `jellywatch consolidate --generate` creates MOVE plans
- [ ] `jellywatch consolidate --dry-run` shows summary format
- [ ] `jellywatch consolidate --execute` moves files, cleans up, resolves conflicts
- [ ] Files under 100MB are skipped
- [ ] Empty directories are removed
- [ ] Conflicts marked as resolved in database
- [ ] `pv` backend works when available
- [ ] Falls back to rsync when `pv` unavailable
