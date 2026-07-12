# Web UI gap-fill design (3 PRs)

**Date:** 2026-07-12  
**Status:** Approved

## Problem

The web UI is missing everyday affordances that the backend already supports (logout), hides real pages (Jellyfin identification), and leaves operator tools only in the setup wizard. Separately, “recently added” with posters should be a default dashboard feature via Jellyfin—not gated on Jellystat.

## Goals

1. Users can sign out and reach every primary surface from nav.
2. Operators can re-check Arr compatibility, plugin health, and scan/health without the installer.
3. Dashboard shows recently added library items with poster art whenever Jellyfin is configured.

## Non-goals

- MediaManager integration
- Redesigning the whole mobile icon set beyond logout + Jellyfin
- Jellystat-backed recently-added (Jellystat stays watch-stats only)
- Per-item Jellystat play stats (deferred)

## PR slice

### PR 1 — Auth & navigation

- Logout control: sidebar footer, Security settings, mobile account affordance
- Uses existing `POST /auth/logout` + `useLogout()`; invalidate auth query; hard-navigate to login/home
- Add **Jellyfin** to sidebar + mobile nav (`/jellyfin`)
- Settings overview cards: include Jellystat, Security, Daemon, Database, Indexing, Permissions (match settings side nav)

### PR 2 — Operator controls

- Sonarr/Radarr settings: compatibility check + fix (reuse setup-wizard / API `…/compatibility` + `…/fix`)
- Jellyfin settings: companion plugin status / verify (CLI-equivalent surface; soft-fail)
- Daemon (or small Ops strip): trigger library scan + show health / Arr issues summary

### PR 3 — Recently added (Jellyfin-native)

- Default dashboard section when `[jellyfin]` enabled + reachable
- Soft-empty / hide when Jellyfin off or unreachable (do not break other cards)
- Backend mirrors Jellystat’s mechanism against Jellyfin:
  - `GET /Users/{adminOrDefault}/Items/Latest?Limit=N` with `Fields` including `DateCreated`
  - Filter out `LocationType=Virtual`
- Image proxy: `GET /api/v1/jellyfin/items/{id}/image/primary` → Jellyfin Primary image with server API key (browser never sees the key)
- Episode cards use series id for poster when appropriate (same as Jellystat Home)

## Data flow (PR 3)

```
Dashboard → GET /jellyfin/recently-added
         → GET /jellyfin/items/{id}/image/primary (img src)
Web API  → jellyfin.Client.DefaultUserID + Items/Latest
         → jellyfin.Client image GET /Items/{id}/Images/Primary
Jellyfin
```

Jellystat overview cards remain independent and optional.

## Testing

- PR1: component tests for logout invalidate + nav links; AuthGuard still gates
- PR2: API handler tests for compatibility wiring if new wrappers; UI smoke for buttons
- PR3: jellyfin client unit tests for Latest + image proxy; dashboard section renders / hides correctly

## Rollout

Three stacked or parallel PRs against `main`. Prefer ship PR1 first (unblocks daily use), then PR2, then PR3.
