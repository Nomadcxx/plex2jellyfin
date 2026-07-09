# Plex2Jellyfin + Jellyfin Integration: Comprehensive Handover (2026-03-01)

## 1. Mission For Next Agent
You are taking over a **3-track incident/regression investigation**:

1. Web UI service regression (`http://localhost:5522` served placeholder/minimal page).
2. Jellyfin plugin webhook auth/contract mismatch and installer integration gap.
3. Plex2Jellyfin source-of-truth pipeline drift (Sonarr/Radarr import still active), plus local "The Pitt" visibility anomaly.

This handover contains validated evidence, architecture context, git history context, and exact reproduction commands.

## 2. Scope + Expected Deliverables
Deliverables expected from you:

1. Confirm/fix root cause for web UI serving placeholder output.
2. Confirm/fix webhook auth behavior across daemon/API/plugin with secure defaults and installer UX.
3. Restore/verify intended model: Plex2Jellyfin DB and move logic as source-of-truth, not Sonarr/Radarr import.
4. Produce a local-state incident report specifically for The Pitt S02E08 visibility anomaly.
5. Add/extend tests for all behavior changes.
6. Provide rollback plan and operational playbook.

## 3. Repository + Branch Context
- Repo path: `/home/nomadx/Documents/plex2jellyfin`
- Current branch: `fix/jellyfin-10.10-compat`
- Worktree is dirty with extensive ongoing changes (do not assume clean baseline).

Key commit context from recent history:
- `249568a` docs(plugin): test-ready build/install flow and packaging script fixes
- `1d62843` fix(plugin): Jellyfin 10.10 API migration + webhook contract
- `e1618e1` feat: add Jellyfin plugin support with webhook integration
- `50562b1` feat(api): static serving + SPA fallback
- `4bbb4cb` fix(installer): set SUDO_USER in systemd service
- Scanner/watch pipeline evolution:
  - `b59e6cc`, `44a067c`, `47bf3c0`, `ac3db21`, `dd5aa52`

Architecture references:
- `docs/architecture.md`
- `README.md`

## 4. High-Confidence Findings (Already Validated)

## 4.1 Issue 1: Web UI Placeholder Page
Symptoms:
- `plex2jellyfin-web` service log returns `GET /` as `200 52B`.
- Browser showed a minimal/unstyled page.

Validated causes observed:
1. Runtime embedding pointed at `embedded/web` in `embed.go` (historically placeholder content).
2. `embedded/frontend/index.html` had also been overwritten to placeholder text (`placeholder`) in local tree.

Evidence:
- `/etc/systemd/system/plex2jellyfin-web.service` runs `/usr/local/bin/plex2jellyfin-web --host 0.0.0.0 --port 5522`
- `systemctl status plex2jellyfin-web --no-pager -n 200` showed repeated `200 52B` responses for `/`
- `embedded/web/index.html` content: minimal placeholder
- `embedded/frontend/index.html` content was placeholder until restored from `web/out/index.html`

## 4.2 Issue 2: Jellyfin Plugin Webhook Auth Path
Symptoms:
- Plugin appears installed but events appear non-functional.
- Local webhook probes to both daemon and web API returned 401.

Validated causes:
1. Installer writes `[jellyfin]` URL/API key but does not collect/write `webhook_secret`.
2. Server handlers originally required non-empty secret always; empty secret => hard reject.

Evidence:
- `~/.config/plex2jellyfin/config.toml` had no `jellyfin.webhook_secret`.
- Probe:
  - `POST http://localhost:8686/api/v1/webhooks/jellyfin` => `401 unauthorized`
  - `POST http://localhost:5522/api/v1/webhooks/jellyfin` => `401 unauthorized`

Files involved:
- `internal/daemon/server.go` (`validateWebhookSecret`)
- `internal/api/webhooks.go` (`validateWebhookSecret`)
- Installer config generation in `cmd/installer/tasks.go`

## 4.3 Issue 3: Source-of-Truth Drift + The Pitt Anomaly
This is the most important track.

### 4.3.1 Source-of-truth drift (critical)
User intent is Plex2Jellyfin-first import/move/naming. Live system state contradicts that.

Validated via Sonarr/Radarr APIs:
- Sonarr download client config: `enableCompletedDownloadHandling = true`
- Radarr download client config: `enableCompletedDownloadHandling = true`
- Sonarr naming config: `renameEpisodes = false`
- Radarr naming config: `renameMovies = false`

Implication:
- Sonarr/Radarr are still auto-importing from SAB watch dirs into library.
- Plex2Jellyfin scanner sees watch dirs mostly empty (hence periodic `processed=0`).
- Renaming disabled means raw release names can land directly in library.

### 4.3.2 The Pitt S02E08 specific anomaly
Validated behavior:
- Sonarr history shows event `downloadFolderImported` for The Pitt S02E08 from SAB path to:
  - `/mnt/STORAGE5/TVSHOWS/The Pitt (2025)/Season 02/The.Pitt.S02E08.2.00.P.M.1080p.AMZN.WEB-DL.DD+5.1.H.264-playWEB.mkv`
- Jellyfin series search finds `The Pitt`.
- Jellyfin season endpoint for The Pitt season 2 returns episodes 2-7 only.
- Jellyfin raw search for `The.Pitt.S02E08` returns an `Episode` item with:
  - `SeriesId: null`
  - `SeasonId: null`

Interpretation:
- Not classic DB corruption evidence.
- More consistent with **metadata parsing/linkage failure for that file**, producing an orphan episode object not attached to series/season tree.

## 5. Local Runtime Snapshot (Important)

Services:
- `plex2jellyfin-daemon`: active, periodic scans every 5m, usually `processed=0`
- `plex2jellyfin-web`: active, on `5522`

Current watch/library config (`~/.config/plex2jellyfin/config.toml`):
- Watch:
  - `/mnt/NVME3/Sabnzbd/complete/movies`
  - `/mnt/NVME3/Sabnzbd/complete/tv`
- Libraries:
  - `/mnt/STORAGE{1..8,10}/MOVIES`
  - `/mnt/STORAGE{1..8,10}/TVSHOWS`

Jellyfin library virtual folders:
- Uses container-internal mounts (`/tv1.. /tv10`, `/movies1.. /movies10`), mapping assumed to host `/mnt/STORAGE*`.

## 6. Code Paths To Audit Deeply

1. File ingestion lifecycle:
- `internal/watcher/watcher.go`
- `internal/scanner/periodic.go`
- `internal/daemon/handler.go`
- `internal/daemon/path_ingest.go`

2. Webhook/event ingestion:
- `internal/daemon/server.go`
- `internal/api/webhooks.go`
- plugin code:
  - `plugin/EventHandlers/EventForwarder.cs`
  - `plugin/Configuration/PluginConfiguration.cs`
  - `plugin/Configuration/configPage.html`

3. Static frontend serving:
- `embed.go`
- `internal/api/static.go`
- `internal/api/server.go`
- `embedded/frontend/*`
- `embedded/web/*`

4. Installer integration paths:
- `cmd/installer/tasks.go`
- `cmd/installer/screens.go`
- `cmd/installer/inputs.go`
- `cmd/installer/types.go`
- `cmd/installer/update.go`

## 7. Reproduction Commands (Use As Baseline)

## 7.1 Sonarr/Radarr control plane
- Sonarr download client config:
  - `GET /api/v3/config/downloadClient`
- Sonarr naming config:
  - `GET /api/v3/config/naming`
- Sonarr recent history:
  - `GET /api/v3/history?includeSeries=true&pageSize=50&page=1&sortDirection=descending`
- Radarr download client config:
  - `GET /api/v3/config/downloadClient`
- Radarr naming config:
  - `GET /api/v3/config/naming`

## 7.2 Jellyfin verification
- Library paths:
  - `GET /Library/VirtualFolders`
- Series search:
  - `GET /Items?Recursive=true&SearchTerm=The%20Pitt&IncludeItemTypes=Series,Episode&Limit=50`
- The Pitt season episodes:
  - `GET /Shows/<ThePittSeriesId>/Episodes?Season=2&Limit=200`
- Raw filename search:
  - `GET /Items?Recursive=true&SearchTerm=The.Pitt.S02E08&IncludeItemTypes=Episode,Video&Limit=50`

## 7.3 Plex2Jellyfin runtime
- `systemctl status plex2jellyfin-daemon --no-pager -n 200`
- `journalctl -u plex2jellyfin-daemon --no-pager -n 240`
- `systemctl status plex2jellyfin-web --no-pager -n 200`

## 8. Hypotheses To Confirm/Reject

1. Web UI issue was purely embed-path/content drift (likely true).
2. Plugin failures were primarily empty-secret policy + installer omission (likely true).
3. Processed=0 scans are mostly due watch dirs emptied by Sonarr/Radarr importer (likely true).
4. The Pitt missing in series view is due orphan episode linkage (`SeriesId/SeasonId null`) from raw naming parse edge case, not DB corruption (currently most likely).

## 9. Required Workstreams For Next Agent

## WS-A: Hardening source-of-truth enforcement
- Introduce explicit installer/runtime guardrails:
  - If Plex2Jellyfin source-of-truth mode is enabled, enforce/verify:
    - Sonarr `enableCompletedDownloadHandling=false`
    - Radarr `enableCompletedDownloadHandling=false`
    - Sonarr `renameEpisodes=true` (or document required alternative)
    - Radarr `renameMovies=true` (or document required alternative)
- Add a health/audit command that reports these flags live.

## WS-B: Jellyfin orphan episode remediation
- Build detection logic for episodes with null series/season linkage.
- Validate remediation options:
  - Force metadata refresh for affected paths.
  - Rename to canonical pattern, then refresh.
  - Identify from provider IDs if available.
- Produce a safe remediation script/procedure.

## WS-C: Webhook auth + installer UX
- Add webhook secret field in installer flow.
- Generate/store secret when omitted (or explicit local-only mode with clear warnings).
- Verify plugin config UX/documentation alignment.
- Ensure daemon/API/web endpoints have consistent policy and tests.

## WS-D: Web static pipeline consistency
- Single source of truth for embedded frontend path.
- Ensure build step updates whichever embed directory is authoritative.
- Add CI/test guard that fails when embedded `index.html` is placeholder/minimal.

## 10. Test Expectations
At minimum:

1. Unit tests:
- webhook secret logic (empty secret + loopback/non-loopback policy)
- embed filesystem content sanity checks
- installer config generation includes webhook secret behavior

2. Integration tests:
- Simulated Sonarr import disabled path with Plex2Jellyfin watcher processing
- Simulated Sonarr import enabled path showing expected skip behavior and warning
- Jellyfin API verification for series episode linkage after rename/refresh

3. Manual validation matrix:
- Fresh install
- Update install
- Web service enabled/disabled
- Plugin installed with/without secret

## 11. Open Questions
1. Should local-loopback-without-secret be allowed long-term, or require secret always?
2. Should Plex2Jellyfin ship a first-run "arr policy reconciler" that mutates Sonarr/Radarr settings automatically?
3. How should raw release naming be handled when arr rename is intentionally off?

## 12. Final Note
The local evidence strongly indicates **configuration drift + naming policy mismatch**, not definitive Jellyfin DB corruption. However, orphaned episodes are operationally similar to corruption from a user perspective and should be treated as high priority to detect/remediate automatically.

## 13. Fresh Runtime Addendum (2026-03-01 evening)
Additional verification after initial handover draft:

1. `plex2jellyfin-daemon` and `plex2jellyfin-web` are both active for multiple days with no crash-loop.
2. `plex2jellyfin-daemon` continues periodic scans every 5 minutes with `processed=0` repeatedly.
3. `plex2jellyfin-web` still serves `GET /` as `200 52B` in current runtime.
4. Sonarr and Radarr still have `enableCompletedDownloadHandling=true`.
5. Sonarr and Radarr still have `renameEpisodes=false` / `renameMovies=false`.
6. On disk, The Pitt season folder contains:
   - Canonical files `The Pitt S02E02..S02E07.mkv`
   - A raw import for episode 8:
     - `/mnt/STORAGE5/TVSHOWS/The Pitt (2025)/Season 02/The.Pitt.S02E08.2.00.P.M.1080p.AMZN.WEB-DL.DD+5.1.H.264-playWEB.mkv`
     - Owned by `sonarr:media`, consistent with Sonarr importer ownership.
7. Jellyfin search for the raw S02E08 token returns an `Episode` item with null linkage fields (`SeriesId=null`, `SeasonId=null`), reinforcing orphan metadata linkage behavior.
