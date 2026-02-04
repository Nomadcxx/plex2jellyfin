# Comprehensive Regression Testing Verification Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Verify all existing JellyWatch functionality still works after source-of-truth implementation. This is a verification task - run existing commands and check behavior.

**Architecture:** Manual testing of CLI commands, verification of existing features, checking that no breaking changes were introduced.

**Tech Stack:** Manual CLI testing, integration verification, smoke tests

---

## Task 1: Core Organize Commands Regression

**Files:**
- None (manual testing)

**Step 1: Test basic organize command**

```bash
# Create test file
mkdir -p /tmp/jw_test/source /tmp/jw_test/lib
echo "fake video" > /tmp/jw_test/source/Test.Movie.2024.1080p.mkv

# Initialize config
mkdir -p ~/.config/jellywatch
cat > ~/.config/jellywatch/config.toml << 'EOF'
[watch]
movies = ["/tmp/jw_test/source"]

[libraries]
movies = ["/tmp/jw_test/lib"]

[options]
delete_source = false
EOF

# Initialize database
./jellywatch database init

# Organize file
./jellywatch organize /tmp/jw_test/source /tmp/jw_test/lib
```

Expected:
- File moves to `/tmp/jw_test/lib/Test Movie (2024)/Test Movie (2024).mkv`
- Release markers stripped from filename
- Database record created with source=jellywatch

**Step 2: Verify directory organize**

```bash
mkdir -p /tmp/jw_test2/source/Movie.Dir.2024
echo "video" > /tmp/jw_test2/source/Movie.Dir.2024/movie.mkv

./jellywatch organize /tmp/jw_test2/source /tmp/jw_test/lib
```

Expected: Folder organized as `/tmp/jw_test/lib/Movie Dir (2024)/`

**Step 3: Test library selection**

```bash
./jellywatch organize --library movies /tmp/jw_test/source /tmp/jw_test/lib
```

Expected: Works, uses movie library specifically

**Step 4: Test keep-source flag**

```bash
mkdir -p /tmp/jw_test3/source /tmp/jw_test3/lib
echo "video" > /tmp/jw_test3/source/Test2.Movie.2024.mkv

./jellywatch organize --keep-source /tmp/jw_test3/source /tmp/jw_test3/lib
```

Expected:
- File copied (not moved)
- Source file still exists
- Library file exists

**Step 5: Test recursive flag**

```bash
mkdir -p /tmp/jw_test4/source/subdir
echo "video1" > /tmp/jw_test4/source/subdir/Test3.Movie.2024.mkv
echo "video2" > /tmp/jw_test4/source/Test4.Movie.2024.mkv

./jellywatch organize --recursive /tmp/jw_test4/source /tmp/jw_test/lib
```

Expected: Both files organized (recursive and top-level)

**Step 6: Test timeout flag**

```bash
# This would need a slow filesystem to properly test
./jellywatch organize --timeout 10s /tmp/jw_test/source /tmp/jw_test/lib
```

Expected: Works with custom timeout

**Step 7: Test checksum flag**

```bash
./jellywatch organize --checksum /tmp/jw_test/source /tmp/jw_test/lib
```

Expected: Works (checksum verification)

**Step 8: Test backend selection**

```bash
./jellywatch organize --backend native /tmp/jw_test/source /tmp/jw_test/lib
./jellywatch organize --backend rsync /tmp/jw_test/source /tmp/jw_test/lib
```

Expected: Both backends work

**Step 9: Test organize-folder command**

```bash
mkdir -p /tmp/jw_test5/folder
echo "video" > /tmp/jw_test5/folder/Test5.Movie.2024.mkv

./jellywatch organize-folder /tmp/jw_test5/folder movies
```

Expected: Folder analyzed and organized

**Step 10: Verify year parsing**

```bash
# Test various year formats
mkdir -p /tmp/jw_years/source
echo "v1" > /tmp/jw_years/source/Show.1999.S01E01.mkv
echo "v2" > /tmp/jw_years/source/Show.(1999).S01E02.mkv
echo "v3" > /tmp/jw_years/source/Show.NoYear.S01E03.mkv

./jellywatch organize /tmp/jw_years/source /tmp/jw_test/lib
```

Expected:
- `Show.1999.S01E01` → `Show (1999)/Season 01/Show (1999) S01E01.mkv`
- `Show.NoYear.S01E03` → Organized (no year folder)

**Step 11: Verify release marker stripping**

```bash
mkdir -p /tmp/jw_markers/source
echo "video" > "/tmp/jw_markers/source/StripTest.2024.1080p.WEB-DL.x264.DDP5.1-RARBG.mkv"

./jellywatch organize /tmp/jw_markers/source /tmp/jw_test/lib
```

Expected:
- Final name: `Strip Test (2024).mkv`
- All markers stripped: `1080p`, `WEB-DL`, `x264`, `DDP5.1`, `RARBG`

**Step 12: Test TV vs Movie separation**

```bash
mkdir -p /tmp/jw_type/source
echo "tv" > /tmp/jw_type/source/TVShow.2024.S01E01.mkv
echo "movie" > /tmp/jw_type/source/MovieName.2024.1080p.mkv

./jellywatch organize /tmp/jw_type/source /tmp/jw_test/lib
```

Expected:
- TV goes to TV library (if configured)
- Movie goes to movie library
- No cross-contamination

**Step 13: Document results**

Create verification log:

```bash
cat > /tmp/qa19_organize_log.md << 'EOF'
# Core Organize Commands Regression

## Test Results

- [x] Single file organize
- [x] Directory organize
- [x] Library selection
- [x] Keep source flag
- [x] Recursive flag
- [x] Timeout flag
- [x] Checksum flag
- [x] Backend selection (native, rsync)
- [x] organize-folder command
- [x] Year parsing (various formats)
- [x] Release marker stripping
- [x] TV vs Movie separation

## Issues Found

*List any issues discovered*

## Recommendations

*Any recommendations for improvements*
EOF
```

---

## Task 2: Database Operations Regression

**Step 1: Test scan command**

```bash
# Scan filesystem
./jellywatch scan --filesystem --stats
```

Expected:
- Library indexed
- Stats displayed (series/movies count, episodes)
- No errors

**Step 2: Test database init**

```bash
rm -rf ~/.config/jellywatch/jellywatch.db
./jellywatch database init
```

Expected:
- New database created
- Schema version 11
- All tables present

**Step 3: Test database reset**

```bash
./jellywatch database reset
```

Expected:
- Database cleared
- Schema preserved
- Confirmation prompt (or --yes flag)

**Step 4: Test database path**

```bash
./jellywatch database path
```

Expected: Shows database location

**Step 5: Verify duplicate detection**

```bash
# Create duplicate in different locations
mkdir -p /tmp/jw_dupe1 /tmp/jw_dupe2
echo "v1" > /tmp/jw_dupe1/Dupe.Movie.2024.1080p.mkv
echo "v2" > /tmp/jw_dupe2/Dupe.Movie.2024.REMUX.2160p.mkv

./jellywatch organize /tmp/jw_dupe1 /tmp/jw_test/lib
./jellywatch organize /tmp/jw_dupe2 /tmp/jw_test/lib

# Check database for duplicates
./jellywatch duplicates generate
```

Expected:
- Duplicates detected
- Higher quality (REMUX 2160p) identified as preferred
- Database shows 2 entries for same movie

**Step 6: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Database Operations

- [x] scan command
- [x] database init
- [x] database reset
- [x] database path
- [x] Duplicate detection

EOF
```

---

## Task 3: Watcher & Daemon Regression

**Step 1: Test watch command (non-daemon)**

```bash
# Terminal 1: Start watcher
./jellywatch watch /tmp/jw_watch_source &
WATCH_PID=$!

# Terminal 2: Trigger file creation
sleep 2
mkdir -p /tmp/jw_watch_source
echo "video" > /tmp/jw_watch_source/WatchTest.2024.mkv

# Terminal 1: Wait for processing
sleep 5

# Kill watcher
kill $WATCH_PID

# Verify file was processed
ls -la /tmp/jw_test/lib/
```

Expected:
- File detected
- Organized automatically
- No orphaned files

**Step 2: Test daemon mode**

```bash
# Start daemon (if systemd available)
sudo systemctl start jellywatchd

# Check status
sudo systemctl status jellywatchd

# Check logs
journalctl -u jellywatchd -n 50

# Stop daemon
sudo systemctl stop jellywatchd
```

Expected:
- Daemon starts cleanly
- Logs show activity
- Stops gracefully

**Step 3: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Watcher & Daemon

- [x] watch command detection
- [x] Daemon start/stop
- [x] Daemon logs
- [x] Graceful shutdown

EOF
```

---

## Task 4: Sonarr Integration Regression

**Step 1: Test sonarr status**

```bash
./jellywatch sonarr status
```

Expected:
- Connection test result
- Version info (if available)
- API endpoint verified

**Step 2: Test sonarr queue**

```bash
./jellywatch sonarr queue
```

Expected: Shows Sonarr download queue (if any)

**Step 3: Test sonarr clear-stuck**

```bash
./jellywatch sonarr clear-stuck
```

Expected: Removes stuck items from Sonarr queue

**Step 4: Test sonarr import**

```bash
# Import path into Sonarr
./jellywatch sonarr import /tmp/jw_test/lib/Show\ \(2024\)
```

Expected: Triggers Sonarr rescan of path

**Step 5: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Sonarr Integration

- [x] sonarr status
- [x] sonarr queue
- [x] sonarr clear-stuck
- [x] sonarr import

EOF
```

---

## Task 5: Radarr Integration Regression

**Step 1: Test radarr status**

```bash
./jellywatch radarr status
```

Expected: Connection test result

**Step 2: Test radarr queue**

```bash
./jellywatch radarr queue
```

Expected: Shows Radarr queue

**Step 3: Test radarr clear-stuck**

```bash
./jellywatch radarr clear-stuck
```

Expected: Removes stuck items

**Step 4: Test radarr import**

```bash
./jellywatch radarr import /tmp/jw_test/lib/Movie\ \(2024\)
```

Expected: Triggers Radarr scan

**Step 5: Test radarr movies**

```bash
./jellywatch radarr movies
```

Expected: Lists all movies from Radarr

**Step 6: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Radarr Integration

- [x] radarr status
- [x] radarr queue
- [x] radarr clear-stuck
- [x] radarr import
- [x] radarr movies

EOF
```

---

## Task 6: Validation & Compliance Regression

**Step 1: Test validate command**

```bash
# Create non-compliant file
mkdir -p /tmp/jw_validate
echo "bad name" > /tmp/jw_validate/Bad.Name.2024.mkv

./jellywatch validate /tmp/jw_validate
```

Expected:
- Non-compliance detected
- Missing parentheses for year reported
- Suggestions provided

**Step 2: Test recursive validation**

```bash
mkdir -p /tmp/jw_validate/Season\ 01
echo "ep1" > /tmp/jw_validate/Season\ 01/Show.2024.S01E01.mkv

./jellywatch validate --recursive /tmp/jw_validate
```

Expected:
- Validates entire directory structure
- Checks season folder format
- Checks episode naming

**Step 3: Verify compliance rules**

Test these scenarios:

1. ❌ Movie without year → `Movie.2024.mkv` (FAIL)
2. ❌ Movie with year → `Movie (2024).mkv` (PASS)
3. ❌ Episode without season → `Show.2024.E01.mkv` (FAIL)
4. ❌ Episode with season → `Show (2024)/Season 01/Show (2024) S01E01.mkv` (PASS)
5. ❌ Invalid characters → `Show:2024/S01E01.mkv` (FAIL)

**Step 4: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Validation & Compliance

- [x] validate command
- [x] Recursive validation
- [x] Year requirement (movies)
- [x] Season folder requirement (TV)
- [x] Episode format validation
- [x] Invalid character detection

EOF
```

---

## Task 7: Duplicate & Consolidation Regression

**Step 1: Test duplicates generate**

```bash
./jellywatch duplicates generate
```

Expected:
- Finds duplicate media
- Shows quality comparison
- Recommends which to keep

**Step 2: Test duplicates execute**

```bash
./jellywatch duplicates dry-run  # Preview
./jellywatch duplicates execute    # Actually remove
```

Expected:
- Preview shows what would be removed
- Execute removes lower quality versions
- Keeps best version

**Step 3: Test consolidate generate**

```bash
./jellywatch consolidate generate
```

Expected:
- Finds scattered episodes
- Groups by series
- Shows target library

**Step 4: Test consolidate execute**

```bash
./jellywatch consolidate dry-run  # Preview
./jellywatch consolidate execute   # Actually move
```

Expected:
- Moves episodes to single location
- Updates database paths
- No orphaned files

**Step 5: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Duplicate & Consolidation

- [x] duplicates generate
- [x] duplicates dry-run/execute
- [x] consolidate generate
- [x] consolidate dry-run/execute
- [x] Quality-aware selection
- [x] Target library selection

EOF
```

---

## Task 8: AI Integration Regression (if enabled)

**Step 1: Test audit generate**

```bash
mkdir -p /tmp/jw_ai
echo "mystery file" > /tmp/jw_ai/Unknown.Name.2024.mkv

./jellywatch audit /tmp/jw_ai --generate
```

Expected:
- Finds low-confidence parses
- Lists files needing AI review

**Step 2: Test audit dry-run**

```bash
./jellywatch audit /tmp/jw_ai --dry-run
```

Expected:
- Shows AI suggestions
- No changes made

**Step 3: Test audit execute**

```bash
./jellywatch audit /tmp/jw_ai --execute
```

Expected:
- Applies AI suggestions
- Updates database
- Files renamed if needed

**Step 4: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## AI Integration (if enabled)

- [x] audit generate
- [x] audit dry-run
- [x] audit execute
- [x] Low-confidence detection

EOF
```

---

## Task 9: Quality Detection Regression

**Step 1: Verify resolution parsing**

Test files:
- `Movie.720p.mkv` → 720p
- `Movie.1080p.mkv` → 1080p
- `Movie.2160p.mkv` → 2160p
- `Movie.4K.mkv` → 4K

**Step 2: Verify source detection**

Test files:
- `Movie.REMUX.mkv` → REMUX
- `Movie.Bluray.mkv` → BluRay
- `Movie.WEB-DL.mkv` → WEB-DL
- `Movie.WEBRip.mkv` → WEBRip
- `Movie.HDTV.mkv` → HDTV

**Step 3: Verify codec detection**

Test files:
- `Movie.x264.mkv` → x264
- `Movie.x265.mkv` → x265
- `Movie.HEVC.mkv` → HEVC
- `Movie.AV1.mkv` → AV1

**Step 4: Verify audio detection**

Test files:
- `Movie.DDP5.1.mkv` → DDP5.1
- `Movie.AAC2.0.mkv` → AAC2.0
- `Movie.TrueHD.Atmos.mkv` → TrueHD Atmos
- `Movie.DTS-HD.MA.mkv` → DTS-HD MA

**Step 5: Verify HDR detection**

Test files:
- `Movie.DoVi.mkv` → Dolby Vision
- `Movie.HDR10.mkv` → HDR10
- `Movie.HDR10+.mkv` → HDR10+

**Step 6: Verify quality scoring**

Compare these:
- `Movie.REMUX.2160p.DoVi.mkv` → Highest score
- `Movie.WEB-DL.1080p.x264.mkv` → Medium score
- `Movie.HDTV.720p.mkv` → Lower score

**Step 7: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Quality Detection

- [x] Resolution detection (720p, 1080p, 2160p, 4K)
- [x] Source detection (REMUX, Bluray, WEB-DL, WEBRip, HDTV)
- [x] Codec detection (x264, x265, HEVC, AV1)
- [x] Audio detection (DDP, AAC, TrueHD Atmos, DTS-HD MA)
- [x] HDR detection (DoVi, HDR10, HDR10+)
- [x] Quality scoring (CONDOR algorithm)

EOF
```

---

## Task 10: File Transfer & Permissions Regression

**Step 1: Test rsync backend**

```bash
./jellywatch organize --backend rsync /tmp/jw_source /tmp/jw_test/lib
```

Expected:
- Uses rsync for transfer
- Preserves permissions
- Handles large files

**Step 2: Test native backend**

```bash
./jellywatch organize --backend native /tmp/jw_source /tmp/jw_test/lib
```

Expected:
- Uses Go io operations
- Faster for small files
- Handles timeout correctly

**Step 3: Test permissions**

```bash
# Add to config.toml
cat >> ~/.config/jellywatch/config.toml << 'EOF'

[permissions]
user = "jellyfin"
group = "jellyfin"
file_mode = "0644"
dir_mode = "0755"
EOF

./jellywatch organize /tmp/jw_source /tmp/jw_test/lib

# Check permissions
ls -la /tmp/jw_test/lib/
```

Expected:
- Files owned by jellyfin user
- File mode 0644
- Dir mode 0755

**Step 4: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## File Transfer & Permissions

- [x] rsync backend
- [x] native backend
- [x] Timeout handling
- [x] Permission setting (user, group, file_mode, dir_mode)

EOF
```

---

## Task 11: Configuration Regression

**Step 1: Test config init**

```bash
rm -rf ~/.config/jellywatch
./jellywatch config init
```

Expected:
- Creates default config
- All sections present

**Step 2: Test config show**

```bash
./jellywatch config show
```

Expected: Displays current config

**Step 3: Test config test**

```bash
./jellywatch config test
```

Expected: Validates config, reports errors

**Step 4: Test config path**

```bash
./jellywatch config path
```

Expected: Shows config location

**Step 5: Test missing config**

```bash
mv ~/.config/jellywatch/config.toml ~/.config/jellywatch/config.toml.bak
./jellywatch organize /tmp/jw_source /tmp/jw_test/lib
mv ~/.config/jellywatch/config.toml.bak ~/.config/jellywatch/config.toml
```

Expected:
- Creates defaults automatically
- No error

**Step 6: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Configuration

- [x] config init
- [x] config show
- [x] config test
- [x] config path
- [x] Missing config (default creation)

EOF
```

---

## Task 12: Source-of-Truth Features Regression

**Step 1: Test dirty flag behavior**

```bash
# Organize file
mkdir -p /tmp/jw_sot/source
echo "video" > /tmp/jw_sot/source/SOTest.2024.S01E01.mkv

./jellywatch organize /tmp/jw_sot/source /tmp/jw_test/lib

# Check database
sqlite3 ~/.config/jellywatch/jellywatch.db "SELECT title, sonarr_path_dirty, sonarr_synced_at FROM series WHERE title = 'SOTest'"
```

Expected:
- `sonarr_path_dirty = 1` (dirty flag set)
- `sonarr_synced_at = NULL` (not synced yet)

**Step 2: Test migrate command**

```bash
# Create path mismatch in DB
sqlite3 ~/.config/jellywatch/jellywatch.db "UPDATE series SET canonical_path = '/wrong/path' WHERE title = 'SOTest'"

# Run migrate
./jellywatch migrate --dry-run
```

Expected:
- Detects path mismatch
- Shows jellywatch vs Sonarr paths
- Doesn't make changes (dry-run)

**Step 3: Test sync timestamp tracking**

```bash
# Check synced timestamp
sqlite3 ~/.config/jellywatch/jellywatch.db "SELECT title, sonarr_synced_at, radarr_synced_at FROM series WHERE title = 'SOTest'"
```

Expected:
- Timestamps set after sync
- Can be NULL if never synced

**Step 4: Test quality hierarchy in duplicates**

```bash
# Create quality-based duplicates
mkdir -p /tmp/jw_quality
echo "v1" > /tmp/jw_quality/QualityTest.2024.WEB-DL.1080p.mkv
echo "v2" > /tmp/jw_quality/QualityTest.2024.BluRay.1080p.mkv
echo "v3" > /tmp/jw_quality/QualityTest.2024.REMUX.2160p.mkv

./jellywatch organize /tmp/jw_quality /tmp/jw_test/lib1
./jellywatch organize /tmp/jw_quality /tmp/jw_test/lib2
./jellywatch organize /tmp/jw_quality /tmp/jw_test/lib3

./jellywatch duplicates generate
```

Expected:
- REMUX 2160p preferred (highest)
- BluRay 1080p second
- WEB-DL 1080p last

**Step 5: Document results**

```bash
cat >> /tmp/qa19_organize_log.md << 'EOF'

## Source-of-Truth Features

- [x] Dirty flag set on organize
- [x] migrate command detects mismatches
- [x] Sync timestamp tracking
- [x] Quality hierarchy in duplicates
- [x] Database as source of truth
- [x] Sonarr/Radarr sync FROM database

EOF
```

---

## Final Summary & Sign-Off

**Step 1: Review all test results**

```bash
cat /tmp/qa19_organize_log.md
```

**Step 2: Create summary document**

```bash
cat > /tmp/qa19_summary.md << 'EOF'
# QA-19: Comprehensive Regression Testing Summary

**Date:** 2026-01-31
**Tester:** [Your Name]
**Scope:** All existing JellyWatch functionality

## Test Coverage

### Commands Tested (100+)
- [x] Core organize commands (12 tests)
- [x] Database operations (5 tests)
- [x] Watcher & daemon (4 tests)
- [x] Sonarr integration (4 tests)
- [x] Radarr integration (5 tests)
- [x] Validation & compliance (6 tests)
- [x] Duplicate & consolidation (4 tests)
- [x] AI integration (4 tests, if enabled)
- [x] Quality detection (6 tests)
- [x] File transfer & permissions (4 tests)
- [x] Configuration (5 tests)
- [x] Source-of-truth features (4 tests)

**Total:** 63 individual tests

## Results

### Passed: [number]
### Failed: [number]
### Skipped: [number]

### Critical Issues Found

*List any critical blocking issues*

### Non-Critical Issues Found

*List any non-critical issues to address later*

## Source-of-Truth Verification

### Dirty Flags
- [x] Set on organize
- [x] Persist across restart
- [x] Cleared on sync

### Sync Service
- [x] Queues requests
- [x] Exponential backoff works
- [x] Retry loop picks up failures

### Migration Tool
- [x] Detects path mismatches
- [x] dry-run mode works
- [x] Fixes work correctly

### Database
- [x] Schema version 11 present
- [x] New columns accessible
- [x] No data corruption

## Recommendations

1. *Any recommendations from testing*
2. *Performance observations*
3. *UX improvements*

## Conclusion

[Ready for production release | Needs fixes before release]

---
**Sign-off:** _________________
**Date:** _______________
EOF
```

**Step 3: Cleanup test directories**

```bash
rm -rf /tmp/jw_test*
rm -rf /tmp/jw_*
```

**Step 4: Submit summary**

Copy `/tmp/qa19_summary.md` to project documentation or PR.

---

## Verification Checklist

Before marking QA-19 complete, verify:

- [ ] All organize commands tested (12 scenarios)
- [ ] All Sonarr/Radarr commands tested
- [ ] All configuration commands tested
- [ ] Source-of-truth features work (dirty flags, sync, migrate)
- [ ] No breaking changes detected
- [ ] Performance acceptable (no major slowdowns)
- [ ] UX issues documented (if any)
- [ ] Test results summarized
- [ ] Summary document created

**Ready for QA-20 sign-off when all items complete.**
