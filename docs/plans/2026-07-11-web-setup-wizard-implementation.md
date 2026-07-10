# Web Setup Wizard Implementation Plan

> Execute tasks in order with test-first changes. Keep the setup domain reusable by the later CLI wizard, but do not build CLI UI in this change.

**Goal:** Gate fresh installations behind a container-aware Web setup wizard that atomically configures Plex2Jellyfin, verifies optional integrations, activates the daemon, and unlocks the dashboard only after readiness succeeds.

**Architecture:** Add setup metadata to the existing config schema and a small `internal/setup` package for pure readiness, validation, runtime, and config-assembly logic. Add manually mounted setup handlers that share the server's configuration lock and existing IPC/launcher. Build a dedicated static-export-safe `/setup` page and route it from `AuthGuard` after password creation.

**Stack:** Go standard library, chi, existing config/daemon/service packages, Next.js static export, React Query, existing shadcn/Radix controls, Vitest/Testing Library.

---

## Task 1: Setup configuration semantics

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/setup/setup.go`
- Create: `internal/setup/setup_test.go`

1. Add failing config round-trip tests for `[setup]` metadata and Jellyfin path mappings.
2. Add failing pure tests for blank readiness, legacy configured bypass, interrupted versioned setup, either TV or Movies alone, incomplete media pairs, container permission rejection, invalid duration/modes/URLs, secret preservation, and setup draft application.
3. Add `SetupConfig`, serialize setup metadata and Jellyfin path mappings, and implement the minimum pure setup types/functions.
4. Run `go test ./internal/config ./internal/setup`.
5. Commit the task.

## Task 2: Shared configuration ownership

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/settings_handlers.go`
- Modify: `internal/api/paths_handlers.go`
- Modify: `internal/api/settings_handlers_test.go`
- Modify: `internal/api/paths_handlers_test.go`

1. Add a regression test showing a settings write followed by a path write preserves both changes.
2. Add one server-level configuration mutex and snapshot replacement helper; pass the shared lock/update callback into existing handlers.
3. Make path/library reads and mutations load the latest disk config rather than serializing retained stale pointers.
4. Run `go test ./internal/api -run 'Settings|Paths|Libraries|Config'`.
5. Commit the task.

## Task 3: Setup status and atomic apply API

**Files:**
- Create: `internal/api/setup_handlers.go`
- Create: `internal/api/setup_handlers_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/daemon_handlers.go`
- Modify: `internal/api/preflight_handlers.go`
- Modify: `internal/api/preflight_handlers_test.go`
- Modify: `api/openapi.yaml`
- Regenerate: `web/src/types/api.ts`

1. Add failing handler tests for masked status drafts, legacy completion, fresh required state, unknown-field rejection, path-preflight rejection, atomic incomplete marker, reload of a running daemon, launch of a stopped daemon, readiness timeout, and successful completion marker.
2. Extract reusable path preflight logic and verify incoming and library directory access without leaving probe files.
3. Introduce a small launcher interface so activation is testable with the existing launcher implementation.
4. Implement `GET /setup/status` and `POST /setup/apply` under normal auth middleware.
5. Apply the full draft to the latest disk config, preserve auth/secrets, generate Jellyfin secrets with `crypto/rand`, save incomplete, activate/reload, poll IPC, then save complete.
6. Document the endpoints and schemas in OpenAPI and run `cd web && npm run types`.
7. Run `go test ./internal/api ./internal/setup ./internal/config`.
8. Commit the task.

## Task 4: Sonarr/Radarr compatibility actions

**Files:**
- Modify: `internal/api/test_handlers.go`
- Modify: `internal/api/test_handlers_test.go`
- Modify: `internal/api/server.go`
- Modify: `api/openapi.yaml`
- Regenerate: `web/src/types/api.ts`

1. Add failing tests for draft-credential compatibility checks and explicit fixes for both services.
2. Add check/fix handlers that reuse `service.Check*Config` and `service.Fix*Issues`; never modify external services from a connection test.
3. Mount and document the four routes.
4. Run `go test ./internal/api -run 'Sonarr|Radarr|Compatibility'` and regenerate types.
5. Commit the task.

## Task 5: First-run routing

**Files:**
- Create: `web/src/hooks/useSetup.ts`
- Modify: `web/src/components/auth/AuthGuard.tsx`
- Create: `web/src/components/auth/AuthGuard.test.tsx`
- Replace: `web/src/app/onboarding/page.tsx`

1. Add failing frontend tests for password-first behavior, unauthenticated login, incomplete-setup redirect/content, configured dashboard pass-through, `/setup` loop avoidance, and status loading/error states.
2. Add typed setup status/apply hooks.
3. Gate authenticated children on setup status and route incomplete installs to `/setup` without flashing dashboard content.
4. Replace legacy onboarding with a static-export-safe redirect to `/setup`.
5. Run `cd web && npm test -- --run src/components/auth/AuthGuard.test.tsx`.
6. Commit the task.

## Task 6: Branded wizard UI and validation

**Files:**
- Create: `web/src/app/setup/page.tsx`
- Create: `web/src/components/setup/SetupWizard.tsx`
- Create: `web/src/components/setup/MediaStep.tsx`
- Create: `web/src/components/setup/ServicesStep.tsx`
- Create: `web/src/components/setup/AIStep.tsx`
- Create: `web/src/components/setup/RuntimeStep.tsx`
- Create: `web/src/components/setup/ReviewStep.tsx`
- Create: `web/src/components/setup/setupDraft.ts`
- Create: `web/src/components/setup/setupDraft.test.ts`
- Reuse: `web/src/components/settings/ModelSelect.tsx`
- Reuse: `web/src/components/settings/PathListEditor.tsx`

1. Add failing pure tests for default draft hydration, either-media validation, incomplete pairs, path-check requirements, enabled-service test requirements, AI model requirements, and container/native runtime fields.
2. Implement the smallest draft reducer/validation helpers.
3. Build the desktop step rail and mobile progress header using the current dark console tokens and P2J brand asset.
4. Implement media path entry/preflight, optional service tests and Arr repair confirmations, Jellyfin path mappings, Ollama discovery/model selection/prompt test, runtime settings, review/apply, activation error recovery, optional initial scan, and dashboard entry.
5. Ensure all controls have labels, focus states, stable dimensions, and no nested cards/gradients.
6. Run focused setup tests and `cd web && npm run typecheck`.
7. Commit the task.

## Task 7: Integration and generated frontend

**Files:**
- Modify as required by integration findings only.
- Regenerate: `embedded/frontend/**`

1. Run all frontend tests: `cd web && npm test -- --run`.
2. Run `cd web && npm run typecheck`.
3. Run `cd web && npm run build` and verify `web/out/setup/index.html` exists.
4. Run `make frontend` and `diff -qr web/out embedded/frontend`.
5. Run `make check-frontend`.
6. Run `go test ./...`.
7. Run `git diff --check` and inspect the complete diff for unrelated changes or exposed secrets.
8. Commit regenerated assets and integration fixes.

## Task 8: Runtime smoke verification and delivery

**Files:**
- Modify only if smoke verification finds a defect.

1. Build `plex2jellyfin-web` and `plex2jellyfin-daemon`.
2. Start the Web server with an isolated `HOME`, verify `/setup/` returns 200, create auth, confirm setup is required, apply temporary TV-only and movie-only configurations in separate runs, and confirm invalid half-pairs are rejected.
3. Verify an existing legacy config bypasses setup and a versioned incomplete config does not.
4. Verify static assets and API routes return no 404s.
5. Stop every test server/process and remove temporary runtime data.
6. Run the full verification set once more after any fixes.
7. Push `main`, confirm `origin/main` contains all commits, and report exact verification results.
