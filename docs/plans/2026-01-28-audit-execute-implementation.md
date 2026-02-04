# Audit --execute Actions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement audit command --execute flag with rename and delete actions

**Architecture:** Extend internal/plans package with ExecuteAuditAction() function. File-first pattern: move file on disk first, then update database. Keeps database as source of truth.

**Tech Stack:** Go, SQLite, existing transfer package, existing plans package

---

## Task 1: Add ExecuteAuditAction Skeleton

**Files:**
- Modify: `internal/plans/plans.go` (after ArchiveAuditPlans function)
- Test: `internal/plans/plans_test.go`

**Step 1: Write the failing test**

Add to `internal/plans/plans_test.go`:

```go
func TestExecuteAuditAction_Rename(t *testing.T) {
	// Setup test database
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert test file
	_, err := db.Exec(`INSERT INTO media_files (path, size, modified_at, media_type,
		normalized_title, source, source_priority, library_root, confidence)
		VALUES (?, 1000, datetime('now'), 'movie', 'Old Title', 'test', 50, '/test', 0.5)`,
		"/test/Old.Title.2020.mkv")
	if err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	// Create temp file to rename
	tempDir := t.TempDir()
	oldPath := filepath.Join(tempDir, "Old.Title.2020.mkv")
	if err := os.WriteFile(oldPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create action
	newTitle := "New Title"
	year := 2020
	action := AuditAction{
		Action:     "rename",
		NewPath:    filepath.Join(tempDir, "New Title (2020).mkv"),
		NewTitle:   newTitle,
		NewYear:    &year,
		Confidence: 0.9,
	}

	// Execute action
	err := ExecuteAuditAction(db, action)
	if err != nil {
		t.Fatalf("ExecuteAuditAction failed: %v", err)
	}

	// Verify file was renamed
	if _, err := os.Stat(action.NewPath); os.IsNotExist(err) {
		t.Errorf("New file should exist: %s", action.NewPath)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("Old file should not exist: %s", oldPath)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plans -run TestExecuteAuditAction_Rename -v`
Expected: FAIL with "undefined: ExecuteAuditAction"

**Step 3: Add skeleton function**

Add to `internal/plans/plans.go` after `ArchiveAuditPlans()`:

```go
// ExecuteAuditAction executes a single audit action (rename or delete)
func ExecuteAuditAction(db *database.MediaDB, action AuditAction) error {
	return fmt.Errorf("not implemented")
}
```

Add required import if missing:
```go
import (
	"github.com/Nomadcxx/jellywatch/internal/database"
)
```

**Step 4: Run test to verify different failure**

Run: `go test ./internal/plans -run TestExecuteAuditAction_Rename -v`
Expected: FAIL with "not implemented"

**Step 5: Commit**

```bash
git add internal/plans/plans_test.go internal/plans/plans.go
git commit -m "feat(plans): add ExecuteAuditAction skeleton"
```

---

## Task 2: Implement executeRename Helper

**Files:**
- Modify: `internal/plans/plans.go`
- Test: `internal/plans/plans_test.go`

**Step 1: Add database helper first**

Check if database has `UpdateMediaFile` method. If not, add to `internal/database/media_files.go`:

```go
// UpdateMediaFile updates a media file's path and metadata
func (db *MediaDB) UpdateMediaFile(id int64, path string, title string, year *int, season *int, episode *int, confidence float64) error {
	query := `
		UPDATE media_files
		SET path = ?, normalized_title = ?, year = ?, season = ?, episode = ?,
		    confidence = ?, parse_method = 'ai', needs_review = 0,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := db.db.Exec(query, path, title, year, season, episode, confidence, id)
	return err
}
```

**Step 2: Implement executeRename**

Add to `internal/plans/plans.go`:

```go
// executeRename handles rename action: move file, then update DB
func executeRename(db *database.MediaDB, action AuditAction) error {
	// Find the file in database by path
	file, err := db.GetMediaFile(action.Path)
	if err != nil {
		return fmt.Errorf("failed to find file in database: %w", err)
	}
	if file == nil {
		return fmt.Errorf("file not found in database: %s", action.Path)
	}

	// Move file on disk first (file-first pattern)
	if err := os.Rename(action.Path, action.NewPath); err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	// Update database with new path and metadata
	if err := db.UpdateMediaFile(file.ID, action.NewPath, action.NewTitle,
		action.NewYear, action.NewSeason, action.NewEpisode, action.Confidence); err != nil {
		// File moved but DB update failed - log warning
		return fmt.Errorf("file moved but database update failed: %w", err)
	}

	return nil
}
```

**Step 3: Update ExecuteAuditAction to use executeRename**

```go
func ExecuteAuditAction(db *database.MediaDB, action AuditAction) error {
	switch action.Action {
	case "rename":
		return executeRename(db, action)
	case "delete":
		return fmt.Errorf("delete not yet implemented")
	default:
		return fmt.Errorf("unknown action: %s", action.Action)
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/plans -run TestExecuteAuditAction_Rename -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plans/plans.go internal/database/media_files.go
git commit -m "feat(plans): implement executeRename helper"
```

---

## Task 3: Implement executeDelete Helper

**Files:**
- Modify: `internal/plans/plans.go`
- Test: `internal/plans/plans_test.go`

**Step 1: Write failing test**

Add to `internal/plans/plans_test.go`:

```go
func TestExecuteAuditAction_Delete(t *testing.T) {
	// Setup test database
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create temp file
	tempDir := t.TempDir()
	testPath := filepath.Join(tempDir, "test.mkv")
	if err := os.WriteFile(testPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert test file
	_, err := db.Exec(`INSERT INTO media_files (path, size, modified_at, media_type,
		normalized_title, source, source_priority, library_root, confidence)
		VALUES (?, 1000, datetime('now'), 'movie', 'Test', 'test', 50, '/test', 0.5)`,
		testPath)
	if err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	// Create action
	action := AuditAction{
		Action:     "delete",
		Path:       testPath,
		Confidence: 0.9,
	}

	// Execute action
	err = ExecuteAuditAction(db, action)
	if err != nil {
		t.Fatalf("ExecuteAuditAction failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(testPath); !os.IsNotExist(err) {
		t.Errorf("File should be deleted: %s", testPath)
	}

	// Verify removed from database
	file, _ := db.GetMediaFile(testPath)
	if file != nil {
		t.Errorf("File should be removed from database: %s", testPath)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plans -run TestExecuteAuditAction_Delete -v`
Expected: FAIL with "delete not yet implemented"

**Step 3: Implement executeDelete**

Add to `internal/plans/plans.go`:

```go
// executeDelete handles delete action: delete file, then remove from DB
func executeDelete(db *database.MediaDB, action AuditAction) error {
	// Delete file on disk first
	if err := os.Remove(action.Path); err != nil {
		if os.IsNotExist(err) {
			// File already gone, that's fine
		} else {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}

	// Remove from database
	if err := db.DeleteMediaFile(action.Path); err != nil {
		return fmt.Errorf("file deleted but failed to remove from database: %w", err)
	}

	return nil
}
```

**Step 4: Update ExecuteAuditAction switch**

```go
func ExecuteAuditAction(db *database.MediaDB, action AuditAction) error {
	switch action.Action {
	case "rename":
		return executeRename(db, action)
	case "delete":
		return executeDelete(db, action)
	default:
		return fmt.Errorf("unknown action: %s", action.Action)
	}
}
```

**Step 5: Run tests**

Run: `go test ./internal/plans -run TestExecuteAuditAction -v`
Expected: PASS (both rename and delete tests)

**Step 6: Commit**

```bash
git add internal/plans/plans.go internal/plans/plans_test.go
git commit -m "feat(plans): implement executeDelete helper"
```

---

## Task 4: Wire Up to Audit Command

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go` (runAuditExecute function)

**Step 1: Read current implementation**

Read lines 200-260 of audit_cmd.go to understand current runAuditExecute structure.

**Step 2: Modify runAuditExecute**

Replace the TODO sections with actual execution:

```go
func runAuditExecute(db *database.MediaDB) error {
	plan, err := plans.LoadAuditPlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending audit plan.")
		fmt.Println("Run 'jellywatch audit --generate' first.")
		return nil
	}

	// Filter actions with sufficient confidence
	var actionsToExecute []plans.AuditAction
	for _, action := range plan.Actions {
		if action.Confidence >= 0.8 {
			actionsToExecute = append(actionsToExecute, action)
		}
	}

	if len(actionsToExecute) == 0 {
		fmt.Println("No actions with sufficient confidence (>= 0.8).")
		fmt.Println("Run 'jellywatch audit --dry-run' to see suggestions.")
		return nil
	}

	fmt.Printf("Will execute %d actions:\n", len(actionsToExecute))
	for i, action := range actionsToExecute {
		fmt.Printf("  %d. %s: %s\n", i+1, action.Action, action.Reasoning)
	}
	fmt.Print("\nContinue? [y/N]: ")

	var response string
	fmt.Scanln(&response)
	if response != "y" && response != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	// Execute actions
	var succeeded, failed int
	for _, action := range actionsToExecute {
		if err := plans.ExecuteAuditAction(db, action); err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			failed++
		} else {
			fmt.Printf("✓ %s: %s\n", action.Action, action.NewPath)
			succeeded++
		}
	}

	fmt.Printf("\n=== Complete ===\n")
	fmt.Printf("Succeeded: %d\n", succeeded)
	if failed > 0 {
		fmt.Printf("Failed: %d\n", failed)
	}

	// Delete plan file on success
	if failed == 0 {
		if err := plans.DeleteAuditPlans(); err != nil {
			fmt.Printf("Warning: failed to delete plan: %v\n", err)
		}
	}

	return nil
}
```

**Step 3: Remove old TODO code**

Delete the old TODO comments and placeholder code (lines ~235-250 in original).

**Step 4: Build to verify**

Run: `go build ./cmd/jellywatch`
Expected: No errors

**Step 5: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat(audit): wire up ExecuteAuditAction to --execute"
```

---

## Task 5: Add Dry-Run Support

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`
- Modify: `internal/plans/plans.go`

**Step 1: Add dryRun parameter**

Modify `ExecuteAuditAction` signature:

```go
func ExecuteAuditAction(db *database.MediaDB, action AuditAction, dryRun bool) error
```

**Step 2: Update helpers for dry-run**

```go
func executeRename(db *database.MediaDB, action AuditAction, dryRun bool) error {
	if dryRun {
		fmt.Printf("[DRY RUN] Would rename: %s -> %s\n", action.Path, action.NewPath)
		return nil
	}
	// ... actual implementation
}

func executeDelete(db *database.MediaDB, action AuditAction, dryRun bool) error {
	if dryRun {
		fmt.Printf("[DRY RUN] Would delete: %s\n", action.Path)
		return nil
	}
	// ... actual implementation
}
```

**Step 3: Update audit command to pass dryRun flag**

In `runAuditExecute`, pass `dryRun` parameter from command context.

**Step 4: Build and test**

Run: `go build ./cmd/jellywatch`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go internal/plans/plans.go
git commit -m "feat(audit): add dry-run support"
```

---

## Task 6: Final Testing

**Files:**
- All test files

**Step 1: Run all plans tests**

Run: `go test ./internal/plans -v`
Expected: All PASS

**Step 2: Run all cmd/jellywatch tests**

Run: `go test ./cmd/jellywatch -v`
Expected: All PASS

**Step 3: Run full test suite**

Run: `go test ./...`
Expected: All PASS

**Step 4: Build final binary**

Run: `go build -o jellywatch ./cmd/jellywatch`
Expected: Binary created successfully

**Step 5: Commit**

```bash
git commit -m "test: add audit execute tests"
```

---

## Summary

**Files changed:**
- `internal/plans/plans.go` - ExecuteAuditAction, executeRename, executeDelete
- `internal/plans/plans_test.go` - Test coverage for both actions
- `internal/database/media_files.go` - UpdateMediaFile helper (if needed)
- `cmd/jellywatch/audit_cmd.go` - Wire up execution

**Testing:**
- Unit tests for rename and delete
- Dry-run mode verification
- Full test suite passes

**Ready for:** @superpowers:executing-plans
