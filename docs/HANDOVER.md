# Plex2Jellyfin Project Handover

**Last Updated**: 2026-02-21  
**Session Context**: Visual polish completion + installer bug discovery

---

## Project Overview

Plex2Jellyfin is a media file watcher and organizer for Jellyfin libraries. It consists of:
- **Go backend** (`internal/`): 28 packages covering naming, database, transfer, sync, AI, API, etc.
- **Next.js web UI** (`web/`): TypeScript + Tailwind + shadcn/ui
- **CLI tool** (`cmd/plex2jellyfin/`): Cobra-based with 20+ subcommands
- **Daemon service** (`cmd/plex2jellyfin-daemon/`): Background file watcher with systemd integration
- **Web server** (`cmd/plex2jellyfin-web/`): Serves the Next.js UI on port 5522
- **Installer** (`cmd/installer/`): Bubble Tea TUI for interactive setup

**Architecture**:
- `plex2jellyfin-daemon` - Background daemon (file watching, organizing, health checks at :8686)
- `plex2jellyfin-web` - Web server (serves Next.js UI + REST API at :5522)
- Both run as systemd services via installer

---

## What Was Worked On

### Web UI Visual Polish (COMPLETED)

Goal: Systematically improve visual design/UX of all 11 routes.

**Completed Work**:

1. **Foundation** (`web/src/app/globals.css`)
   - Added violet accent CSS variables (`--color-accent`, `--color-accent-foreground`)
   - Added glass utility classes (`.glass`, `.glass-hover`, `.glass-border`)
   - Zinc-950 dark theme preserved

2. **Enhanced Components** (`web/src/components/ui/`)
   - `status-dot.tsx` (NEW) - Presence/activity indicator with pulse animation
   - `badge.tsx` - Added `success`, `info`, `warning` variants
   - `card.tsx` - Added hover states, gradient variants (`gradient-border`)

3. **Improved Layout** (`web/src/components/layout/`)
   - `AppShell.tsx` - Background radial gradients, floating header, smooth animations
   - `Sidebar.tsx` - Active state indicators, refined branding, hover effects

4. **Loading States** (`web/src/app/*/loading.tsx`)
   - Added skeleton screens for all pages

5. **All Pages Polished**:
   - `page.tsx` - Dashboard with hero, stat cards, quick actions
   - `onboarding/page.tsx` - 4-step wizard with progress indicators
   - `duplicates/page.tsx` - Quality comparison cards
   - `consolidation/page.tsx` - Path visualization
   - `queue/page.tsx` - Tabbed interface with progress bars
   - `activity/page.tsx` - Timeline visualization
   - `settings/page.tsx` - Card-based config display
   - `login/page.tsx` - Frosted glass card with branding
   - Plus 3 additional routes

6. **Illustrations** (`web/public/illustrations/`)
   - Created 9 SVG fallback illustrations for empty states:
     - `empty-dashboard.svg`
     - `empty-duplicates.svg`
     - `empty-consolidation.svg`
     - `empty-queue.svg`
     - `empty-activity.svg`
     - `onboarding-scan.svg`
     - `onboarding-duplicates.svg`
     - `onboarding-consolidate.svg`
     - `onboarding-complete.svg`

---

## Build Status

| Command | Status |
|---------|--------|
| `npm run build` | ✅ PASSES - All 11 routes build successfully |
| `go build ./...` | ✅ PASSES - No errors |

**Notes**:
- `npm run build` produces static output (11 routes, first load ~87.4kB)
- Non-blocking warning: Failed to patch lockfile (TypeError in patch-incorrect-lockfile.js)
- Non-blocking warning: rewrites not applied in export mode (expected for static export)

---

## Critical Bug Discovered (UNRESOLVED)

### Issue: Web Server Silently Skipped During Installation

**Problem**: User ran `./installer` but never saw the web server start. The installer did not fail — it silently skipped the `plex2jellyfin-web` service.

**Root Cause**: The installer shares the same `serviceEnabled`/`serviceStartNow` flags for both `plex2jellyfin-daemon` (daemon) AND `plex2jellyfin-web` (web server). If the user said "No" to either prompt, the web service was skipped with no error or URL shown.

**Affected Files**:

1. **`cmd/installer/screens.go`** - `renderService()` function
   ```go
   // Only 3 fields shown - NO separate web service prompt
   fields := []struct {
       label string
       value string
   }{
       {"Enable on boot", boolToYesNo(m.serviceEnabled)},
       {"Start now", boolToYesNo(m.serviceStartNow)},
       {"Scan frequency", scanFrequencyOptions[m.scanFrequency]},
   }
   ```

2. **`cmd/installer/tasks.go`** - Service setup functions
   ```go
   func setupWebSystemd(m *model) error {
       if !m.serviceEnabled {
           return nil  // ← Silent skip
       }
       // ...
   }

   func startWebService(m *model) error {
       if !m.serviceEnabled || !m.serviceStartNow {
           return nil  ← Silent skip
       }
       // ...
   }
   ```

**Expected Behavior**: User should be able to:
- Choose separately whether to enable/start the daemon (`plex2jellyfin-daemon`)
- Choose separately whether to enable/start the web UI (`plex2jellyfin-web`)
- At minimum, always see the web UI URL (`:5522`) at the end of installation

**Current Behavior**: Both services share flags, and no URL is shown if user opts out.

---

## Key Files Reference

### Web UI (Polished)
```
web/src/app/
├── globals.css                      # Theme + glass utilities
├── page.tsx                         # Dashboard (polished)
├── onboarding/page.tsx              # Onboarding (polished)
├── duplicates/page.tsx              # Duplicates (polished)
├── consolidation/page.tsx           # Consolidation (polished)
├── queue/page.tsx                   # Queue (polished)
├── activity/page.tsx                # Activity (polished)
├── settings/page.tsx                # Settings (polished)
├── login/page.tsx                   # Login (polished)
└── *[3 additional routes]*/page.tsx

web/src/components/ui/
├── status-dot.tsx                   # NEW component
├── badge.tsx                        # Enhanced variants
└── card.tsx                         # Enhanced variants

web/src/components/layout/
├── AppShell.tsx                     # Enhanced layout
└── Sidebar.tsx                      # Enhanced navigation

web/public/illustrations/            # 9 SVG files
```

### Installer (Bug Location)
```
cmd/installer/
├── screens.go                       # UI prompts - NEEDS SEPARATE WEB SERVICE PROMPT
├── tasks.go                         # Service setup - NEEDS SEPARATE FLAGS
├── model.go                         # Data model - NEEDS NEW FIELDS
└── installer.go                     # Main flow
```

### Web Server (Works)
```
cmd/plex2jellyfin-web/main.go                 # Web server entrypoint (works correctly)
internal/api/server.go               # REST API + static file serving
```

### Daemon (Works)
```
cmd/plex2jellyfin-daemon/main.go              # Daemon entrypoint (works correctly)
internal/daemon/                     # Background service logic
```

---

## What Was NOT Done

1. **Installer Bug Fix** - The web service silent skip bug was discovered but not fixed
2. **PNG Illustrations** - SVG fallbacks created, but PNGs were not generated (requires Kimi CLI or external tool)
3. **CI/CD** - No GitHub Actions workflows exist (noted in AGENTS.md as missing)

---

## Next Steps

### Priority 1: Fix Installer Bug

1. Add `webServiceEnabled` and `webServiceStartNow` fields to `model.go`
2. Add separate "Enable web UI service" and "Start web UI now" prompts in `screens.go` `renderService()`
3. Update `tasks.go` to use new fields in `setupWebSystemd()` and `startWebService()`
4. Ensure `renderComplete()` always shows the web UI URL (`:5522`) regardless of service choices
5. Test the full installer flow end-to-end

### Priority 2: Verification

1. Run `./installer` with both services enabled
2. Verify web UI accessible at http://localhost:5522
3. Test that daemon runs independently of web service

### Priority 3: Optional

1. Generate PNG illustrations using prompts in `docs/kimi-image-generation-prompt.md`
2. Add GitHub Actions CI/CD workflow (see AGENTS.md)

---

## Commands Reference

```bash
# Build web UI
cd web && npm run build

# Build Go binaries
cd /home/nomadx/Documents/plex2jellyfin && go build ./...

# Run web dev server
cd web && npm run dev

# Run full build
make all

# Run installer
sudo ./installer

# Start web server manually
plex2jellyfin-web --port 5522 --host 0.0.0.0

# Start daemon manually
sudo plex2jellyfin-daemon

# Check systemd services
systemctl status plex2jellyfin-web
systemctl status plex2jellyfin-daemon
```

---

## Session Context Summary

- **Goal achieved**: Web UI fully polished and builds passing
- **Bug found**: Installer silently skips web service under certain conditions
- **User impact**: Installation appears to succeed but web UI never starts
- **Fix required**: Separate web service flags + always show URL at completion
