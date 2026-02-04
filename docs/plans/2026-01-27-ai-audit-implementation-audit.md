# AI Audit Implementation Plan - Audit Report

**Date:** 2026-01-27
**Auditor:** Verification Agent
**Plan File:** `docs/plans/2026-01-27-ai-audit-implementation.md`

---

## Critical Issues Found

### 1. ❌ Database Schema Mismatch - FATAL

**Location:** Task 2, Step 4

**Problem:** Implementation plan queries `media_files.confidence` field which **does not exist** in the database schema.

**Actual Schema:**
```sql
CREATE TABLE media_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    size INTEGER NOT NULL,
    media_type TEXT NOT NULL,
    normalized_title TEXT NOT NULL,
    year INTEGER,
    season INTEGER,
    episode INTEGER,
    -- ... other fields ...
    -- NO confidence field!
)
```

**Confidence field is in `ai_parse_cache` table, not `media_files`.**

**Impact:** Database query will fail with "no such column: confidence"

**Required Fix:**
The audit should NOT query based on confidence. Instead, it should:
1. Scan `media_files` table for non-compliant files OR scan all files
2. For each file, run AI matcher to get fresh parse with confidence
3. Compare AI result with existing normalized_title
4. Flag as correction candidate if AI confidence > threshold AND differs from current

---

### 2. ❌ Missing `scan` Flag

**Location:** Task 1, Step 1

**Problem:** Design document shows a `scan` flag in CLI setup:

```go
var (
    scan       bool  // MISSING in implementation plan
    review     bool
    fix        bool
    verbose    bool
)
```

**Impact:** Implementation plan doesn't match design specification.

**Required Fix:** Add `scan` flag to command setup, even if not used initially.

---

### 3. ❌ Correction Struct Mismatch

**Location:** Task 2, Step 1

**Problem:** Implementation plan defines:

```go
type Correction struct {
    FileID         int64
    Filename       string
    CurrentTitle    string
    CurrentYear    *int
    SuggestedTitle string  // DESIGN uses "Suggested"
    SuggestedYear  *int
    Source         string
    Confidence     float64
    Action         string
}
```

**Design specifies:**
```go
type Correction struct {
    ID          int64       // Plan uses "FileID"
    Filename    string
    Current     string      // Plan uses "CurrentTitle"
    Suggested   string      // Plan uses "SuggestedTitle"
    Source      string
    Confidence  float64
    Action      string
}
```

**Impact:** Minor but plan doesn't match design document exactly.

**Required Fix:** Match field names exactly or note the difference as intentional.

---

### 4. ❌ Missing CLIContext Enum

**Location:** Task 3, Step 1

**Problem:** Design document specifies:

```go
type CLIContext int

const (
    CLIContext CLI = iota
    DaemonBackground
    ManualScan
)
```

**Implementation plan uses:**
```go
type RequestSource int

const (
    SourceDaemon RequestSource = iota
    SourceScanner
    SourceCLI
)
```

**Impact:** Different naming, different enum values. Doesn't match design.

**Required Fix:** Use `CLIContext` and the specific enum values from design.

---

### 5. ❌ Incorrect Method Signature

**Location:** Task 3, Step 2

**Problem:** Design specifies:

```go
func (i *Integrator) EnhanceTitleCLI(
    ctx context.Context,
    regexTitle, filename, mediaType string,
    requestSource CLIContext  // Parameter present
) (string, ParseSource, error)
```

**Implementation plan uses:**
```go
func (i *Integrator) EnhanceTitleCLI(
    ctx context.Context,
    filename, mediaType string,
    threshold float64  // Different parameter
) (*ai.Result, error)  // Different return types
```

**Impact:** Method signature doesn't match design at all.

**Required Fix:** Match the exact signature from design document.

---

### 6. ❌ Progress Bar Not Implemented

**Location:** Task 6, Step 1

**Problem:** Design mentions progress bar:

```go
func (e *AuditEngine) RunAudit(
    ctx context.Context,
    progress *Progress  // Progress parameter
) error {
    files := e.findLowConfidenceFiles(threshold)
    total := len(files)
    for i, file := range files {
        progress.Update(i, total, fmt.Sprintf("Checking: %s", file.Filename))
        // Process file...
    }
}
```

**Implementation plan doesn't mention progress bar or Progress struct.**

**Impact:** Missing design requirement for progress reporting.

**Required Fix:** Add progress reporting to runAudit implementation.

---

### 7. ❌ Security/Privacy Missing

**Location:** Not addressed in implementation plan

**Problem:** Design document specifies security considerations:

1. **AI Communication**
   - Verify Ollama endpoint is localhost or LAN only
   - No API keys or credentials in logs
   - Consider adding request timeout (30s default)

2. **Database Operations**
   - All AI corrections go through existing media_files table
   - Track who made changes (audit trail in ai_improvements table)
   - Support rollback if needed

3. **User Control**
   - Always ask before applying corrections
   - Show clear before/after comparison
   - Allow user to skip files or accept/reject suggestions

**Implementation plan doesn't address any of these.**

**Impact:** Missing security requirements from design.

**Required Fix:** Add security checks and considerations in implementation.

---

## Minor Issues

### 8. ⚠️ Output Format Not Specified

**Location:** Not addressed in implementation plan

**Problem:** Design shows specific output format:

```
=== Title Audit Report ===

Files with Low Confidence: 15
Files Corrected by AI: 12

[1] The.Matrix.1999.1080p.Bluray.x264-GROUP
    Current:    "The Matrix 1999.1080p Bluray x264" [Regex: 0.95]
    Sonarr:      "The Matrix (1999)" [No match]
    AI:          "The Matrix (1999)" [Confidence: 0.98]

    Action: [C] Accept AI suggestion
    Reason:  Special edition "1080p" incorrectly parsed as part of title
```

**Implementation plan doesn't specify output format.**

**Impact:** Output might not match design expectations.

---

### 9. ⚠️ Manual Edit Not Implemented

**Location:** Task 5, Step 3

**Problem:** Implementation plan has TODO comment:

```go
case "manual":
    // TODO: implement manual edit
    fmt.Println("Manual edit not yet implemented")
    skipped++
```

**Impact:** Feature promised in interactive review but not implemented.

---

## What the Implementation Plan Does Well

### ✅ Task Breakdown

- Tasks are bite-sized (2-5 minutes each)
- TDD approach with write test → run → implement cycle
- Frequent commits specified

### ✅ Code Structure

- Creates new audit_cmd.go as specified
- Uses existing Sonarr/Radarr clients
- Reuses ai_improvements table

### ✅ Testing Strategy

- Adds tests for AI integrator
- Adds tests for audit command
- Verifies database operations

### ✅ Documentation

- Updates README.md
- Adds usage examples

---

## Recommended Fixes

### Priority 1: CRITICAL - Must Fix Before Implementation

1. **Fix Database Query:** Remove confidence-based query from media_files table. Instead:
   - Query all media files or filter by is_jellyfin_compliant = 0
   - For each file, run AI matcher
   - Compare AI result with existing normalized_title

2. **Match Method Signatures:** Use exact signatures from design document:
   - `EnhanceTitleCLI(ctx, regexTitle, filename, mediaType, requestSource CLIContext) (string, ParseSource, error)`

3. **Use Correct Enums:** Use `CLIContext` enum with `CLIContext`, `DaemonBackground`, `ManualScan` values

### Priority 2: HIGH - Should Fix

4. **Add Progress Bar:** Implement progress reporting as specified in design

5. **Add Security Checks:** Verify Ollama endpoint, no credentials in logs, request timeouts

6. **Match Field Names:** Use exact field names from design (Correction struct)

### Priority 3: MEDIUM - Nice to Have

7. **Implement Manual Edit:** Allow users to manually edit AI suggestions

8. **Specify Output Format:** Define exact output matching design document

---

## Recommended Revision Workflow

1. **Stop** current implementation
2. **Fix** the CRITICAL issues in the plan
3. **Verify** database schema assumptions by running actual query tests
4. **Update** the implementation plan with corrected code
5. **Proceed** with implementation using the revised plan

---

## Conclusion

**Status:** ❌ IMPLEMENTATION PLAN NOT READY FOR EXECUTION

**Blockers:**
- Database schema mismatch (FATAL)
- Method signature mismatches (FATAL)
- Missing security requirements (HIGH)
- Missing progress reporting (HIGH)

**Recommended Action:** Rewrite the implementation plan with correct database queries, exact method signatures from design, and all security/progress requirements included before execution.

---

**Next Steps:**

1. Acknowledge this audit report
2. Decide whether to fix the plan or revise the approach
3. Re-run verification before claiming completion
