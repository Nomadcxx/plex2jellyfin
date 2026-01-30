# Architecture Improvements

## Summary

Documenting proposed improvements to JellyWatch architecture based on research into:
1. Licensing
2. Jellyfin API integration possibilities
3. Naming compliance edge cases
4. Overall architecture resilience

---

## 1. License Change

**Current**: MIT
**Proposed**: GPL-3.0 or later

**Rationale**: Copyleft license ensures derivative works share same license. User preference.

---

## 2. Jellyfin API Integration

### Current Approach

JellyWatch currently uses Sonarr and Radarr APIs for:
- Quality-based decisions (keep REMUX > BluRay > WEB-DL)
- Initial metadata lookups (provider IDs, episode/movie info)
- Download client interaction (check queues, grab status)

### Jellyfin API Capabilities

Research from official Jellyfin API (https://api.jellyfin.org, OpenAPI spec v10.11.6):

**What Jellyfin Offers**:
- ✅ Library management (add/remove paths, refresh libraries)
- ✅ Query library items (`GET /Items` with rich filters)
- ✅ Update item metadata (`POST /Items/{id}`)
- ✅ Delete items (`DELETE /Items/{id}`)
- ✅ Provider IDs (TMDB, TVDB, IMDB) in item responses
- ✅ Single API for movies and TV

**What Jellyfin Lacks** (that Sonarr/Radarr have):
- ❌ Quality metadata or quality profiles
- ❌ Built-in duplicate detection API
- ❌ File system operations (rename/move only updates Jellyfin DB)
- ❌ Download/import integration (Jellyfin is passive)
- ❌ Real-time file watching (periodic scans only)

### Recommended Integration Strategy

**Phase 1: Complement Sonarr/Radarr** (Immediate)

Use Jellyfin API for:
1. **Duplicate Detection** after organization
   ```go
   // Check if organized file already exists in Jellyfin library
   existingItems, _ := jellyfin.GetItems(GetItemsParams{
       SearchTerm: &newTitle,
       IncludeItemTypes: []BaseItemKind{Movie, Series},
   })
   if len(existingItems) > 0 {
       log.Warn("Duplicate found in Jellyfin library")
       // Handle deduplication
   }
   ```

2. **Library Verification** after organization
   ```go
   // Query Jellyfin to confirm file is recognized
   item, _ := jellyfin.GetItemById(newItemID)
   if item != nil && item.Path == expectedPath {
       log.Info("✅ Jellyfin recognized: ", expectedPath)
   }
   ```

3. **Metadata Refresh** trigger after adding content
   ```go
   jellyfin.PostItemsItemidRefresh(PostItemsItemidRefreshParams{
       ItemId: libraryID,
       Recursive: true,
       MetadataRefreshMode: "Default",
   })
   ```

4. **Library Path Configuration** (optional - programmatic setup)
   ```go
   jellyfin.PostLibraryVirtualFoldersPaths(PostLibraryVirtualFoldersPathsParams{
       PathInfo: &LibraryPathInfo{
           Name: "JellyWatch Movies",
           Path: "/mnt/storage1/Movies",
       },
   })
   ```

**Keep Sonarr/Radarr for**:
- Quality-based decisions (their strong suit)
- Initial metadata lookups (they have dedicated provider databases)
- Download client management

**Phase 2: Gradual Migration** (Optional - Future)

If Jellyfin adds quality metadata or plugin APIs, consider:
1. Direct Jellyfin metadata provider queries
2. Move quality decisions from Sonarr/Radarr to JellyWatch
3. Use Jellyfin's library refresh triggers more actively

---

## 3. Naming Compliance Improvements

### Critical Gap: Duplicate Year Detection

**Problem**: Files like `Matrix (2001) (2001).mkv` are flagged as **Jellyfin-compliant** even though they have duplicate years.

**Current Behavior**:
1. `ParseMovieName()` extracts FIRST year only: `"2001"` from `Matrix (2001) (2001)`
2. `removeYearAdvanced()` removes ALL year instances
3. `FormatMovieFilename()` adds single year in parentheses
4. Result: `"Matrix (2001).mkv"` (looks correct to compliance checker!)
5. `compliance.CheckMovie()` validates: ✅ Year in parentheses, ✅ No release markers
6. Database stores: `is_jellyfin_compliant = TRUE` ❌

**Where Detection Happens**:
- `internal/naming/confidence.go`: `hasDuplicateYear()` detects duplicate year pattern
- Applied to confidence scoring: `-0.5` penalty
- ❌ **NOT called by compliance checker**

**Fix Required**:

Add duplicate year check to `internal/compliance/compliance.go`:

```go
func CheckMovie(fullPath string) ComplianceResult {
    // ... existing checks ...
    
    // NEW: Check for duplicate years
    if hasDuplicateYear(filename) {
        return ComplianceResult{
            IsJellyfinCompliant: false,
            ComplianceIssues: []string{
                "duplicate_year: year appears multiple times in title",
            },
        }
    }
    
    // ... rest of checks
}
```

**Regex Pattern** (already in `internal/naming/confidence.go`):
```go
parenYearRegex := regexp.MustCompile(`\((19\d{2}|20\d{2})\)`)
plainYearRegex := regexp.MustCompile(`\b(19|20)\d{2}\b`)

func hasDuplicateYear(s string) bool {
    // Find parenthesized years and plain years
    // Check if same year appears multiple times (count > 1)
    // Exclude cases like "2001 A Space Odyssey" (year in title)
}
```

### Other Edge Cases to Address

| Issue | Current Handling | Needed Improvement |
|--------|------------------|-------------------|
| Multiple years: `"Title (2001) (2002) Remastered"` | ExtractLastYear keeps last valid year ✅ | Could flag in compliance if years conflict |
| Colons in title: `"Movie: The Beginning"` | `CleanMovieName()` replaces `:` with ` ` | Add to compliance checker: "colon not allowed in title" |
| Special chars in path: `< > : " / \ | ? *` | Compliance strips ✅ | Ensure all filesystem paths validate before moving |
| Parentheses in folder names: `Movie Name (2004)` | Allowed in filename, but folder paths need validation | Add folder name compliance check |

---

## 4. Overall Architecture Improvements

### Database Schema Enhancements

**Current** (schema v10):
```go
is_jellyfin_compliant BOOLEAN
compliance_issues TEXT
confidence REAL
needs_review BOOLEAN
```

**Proposed Additions**:
```go
// Add compliance issue tracking
compliance_issues TEXT       // JSON array: ["duplicate_year", "missing_year"]
compliance_timestamp TIMESTAMP // When issue was detected
last_audit_at TIMESTAMP      // Last time file was reviewed

// Add file metadata
original_filename TEXT          // Store original filename before parsing
parsed_title TEXT             // What we extracted
parsed_year TEXT              // Extracted year
parse_confidence REAL          // Initial confidence score
```

### Error Handling Strategy

**Current**: Some operations may partially fail (file moves, API calls)

**Proposed**:
1. **Atomic Operations**: Either complete all changes or rollback
2. **Retry with Exponential Backoff** (especially for external APIs)
3. **Graceful Degradation**: If Jellyfin API unavailable, queue operation and retry
4. **Deadlock Prevention**: Timeouts on file operations (use `internal/transfer` package)
5. **Comprehensive Logging**: Log all failures with context for debugging

### Audit Trail

**Proposed**: Track all file operations in database:
```go
CREATE TABLE operations_log (
    id INTEGER PRIMARY KEY,
    file_id INTEGER REFERENCES media_files(id),
    operation_type TEXT,  -- 'move', 'rename', 'delete', 'audit_fix'
    from_path TEXT,
    to_path TEXT,
    timestamp TIMESTAMP,
    success BOOLEAN,
    error_message TEXT
);
```

### Configuration Validation

**Current**: Loaded from TOML, basic validation exists

**Proposed**:
1. **Schema Validation**: Define expected structure at startup
2. **Path Existence Checks**: Validate watch and library paths before daemon starts
3. **API Connectivity Tests**: Test Sonarr/Radarr/Jellyfin connections
4. **AI Availability Check**: Verify Ollama endpoint before enabling AI features
5. **Disk Space Monitoring**: Warn if target drive < 10% free

### Multi-Drive Architecture

**Current**: Library selection in `internal/library/` picks target by space/existing content

**Proposed Enhancements**:
1. **Drive Health Checks**: Test write access before choosing drive
2. **Redundancy Tracking**: Mark files that exist on multiple drives (RAID)
3. **Smart Consolidation**: Detect when content spans drives and auto-suggest consolidation
4. **Load Balancing**: Distribute new content across drives by utilization

---

## Implementation Priority

1. **High Priority**: License change, duplicate year fix
2. **Medium Priority**: Jellyfin API integration for verification
3. **Low Priority**: Architecture improvements (audit trail, enhanced schema)

---

## Testing Strategy

For each improvement:

1. Add unit test
2. Add integration test (if external API)
3. Test edge cases manually
4. Document breaking changes

### Duplicate Year Test Cases

```go
func TestDuplicateYearCompliance(t *testing.T) {
    tests := []struct {
        input    string
        compliant bool
        issues    []string
    }{
        {"Matrix (2001) (2001)", false, []string{"duplicate_year"}},
        {"2001 A Space Odyssey (2001)", true, nil},  // Title year is OK
        {"Matrix (2001)", true, nil},                   // Single year, no duplicate
        {"Movie (2020) 2020", false, []string{"duplicate_year"}},
        {"Dune (2021)", true, nil},                  // Year in title only, valid
    }
    
    for _, tc := range tests {
        result := compliance.CheckMovie(tc.input)
        if result.IsJellyfinCompliant != tc.compliant {
            t.Errorf("FAIL: %s", tc.input)
        }
        if !sliceEqual(result.ComplianceIssues, tc.issues) {
            t.Errorf("ISSUES MISMATCH: %s", tc.input)
        }
    }
}
```
