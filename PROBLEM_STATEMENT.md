# AI Integration Problems - State Document

**Date:** 2026-01-27
**Status:** BLOCKED - Critical architectural issues identified

---

## Executive Summary

The AI audit feature is **completely blocked** due to a fundamental missing piece: **the regex confidence calculation system exists only in jellysink, not in jellywatch**.

This is NOT a small fix - it's a **significant feature gap** that was never ported from the previous project.

---

## Problem #1: Missing Regex Confidence System

### What's Missing

Jellywatch has **NO** mechanism to calculate confidence for regex-based parsing.

### What Exists in jellysink (but NOT in jellywatch)

**File:** `/home/nomadx/Documents/jellysink/internal/scanner/tv_sorter.go`

```go
func calculateTitleConfidence(title, original string) float64 {
    confidence := 1.0

    // Major penalty for garbage titles (release group artifacts)
    isGarbage := IsGarbageTitle(title)
    if isGarbage {
        confidence -= 0.8
    }

    // Penalty for very short titles (likely truncated)
    if len(title) < 3 {
        confidence -= 0.5
    }

    // Penalty for single-word titles (often incomplete)
    if !strings.Contains(title, ") {
        confidence -= 0.3
    }

    // Bonus for year presence in original
    if strings.Contains(original, "(") && strings.Contains(original, ")") {
        confidence += 0.1
    }

    // Penalty if original has lots of release markers
    releaseMarkers := []string{"1080p", "720p", "x264", "BluRay", "WEB-DL"}
    for _, marker := range releaseMarkers {
        if strings.Contains(strings.ToUpper(original), marker) {
            confidence -= 0.1
            break
        }
    }

    // Clamp to 0.0-1.0 range
    return confidence
}
```

### What Jellywatch Has Instead

- Simple regex parsing in `internal/naming/`
- NO confidence scoring
- NO `calculateTitleConfidence()` function
- NO way to distinguish between "clean parse" vs "messy parse"

---

## Problem #2: Database Schema Mismatch

### Current Schema (Jellywatch)

**File:** `internal/database/schema.go`, version 4

```sql
CREATE TABLE media_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    size INTEGER NOT NULL,
    modified_at DATETIME,

    -- Classification
    media_type TEXT NOT NULL CHECK(media_type IN ('movie', 'episode')),
    parent_movie_id INTEGER,
    parent_series_id INTEGER,
    parent_episode_id INTEGER,

    -- Normalized identity
    normalized_title TEXT NOT NULL,
    year INTEGER,
    season INTEGER,
    episode INTEGER,

    -- Quality metadata
    resolution TEXT,
    source_type TEXT,
    codec TEXT,
    audio_format TEXT,
    quality_score INTEGER NOT NULL DEFAULT 0,

    -- Jellyfin compliance
    is_jellyfin_compliant BOOLEAN NOT NULL DEFAULT 0,
    compliance_issues TEXT,

    -- Provenance
    source TEXT NOT NULL DEFAULT 'filesystem',
    source_priority INTEGER NOT NULL DEFAULT 50,
    library_root TEXT,

    -- Timestamps
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (parent_movie_id) REFERENCES movies(id) ON DELETE SET NULL,
    FOREIGN KEY (parent_series_id) REFERENCES series(id) ON DELETE SET NULL,
    FOREIGN KEY (parent_episode_id) REFERENCES episodes(id) ON DELETE SET NULL
)
```

**MISSING:** `confidence REAL` field

### What Design Assumes

```go
// From design document:
confidenceThreshold = 0.6

// When to use AI
if regexConfidence < 0.6 {
    return AIEnhance  // Use Ollama
} else {
    return Regex  // Use existing regex
}
```

**BLOCKER:** `regexConfidence` doesn't exist, can't be compared to threshold.

---

## Problem #3: AI Integration Gap

### What Exists (Jellywatch)

**AI Infrastructure:**
- ✅ `internal/ai/matcher.go` - Ollama client, AI parsing
- ✅ `internal/ai/integrator.go` - Integration logic, cache, circuit breaker
- ✅ `internal/ai/cache.go` - Database caching of AI results
- ✅ `internal/ai/recovery.go` - Malformed JSON recovery
- ✅ `internal/ai/queue.go` - Background processing queue
- ✅ `internal/ai/status.go` - Status tracking
- ✅ `internal/ai/keepalive.go` - Ollama keepalive
- ✅ `internal/ai/circuit_breaker.go` - Circuit breaker logic
- ✅ `internal/database/ai_improvements.go` - AI improvements tracking

**Main Workflow Components:**
- ✅ `internal/scanner/scanner.go` - File scanning, creates media_files records
- ✅ `internal/daemon/handler.go` - File processing daemon
- ✅ `internal/organizer/organizer.go` - File organization
- ✅ `internal/naming/*` - Regex-based name parsing

### What's Missing

**No connection between:**
1. Scanner → AI Integrator
2. Handler → AI Integrator
3. AI Integrator → main workflow

**Current state:**
- AI infrastructure exists but is **completely disconnected** from main workflow
- Scanner uses ONLY regex parsing (`internal/naming`)
- Handler processes files with regex titles, never calls AI
- Confidence threshold 0.8 makes AI almost never trigger even if called

### Evidence from Code

**Scanner (`internal/scanner/scanner.go`):**
```go
func (s *FileScanner) processFile(...) error {
    // Parse title and episode/movie details
    var normalizedTitle string
    var year *int
    var season, episode *int

    if isEpisode {
        tv, err := naming.ParseTVShowName(filename)  // REGEX ONLY
        // ... process TV
    } else {
        movie, err := naming.ParseMovieName(filename)  // REGEX ONLY
        // ... process movie
    }

    // NO AI CALL HERE
    // NO CONFIDENCE CALCULATION
}
```

**Handler (`internal/daemon/handler.go`):**
```go
func (h *MediaHandler) processFile(path string) {
    // Parse with regex
    if isTVEpisode {
        if tvInfo, err := naming.ParseTVShowName(path); err == nil {
            parsedTitle = tvInfo.Title  // REGEX ONLY
            // ...
        }
    } else {
        if movieInfo, err := naming.ParseMovieName(path); err == nil {
            parsedTitle = movieInfo.Title  // REGEX ONLY
            // ...
        }
    }

    aiConfidence := 0.0  // HARDCODED, NEVER CALCULATED

    // NO AI ENHANCEMENT CALL
    // Uses aiMatcher.EnhanceTitle() ONLY in unused code paths
}
```

---

## Problem #4: Implementation Plan Inaccuracy

### Original Design Document Says:

```go
1. Scan media_files table for low confidence titles
2. For each low confidence file:
   - Check Sonarr/Radarr for metadata
   - Query AI for title correction
   - Display comparison (current vs AI)
   - Wait for user approval/correction
3. User approves/corrects:
4. Update media_files record with AI-enhanced metadata
5. Report summary
```

### What's Needed But Missing:

1. **regexConfidence calculation function** (exists in jellysink, not jellywatch)
2. **confidence column in media_files table** (doesn't exist)
3. **Scanner updated to calculate and store confidence** (currently doesn't)
4. **Query by confidence threshold** (can't without column)

### What Was Proposed Instead (WRONG):

- Remove confidence filtering entirely
- Query all files or non-compliant files
- Run AI for every file
- Compare AI results

**This is fundamentally different from design intent.**

---

## Problem #5: Comprehensive Lack of AI Integration

### AI Status Analysis

**Existing AI Components (all complete but unused):**

| Component | Status | Used in Production |
|-----------|---------|-------------------|
| Matcher (Ollama client) | ✅ Complete | ❌ NO |
| Integrator (enhancement logic) | ✅ Complete | ❌ NO |
| Cache (result caching) | ✅ Complete | ❌ NO |
| Queue (background processing) | ✅ Complete | ❌ NO |
| Circuit Breaker | ✅ Complete | ❌ NO |
| Keepalive (Ollama health) | ✅ Complete | ❌ NO |
| Status tracking | ✅ Complete | ❌ NO |
| AI Improvements DB | ✅ Complete | ✅ Used (but empty) |
| Confidence calculation | ❌ MISSING | ❌ N/A |

### Integration Points Where AI Should Connect

1. **Scanner:**
   - Calculate regex confidence during file scan
   - Store confidence in media_files table
   - Call AI Integrator to enhance titles (optional, threshold-based)

2. **Handler:**
   - Use AI Integrator for difficult files (low confidence, obfuscated)
   - Cache AI results for performance
   - Track AI improvements in database

3. **New Audit Command:**
   - Query files by confidence threshold (< 0.6)
   - For each, call AI to suggest improvement
   - Compare with Sonarr/Radarr metadata
   - Present to user for approval
   - Update media_files with accepted corrections

**NONE of these connections exist.**

---

## Root Cause Analysis

### Why This Happened

1. **Separate Projects:** jellysink and jellywatch developed independently
2. **Feature Gap:** Regex confidence system only developed in jellysink
3. **No Porting:** Confidence system never ported to jellywatch
4. **AI Isolated:** AI infrastructure added later, never integrated into main workflow
5. **Assumption Gap:** Design document assumed confidence exists, but it doesn't

### Technical Debt

- ~2000 lines of AI code in jellywatch **completely unused**
- Confidence calculation system exists but inaccessible
- Database schema needs migration for confidence field
- Scanner needs significant refactoring for confidence tracking
- Handler needs AI integration

---

## What's Required to Proceed

### Option A: Port Confidence System from jellysink (HUGE EFFORT)

**Required Changes:**

1. **Port `calculateTitleConfidence()`**
   - Copy function from jellysink
   - Adapt to jellywatch naming structures
   - Add tests

2. **Add `IsGarbageTitle()`**
   - Port from jellysink
   - Test against jellywatch naming patterns

3. **Schema Migration (v4 → v5)**
   ```sql
   ALTER TABLE media_files ADD COLUMN confidence REAL;
   UPDATE media_files SET confidence = 1.0 WHERE confidence IS NULL;
   CREATE INDEX idx_media_files_confidence ON media_files(confidence);
   ```

4. **Update Scanner**
   - Calculate confidence during file processing
   - Store confidence in MediaFile struct
   - Pass to UpsertMediaFile()

5. **Backfill Existing Data**
   - Run confidence calculation for all existing media_files
   - This is EXPENSIVE (scan every file and recalculate)

**Estimated Effort:** 2-3 days
**Risk:** Medium (backfill performance, regression risk)

### Option B: Simplified Approach - NO Confidence (MEDIUM EFFORT)

**Changes:**

1. **Schema Migration**
   ```sql
   ALTER TABLE media_files ADD COLUMN confidence REAL;
   ```

2. **Scanner Update**
   - Set confidence = 1.0 for ALL regex parses (simple heuristic)
   - Skip complex confidence calculation

3. **Audit Command Uses AI Differently**
   - Query non-compliant files only
   - For each, run AI matcher
   - Compare AI suggestion with current
   - User decides to accept or reject

**Estimated Effort:** 1 day
**Risk:** Low (less code to port, simpler logic)

### Option C: Redesign Without Confidence (LOW EFFORT, CHANGES DESIGN)

**Changes:**

1. **No schema change**
2. **No scanner change**
3. **Audit command:**
   - Query non-compliant files
   - Run AI for each
   - Present AI suggestions alongside current
   - User approves/rejects

**Estimated Effort:** 4-6 hours
**Risk:** Low (minimal changes, design change)

---

## Critical Questions Requiring Answer

### For Technical Approach:

1. **Which option do we pursue?**
   - A: Port full jellysink confidence system (2-3 days)
   - B: Simplified confidence heuristics (1 day)
   - C: Redesign to not require confidence (4-6 hours)

2. **If we choose B or C:**
   - Is design document acceptable to change?
   - Should we update design to reflect new approach?

3. **Schema migration:**
   - Should we run backfill for existing files?
   - Or only set confidence for new files going forward?

### For AI Integration:

4. **Do we want full AI integration into main workflow?**
   - Scanner calls AI for low-confidence files automatically?
   - Or keep AI only in audit command (on-demand)?

5. **AI confidence threshold:**
   - Design says lower to 0.6 for CLI mode
   - But daemon uses 0.8
   - Should scanner also use 0.6, or keep 0.8?

### For Testing:

6. **What confidence values should trigger AI?**
   - < 0.6 always AI?
   - < 0.8 sometimes AI?
   - What's the test coverage needed?

---

## Immediate Blockers

### Cannot Implement ANY of These Until:

1. ✅ Decision on technical approach (Option A/B/C)
2. ✅ Schema migration strategy
3. ✅ Backfill strategy (if Option A)
4. ✅ AI integration scope (audit-only vs full integration)
5. ✅ Threshold strategy (scanner vs CLI vs daemon)
6. ✅ Updated implementation plan reflecting decisions

---

## Technical Debt Summary

### Unused Infrastructure

| Component | Lines of Code | Status | Action Needed |
|-----------|-----------------|---------|---------------|
| AI Matcher | ~500 | Built but unused | Integrate into scanner/handler |
| AI Integrator | ~400 | Built but unused | Connect to main workflow |
| AI Cache | ~200 | Built but unused | Use in scanner/handler |
| AI Queue | ~200 | Built but unused | Decide if needed |
| AI Status | ~200 | Built but unused | Use in audit command |
| Keepalive | ~200 | Built but unused | Integrate if using AI |
| Circuit Breaker | ~200 | Built but unused | Use in audit command |
| AI Improvements DB | ~250 | Built, schema exists | Use in audit command |

**Total Unused Code:** ~2150 lines

### Missing Infrastructure

| Component | Estimated Lines | Priority |
|-----------|-----------------|----------|
| Confidence calculation | ~200 | CRITICAL |
| Schema migration | ~50 | CRITICAL |
| Scanner confidence integration | ~100 | CRITICAL |
| Audit command | ~600 | HIGH |
| AI CLI mode | ~50 | HIGH |
| Sonarr/Radarr metadata comparison | ~100 | MEDIUM |

**Total Needed Code:** ~1100 lines

---

## Recommendations

### Immediate (Before Any Implementation):

1. **STOP** - Do NOT proceed with current implementation plan
2. **DECIDE** - Choose technical approach (Option A/B/C)
3. **PLAN** - Create revised implementation plan reflecting decision
4. **TEST** - Consider adding confidence system tests before backfilling

### Short-Term (Next 1-2 days):

1. Implement chosen option (A/B/C)
2. Test thoroughly
3. Update design document to reflect actual implementation
4. Proceed with audit command implementation

### Long-Term (Future consideration):

1. Evaluate whether full AI integration into scanner/handler is valuable
2. Consider removing unused AI components if never used
3. Performance testing of AI integration
4. Monitoring of AI effectiveness

---

## Dependencies and Impact

### If We Proceed Without Fixing This:

**What Will Happen:**
- Implementation plan cannot be followed (queries will fail)
- Audit command will need complete rewrite of logic
- Database queries will fail (no confidence column)
- Scanner won't provide needed confidence data
- User will need to manually recalculate all confidences

**Timeline Impact:**
- Current plan: 4-7 days (unrealistic)
- Actual: +2-3 days (port confidence) or +1 day (simplify)
- Total: 6-10 days to working audit feature

### If We Fix This First:

**What We Get:**
- Realistic implementation plan
- Working confidence threshold filtering
- Proper AI integration path
- Clear scope and accurate effort estimates
- Testable incremental steps

**Timeline Impact:**
- Decision: 30 minutes
- Revised plan: 1-2 hours
- Implementation: 1-3 days (depending on option)
- **Total: 2-5 days to working audit feature**

---

## Success Criteria Clarification

### What "AI Audit" Means After Fixing This:

**Option A (Full Port):**
- ✅ Regex confidence calculated for all files during scan
- ✅ Confidence stored in media_files table
- ✅ Audit command queries files with confidence < 0.6
- ✅ AI used to enhance low-confidence titles
- ✅ Sonarr/Radarr validation
- ✅ User review and approval workflow
- ✅ Corrections applied to media_files

**Option B (Simplified):**
- ✅ Confidence set to 1.0 for regex parses
- ✅ Audit command queries non-compliant files
- ✅ AI used to suggest corrections
- ✅ User review and approval workflow
- ✅ Corrections applied to media_files

**Option C (No Confidence):**
- ✅ Audit command queries non-compliant files
- ✅ AI used to suggest corrections
- ✅ User review and approval workflow
- ✅ Corrections applied to media_files
- ⚠️ No confidence threshold filtering (design change)

---

## File Inventory for Fixing

### Files to Review/Modify:

**If Option A (Full Port):**
- `internal/scanner/scanner.go` - Add confidence calculation
- `internal/naming/garbage.go` - Port IsGarbageTitle (create if doesn't exist)
- `internal/database/schema.go` - Add confidence column, v5 migration
- `cmd/jellywatch/audit_cmd.go` - Implement as designed
- `internal/ai/integrator.go` - Add CLI mode

**If Option B (Simplified):**
- `internal/database/schema.go` - Add confidence column, v5 migration
- `internal/scanner/scanner.go` - Set confidence = 1.0
- `cmd/jellywatch/audit_cmd.go` - Implement with no threshold query

**If Option C (No Confidence):**
- `cmd/jellywatch/audit_cmd.go` - Implement without confidence
- No schema changes
- No scanner changes

### Files to Reference (Jellysink):

- `/home/nomadx/Documents/jellysink/internal/scanner/tv_sorter.go` - calculateTitleConfidence()
- `/home/nomadx/Documents/jellysink/internal/scanner/` - IsGarbageTitle()

---

## Conclusion

### Current State: BLOCKED

The AI audit feature cannot be implemented as designed because:

1. **Missing Core Infrastructure:** Regex confidence calculation system
2. **Database Schema Gap:** No confidence column in media_files table
3. **Integration Gap:** AI infrastructure completely disconnected from workflow
4. **Design Incompatibility:** Implementation plan assumes existence of missing pieces

### Required Action Path:

1. **CHOOSE** - Decide on technical approach (A/B/C)
2. **PORT or CREATE** - Implement confidence system or work around it
3. **MIGRATE** - Add confidence column to database
4. **INTEGRATE** - Connect AI to main workflow (or keep audit-only)
5. **IMPLEMENT** - Build audit command with working assumptions
6. **TEST** - Verify all pieces work together

### Estimated Timeline:

- **Decision phase:** 30-60 minutes
- **Plan revision:** 1-2 hours
- **Implementation:** 1-3 days (depending on option)
- **Testing:** 4-8 hours
- **Total:** 2-5 days to working feature

---

**Status:** Awaiting decision on approach before proceeding

**Next Step:** User to review options A/B/C and provide direction
