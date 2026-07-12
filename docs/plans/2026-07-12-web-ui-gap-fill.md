# Web UI Gap-Fill Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close web UI gaps in three PRs—auth/nav, operator controls, and Jellyfin-native recently-added posters on the dashboard.

**Architecture:** Frontend-only for PR1 (existing logout API). PR2 wires existing Arr/plugin/scan APIs into settings. PR3 adds thin Jellyfin Latest + image-proxy endpoints and a dashboard strip; Jellystat remains watch-stats only.

**Tech Stack:** Go (`internal/jellyfin`, `internal/api`), Next.js web UI (React Query, AppShell/Sidebar), Vitest, `go test`.

**Design:** `docs/plans/2026-07-12-web-ui-gap-fill-design.md`

---

## PR 1 — Auth & navigation

### Task 1: Logout control in Sidebar

**Files:**
- Modify: `web/src/components/layout/Sidebar.tsx`
- Test: `web/src/components/layout/Sidebar.test.tsx` (create if missing)

**Step 1: Write failing test**

Assert a Logout button calls `useLogout` / triggers logout mutation (mock `useLogout`).

**Step 2: Run test — expect fail**

Run: `cd web && npx vitest run src/components/layout/Sidebar.test.tsx`

**Step 3: Implement**

- Import `useLogout` from `@/hooks/useAuth`
- Footer below nav: button “Log out”
- onClick: `await logout.mutateAsync()`, then `window.location.replace('/login')` (or `/` — AuthGuard will show LoginForm)
- Prefer `window.location.replace('/')` after invalidate so AuthGuard shows login without a dedicated /login dependency
- Disable while pending; show error toast/text on failure if trivial

**Step 4: Tests pass; commit**

```bash
git add web/src/components/layout/Sidebar.tsx web/src/components/layout/Sidebar.test.tsx
git commit -m "feat(web): add logout control to sidebar"
```

### Task 2: Logout on Security settings + mobile

**Files:**
- Modify: `web/src/app/settings/security/page.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx` (`MobileNav`)
- Test: extend security page test or Sidebar test

**Step 1:** Failing test — Security page has Log out control.

**Step 2:** Implement Security card section + mobile logout (compact control in mobile bar or overflow). Keep mobile simple: small “Log out” text button at end of scroll row.

**Step 3:** Commit

```bash
git commit -m "feat(web): logout on security page and mobile nav"
```

### Task 3: Jellyfin nav entry

**Files:**
- Modify: `web/src/components/layout/Sidebar.tsx` (`navigation` array)
- Test: Sidebar test asserts Jellyfin link `href="/jellyfin"`

**Step 1:** Add `{ name: 'Jellyfin', href: '/jellyfin', icon: ListChecks }` (or similar lucide icon already used on settings) after Activity or before Trace.

**Step 2:** Commit

```bash
git commit -m "feat(web): expose jellyfin identification page in nav"
```

### Task 4: Settings overview cards

**Files:**
- Modify: `web/src/app/settings/page.tsx`
- Test: optional render test listing new hrefs

**Step 1:** Extend `sections` to include jellystat, security, daemon, database, indexing, permissions (icons already imported in settings layout).

**Step 2:** Commit

```bash
git commit -m "feat(web): complete settings hub cards"
```

### Task 5: Rebuild embed + verify PR1

**Step 1:** `make frontend && go build ./cmd/plex2jellyfin-web`

**Step 2:** Manual: login → logout → login; open Jellyfin from nav; settings hub shows new cards.

**Step 3:** Open PR titled `feat(web): logout, jellyfin nav, settings hub`

---

## PR 2 — Operator controls

### Task 6: Arr compatibility on Sonarr/Radarr settings

**Files:**
- Modify: `web/src/app/settings/sonarr/page.tsx`, `web/src/app/settings/radarr/page.tsx`
- Create or reuse: small `CompatibilityPanel` component (check from SetupWizard patterns)
- API already: `POST /settings/sonarr/compatibility`, `…/fix`, radarr twins

**Steps:** Add “Check compatibility” / “Fix” buttons; display issues list; wire `api.post`. Unit-test panel with MSW. Commit. Repeat for Radarr or share one component with `service` prop.

### Task 7: Plugin status on Jellyfin settings

**Files:**
- Modify: `web/src/app/settings/jellyfin/page.tsx` (may need custom page beyond `SettingsSectionPage`)
- Backend: reuse CLI plugin status if an HTTP route exists; otherwise add thin `GET /jellyfin/plugin/status` wrapping `plugininstall.Engine.Inspect` (only if no existing route—search first)

**Steps:** Status line + Verify button; soft-fail errors. Commit.

### Task 8: Scan + health on Daemon settings

**Files:**
- Modify: `web/src/app/settings/daemon/page.tsx` (and/or header already has ScanButton—surface health there)
- API: existing scan + health endpoints

**Steps:** Show last health / Arr issue count; link or button to re-run checks. Commit. Open PR2.

---

## PR 3 — Recently added (Jellyfin)

### Task 9: Client `GetLatestItems`

**Files:**
- Modify: `internal/jellyfin/items.go` (or new `latest.go`)
- Test: `internal/jellyfin/items_test.go` / `latest_test.go`

**Step 1: Failing test** — mock `/Users` + `/Users/admin/Items/Latest`, assert query Limit/Fields, filter Virtual.

**Step 2: Implement**

```go
func (c *Client) GetLatestItems(ctx context.Context, limit int) ([]Item, error) {
  // DefaultUserID; GET /Users/{id}/Items/Latest?Limit=N&Fields=DateCreated,MediaSources,Genres,...
  // drop LocationType == "Virtual"
}
```

Add `LocationType` to `Item` if missing.

**Step 3: Commit** `feat(jellyfin): fetch Items/Latest for recently added`

### Task 10: API recently-added + image proxy

**Files:**
- Modify: `internal/api/server.go` (routes)
- Create: `internal/api/jellyfin_recent_handlers.go` (+ tests)
- OpenAPI optional (manual mount OK per repo convention)

**Endpoints:**
- `GET /api/v1/jellyfin/recently-added?limit=24`
- `GET /api/v1/jellyfin/items/{id}/image/primary` → stream JPEG/PNG from Jellyfin `/Items/{id}/Images/Primary?fillHeight=320&fillWidth=213&quality=50`

Auth: session-required (not public). Return `{enabled:false}` style when Jellyfin disabled.

**Commit:** `feat(api): jellyfin recently-added and primary image proxy`

### Task 11: Dashboard UI strip

**Files:**
- Modify: `web/src/app/page.tsx`
- Create: `web/src/hooks/useRecentlyAdded.ts`, `web/src/components/dashboard/RecentlyAdded.tsx`
- Test: render with MSW fixture; hide when enabled false

**Step 1:** Horizontal scroll of posters; img src `/api/v1/jellyfin/items/{image_item_id}/image/primary` (credentials include cookies via same-origin).

**Step 2:** `make frontend`; manual verify against live Jellyfin.

**Step 3:** Open PR3.

---

## Execution notes

- Branch per PR from updated `main`: `feat/web-auth-nav`, `feat/web-operator-controls`, `feat/web-recently-added`
- Avoid AI attribution trailers in commits (commit-msg hook)
- Do not mix unrelated brand asset churn into these PRs unless already on the branch
- After each PR: `make frontend` before tagging web binary for local verify
