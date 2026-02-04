# Remaining Code Quality Improvements Design

**Date:** 2026-01-28
**Goal:** Complete remaining code quality improvements: replace panics, add tests, reduce fmt.Print, configurable sleeps

---

## 1. Replace Panics with Error Handling (Task B)

**Problem:** Panics crash the entire program and are not recoverable

**Files:**
- `internal/transfer/transfer.go:212` - MustNew panics on error
- `internal/organizer/organizer.go:53` - panic on transferer creation

**Solution:**

Change `MustNew(backend Backend) Transferer` to `New(backend Backend) (Transferer, error)`

Update all callers to handle errors:
```go
transferer, err := transfer.New(backend)
if err != nil {
    return fmt.Errorf("failed to create transferer: %w", err)
}
```

**Migration:**
1. Rename MustNew to New, add error return
2. Find all callers with grep
3. Update each to handle error
4. Mark MustNew as deprecated (optional)

---

## 2. Add Tests for Untested Packages (Task A)

**Priority order:**
1. internal/ai/matcher.go (core AI functionality)
2. internal/consolidate/operations.go (complex logic)
3. internal/api/handlers.go (API endpoints)
4. internal/daemon/*.go (background processes)

**Approach:** Table-driven tests with test fixtures

**Structure:**
```go
func TestMatcherParse(t *testing.T) {
    tests := []struct{
        name     string
        filename string
        want     *Result
        wantErr  bool
    }{
        {name: "clean movie", filename: "Movie.Title.2020.mkv", ...},
        // more cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test
        })
    }
}
```

**For AI testing:**
- Mock Ollama responses with httptest server
- Test successful and error responses
- Test parsing of various filename patterns

**For consolidation testing:**
- Mock filesystem with temp directories
- Test dry-run separately from execution
- Use bytes.Buffer to capture output

---

## 3. Reduce fmt.Print in Operations (Task C)

**Problem:** internal/consolidate/operations.go has 322 fmt.Printf statements

**Solution:** Writer interface injection

**Design:**
```go
type Executor struct {
    writer io.Writer  // For output
    dryRun bool
}

func NewExecutor(writer io.Writer, dryRun bool) *Executor {
    return &Executor{writer: writer, dryRun: dryRun}
}

func (e *Executor) Printf(format string, args ...interface{}) {
    fmt.Fprintf(e.writer, format, args...)
}
```

**Migration:**
1. Add `writer` field to Executor
2. Add Printf method that writes to writer
3. Replace all `fmt.Printf` with `e.Printf`
4. Update constructor to accept writer
5. Default to `os.Stdout` in production

**Testing:**
- Tests provide `bytes.Buffer` to capture output
- Easy to verify expected output

---

## 4. Make Hardcoded Sleeps Configurable (Task D)

**Problem:** Hardcoded values:
- cmd/installer/validation.go:396 (200ms)
- internal/ai/queue.go:209 (100ms)

**Solution:** Configurable duration fields

**Design:**
```go
type QueueConfig struct {
    RetryDelay    time.Duration  // Default: 100ms
    Timeout       time.Duration  // Default: 30s
    MaxRetries    int            // Default: 3
}

type ValidationConfig struct {
    InputDelay time.Duration  // Default: 200ms
}
```

**Benefits:**
- Override in tests for faster execution
- Users can tune for their environment
- No global state
- Context-aware (respects context cancellation)

**Migration:**
1. Add duration fields to config structs
2. Replace hardcoded values with config fields
3. Provide sensible defaults
4. Update constructors to accept config

---

## Implementation Order

**Priority 1 (MEDIUM): Replace panics**
- Highest impact on reliability
- Affects critical paths
- Smallest scope

**Priority 2 (MEDIUM): Add tests for AI matcher**
- Core functionality needs coverage
- Other tests can follow

**Priority 3 (LOW): Reduce fmt.Print**
- Improves testability
- Non-breaking change

**Priority 4 (LOW): Configurable sleeps**
- Nice to have
- Lowest priority

---

## Success Criteria

- [ ] All panics replaced with error returns
- [ ] internal/ai/matcher.go has tests
- [ ] internal/consolidate/operations.go uses writer interface
- [ ] Sleeps are configurable
- [ ] All existing tests still pass
- [ ] New tests pass
