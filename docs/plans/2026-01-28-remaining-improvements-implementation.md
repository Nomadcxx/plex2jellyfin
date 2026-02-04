# Remaining Code Quality Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace panics with errors, add tests for AI matcher, reduce fmt.Print in consolidate, make sleeps configurable

**Architecture:** Refactor MustNew to return errors, add writer interface for output, use config structs for timing, TDD approach for new tests

**Tech Stack:** Go, httptest for AI mocking, temp directories for file testing, bytes.Buffer for output capture

---

## Task 1: Replace MustNew Panic with Error Return

**Files:**
- Modify: `internal/transfer/transfer.go:200-220`
- Modify: `internal/organizer/organizer.go:50-60`
- Search: All files using `transfer.MustNew`

**Step 1: Read current MustNew implementation**

Run: `grep -n "func MustNew" internal/transfer/transfer.go`
Read lines 200-220 of internal/transfer/transfer.go

**Step 2: Change MustNew signature to New**

Modify `internal/transfer/transfer.go`:

```go
// New creates a transferer for the given backend, returning an error
func New(backend Backend) (Transferer, error) {
	switch backend {
	case BackendNative:
		return &NativeTransferer{}, nil
	case BackendRsync:
		if !rsyncAvailable() {
			return nil, fmt.Errorf("rsync not available in PATH")
		}
		return &RsyncTransferer{}, nil
	case BackendPV:
		if !pvAvailable() {
			return nil, fmt.Errorf("pv not available in PATH")
		}
		return &PVTransferer{}, nil
	default:
		return nil, fmt.Errorf("unknown backend: %v", backend)
	}
}
```

**Step 3: Add MustNew wrapper (optional, deprecated)**

```go
// MustNew creates a transferer, panicking on error.
// Deprecated: Use New instead to handle errors properly.
func MustNew(backend Backend) Transferer {
	t, err := New(backend)
	if err != nil {
		panic(fmt.Sprintf("transfer.MustNew: %v", err))
	}
	return t
}
```

**Step 4: Find all MustNew usages**

Run: `grep -rn "transfer.MustNew\|MustNew(" --include="*.go"`
Expected: Shows files using MustNew

**Step 5: Update organizer.go**

Read `internal/organizer/organizer.go` lines 45-65

Replace panic with error handling:

```go
// Before (line 53):
transferer := transfer.MustNew(backend)

// After:
transferer, err := transfer.New(backend)
if err != nil {
	return nil, fmt.Errorf("failed to create transferer: %w", err)
}
```

**Step 6: Build to check for other callers**

Run: `go build ./...`
Expected: May show other errors if MustNew is used elsewhere

**Step 7: Fix any remaining callers**

For each file with MustNew:
- Change to transfer.New(backend)
- Handle error appropriately

**Step 8: Run tests**

Run: `go test ./internal/transfer -v`
Expected: PASS

Run: `go test ./internal/organizer -v`
Expected: PASS

**Step 9: Commit**

```bash
git add internal/transfer/transfer.go internal/organizer/organizer.go
git commit -m "feat(transfer): replace MustNew panic with error return"
```

---

## Task 2: Add Tests for AI Matcher

**Files:**
- Create: `internal/ai/matcher_test.go`
- Modify: `internal/ai/matcher.go` (if needed for testability)

**Step 1: Read current matcher implementation**

Read `internal/ai/matcher.go` to understand:
- Parse() function signature
- Result struct fields
- Dependencies (Ollama client, etc.)

**Step 2: Write failing test for Parse**

Create `internal/ai/matcher_test.go`:

```go
package ai

import (
	"testing"
)

func TestMatcher_ParseMovie(t *testing.T) {
	// Create mock Ollama server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return mock response
		response := `{
			"response": "{\"title\": \"The Matrix\", \"year\": 1999}"
		}`
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	// Create matcher with mock server
	cfg := Config{
		OllamaEndpoint: server.URL,
		Model:          "test-model",
	}
	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	// Test parsing movie filename
	result, err := matcher.Parse("The.Matrix.1999.1080p.BluRay.mkv")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify result
	if result.Title != "The Matrix" {
		t.Errorf("Expected title 'The Matrix', got '%s'", result.Title)
	}
	if result.Year == nil || *result.Year != 1999 {
		t.Errorf("Expected year 1999, got %v", result.Year)
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/ai -run TestMatcher_ParseMovie -v`
Expected: FAIL with error (mock server or interface issues)

**Step 4: Add test for TV episode**

```go
func TestMatcher_ParseTVEpisode(t *testing.T) {
	// Create mock Ollama server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"response": "{\"title\": \"Breaking Bad\", \"season\": 1, \"episode\": 1}"
		}`
		w.WriteHeader(200)
		w.Write([]byte(response))
	}))
	defer server.Close()

	cfg := Config{
		OllamaEndpoint: server.URL,
		Model:          "test-model",
	}
	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	result, err := matcher.Parse("Breaking.Bad.S01E01.mkv")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "Breaking Bad" {
		t.Errorf("Expected title 'Breaking Bad', got '%s'", result.Title)
	}
	if result.Season == nil || *result.Season != 1 {
		t.Errorf("Expected season 1, got %v", result.Season)
	}
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/ai -v`
Expected: PASS (may need matcher.go adjustments)

**Step 6: Add edge case tests**

```go
func TestMatcher_ParseObfuscated(t *testing.T) {
	// Test with obfuscated filename
	result, err := matcher.Parse("abc123def.mkv")
	// Should handle gracefully
}

func TestMatcher_ParseEmptyResponse(t *testing.T) {
	// Test with empty/invalid AI response
}
```

**Step 7: Run all tests**

Run: `go test ./internal/ai -v`
Expected: All PASS

**Step 8: Commit**

```bash
git add internal/ai/matcher_test.go
git commit -m "test(ai): add tests for AI matcher"
```

---

## Task 3: Add Writer Interface to Consolidate Operations

**Files:**
- Modify: `internal/consolidate/operations.go`
- Modify: `internal/consolidate/executor.go`
- Test: `internal/consolidate/operations_test.go`

**Step 1: Read current operations.go**

Run: `wc -l internal/consolidate/operations.go`
Read first 100 lines to understand structure

**Step 2: Add Writer field to Executor**

Modify `internal/consolidate/executor.go`:

```go
type Executor struct {
	// existing fields...
	writer io.Writer
}

// NewExecutor creates a new executor with output writer
func NewExecutor(cfg Config, writer io.Writer) *Executor {
	if writer == nil {
		writer = os.Stdout
	}
	return &Executor{
		// existing fields...
		writer: writer,
	}
}
```

**Step 3: Add Printf method**

```go
// Printf writes formatted output
func (e *Executor) Printf(format string, args ...interface{}) {
	fmt.Fprintf(e.writer, format, args...)
}
```

**Step 4: Replace fmt.Printf with e.Printf**

In `internal/consolidate/operations.go`:

Replace all `fmt.Printf` with `e.Printf`:
```go
// Before:
fmt.Printf("Processing %d conflicts\n", len(conflicts))

// After:
e.Printf("Processing %d conflicts\n", len(conflicts))
```

**Step 5: Build to verify**

Run: `go build ./internal/consolidate`
Expected: PASS

**Step 6: Write test using buffer**

Create `internal/consolidate/operations_test.go`:

```go
package consolidate

import (
	"bytes"
	"testing"
)

func TestExecutorOutput(t *testing.T) {
	var buf bytes.Buffer

	executor := NewExecutor(Config{}, &buf)

	// Execute some operation
	executor.Printf("Test message: %d\n", 42)

	output := buf.String()
	if !strings.Contains(output, "Test message: 42") {
		t.Errorf("Expected output to contain 'Test message: 42', got '%s'", output)
	}
}
```

**Step 7: Run tests**

Run: `go test ./internal/consolidate -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/consolidate/executor.go internal/consolidate/operations.go internal/consolidate/operations_test.go
git commit -m "refactor(consolidate): add writer interface for testability"
```

---

## Task 4: Make Hardcoded Sleeps Configurable

**Files:**
- Modify: `cmd/installer/validation.go:390-400`
- Modify: `internal/ai/queue.go:200-215`

**Step 1: Read current sleep usage**

Read `cmd/installer/validation.go` lines 390-400
Read `internal/ai/queue.go` lines 200-215

**Step 2: Add InputDelay to installer config**

Modify `cmd/installer/validation.go`:

```go
type ValidationConfig struct {
	InputDelay time.Duration // Default: 200ms
}

func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		InputDelay: 200 * time.Millisecond,
	}
}
```

**Step 3: Replace hardcoded sleep**

```go
// Before:
time.Sleep(200 * time.Millisecond)

// After:
time.Sleep(cfg.InputDelay)
```

**Step 4: Add RetryDelay to AI queue config**

Modify `internal/ai/queue.go`:

```go
type QueueConfig struct {
	RetryDelay time.Duration // Default: 100ms
	MaxRetries int           // Default: 3
}

func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		RetryDelay: 100 * time.Millisecond,
		MaxRetries: 3,
	}
}
```

**Step 5: Replace hardcoded sleep in queue**

```go
// Before:
time.Sleep(100 * time.Millisecond)

// After:
time.Sleep(cfg.RetryDelay)
```

**Step 6: Build to verify**

Run: `go build ./cmd/installer ./internal/ai`
Expected: PASS

**Step 7: Write tests with fast config**

```go
func TestQueueWithFastConfig(t *testing.T) {
	cfg := DefaultQueueConfig()
	cfg.RetryDelay = 1 * time.Millisecond // Fast for tests

	queue := NewQueue(cfg)
	// Test queue operations
}
```

**Step 8: Commit**

```bash
git add cmd/installer/validation.go internal/ai/queue.go
git commit -m "feat: make hardcoded sleeps configurable"
```

---

## Task 5: Final Verification

**Files:**
- All modified files

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: All PASS

**Step 2: Build all binaries**

Run: `go build ./cmd/jellywatch`
Expected: SUCCESS

Run: `go build ./cmd/installer`
Expected: SUCCESS

Run: `go build ./cmd/jellywatchd`
Expected: SUCCESS

**Step 3: Check for any remaining issues**

Run: `go vet ./...`
Expected: No issues

**Step 4: Commit final changes**

```bash
git commit -m "chore: complete remaining code quality improvements"
```

---

## Summary

**Files changed:**
- `internal/transfer/transfer.go` - Replace MustNew with New
- `internal/organizer/organizer.go` - Handle error from New
- `internal/ai/matcher_test.go` - New tests for AI matcher
- `internal/consolidate/executor.go` - Add writer field
- `internal/consolidate/operations.go` - Use e.Printf instead of fmt.Printf
- `internal/consolidate/operations_test.go` - New tests
- `cmd/installer/validation.go` - Configurable sleep
- `internal/ai/queue.go` - Configurable sleep

**Testing:**
- AI matcher has unit tests
- Consolidate operations use writer interface
- All existing tests pass
- Configurable sleeps allow fast tests

**Ready for:** @superpowers:executing-plans
