# Fix Audit System Critical Bugs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix cross-device rename error and AI hallucination issues in audit command to ensure reliable file operations and accurate title matching.

**Architecture:**
1. Replace os.Rename() with internal/transfer.Move() for cross-device support in plans.go and consolidate/executor.go
2. Enhance AI prompt to include library context (folder path, media type, existing metadata) to prevent hallucinations

**Tech Stack:** Go 1.21+, internal/transfer package (rsync/native backends), Ollama AI integration

---

## Task 1: Fix Cross-Device Bug in plans.go (Audit Rename)

**Files:**
- Modify: `internal/plans/plans.go:464`
- Test: `internal/plans/plans_test.go` (create new)

**Context:** The executeRename() function uses os.Rename() which fails when moving files between different filesystems (e.g., /mnt/STORAGE5 → /mnt/STORAGE1). The internal/transfer package handles this via copy-then-delete with retry/timeout support.

**Step 1: Write failing test for cross-device scenario**

Create test file `internal/plans/plans_test.go`:

```go
package plans

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecuteRename_CrossDevice(t *testing.T) {
	// This test documents the EXDEV error behavior
	// We can't truly simulate cross-device in tests, but we verify
	// that the code uses transfer.Move() instead of os.Rename()

	db := setupTestDB(t)
	defer db.Close()

	// Create test file
	testDir := t.TempDir()
	srcPath := filepath.Join(testDir, "test.movie.2024.mkv")
	dstPath := filepath.Join(testDir, "Test Movie (2024).mkv")

	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:            srcPath,
		NormalizedTitle:  "test.movie",
		Year:           2024,
		MediaType:      "movie",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	item := AuditItem{ID: file.ID, Path: srcPath}
	action := AuditAction{
		Action:   "rename",
		NewTitle: "Test Movie",
		NewPath:  dstPath,
	}

	// Execute rename
	err := executeRename(db, item, action, false)

	// Should succeed (not fail with cross-device error)
	if err != nil {
		t.Errorf("executeRename failed: %v", err)
	}

	// Verify file moved
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Error("Destination file does not exist")
	}
	if _, err := os.Stat(srcPath); err == nil {
		t.Error("Source file still exists")
	}

	// Verify database updated
	updatedFile, err := db.GetMediaFileByID(file.ID)
	if err != nil {
		t.Fatalf("Failed to get updated file: %v", err)
	}
	if updatedFile.Path != dstPath {
		t.Errorf("Database path not updated: got %s, want %s", updatedFile.Path, dstPath)
	}
}
```

**Step 2: Run test to verify it fails with cross-device error**

Run: `go test ./internal/plans/... -run TestExecuteRename_CrossDevice -v`

Expected: FAIL with "failed to rename file: rename ...: invalid cross-device link" (if you actually test across devices) OR FAIL because os.Rename() is still being used instead of transfer.Move()

**Step 3: Implement transfer.Move() in executeRename()**

Modify `internal/plans/plans.go` at line 464:

Find this code:
```go
// Perform filesystem move
if err := os.Rename(file.Path, action.NewPath); err != nil {
	return fmt.Errorf("failed to rename file: %w", err)
}
```

Replace with:
```go
// Perform filesystem move using transfer package (handles cross-device)
transferer, err := transfer.New(transfer.BackendAuto)
if err != nil {
	return fmt.Errorf("failed to create transferer: %w", err)
}

result, err := transferer.Move(file.Path, action.NewPath, transfer.DefaultOptions())
if err != nil {
	return fmt.Errorf("failed to move file: %w", err)
}
if !result.Success {
	return fmt.Errorf("file transfer failed: %v", result.Error)
}
```

Also update rollback logic at line 483. Find:
```go
_ = os.Rename(action.NewPath, file.Path)
```

Replace with:
```go
// Attempt rollback on DB failure
rollbackResult, rollbackErr := transferer.Move(action.NewPath, file.Path, transfer.DefaultOptions())
if rollbackErr != nil {
	e.Printf("Failed to rollback file move: %v\n", rollbackErr)
} else if !rollbackResult.Success {
	e.Printf("Rollback transfer failed: %v\n", rollbackResult.Error)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/plans/... -run TestExecuteRename_CrossDevice -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/plans/plans.go internal/plans/plans_test.go
git commit -m "fix(plans): use transfer.Move() for cross-device support in audit rename

Replace os.Rename() with transfer.Move() to handle cross-filesystem
moves. Fixes 'invalid cross-device link' errors when moving files
between different mount points.

- Add test for cross-device scenario
- Use internal/transfer package (rsync/native backends)
- Update rollback logic to use transfer package

Fixes: audit --execute failing on cross-device moves"
```

---

## Task 2: Fix Cross-Device Bug in consolidate/executor.go

**Files:**
- Modify: `internal/consolidate/executor.go:256`
- Test: `internal/consolidate/executor_test.go` (create new)

**Context:** Same issue as Task 1 - consolidate rename uses os.Rename() which fails across devices.

**Step 1: Write failing test for consolidate cross-device**

Create test file `internal/consolidate/executor_test.go`:

```go
package consolidate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestExecuteRename_CrossDevice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db, false, false)

	testDir := t.TempDir()
	srcPath := filepath.Join(testDir, "show.s01e01.mkv")
	dstPath := filepath.Join(testDir, "TV Shows/Test Show (2024)/Season 01/Test Show (2024) - S01E01.mkv")

	// Create source directory
	if err := os.MkdirAll(filepath.Dir(srcPath), 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create test file
	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:            srcPath,
		NormalizedTitle:  "show",
		Year:           2024,
		MediaType:      "episode",
		Season:         intPtr(1),
		Episode:        intPtr(1),
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	plan := &ConsolidationPlan{
		SourcePath: srcPath,
		TargetPath: dstPath,
	}

	ctx := context.Background()
	err := executor.executeRename(ctx, plan)

	// Should succeed
	if err != nil {
		t.Errorf("executeRename failed: %v", err)
	}

	// Verify file moved
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/consolidate/... -run TestExecuteRename_CrossDevice -v`

Expected: FAIL (either EXDEV error or os.Rename() still being used)

**Step 3: Implement transfer.Move() in executeRename()**

Modify `internal/consolidate/executor.go` at line 256:

Find:
```go
// Rename the file
if err := os.Rename(plan.SourcePath, plan.TargetPath); err != nil {
	return fmt.Errorf("failed to rename file: %w", err)
}
```

Replace with:
```go
// Rename the file using transfer package (handles cross-device)
transferer, err := transfer.New(transfer.BackendAuto)
if err != nil {
	return fmt.Errorf("failed to create transferer: %w", err)
}

result, err := transferer.Move(plan.SourcePath, plan.TargetPath, transfer.DefaultOptions())
if err != nil {
	return fmt.Errorf("failed to move file: %w", err)
}
if !result.Success {
	return fmt.Errorf("file transfer failed: %v", result.Error)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/consolidate/... -run TestExecuteRename_CrossDevice -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/consolidate/executor.go internal/consolidate/executor_test.go
git commit -m "fix(consolidate): use transfer.Move() for cross-device support

Replace os.Rename() with transfer.Move() to handle cross-filesystem
moves in consolidation. Fixes 'invalid cross-device link' errors when
moving scattered episodes between mount points.

- Add test for cross-device scenario
- Use internal/transfer package
- Maintain existing database update logic

Fixes: consolidate execute failing on cross-device moves"
```

---

## Task 3: Enhance AI Prompt with Library Context

**Files:**
- Modify: `internal/ai/matcher.go:247-324` (getSystemPrompt function)
- Modify: `cmd/jellywatch/audit_cmd.go:170` (Parse call)
- Modify: `internal/ai/matcher.go:69` (Parse method signature)
- Test: `internal/ai/matcher_test.go` (update existing tests)

**Context:** AI receives only filename basename with zero context about folder location, library type, or existing metadata. This causes hallucinations (e.g., "pb.s04e15.mkv" could be Prison Break, Pitch Black, etc.).

**Step 1: Extend Parse() method to accept context**

Modify `internal/ai/matcher.go` - find the Parse method at line 68:

Current:
```go
// Parse sends a filename to Ollama and returns parsed metadata
func (m *Matcher) Parse(ctx context.Context, filename string) (*Result, error) {
	return m.parseWithModel(ctx, filename, m.config.Model)
}
```

Replace with:
```go
// Parse sends a filename to Ollama and returns parsed metadata
func (m *Matcher) Parse(ctx context.Context, filename string) (*Result, error) {
	return m.parseWithModel(ctx, filename, m.config.Model)
}

// ParseWithContext sends a filename with additional library context to Ollama
func (m *Matcher) ParseWithContext(ctx context.Context, filename string, libraryType string, folderPath string, currentTitle string, currentConfidence float64) (*Result, error) {
	// Build context-enhanced prompt
	contextPrompt := m.systemPrompt

	// Add library type context
	if libraryType != "" {
		contextPrompt += fmt.Sprintf("\n\n## File Context\n- File is in a %s library\n", libraryType)
	}

	// Add folder path context (redact sensitive paths)
	if folderPath != "" {
		sanitizedPath := filepath.Base(folderPath)
		contextPrompt += fmt.Sprintf("- Folder name: %s\n", sanitizedPath)
	}

	// Add existing metadata context
	if currentTitle != "" {
		contextPrompt += fmt.Sprintf("- Current title: %s (confidence: %.2f)\n", currentTitle, currentConfidence)
	}

	return m.parseWithModel(ctx, contextPrompt+"\n\nNow parse this filename: "+filename, m.config.Model)
}
```

Add import at top if not present:
```go
import (
	"fmt"
	"path/filepath"
)
```

**Step 2: Update system prompt to use context**

Modify `internal/ai/matcher.go` getSystemPrompt() function. Add after the "Now parse this filename:" line:

Find at line 323:
```go
Now parse this filename:`
```

Replace with:
```go
## Using Context
The context above (File Context section) provides critical information:
- Library type (movies vs TV shows): Use this to disambiguate titles
- Folder name: May contain additional clues about the content
- Current title: The existing parse that needs improvement - use as a hint but don't be bound by it
- Confidence: Low confidence (e.g., < 0.8) indicates the current parse may be wrong

Example:
- Library type: episode, Folder: "Prison Break", Current: "pb" → Likely "Prison Break"
- Library type: movie, Folder: "MOVIES", File: "pb.s04e15.mkv" → Probably incorrect file location OR TV show misclassified

Now parse this filename:`
```

**Step 3: Update audit_cmd.go to pass context**

Modify `cmd/jellywatch/audit_cmd.go` at line 170:

Find:
```go
ctx := context.Background()
aiResult, err := matcher.Parse(ctx, filepath.Base(file.Path))
```

Replace with:
```go
ctx := context.Background()

// Build library context for AI
libraryType := "unknown"
if file.MediaType == "movie" {
	libraryType = "movie library"
} else if file.MediaType == "episode" {
	libraryType = "TV show library"
}

folderPath := filepath.Dir(file.Path)

aiResult, err := matcher.ParseWithContext(
	ctx,
	filepath.Base(file.Path),
	libraryType,
	folderPath,
	file.NormalizedTitle,
	file.Confidence,
)
```

**Step 4: Update tests to use new ParseWithContext()**

In `internal/ai/matcher_test.go`, find tests that call `matcher.Parse()` and update them to use `ParseWithContext()` with appropriate context.

Example - find test code like:
```go
result, err := matcher.Parse(ctx, "The.Matrix.1999.1080p.BluRay.x264-RARBG.mkv")
```

Update to:
```go
result, err := matcher.ParseWithContext(
	ctx,
	"The.Matrix.1999.1080p.BluRay.x264-RARBG.mkv",
	"movie library",
	"/media/Movies",
	"the.matrix",
	0.65,
)
```

**Step 5: Run all AI tests**

Run: `go test ./internal/ai/... -v`

Expected: All tests pass

**Step 6: Commit**

```bash
git add internal/ai/matcher.go cmd/jellywatch/audit_cmd.go internal/ai/matcher_test.go
git commit -m "feat(ai): enhance prompt with library context to reduce hallucinations

AI matcher now receives library type, folder path, and existing metadata
as context to prevent hallucinations. Previously, AI only received filename
basename which caused incorrect matches (e.g., 'pb.s04e15.mkv' guessed
as 'History's Greatest Mysteries' instead of 'Prison Break').

Changes:
- Add ParseWithContext() method with library context
- Update system prompt to explain how to use context
- Pass library type, folder, current title from audit_cmd
- Update tests to use new method

Fixes: AI hallucinating random shows without context"
```

---

## Task 4: Add Integration Test for Cross-Device Fixes

**Files:**
- Create: `tests/integration/cross_device_test.go`

**Context:** Verify end-to-end behavior with simulated cross-device scenarios (or actual if test environment has multiple mounts).

**Step 1: Write integration test**

Create `tests/integration/cross_device_test.go`:

```go
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

func TestAuditMove_CrossDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup database
	db, err := database.OpenInMemory()
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create source and destination directories
	sourceDir := t.TempDir()
	destDir := t.TempDir()

	// Create test file
	srcPath := filepath.Join(sourceDir, "test.movie.2024.1080p.mkv")
	if err := os.WriteFile(srcPath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Insert into database
	file := &database.MediaFile{
		Path:            srcPath,
		NormalizedTitle:  "test.movie",
		Year:           2024,
		MediaType:      "movie",
	}
	if err := db.UpsertMediaFile(file); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	// Create audit action
	item := plans.AuditItem{ID: file.ID, Path: srcPath}
	action := plans.AuditAction{
		Action:   "rename",
		NewTitle: "Test Movie",
		NewPath:  filepath.Join(destDir, "Test Movie (2024).mkv"),
	}

	// Execute rename
	err = plans.ExecuteAuditAction(db, item, action, false)

	// Verify success
	if err != nil {
		t.Errorf("Audit action failed: %v", err)
	}

	// Verify file moved
	if _, err := os.Stat(action.NewPath); os.IsNotExist(err) {
		t.Error("Destination file does not exist after move")
	}
	if _, err := os.Stat(srcPath); err == nil {
		t.Error("Source file still exists after move")
	}
}

func TestTransfer_BackendAvailability(t *testing.T) {
	// Test that at least one transfer backend is available
	backends := []transfer.Backend{
		transfer.BackendAuto,
		transfer.BackendRsync,
		transfer.BackendNative,
	}

	for _, backend := range backends {
		_, err := transfer.New(backend)
		if err == nil {
			t.Logf("Backend %s is available", backend)
			return
		}
		t.Logf("Backend %s not available: %v", backend, err)
	}

	t.Fatal("No transfer backend available (rsync or native required)")
}
```

**Step 2: Run integration test**

Run: `go test ./tests/integration/... -v`

Expected: PASS

**Step 3: Commit**

```bash
git add tests/integration/cross_device_test.go
git commit -m "test: add integration tests for cross-device file moves

Verifies that audit rename and consolidation operations work correctly
when moving files between different filesystems. Also validates that
at least one transfer backend (rsync or native) is available."
```

---

## Task 5: Update Documentation

**Files:**
- Modify: `README.md` (audit section)
- Create: `docs/ai-context.md`

**Step 1: Document AI context enhancement**

Create `docs/ai-context.md`:

```markdown
# AI Title Matching with Context

## Overview

JellyWatch uses local AI (Ollama) to parse media filenames and extract
clean metadata (title, year, season, episode). The AI receives contextual
information to improve accuracy and prevent hallucinations.

## Context Provided to AI

When analyzing a file, the AI receives:

1. **Library Type**: "movie library" or "TV show library"
   - Helps disambiguate titles (e.g., "The Office" could be movie or TV)
   - Influences confidence in file type detection

2. **Folder Path**: Parent directory name (sanitized)
   - May contain series name (e.g., "/Prison Break/Season 4/")
   - Helps identify correct show when filename is ambiguous

3. **Current Metadata**: Existing parse from database
   - Title that was previously extracted
   - Confidence score (low confidence < 0.8 indicates likely error)
   - Used as hint but not bound by it

4. **Filename**: The full release filename
   - Contains quality indicators, release groups, season/episode markers
   - Primary source of information

## Example

### Without Context (OLD BEHAVIOR):
```
Input: "pb.s04e15.720p.brrip.mkv"
AI Guesses: "History's Greatest Mysteries - S07E01" ❌ WRONG
```

### With Context (NEW BEHAVIOR):
```
Input:
- Filename: "pb.s04e15.720p.brrip.mkv"
- Library Type: "TV show library"
- Folder: "Prison Break"
- Current Title: "pb" (confidence: 0.65)

AI Guesses: "Prison Break - S04E15" ✅ CORRECT
```

## Validation

AI suggestions are validated through multiple layers:

1. **Library Path Analysis**: Check if AI's suggested type matches folder location
2. **Sonarr/Radarr Lookup**: Query APIs to confirm title existence
3. **Confidence Threshold**: Reject low-confidence suggestions (< 0.8)

## Configuration

AI matching is controlled via `~/.config/jellywatch/config.toml`:

```toml
[ai]
enabled = true
ollama_endpoint = "http://localhost:11434"
model = "llama3.1"
confidence_threshold = 0.8
timeout_seconds = 30
```

## Troubleshooting

### AI Still Hallucinating?

1. Enable debug mode: `export DEBUG_AI=1`
2. Check what context is being sent: Look for "File Context" in prompts
3. Verify folder structure is clean (no obfuscated names)
4. Increase `confidence_threshold` to reject uncertain suggestions

### Wrong Media Type?

If AI suggests movie for TV show or vice versa:
- Check folder path (should be in correct library)
- Verify library root config in config.toml
- Run `jellywatch scan` to update database
```

**Step 2: Update README audit section**

Modify `README.md`, find the audit command section (around line 67):

Add after the audit commands:
```bash
jellywatch audit generate          # Find low-confidence parses
jellywatch audit dry-run           # Preview AI suggestions
jellywatch audit execute           # Apply fixes
```

Add:
```markdown
### AI Context

The audit command uses local AI (Ollama) to analyze low-confidence files.
AI receives contextual information including:

- Library type (Movies vs TV Shows) - prevents misclassifying shows as movies
- Folder path - helps identify correct series from directory structure
- Existing metadata - current parse and confidence score as hints

See [docs/ai-context.md](docs/ai-context.md) for details.
```

**Step 3: Commit**

```bash
git add README.md docs/ai-context.md
git commit -m "docs: document AI context enhancement and cross-device fixes

Add AI context documentation explaining how library context improves
title matching accuracy. Update README with audit command details.

- docs/ai-context.md: Complete AI context guide
- README.md: Add audit command documentation
```

---

## Verification Tasks

After completing all implementation tasks:

### Run Full Test Suite

```bash
go test ./... -v
```

Expected: All tests pass

### Verify Cross-Device Handling

1. Check that both `internal/plans/plans.go:464` and `internal/consolidate/executor.go:256` use `transfer.Move()`
2. Run integration test: `go test ./tests/integration/... -run CrossDevice -v`

### Verify AI Context

1. Run audit generate on test file
2. Enable debug mode: `export DEBUG_AI=1`
3. Check that prompt includes "File Context" section
4. Verify library type and folder path are included

### Manual Testing (Optional)

If you have multi-mount setup:

```bash
# Test cross-device audit
jellywatch audit generate --threshold 0.0 --limit 1
jellywatch audit --dry-run
```

Check that operations succeed without "invalid cross-device link" errors.

---

## Rollback Plan

If any task causes issues:

1. **Cross-Device Fix**: Revert to os.Rename() - will break on cross-device moves but safe for same-filesystem
2. **AI Context Fix**: Use old `Parse()` method instead of `ParseWithContext()` - will hallucinate more but functional

To rollback specific commits:
```bash
git log --oneline -5  # Find commit to revert
git revert <commit-hash>
```

---

## Success Criteria

- [ ] All unit tests pass
- [ ] Integration tests pass
- [ ] Code uses transfer.Move() in both locations
- [ ] AI prompt includes library context
- [ ] Documentation updated
- [ ] No new lint errors
- [ ] Manual cross-device test succeeds (if applicable)
