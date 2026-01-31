# Permission Handling Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix permission-denied errors during file deletion and ensure database consistency when operations fail.

**Architecture:**
1. Check and fix file permissions before deletion operations (using config [permissions] settings)
2. Add database cleanup on failed delete operations
3. Provide helpful error messages when permission fixes aren't possible

**Tech Stack:** Go 1.21+, internal/config (PermissionsConfig), os.Chown/Chmod

---

## Background

During audit system bug fixes, we identified two permission-related issues:

**Issue 3: Permission-Denied Deletions**
- User symptom: "Failed to delete /mnt/STORAGE5/MOVIES/...mkv: permission denied"
- Root cause: Files owned by different user (e.g., jellyfin) but JellyWatch runs as another user
- Current behavior: Operation fails, user must manually chown files

**Issue 4: Database Inconsistency**
- When `os.Remove()` fails, database entry remains (ghost entry)
- Causes confusion - file shows in duplicates list but doesn't exist
- Workaround: Rescan fixes it, but shouldn't happen

**Configuration Context:**
JellyWatch has a `[permissions]` section in config.toml:
```toml
[permissions]
user = "jellyfin"
group = "jellyfin"
file_mode = "0644"
dir_mode = "0755"
```

This is currently used for CREATING files but not for DELETING them.

---

## Task 1: Add Permission Helper Package

**Files:**
- Create: `internal/permissions/permissions.go`
- Create: `internal/permissions/permissions_test.go`

**Context:** Create reusable permission checking/fixing utilities that respect the config settings.

**Step 1: Write test for permission helper**

Create test file `internal/permissions/permissions_test.go`:

```go
package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanDelete_OwnedByUs(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	canDelete, err := CanDelete(testFile)
	if err != nil {
		t.Errorf("CanDelete failed: %v", err)
	}
	if !canDelete {
		t.Error("Should be able to delete file we own")
	}
}

func TestFixPermissions_MakeDeletable(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "readonly.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0444); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Make readonly
	if err := os.Chmod(testFile, 0444); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	// Fix permissions
	if err := FixPermissions(testFile, -1, -1); err != nil {
		t.Errorf("FixPermissions failed: %v", err)
	}

	// Should now be deletable
	canDelete, err := CanDelete(testFile)
	if err != nil {
		t.Errorf("CanDelete failed: %v", err)
	}
	if !canDelete {
		t.Error("File should be deletable after FixPermissions")
	}
}

func TestGetFileOwnership(t *testing.T) {
	testFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uid, gid, err := GetFileOwnership(testFile)
	if err != nil {
		t.Errorf("GetFileOwnership failed: %v", err)
	}

	// Should return valid UIDs (non-negative)
	if uid < 0 || gid < 0 {
		t.Errorf("Invalid ownership: uid=%d, gid=%d", uid, gid)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/permissions/... -v`

Expected: FAIL - package doesn't exist yet

**Step 3: Implement permission helper**

Create `internal/permissions/permissions.go`:

```go
package permissions

import (
	"fmt"
	"os"
	"syscall"
)

// CanDelete checks if the current process has permission to delete a file
func CanDelete(path string) (bool, error) {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Check if we have write permission on the parent directory
	dir := filepath.Dir(path)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return false, err
	}

	// Get directory permissions
	dirMode := dirInfo.Mode().Perm()
	
	// Check if directory is writable
	if dirMode&0200 == 0 {
		return false, nil
	}

	// Check if file is writable (needed on some systems)
	fileMode := info.Mode().Perm()
	if fileMode&0200 == 0 {
		return false, nil
	}

	return true, nil
}

// FixPermissions attempts to fix file permissions to make it deletable
// Uses uid/gid from config if provided (-1 means don't change ownership)
func FixPermissions(path string, uid, gid int) error {
	// First make file writable
	if err := os.Chmod(path, 0644); err != nil {
		return fmt.Errorf("failed to chmod: %w", err)
	}

	// Change ownership if requested
	if uid >= 0 || gid >= 0 {
		currentUID, currentGID, err := GetFileOwnership(path)
		if err != nil {
			return fmt.Errorf("failed to get current ownership: %w", err)
		}

		// Use current values if not changing
		if uid < 0 {
			uid = currentUID
		}
		if gid < 0 {
			gid = currentGID
		}

		if err := os.Chown(path, uid, gid); err != nil {
			// Chown may fail if we don't have permissions - not fatal
			return fmt.Errorf("failed to chown (may need sudo): %w", err)
		}
	}

	return nil
}

// GetFileOwnership returns the UID and GID of a file
func GetFileOwnership(path string) (int, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return -1, -1, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, -1, fmt.Errorf("failed to get file stat")
	}

	return int(stat.Uid), int(stat.Gid), nil
}

// NeedsOwnershipChange checks if file ownership differs from target
func NeedsOwnershipChange(path string, targetUID, targetGID int) (bool, error) {
	if targetUID < 0 && targetGID < 0 {
		return false, nil // No target ownership specified
	}

	currentUID, currentGID, err := GetFileOwnership(path)
	if err != nil {
		return false, err
	}

	if targetUID >= 0 && currentUID != targetUID {
		return true, nil
	}
	if targetGID >= 0 && currentGID != targetGID {
		return true, nil
	}

	return false, nil
}
```

Add missing import:
```go
import (
	"path/filepath"
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/permissions/... -v`

Expected: PASS (3 tests)

**Step 5: Commit**

```bash
git add internal/permissions/
git commit -m "feat(permissions): add permission checking and fixing utilities

Add helper package for checking file deletion permissions and fixing
ownership/mode issues before operations. Respects config [permissions]
settings.

- CanDelete: Check if file is deletable
- FixPermissions: Make file deletable with chmod/chown
- GetFileOwnership: Get current file UID/GID
- NeedsOwnershipChange: Check if ownership differs from target

Related to: permission denied errors during duplicate deletion"
```

---

## Task 2: Fix Duplicate Deletion with Permission Handling

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go:64`
- Modify: `cmd/jellywatch/duplicates_cmd.go` - add database cleanup on failure

**Context:** The duplicates execute command currently just calls `os.Remove()` and continues on error, leaving database inconsistent.

**Step 1: Write test for permission-aware deletion**

Add to existing test file or create new one in `cmd/jellywatch/duplicates_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestDeleteDuplicateWithPermissions(t *testing.T) {
	db, err := database.OpenInMemory()
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create test file
	testFile := filepath.Join(t.TempDir(), "duplicate.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Make readonly to simulate permission issue
	if err := os.Chmod(testFile, 0444); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
		Size:      4,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Attempt delete with permission fix
	err = deleteDuplicateFile(db, testFile, -1, -1)

	// Should succeed after fixing permissions
	if err != nil {
		t.Errorf("Delete failed even after permission fix: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File still exists after delete")
	}

	// Database should be cleaned up
	dbFile, err := db.GetMediaFile(testFile)
	if err == nil && dbFile != nil {
		t.Error("Database entry still exists after successful delete")
	}
}

func TestDeleteDuplicateDatabaseCleanupOnFailure(t *testing.T) {
	db, err := database.OpenInMemory()
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Use non-existent file
	testFile := "/nonexistent/duplicate.mkv"

	// Insert into database
	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
		Size:      100,
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Attempt delete (will fail - file doesn't exist)
	err = deleteDuplicateFile(db, testFile, -1, -1)

	// Should handle gracefully
	if err != nil {
		t.Logf("Expected error for non-existent file: %v", err)
	}

	// Database should still be cleaned up
	dbFile, err := db.GetMediaFile(testFile)
	if err == nil && dbFile != nil {
		t.Error("Database entry should be removed even when file doesn't exist")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/jellywatch/... -run TestDeleteDuplicate -v`

Expected: FAIL - function doesn't exist

**Step 3: Implement permission-aware deletion**

In `cmd/jellywatch/duplicates_cmd.go`, extract deletion logic into helper function.

Add this function BEFORE `runDuplicatesExecute`:

```go
import (
	"github.com/Nomadcxx/jellywatch/internal/permissions"
)

// deleteDuplicateFile deletes a file with permission fixing and database cleanup
func deleteDuplicateFile(db *database.MediaDB, filePath string, uid, gid int) error {
	// Check if we can delete
	canDelete, err := permissions.CanDelete(filePath)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	// If we can't delete, try to fix permissions
	if !canDelete {
		if err := permissions.FixPermissions(filePath, uid, gid); err != nil {
			// Permission fix failed - still try delete and report detailed error
			if removeErr := os.Remove(filePath); removeErr != nil {
				return fmt.Errorf("permission denied (tried to fix but failed: %v): %w", err, removeErr)
			}
		}
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		// File delete failed - clean up database anyway to avoid ghost entries
		_ = db.DeleteMediaFile(filePath)
		
		if os.IsNotExist(err) {
			// File already gone - not an error, just clean up DB
			return nil
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Remove from database
	if err := db.DeleteMediaFile(filePath); err != nil {
		// File deleted but DB cleanup failed - log but don't fail the operation
		fmt.Printf("  ⚠️  Warning: File deleted but database cleanup failed: %v\n", err)
	}

	return nil
}
```

Now modify the `runDuplicatesExecute` function. Find the deletion block (lines 63-67):

```go
// Delete file
if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
	fmt.Printf("  ❌ Failed to delete %s: %v\n", filePath, err)
	continue
}
```

Replace with:

```go
// Get permissions config
var uid, gid int = -1, -1
if cfg != nil && cfg.Permissions.WantsOwnership() {
	uid, _ = cfg.Permissions.ResolveUID()
	gid, _ = cfg.Permissions.ResolveGID()
}

// Delete file with permission handling
if err := deleteDuplicateFile(db, filePath, uid, gid); err != nil {
	fmt.Printf("  ❌ Failed to delete %s: %v\n", filePath, err)
	continue
}
```

You'll need to pass `cfg *config.Config` to `runDuplicatesExecute`. Update the function signature and caller.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/jellywatch/... -run TestDeleteDuplicate -v`

Expected: PASS (2 tests)

**Step 5: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go cmd/jellywatch/duplicates_test.go
git commit -m "fix(duplicates): handle permission denied errors and database cleanup

Duplicate deletion now:
- Checks file permissions before attempting delete
- Attempts to fix permissions using config [permissions] settings
- Cleans up database even when file delete fails (prevents ghost entries)
- Provides detailed error messages when permission fixes fail

Fixes: permission denied errors during duplicate deletion
Fixes: database ghost entries when deletion fails"
```

---

## Task 3: Fix Audit Delete with Permission Handling

**Files:**
- Modify: `internal/plans/plans.go:435` - executeDelete function

**Context:** Same issue as duplicates - audit delete uses `os.Remove()` without permission handling.

**Step 1: Write test for audit delete with permissions**

Add to `internal/plans/plans_test.go`:

```go
func TestExecuteDelete_WithPermissionFix(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create test file
	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "delete-me.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Make readonly
	if err := os.Chmod(testFile, 0444); err != nil {
		t.Fatalf("Failed to chmod: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	item := AuditItem{ID: file.ID, Path: testFile}
	action := AuditAction{Action: "delete"}

	// Execute delete (should fix permissions automatically)
	err := executeDelete(db, item, action, false)

	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}

	// File should be gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("File still exists after delete")
	}

	// Database should be cleaned up
	dbFile, _ := db.GetMediaFileByID(file.ID)
	if dbFile != nil {
		t.Error("Database entry still exists")
	}
}

func TestExecuteDelete_DatabaseCleanupOnFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Use non-existent file
	testFile := "/nonexistent/audit-delete.mkv"

	// Insert into database
	file := &database.MediaFile{
		Path:      testFile,
		MediaType: "movie",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	item := AuditItem{ID: file.ID, Path: testFile}
	action := AuditAction{Action: "delete"}

	// Execute delete (will fail - file doesn't exist)
	err := executeDelete(db, item, action, false)

	// Should handle gracefully (file not existing is OK)
	if err != nil {
		t.Logf("Non-existent file handled: %v", err)
	}

	// Database should be cleaned up
	dbFile, _ := db.GetMediaFileByID(file.ID)
	if dbFile != nil {
		t.Error("Database entry should be removed even when file doesn't exist")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plans/... -run TestExecuteDelete -v`

Expected: FAIL - permission fixes not implemented

**Step 3: Implement permission-aware delete**

In `internal/plans/plans.go`, find `executeDelete` function (line 418). Replace the deletion block:

Find:
```go
// Delete from filesystem
if err := os.Remove(file.Path); err != nil {
	return fmt.Errorf("failed to delete file: %w", err)
}

// Remove from database
if err := db.DeleteMediaFileByID(file.ID); err != nil {
	return fmt.Errorf("failed to delete media file from database: %w", err)
}
```

Replace with:

```go
// Check if we can delete the file
canDelete, err := permissions.CanDelete(file.Path)
if err != nil {
	return fmt.Errorf("failed to check permissions: %w", err)
}

// If we can't delete, try to fix permissions
// Note: We don't have config access here, so use -1 (don't change ownership)
if !canDelete {
	if err := permissions.FixPermissions(file.Path, -1, -1); err != nil {
		// Permission fix failed - try delete anyway and provide helpful error
		if removeErr := os.Remove(file.Path); removeErr != nil {
			return fmt.Errorf("permission denied (tried chmod but failed: %v): %w", err, removeErr)
		}
	}
}

// Delete from filesystem
if err := os.Remove(file.Path); err != nil {
	// Clean up database even on failure to prevent ghost entries
	_ = db.DeleteMediaFileByID(file.ID)
	
	if os.IsNotExist(err) {
		// File already gone - just clean up database
		return nil
	}
	return fmt.Errorf("failed to delete file: %w", err)
}

// Remove from database
if err := db.DeleteMediaFileByID(file.ID); err != nil {
	// File deleted but DB cleanup failed - log warning
	fmt.Printf("Warning: File deleted but database cleanup failed: %v\n", err)
	return fmt.Errorf("failed to delete media file from database: %w", err)
}
```

Add import:
```go
import (
	"github.com/Nomadcxx/jellywatch/internal/permissions"
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/plans/... -run TestExecuteDelete -v`

Expected: PASS (2 new tests + existing)

**Step 5: Commit**

```bash
git add internal/plans/plans.go internal/plans/plans_test.go
git commit -m "fix(plans): handle permissions in audit delete operations

Audit delete now:
- Checks file permissions before delete
- Attempts chmod to fix readonly files
- Cleans up database even when delete fails (prevents ghost entries)
- Handles non-existent files gracefully

Fixes: permission denied errors in audit delete
Fixes: database inconsistency when audit delete fails"
```

---

## Task 4: Add Config Access to Deletion Operations

**Files:**
- Modify: `internal/plans/plans.go` - Pass config to executeDelete
- Modify: `cmd/jellywatch/duplicates_cmd.go` - Pass config to deletion helper

**Context:** Currently deletion operations don't have access to config [permissions] settings. We need to thread config through so we can use user/group from config when fixing ownership.

**Step 1: Update executeDelete signature**

This is a refactoring task to improve the permission fixes from Tasks 2-3.

In `internal/plans/plans.go`, update `executeDelete` signature:

```go
func executeDelete(db *database.MediaDB, item AuditItem, action AuditAction, dryRun bool, cfg *config.Config) error {
```

Update the permission fix section to use config:

```go
// Get target ownership from config if available
var uid, gid int = -1, -1
if cfg != nil && cfg.Permissions.WantsOwnership() {
	uid, _ = cfg.Permissions.ResolveUID()
	gid, _ = cfg.Permissions.ResolveGID()
}

// Check if we can delete the file
canDelete, err := permissions.CanDelete(file.Path)
if err != nil {
	return fmt.Errorf("failed to check permissions: %w", err)
}

// If we can't delete, try to fix permissions with config ownership
if !canDelete {
	if err := permissions.FixPermissions(file.Path, uid, gid); err != nil {
		// Permission fix failed...
```

**Step 2: Update all callers**

Find all calls to `executeDelete` and add config parameter. In `ExecuteAuditAction`:

```go
func ExecuteAuditAction(db *database.MediaDB, item AuditItem, action AuditAction, dryRun bool) error {
```

Update to:

```go
func ExecuteAuditAction(db *database.MediaDB, item AuditItem, action AuditAction, dryRun bool, cfg *config.Config) error {
```

And update the call to executeDelete:

```go
case "delete":
	return executeDelete(db, item, action, dryRun, cfg)
```

Update all callers in `cmd/jellywatch/audit_cmd.go` to pass config.

**Step 3: Update duplicates to pass config**

Already done in Task 2.

**Step 4: Test everything still works**

Run: `go test ./internal/plans/... ./cmd/jellywatch/... -v`

Expected: All tests pass with new signatures

**Step 5: Commit**

```bash
git add internal/plans/plans.go cmd/jellywatch/audit_cmd.go
git commit -m "refactor(plans): pass config to deletion operations

Thread config through to deletion functions so permission fixes can use
configured user/group ownership settings.

- ExecuteAuditAction accepts config parameter
- executeDelete uses config for chown operations
- All callers updated to pass config

Enables proper ownership fixes during deletion operations"
```

---

## Task 5: Add User-Friendly Error Messages

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go` - Better error messages
- Modify: `internal/plans/plans.go` - Better error messages

**Context:** When permission fixes fail, provide actionable guidance to users.

**Step 1: Create error message helper**

In `internal/permissions/permissions.go`, add:

```go
// PermissionError represents a permission-related error with helpful guidance
type PermissionError struct {
	Path    string
	Op      string // "delete", "move", etc.
	Err     error
	NeedUID int
	NeedGID int
}

func (e *PermissionError) Error() string {
	msg := fmt.Sprintf("permission denied: cannot %s %s: %v", e.Op, e.Path, e.Err)
	
	if e.NeedUID >= 0 || e.NeedGID >= 0 {
		msg += fmt.Sprintf("\n\nTo fix this, run:\n  sudo chown")
		if e.NeedUID >= 0 && e.NeedGID >= 0 {
			msg += fmt.Sprintf(" %d:%d", e.NeedUID, e.NeedGID)
		} else if e.NeedUID >= 0 {
			msg += fmt.Sprintf(" %d", e.NeedUID)
		} else {
			msg += fmt.Sprintf(" :%d", e.NeedGID)
		}
		msg += fmt.Sprintf(" %s", e.Path)
	} else {
		msg += fmt.Sprintf("\n\nTo fix this, run:\n  sudo chmod 644 %s", e.Path)
	}
	
	return msg
}

// NewPermissionError creates a helpful permission error
func NewPermissionError(path, op string, err error, uid, gid int) error {
	return &PermissionError{
		Path:    path,
		Op:      op,
		Err:     err,
		NeedUID: uid,
		NeedGID: gid,
	}
}
```

**Step 2: Use helpful errors in deletion**

In `internal/plans/plans.go`, update error returns:

```go
if !canDelete {
	if err := permissions.FixPermissions(file.Path, uid, gid); err != nil {
		if removeErr := os.Remove(file.Path); removeErr != nil {
			return permissions.NewPermissionError(file.Path, "delete", removeErr, uid, gid)
		}
	}
}
```

**Step 3: Use in duplicates deletion**

Update `cmd/jellywatch/duplicates_cmd.go` similarly.

**Step 4: Test error messages**

Create a test that verifies helpful error message format.

**Step 5: Commit**

```bash
git add internal/permissions/permissions.go internal/plans/plans.go cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(permissions): add helpful error messages with fix instructions

When permission errors occur, provide actionable commands to fix:
- Shows exact chown/chmod command needed
- Includes target UID/GID from config
- Suggests sudo when ownership change is needed

Improves UX when permission issues occur"
```

---

## Verification Tasks

After completing all implementation tasks:

### Run Full Test Suite

```bash
go test ./... -v
```

Expected: All tests pass

### Manual Testing

1. Create test file owned by different user:
```bash
sudo touch /tmp/test-perm.mkv
sudo chown jellyfin:jellyfin /tmp/test-perm.mkv
```

2. Run duplicate deletion without fix - should see helpful error
3. Run with permissions config set - should auto-fix
4. Verify database cleaned up even on failure

### Integration Testing

Test with real Jellyfin setup:
```bash
jellywatch duplicates generate
jellywatch duplicates execute
# Should handle permission errors gracefully
```

---

## Success Criteria

- [ ] Permission helper package created and tested
- [ ] Duplicate deletion handles permissions
- [ ] Audit deletion handles permissions
- [ ] Database cleanup happens even on failure
- [ ] Config [permissions] settings used for ownership
- [ ] Helpful error messages with fix instructions
- [ ] All tests pass
- [ ] No database ghost entries after failed deletes
- [ ] Documentation updated

---

## Rollback Plan

If any task causes issues:

1. **Permission Helper**: Remove package, operations work as before (fail on permission denied)
2. **Database Cleanup**: Comment out cleanup logic, ghost entries remain but operations don't crash
3. **Config Threading**: Revert signatures, use -1 for uid/gid (no ownership changes)

To rollback specific commits:
```bash
git log --oneline -10
git revert <commit-hash>
```
