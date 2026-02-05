# Automatic Sudo Privilege Escalation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically re-exec commands with sudo when root privileges are needed, providing a seamless user experience.

**Architecture:** Create `internal/privilege` package with `NeedsRoot()` and `Escalate()` functions. Commands check for root at startup and re-exec themselves via `syscall.Exec` with sudo, preserving all arguments. The existing `internal/paths` package handles SUDO_USER resolution for config paths.

**Tech Stack:** Go stdlib (`os`, `syscall`, `os/exec`), no external dependencies.

---

## Task 1: Create privilege package with NeedsRoot

**Files:**
- Create: `internal/privilege/escalate.go`
- Create: `internal/privilege/escalate_test.go`

**Step 1: Write the failing test**

Create `internal/privilege/escalate_test.go`:

```go
package privilege

import (
	"os"
	"testing"
)

func TestNeedsRoot_AsNonRoot(t *testing.T) {
	// Skip if actually running as root
	if os.Geteuid() == 0 {
		t.Skip("Test requires non-root user")
	}

	if !NeedsRoot() {
		t.Error("NeedsRoot() = false, want true when not root")
	}
}

func TestNeedsRoot_AsRoot(t *testing.T) {
	// This test only makes sense when running as root
	if os.Geteuid() != 0 {
		t.Skip("Test requires root user")
	}

	if NeedsRoot() {
		t.Error("NeedsRoot() = true, want false when root")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/privilege/... -v`
Expected: FAIL with "package not found" or "NeedsRoot not defined"

**Step 3: Write minimal implementation**

Create `internal/privilege/escalate.go`:

```go
// Package privilege provides automatic sudo escalation for commands requiring root.
package privilege

import (
	"os"
)

// NeedsRoot returns true if the current process is not running as root.
func NeedsRoot() bool {
	return os.Geteuid() != 0
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/privilege/... -v`
Expected: PASS (TestNeedsRoot_AsNonRoot passes, TestNeedsRoot_AsRoot skips)

**Step 5: Commit**

```bash
git add internal/privilege/
git commit -m "feat(privilege): add NeedsRoot function"
```

---

## Task 2: Add Escalate function

**Files:**
- Modify: `internal/privilege/escalate.go`

**Step 1: Add Escalate implementation**

Append to `internal/privilege/escalate.go`:

```go
import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Escalate re-executes the current command with sudo.
// The reason parameter explains why root is needed.
// This function does not return on success (process is replaced).
func Escalate(reason string) error {
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("sudo not found in PATH: %w", err)
	}

	fmt.Printf("Root privileges required to %s.\n", reason)
	fmt.Println("Requesting sudo access...")
	fmt.Println()

	// Build args: ["sudo", "jellywatch", "duplicates", "execute", ...]
	args := append([]string{"sudo"}, os.Args...)

	// Replace current process with sudo
	return syscall.Exec(sudoPath, args, os.Environ())
}
```

**Step 2: Verify build succeeds**

Run: `go build ./internal/privilege/...`
Expected: SUCCESS (no output)

**Step 3: Commit**

```bash
git add internal/privilege/escalate.go
git commit -m "feat(privilege): add Escalate function for sudo re-exec"
```

---

## Task 3: Update duplicates execute command

**Files:**
- Modify: `cmd/jellywatch/duplicates_cmd.go`

**Step 1: Find current root check**

The file contains around line 54-58:
```go
// Require root for file operations with proper permissions
if os.Geteuid() != 0 {
    fmt.Println("Error: This command requires root privileges for proper file permissions.")
    fmt.Println("Please run with: sudo jellywatch duplicates execute")
    return fmt.Errorf("root privileges required")
}
```

**Step 2: Add import and replace root check**

Add to imports:
```go
"github.com/Nomadcxx/jellywatch/internal/privilege"
```

Replace the root check block with:
```go
// Escalate to root if needed for file operations
if privilege.NeedsRoot() {
    return privilege.Escalate("delete files and modify ownership")
}
```

**Step 3: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/duplicates_cmd.go
git commit -m "feat(duplicates): auto-escalate to sudo when needed"
```

---

## Task 4: Update consolidate execute command

**Files:**
- Modify: `cmd/jellywatch/consolidate_cmd.go`

**Step 1: Find current root check**

The file contains around line 17-21:
```go
// Require root for file operations with proper permissions
if os.Geteuid() != 0 {
    fmt.Println("Error: This command requires root privileges for proper file permissions.")
    fmt.Println("Please run with: sudo jellywatch consolidate execute")
    return fmt.Errorf("root privileges required")
}
```

**Step 2: Add import and replace root check**

Add to imports:
```go
"github.com/Nomadcxx/jellywatch/internal/privilege"
```

Replace the root check block with:
```go
// Escalate to root if needed for file operations
if privilege.NeedsRoot() {
    return privilege.Escalate("move files across filesystems and set ownership")
}
```

**Step 3: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/consolidate_cmd.go
git commit -m "feat(consolidate): auto-escalate to sudo when needed"
```

---

## Task 5: Update watch command

**Files:**
- Modify: `cmd/jellywatch/main.go` (watch command definition)

**Step 1: Locate watch command RunE**

Search for the watch command's RunE function and add the escalation check at the start.

**Step 2: Add escalation check**

Add to imports if not present:
```go
"github.com/Nomadcxx/jellywatch/internal/privilege"
```

Add at the start of watch's RunE:
```go
// Escalate to root if needed for file ownership
if privilege.NeedsRoot() {
    return privilege.Escalate("set file ownership to match media server")
}
```

**Step 3: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/main.go
git commit -m "feat(watch): auto-escalate to sudo when needed"
```

---

## Task 6: Update organize command

**Files:**
- Modify: `cmd/jellywatch/main.go` (organize command definition)

**Step 1: Add escalation check to organize RunE**

Add at the start of organize's RunE:
```go
// Escalate to root if needed for file operations
if privilege.NeedsRoot() {
    return privilege.Escalate("move files and set ownership")
}
```

**Step 2: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/jellywatch/main.go
git commit -m "feat(organize): auto-escalate to sudo when needed"
```

---

## Task 7: Update organize-folder command

**Files:**
- Modify: `cmd/jellywatch/main.go` (organize-folder command definition)

**Step 1: Add escalation check to organize-folder RunE**

Add at the start of organize-folder's RunE:
```go
// Escalate to root if needed for file operations
if privilege.NeedsRoot() {
    return privilege.Escalate("move files and set ownership")
}
```

**Step 2: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/jellywatch/main.go
git commit -m "feat(organize-folder): auto-escalate to sudo when needed"
```

---

## Task 8: Update audit execute command

**Files:**
- Modify: `cmd/jellywatch/audit_cmd.go`

**Step 1: Locate audit execute subcommand**

Find the execute subcommand's RunE function.

**Step 2: Add escalation check**

Add to imports:
```go
"github.com/Nomadcxx/jellywatch/internal/privilege"
```

Add at the start of execute's RunE:
```go
// Escalate to root if needed for file operations
if privilege.NeedsRoot() {
    return privilege.Escalate("rename/delete files and modify ownership")
}
```

**Step 3: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/audit_cmd.go
git commit -m "feat(audit): auto-escalate to sudo when needed"
```

---

## Task 9: Update cleanup execute command

**Files:**
- Modify: `cmd/jellywatch/cleanup_cmd.go`

**Step 1: Locate cleanup execute subcommand**

Find the execute subcommand's RunE function.

**Step 2: Add escalation check**

Add to imports:
```go
"github.com/Nomadcxx/jellywatch/internal/privilege"
```

Add at the start of execute's RunE:
```go
// Escalate to root if needed for directory deletion
if privilege.NeedsRoot() {
    return privilege.Escalate("delete empty directories")
}
```

**Step 3: Build to verify**

Run: `go build ./cmd/jellywatch/...`
Expected: SUCCESS

**Step 4: Commit**

```bash
git add cmd/jellywatch/cleanup_cmd.go
git commit -m "feat(cleanup): auto-escalate to sudo when needed"
```

---

## Task 10: Update installer

**Files:**
- Modify: `cmd/installer/main.go`

**Step 1: Add escalation at startup**

Add to imports:
```go
"github.com/Nomadcxx/jellywatch/internal/privilege"
```

Add at the very start of main(), before any TUI setup:
```go
// Escalate to root immediately - installer needs privileges throughout
if privilege.NeedsRoot() {
    if err := privilege.Escalate("install binaries and configure systemd services"); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

**Step 2: Build to verify**

Run: `go build ./cmd/installer/...`
Expected: SUCCESS

**Step 3: Commit**

```bash
git add cmd/installer/main.go
git commit -m "feat(installer): auto-escalate to sudo at startup"
```

---

## Task 11: Build and test all binaries

**Files:** None (verification only)

**Step 1: Run all tests**

Run: `go test ./internal/privilege/... ./cmd/jellywatch/...`
Expected: All tests PASS

**Step 2: Build all binaries**

Run: `go build ./cmd/jellywatch && go build ./cmd/jellywatchd && go build ./cmd/installer`
Expected: SUCCESS (three binaries created)

**Step 3: Manual test (as non-root)**

Run: `./jellywatch duplicates execute`
Expected output:
```
Root privileges required to delete files and modify ownership.
Requesting sudo access...

[sudo] password for <user>:
```

**Step 4: Commit any fixes**

If any changes needed:
```bash
git add -A
git commit -m "fix: address build/test issues"
```

---

## Task 12: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

**Step 1: Add privilege package to WHERE TO LOOK table**

Add row:
```
| Privilege escalation | `internal/privilege/` | Auto sudo via syscall.Exec |
```

**Step 2: Add note about auto-escalation**

Add to NOTES section:
```
- **Auto sudo**: Commands requiring root automatically re-exec with sudo. Uses `internal/privilege` package.
```

**Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: document privilege escalation in AGENTS.md"
```
