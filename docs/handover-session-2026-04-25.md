# Handover: 2026-04-24 Session

## What Was Done

### Critical Bug Fix — `findExistingMediaFile` Deletion Bug
**File:** `internal/organizer/organizer.go` (lines 638-654 and 787-803)

**Problem:** When organizing a new episode into a season folder that already contained a different episode's video file, jellywatchd was deleting the existing episode. This happened because:
1. `findExistingMediaFile(seasonDir)` returned the "best quality" video in the folder — NOT the matching episode
2. The `sameEpisode` check at line 642 correctly identified "different episode" and returned early (didn't overwrite)
3. But the code then fell through to line 695 which still executed `os.Remove(existingFile)` — deleting the previous episode

This is why BEEF lost episodes S01E01, S01E03-E10, S02E01-E05, S02E07-E08 — only S01E02 (moved last in season 1) and S02E06 (moved last in season 2) survived because they were the most recent quality-ranked file at the time of the last move.

**Fix:** When `sameEpisode=false` (different episode already exists), the code now sets `existingFile=""` and `existingQuality=nil` to prevent the deletion block at line 695 from firing. Now different episodes are simply skipped, not deleted.

**Deployment:** Binary rebuilt, deployed to `/usr/local/bin/jellywatchd`, systemd service restarted. Confirmed running cleanly — initial scan completed with 0 errors, periodic scanner and AI enhancement ticker both active.

### Rate Limiter / AI Error Handling Fixes (Prior Session)
**Files:** `internal/ai/types.go`, `internal/ai/matcher.go`, `internal/daemon/handler.go`

- Added `HTTPError` type with `IsPermanent()` for 401/403/404
- Permanent errors no longer consume AI rate limit budget
- Permanent errors trigger immediate blacklisting without 10-retry waste
- Low-confidence AI results now fall back to regex organize path instead of silently returning

### Malware Cleanup
Deleted 7 folders from `/mnt/NVME3/Sabnzbd/complete/tv/` containing `.exe` Windows PE32 executables misnamed as media files. These were NOT valid mkvs — they were malware.

## Still Needs Doing

### Re-grab Missing BEEF Episodes
Sonarr needs to re-download the following lost episodes:

| Season | Episodes Lost |
|--------|---------------|
| S01 | E01, E03, E04, E05, E06, E07, E08, E09, E10 |
| S02 | E01, E02, E03, E04, E05, E07, E08 |

**Do not rely on jellywatch to re-import these** — the files are gone. Use Sonarr's "Season -> Manual Import" or force a re-grab.

### Re-grab Other Affected Shows
The malware cleanup deleted entire show folders. Re-grab via Sonarr:
- The Rookie S08E12
- Shrinking S03E09
- Ghosts 2021 S05E15
- Monarch Legacy of Monsters S02E05
- High Potential S02E16
- Family Guy S24E11
- American Dad S21E06

### One Piece EPxxxx Parser (`EPxxxx` format)
**File likely to need changes:** `internal/naming/`

Files like `One.Piece.EP1156.Episode.1156.1080p.NF.WEB-DL.JPN.AAC2.0.H.264.MSubs-ToonsHub.mkv` are failing to parse. The `EP1156` pattern (absolute episode numbering) is not recognized by the current TV parser. Jellywatch falls back to using the folder name but then fails because no episode markers are found in the parent folders.

This needs an absolute episode parser that:
1. Detects `EPxxxx` pattern
2. Maps absolute episode number to season/episode (or uses absolute numbering directly)

### Daily Show Date-Based Parser
**File likely to need changes:** `internal/naming/`

Files like `The.Daily.Show.2026.04.20.Annalena.Baerbock.1080p.WEB.h264-EDITH.mkv` fail with "no episode information found". The date-based episode naming pattern (YYYY.MM.DD) is not recognized. These are date-based episodes of a nightly talk/show.

This needs either:
1. A date-based episode parser that extracts the date and treats it as the episode identifier, OR
2. A special-case for The Daily Show / date-based shows in the naming conventions

## Ongoing Operations

### Check jellywatchd Status
```bash
sudo systemctl status jellywatchd
```

### Watch jellywatchd Logs
```bash
tail -f /root/.config/jellywatch/logs/jellywatch.log
```

### Check Activity Log
```bash
tail -f /root/.config/jellywatch/activity/activity-$(date +%Y-%m-%d).jsonl
```

### Restart jellywatchd If Needed
```bash
sudo systemctl restart jellywatchd
```

### Key Log Patterns
- `[handler] Processing file` — file being organized
- `[handler] Organization failed` — parse or move error (check error reason)
- `[handler] AI enhancement skipped` — low confidence, file queued for later
- `[organizer] warning: failed to remove existing file` — potential deletion issue
- `[daemon] Periodic scan starting` — scheduled scan triggered
- `JellyWatchd started` — daemon successfully restarted

### jellyfin Library Health
After re-grabbing missing episodes, verify in jellyfin:
1. BEEF should have all 16 episodes across both seasons
2. All re-grabbed shows should have their episodes
3. Run a library scan if episodes don't appear after re-import

## Known Limitations (Not Yet Fixed)
- `EPxxxx` absolute episode numbering not parsed → One Piece episodes sit in watch folder
- Date-based episode naming not parsed → Daily Show episodes sit in watch folder
- Periodic scan panics (nil pointer) — occurred ~864 times over 3 days, cause unknown, appeared to stop after daemon restart. Monitor.
