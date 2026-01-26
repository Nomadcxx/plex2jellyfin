# CLI Formatter and ASCII Header Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create shared formatter utility to eliminate DRY violations and restore ASCII header to CLI output

**Architecture:** Extract duplicate formatBytes() function into shared cmd/jellywatch/formatter.go utility, then add ASCII header display with version info to root command. Uses embed package to include ASCII art at compile time.

**Tech Stack:** Go 1.21+, Cobra CLI framework, embed package for compile-time assets

---

## Task 1: Create formatter.go utility

**Files:**
- Create: `cmd/jellywatch/formatter.go`

**Step 1: Create formatter utility with formatBytes function**

```go
package main

import "fmt"

// formatBytes converts bytes to human-readable format (e.g., "1.5 GB")
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
```

**Step 2: Verify file compiles**

Run: `go build -o /dev/null ./cmd/jellywatch`
Expected: SUCCESS (no output)

**Step 3: Commit**

```bash
git add cmd/jellywatch/formatter.go
git commit -m "refactor(cli): create shared formatter utility"
```

---

## Task 2: Remove formatBytes from main.go

**Files:**
- Modify: `cmd/jellywatch/main.go:984-995`

**Step 1: Delete formatBytes function from main.go**

Remove lines 984-995 (the entire formatBytes function including its body).

**Step 2: Verify formatBytes is still accessible**

Run: `go build -o /dev/null ./cmd/jellywatch`
Expected: SUCCESS (formatBytes now comes from formatter.go)

**Step 3: Commit**

```bash
git add cmd/jellywatch/main.go
git commit -m "refactor(cli): remove duplicate formatBytes from main.go"
```

---

## Task 3: Remove formatBytes from duplicates_cmd.go

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Find formatBytes function in duplicates_cmd.go**

Run: `grep -n "^func formatBytes" cmd/jellywatch/duplicates_cmd.go`
Expected: Shows line number where function is defined

**Step 2: Delete formatBytes function**

Remove the entire formatBytes function (typically around 12 lines based on pattern in main.go).

**Step 3: Verify build**

Run: `go build -o /dev/null ./cmd/jellywatch`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "refactor(cli): remove duplicate formatBytes from duplicates_cmd.go"
```

---

## Task 4: Remove formatBytes from consolidate_cmd.go

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go`

**Step 1: Find formatBytes function in consolidate_cmd.go**

Run: `grep -n "^func formatBytes" cmd/jellywatch/consolidate_cmd.go`
Expected: Shows line number where function is defined

**Step 2: Delete formatBytes function**

Remove the entire formatBytes function.

**Step 3: Verify build**

Run: `go build -o /dev/null ./cmd/jellywatch`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "refactor(cli): remove duplicate formatBytes from consolidate_cmd.go"
```

---

## Task 5: Remove formatBytes from status_cmd.go

**Files:**
- Modify: `cmd/jellywatch/status_cmd.go`

**Step 1: Find formatBytes function in status_cmd.go**

Run: `grep -n "^func formatBytes" cmd/jellywatch/status_cmd.go`
Expected: Shows line number where function is defined

**Step 2: Delete formatBytes function**

Remove the entire formatBytes function.

**Step 3: Verify build**

Run: `go build -o /dev/null ./cmd/jellywatch`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/status_cmd.go
git commit -m "refactor(cli): remove duplicate formatBytes from status_cmd.go"
```

---

## Task 6: Add ASCII header to formatter.go

**Files:**
- Modify: `cmd/jellywatch/formatter.go`

**Step 1: Add embed directive and ASCII art**

Add at the top of the file (after package declaration):

```go
package main

import (
	_ "embed"
	"fmt"
)

//go:embed assets/header.txt
var asciiHeader string

// formatBytes converts bytes to human-readable format (e.g., "1.5 GB")
func formatBytes(bytes int64) string {
	// ... existing implementation
}

// printHeader displays the ASCII header with version info
func printHeader(version string) {
	fmt.Println(asciiHeader)
	fmt.Printf("Version: %s\n\n", version)
}
```

**Step 2: Create assets directory and header file**

Run: `mkdir -p cmd/jellywatch/assets`

Then copy ASCII header:
```bash
cp /home/nomadx/bit/jellywatch.txt cmd/jellywatch/assets/header.txt
```

**Step 3: Verify embed works**

Run: `go build -o /dev/null ./cmd/jellywatch`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/formatter.go cmd/jellywatch/assets/header.txt
git commit -m "feat(cli): add ASCII header with embed support"
```

---

## Task 7: Add version command and variable

**Files:**
- Modify: `cmd/jellywatch/version_cmd.go`
- Modify: `cmd/jellywatch/main.go`

**Step 1: Check if version variable exists**

Run: `grep -n "^var version" cmd/jellywatch/main.go`
Expected: Either shows existing version var or no output

**Step 2: Add version variable to main.go if missing**

Add near top of main.go (after imports, before main function):

```go
var (
	version = "dev" // Set by build flags: -ldflags="-X main.version=1.0.0"
	cfgFile        string
	// ... rest of existing vars
)
```

**Step 3: Check version_cmd.go implementation**

Run: `cat cmd/jellywatch/version_cmd.go`
Expected: Shows current version command implementation

**Step 4: Update version command to use printHeader**

Modify version_cmd.go to call printHeader(version) instead of just printing version text.

Expected pattern:
```go
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			printHeader(version)
		},
	}
}
```

**Step 5: Build and test version command**

Run: `go build -o build/jellywatch ./cmd/jellywatch && ./build/jellywatch version`
Expected: ASCII header displays with "Version: dev"

**Step 6: Commit**

```bash
git add cmd/jellywatch/main.go cmd/jellywatch/version_cmd.go
git commit -m "feat(cli): display ASCII header in version command"
```

---

## Task 8: Add header to root command help

**Files:**
- Modify: `cmd/jellywatch/main.go:40-51`

**Step 1: Add custom help template to root command**

In main() function, after rootCmd definition and before flag setup:

```go
func main() {
	rootCmd := &cobra.Command{
		Use:   "jellywatch",
		Short: "Media file organizer for Jellyfin libraries",
		Long: `JellyWatch monitors download directories and automatically organizes
media files according to Jellyfin naming conventions.

Features:
  - Robust file transfers with timeout handling (won't hang on failing disks)
  - Automatic TV show and movie detection
  - Jellyfin-compliant naming: "Show Name (Year) S01E01.ext"
  - Sonarr integration for queue management`,
	}

	// Add custom help function to show ASCII header
	originalHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd.Name() == "jellywatch" {
			printHeader(version)
		}
		originalHelpFunc(cmd, args)
	})

	// ... rest of main function
}
```

**Step 2: Build and test root help**

Run: `go build -o build/jellywatch ./cmd/jellywatch && ./build/jellywatch --help`
Expected: ASCII header displays at top, followed by normal help text

**Step 3: Test that subcommands don't show header**

Run: `./build/jellywatch organize --help`
Expected: NO ASCII header (only shown for root command)

**Step 4: Commit**

```bash
git add cmd/jellywatch/main.go
git commit -m "feat(cli): display ASCII header in root command help"
```

---

## Task 9: Integration testing

**Files:**
- None (testing only)

**Step 1: Build final binary**

Run: `go build -o build/jellywatch ./cmd/jellywatch`
Expected: SUCCESS

**Step 2: Test all commands with formatBytes usage**

Run each command to verify formatBytes works:
```bash
./build/jellywatch status
./build/jellywatch duplicates
./build/jellywatch consolidate --generate
```
Expected: All display byte sizes correctly (e.g., "1.5 GB")

**Step 3: Test header display**

Run: `./build/jellywatch --help`
Expected: ASCII header at top

Run: `./build/jellywatch version`
Expected: ASCII header with version info

Run: `./build/jellywatch organize --help`
Expected: NO header (subcommand help)

**Step 4: Verify no duplicate formatBytes**

Run: `grep -r "^func formatBytes" cmd/jellywatch --include="*.go"`
Expected: Only one match in `cmd/jellywatch/formatter.go`

**Step 5: Run full test suite**

Run: `go test ./...`
Expected: All tests PASS

---

## Task 10: Final commit and documentation

**Files:**
- Modify: `docs/plans/2026-01-26-cli-formatter-and-header.md` (this file)

**Step 1: Verify all commits are clean**

Run: `git log --oneline -10`
Expected: See 7 commits for this feature

**Step 2: Mark plan as complete**

Add to end of this file:

```markdown
## Implementation Status

✅ Task 1: Create formatter.go utility - COMPLETE
✅ Task 2: Remove formatBytes from main.go - COMPLETE
✅ Task 3: Remove formatBytes from duplicates_cmd.go - COMPLETE
✅ Task 4: Remove formatBytes from consolidate_cmd.go - COMPLETE
✅ Task 5: Remove formatBytes from status_cmd.go - COMPLETE
✅ Task 6: Add ASCII header to formatter.go - COMPLETE
✅ Task 7: Add version command and variable - COMPLETE
✅ Task 8: Add header to root command help - COMPLETE
✅ Task 9: Integration testing - COMPLETE
✅ Task 10: Final commit and documentation - COMPLETE

**Implementation completed:** YYYY-MM-DD
**Total commits:** 7
**Tests passing:** ✅
```

**Step 3: Final commit**

```bash
git add docs/plans/2026-01-26-cli-formatter-and-header.md
git commit -m "docs: mark CLI formatter and header implementation as complete"
```

---

## Testing Checklist

- [ ] `formatBytes()` exists only in `formatter.go`
- [ ] All 4 files (main, duplicates, consolidate, status) use shared formatBytes
- [ ] ASCII header displays in `jellywatch --help`
- [ ] ASCII header displays in `jellywatch version`
- [ ] ASCII header does NOT display in subcommand help (e.g., `organize --help`)
- [ ] All commands that show byte sizes work correctly
- [ ] Full test suite passes (`go test ./...`)
- [ ] Binary builds successfully

---

## Rollback Plan

If issues arise:

1. Revert commits: `git revert HEAD~7..HEAD`
2. Each task is atomic and can be reverted individually
3. Tests verify functionality at each step

---

## Implementation Status

✅ Task 1: Create formatter.go utility - COMPLETE
✅ Task 2: Remove formatBytes from main.go - COMPLETE
✅ Task 3: Remove formatBytes from duplicates_cmd.go - COMPLETE (no duplicate existed)
✅ Task 4: Remove formatBytes from consolidate_cmd.go - COMPLETE (no duplicate existed)
✅ Task 5: Remove formatBytes from status_cmd.go - COMPLETE (no duplicate existed)
✅ Task 6: Add ASCII header to formatter.go - COMPLETE
✅ Task 7: Add version command and variable - COMPLETE
✅ Task 8: Add header to root command help - COMPLETE
✅ Task 9: Integration testing - COMPLETE
✅ Task 10: Final commit and documentation - COMPLETE

**Implementation completed:** 2026-01-26
**Total commits:** 4 (Tasks 1&2 combined, 6, 7, 8)
**Tests passing:** ✅ All tests pass

**Note:** Tasks 3-5 found no duplicate formatBytes functions to remove. Only main.go had a duplicate which was removed in Task 1&2.
