# CLI Messaging Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix CLI messaging confusion by separating reorganization (consolidate) from cleanup (duplicates) and adding direct duplicate deletion capability

**Architecture:** Architectural refactoring to separate concerns - consolidate focuses on moves/renames, duplicates handles deletions, with shared formatting utilities

**Tech Stack:** Go 1.21+, Cobra CLI framework

---

## Task 1: Create shared formatter utility

**Files:**
- Create: `cmd/jellywatch/formatter.go`

**Step 1: Write formatter utility with shared functions**

Create new file with formatBytes(), printSummaryField(), printCommandStep() functions for consistent output across commands.

**Step 2: Verify file compiles**

```bash
go build -o /dev/null ./cmd/jellywatch
```

Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/jellywatch/formatter.go
git commit -m "refactor(formatter): add shared formatting utility functions"
```

---

### Task 2: Fix consolidate --generate messaging

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go:32`

**Step 1: Change "Space to reclaim" to "Files to relocate"**

Line 32: Change formatBytes(spaceToReclaim) output label.

**Step 2: Run build**

```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**Step 3: Test consolidate --generate output**

```bash
./build/jellywatch consolidate --generate
```

Verify "Files to relocate:" shown.

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "fix(cli): consolidate shows 'Files to relocate' not 'Space to reclaim'"
```

---

### Task 3: Add --execute flag to duplicates command

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Add execute bool flag variable**

After line 16, add execute bool.

**Step 2: Register execute flag with Cobra**

Add flag registration after line 42.

**Step 3: Update runDuplicates signature**

Change function to accept execute parameter (line 45, 35).

**Step 4: Run build**

```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**Step 5: Test duplicates --help**

```bash
./build/jellywatch duplicates --help
```

Verify --execute flag appears.

**Step 6: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(duplicates): add --execute flag for duplicate deletion"
```

---

### Task 4: Implement duplicate deletion logic

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Add os import**

Add "os" to imports (line 3).

**Step 2: Add deletion logic before summary**

Insert deletion block before line 169 with execute=false guard.

**Step 3: Remove consolidate suggestions**

Replace lines 77-80 with simple suggestion for --execute only.

**Step 4: Update help text**

Add --execute example to Long description.

**Step 5: Run build**

```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**Step 6: Test duplicates list (no execute)**

```bash
./build/jellywatch duplicates
```

Verify consolidation suggestions removed.

**Step 7: Test duplicates --execute**

```bash
./build/jellywatch duplicates --execute
```

Verify files deleted and database updated.

**Step 8: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(duplicates): implement --execute flag to delete duplicate files"
```

---

### Task 5: Verify consolidate cannot delete files

**Files:**
- Review: `internal/consolidate/` package

**Step 1: Search for deletion patterns**

```bash
grep -r "os.Remove\|io.Remove\|Delete" internal/consolidate/ --include="*.go"
```

Expected: No matches.

**Step 2: Check consolidation_plans schema**

Verify action types are only "move", "rename".

**Step 3: Document findings**

Note: "✅ Consolidate package verified - uses only move/rename operations, no file deletion"

---

### Task 6: Integration verification

**Step 1: Build all binaries**

```bash
go build -o build/jellywatch ./cmd/jellywatch
```

Expected: SUCCESS

**Step 2: Test consolidate --generate**

```bash
./build/jellywatch consolidate --generate
```

Verify "Files to relocate:" shown correctly.

**Step 3: Test duplicates --help**

```bash
./build/jellywatch duplicates --help
```

Verify --execute flag shown, examples include --execute.

**Step 4: Test all package tests**

```bash
go test ./...
```

Expected: All PASS.

**Step 5: Final commit**

```bash
git add cmd/jellywatch/
git commit -m "feat(cli): complete CLI messaging fixes - consolidate terminology and duplicates execute flag"
```

---

## Remember

- Exact file paths always
- Complete code in plan (not "add validation")
- Exact commands with expected output
- Reference relevant skills with @ syntax
- DRY, YAGNI, TDD, frequent commits
- formatBytes() function already exists in main.go, move to formatter.go
- Run test before claiming completion
- Verify consolidate package doesn't delete files (Task 5 critical check)
