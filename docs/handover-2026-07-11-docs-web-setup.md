# Handover: Documentation, Web UI Reskin, and Setup Wizard - 2026-07-11

**Audience:** the engineer continuing Plex2Jellyfin after the 2026-07-10/11 session.

## Repository State

| Item | State |
|---|---|
| Repository | `/home/nomadx/Documents/plex2jellyfin` |
| Branch | `main` |
| Remote | `https://github.com/Nomadcxx/plex2jellyfin.git` |
| Session head | `8125d62bfd57da5eecea18f503138a7ead9b4942` |
| Remote state | `origin/main` matched session head after push |
| Worktree at wizard completion | Clean |
| Documentation | Fumadocs static site deployed through GitHub Pages |
| Documentation URL | `https://nomadcxx.github.io/plex2jellyfin/` |

The session started from `docs/handover-2026-07-10-rebrand-review.md` and actioned its immediate release, documentation, and Web UI follow-ups.

## Work Completed

### 1. Replaced MkDocs with a branded Fumadocs site

The MkDocs site did not offer enough control over the Plex2Jellyfin brand. The replacement uses the existing Next.js/Fumadocs stack and exports static files for GitHub Pages.

Key files:

- `docs-site/`
- `.github/workflows/docs.yml`
- `docs/plans/2026-07-10-fumadocs-site-design.md`
- `docs/plans/2026-07-10-fumadocs-site-implementation.md`

The site now has:

- a dark-only operator-console theme;
- Plex2Jellyfin and P2J ASCII wordmarks rendered as transparent PNGs;
- migrated reference, installation, Docker, package, plugin, troubleshooting, and workflow content;
- a static GitHub Pages export under the `/plex2jellyfin/` base path;
- responsive navigation and a compact documentation landing page.

Relevant commits:

```text
a9bc2d9b docs: design branded fumadocs site
9e56d31a docs: plan fumadocs migration
4a0b8320 build(docs): scaffold static fumadocs site
bfeb1935 brand(docs): add transparent ASCII wordmarks
0eb5a8ef docs: migrate content to fumadocs
051d1dc7 feat(docs): add operator-console theme
adc90840 feat(docs): add branded documentation home
05d1e386 ci(docs): deploy fumadocs to pages
e1e407ad fix(docs): polish responsive documentation layout
43753d82 build(docs): resolve postcss advisory
44ea7b8a ci(docs): update pages actions
```

### 2. Fixed the documentation landing-page sidebar

The static sidebar looked double-width on `/docs/` because the content wrapper added desktop space that the expanded sidebar already occupied. A scoped layout rule now pins the expanded sidebar correctly without changing the hover drawer or article layouts.

The documentation home also stopped presenting the installer pipe followed by a raw `scan` command as one installation method. It now separates installer, package, Docker, and source workflows.

Relevant commits:

```text
5914254f fix(docs-site): drop full-viewport hero on the landing page
80a8ce37 fix(docs): pin sidebar and clarify install paths
```

### 3. Actioned rebrand-review follow-ups

This session also landed release and product corrections that came out of the previous handover:

- removed the hardcoded `SUDO_USER` from shipped systemd units;
- moved installation guidance higher in the README;
- added first-run password creation and password change support;
- clarified the Jellyfin companion plugin requirement;
- added per-file trace journeys to the CLI, API, and Web UI;
- added optional Jellystat watch statistics;
- moved the Jellyfin plugin to `net9.0` for Jellyfin 10.11 packages;
- expanded package, plugin, SABnzbd, and Arr health documentation.

Relevant commits:

```text
2f258042 fix: apply rebrand review corrections
f39701e7 fix(packaging): remove hardcoded SUDO_USER from shipped systemd units
4b10bce1 docs(readme): move installation up and add package configuration walkthrough
51c58e55 feat(web): auth lifecycle - forced first-run setup and password change
fabf90a6 docs(readme): plugin is required for the feedback loop; concrete download clients in diagram
2bc305fe feat(trace): per-file pipeline journeys via web UI, API, and CLI
119c3cf1 feat(jellystat): optional watch statistics integration
456b5f10 fix(plugin): target net9.0 so it builds against Jellyfin 10.11 packages
c891b620 docs(site): packages walkthrough, plugin guide, sabnzbd + health --fix guides
```

### 4. Audited the current Web UI and installer flow

The audit identified the main bootstrap problem: fresh installs start the Web service with default configuration, while the daemon exits because it is disabled and has no watch paths. Existing settings endpoints then try to reload the absent daemon and roll back their writes. Docker users therefore had no usable first-run Web path.

The audit also found that settings, watch-path, and library-path handlers retained separate configuration pointers. A later write could serialize stale state and undo a previous change.

Decisions made with the project owner:

- Build the Web wizard first, then a CLI wizard.
- Focus on Docker and packaged Linux installations.
- Defer native Windows and macOS support.
- Require setup before exposing the dashboard on a fresh installation.
- Show both TV and Movies; permit either one or both.
- Require both incoming and library paths for each configured media type.
- Keep the initial scan optional.
- Let packages and containers install binaries/services. The Web process configures Plex2Jellyfin and activates an existing daemon.
- Keep the current first-run unauthenticated API posture for now. The owner accepted it as comparable to other self-hosted media services.

### 5. Merged the terminal-console Web UI reskin

The staged Web UI reskin became the base for the setup wizard.

The reskin adds:

- dark operator-console styling;
- terminal-style navigation and prompts;
- restrained amber focus states;
- updated dashboard, activity, trace, settings, queue, duplicates, scheduler, and lifecycle surfaces;
- regenerated embedded frontend assets.

Commits:

```text
4012cbab test(cli): include trace in visible commands
9290414c feat(web): apply terminal console reskin
```

### 6. Designed and implemented the Web setup wizard

Design and implementation records:

- `docs/plans/2026-07-11-web-setup-wizard-design.md`
- `docs/plans/2026-07-11-web-setup-wizard-implementation.md`

The frontend route is `/setup`. `AuthGuard` now enforces this order:

1. create a Web UI password;
2. complete application setup;
3. enter the dashboard;
4. optionally start the initial scan.

Existing configurations with at least one structurally complete TV or movie pair bypass the wizard.

#### Wizard steps

1. **Media paths**
   - TV and Movies appear by default.
   - Either section may remain empty.
   - Each configured section requires incoming and library paths.
   - Every path uses the preflight API before the user can continue.

2. **Connected services**
   - Sonarr, Radarr, and Jellyfin are optional.
   - Tests use unsaved draft URLs and API keys.
   - Sonarr/Radarr compatibility checks report completed-download and rename settings.
   - Fixes require a separate confirmation action.
   - Jellyfin supports mount-prefix mappings.

3. **AI matching**
   - Ollama remains optional.
   - The wizard tests the endpoint, discovers installed models, selects primary/fallback models, and can run a prompt check.

4. **Runtime behavior**
   - Scan frequency, move/copy behavior, and checksum verification are configurable.
   - Native installs may configure user, group, file mode, and directory mode.
   - Container installs show effective UID/GID and omit unsupported ownership controls.

5. **Review and activation**
   - The browser submits one typed draft.
   - The server validates the draft and paths again.
   - The server writes an incomplete setup marker and the full config atomically.
   - It reloads a running daemon or launches a stopped daemon.
   - It polls IPC readiness, then marks setup complete.
   - Activation failure leaves setup incomplete and returns the user to Review.

6. **Completion**
   - The user may open the dashboard or start an initial scan first.

#### Setup state

`internal/config.Config` now contains:

```toml
[setup]
version = 1
completed = true
```

State handling:

- version `0` plus a complete media pair is a legacy configured install;
- version `1`, `completed = false` records an interrupted or failed activation;
- version `1`, `completed = true` records completed setup.

#### Setup API

```text
GET  /api/v1/setup/status
POST /api/v1/setup/apply
```

The status response includes runtime type, effective UID/GID, daemon state, masked secrets, and a normalized draft.

The apply handler preserves authentication and masked secrets. It generates Jellyfin webhook/plugin secrets when required.

#### Arr compatibility API

```text
POST /api/v1/settings/sonarr/compatibility
POST /api/v1/settings/sonarr/compatibility/fix
POST /api/v1/settings/radarr/compatibility
POST /api/v1/settings/radarr/compatibility/fix
```

Connection tests do not modify Sonarr or Radarr. Only the confirmed `/fix` calls issue updates.

### 7. Fixed supporting configuration and daemon defects

The setup work exposed and fixed several existing defects:

1. **Jellyfin path mappings disappeared on save**
   - `Config.ToTOML()` did not serialize `[[jellyfin.path_mappings]]`.
   - Config round-trip coverage now protects the mappings.

2. **Settings handlers could overwrite each other**
   - Settings, path, library, setup, and AI mutations now share one server configuration lock and pointer.
   - Regression tests cover settings-to-path and AI-to-path write sequences.

3. **Configured daemon health address was ignored**
   - `cmd/plex2jellyfin-daemon/main.go` constructed the health server before applying `cfg.Daemon.HealthAddr`.
   - The daemon now resolves the configured address before construction while retaining explicit CLI flag precedence.

4. **Static export caused a setup redirect loop**
   - GitHub/static routes use `/setup/`, while `AuthGuard` compared only `/setup`.
   - The guard now normalizes trailing slashes. A regression test covers `/setup/`.

5. **Permissions used the wrong JSON field names**
   - `PermissionsConfig` emitted `User` and `FileMode` instead of `user` and `file_mode`.
   - JSON tags now match OpenAPI and the frontend draft.

6. **Empty collections serialized as `null`**
   - Setup status now returns `[]` for paths and Jellyfin mappings, matching the API contract.

### 8. Regenerated and embedded the frontend

`make frontend` regenerated `embedded/frontend/`, including:

- `/setup/index.html`;
- the setup JavaScript bundle;
- the transparent `p2j-mark.png`;
- updated auth guard and onboarding redirect bundles.

`web/out` and `embedded/frontend` matched exactly after generation.

## Setup-Wizard Commit Series

```text
8667fa94 docs: record web setup wizard design
7abb44f1 docs: plan web setup wizard implementation
e36b84de feat(setup): define first-run config state
94294e0c fix(api): serialize config mutations
d569cd5c feat(api): add atomic setup bootstrap
37a7569e feat(setup): add arr compatibility actions
123314ab feat(web): gate dashboard on setup state
fa203933 feat(web): add first-run setup wizard
11aa5fbd fix(setup): verify exported wizard lifecycle
8125d62b fix(api): retain shared config after AI apply
```

## Verification Completed

### Automated checks

```text
Frontend tests:       14 files, 31 tests passed
TypeScript:           npm run typecheck passed
Next production build: npm run build passed
Static setup route:   web/out/setup/index.html exists
Embedded equality:    diff -qr web/out embedded/frontend returned no differences
Embedded gate:        make check-frontend passed
Go suite:             go test ./... passed
Diff gate:            git diff --check passed
```

### Live binary smoke tests

The smoke tests built and ran the real `plex2jellyfin-web` and `plex2jellyfin-daemon` binaries against isolated temporary homes.

Verified behavior:

- `/setup/` returned HTTP 200;
- `p2j-mark.png` returned HTTP 200;
- fresh status returned `required: true` and `daemon_state: stopped`;
- TV-only configuration saved, launched the daemon, and completed setup;
- Movies-only configuration passed in a separate home with no retained TV state;
- a half-configured TV draft returned HTTP 422;
- a versionless legacy Movies configuration returned `required: false`;
- failed activation retained `version = 1`, `completed = false`;
- daemon stop returned HTTP 202;
- all temporary Web, daemon, and Chromium processes stopped;
- temporary smoke data under `/tmp/p2j-setup-smoke` was removed.

### Browser layout checks

Chromium rendered the live embedded site with an authenticated setup session.

- Desktop viewport: 1440px wide, no horizontal overflow.
- Mobile viewport: 390px wide, no horizontal overflow.
- Desktop uses a compact 240px step rail.
- Mobile uses a progress header and single-column fields.

## Known Remaining Work

### 1. Build the CLI setup wizard

The owner requested the Web wizard first and a CLI wizard next. `internal/setup` contains the reusable draft, validation, readiness, runtime, and config-assembly logic. The CLI should call this package instead of copying Web handler logic.

CLI requirements already agreed:

- run through the installed `plex2jellyfin` binary;
- work for AUR, Debian, Fedora, and other package installs;
- support TV-only, Movies-only, or both;
- preflight paths;
- test Sonarr, Radarr, Jellyfin, and Ollama;
- discover and select Ollama models;
- offer explicit Arr compatibility fixes;
- save atomically and activate/reload the daemon;
- avoid source builds and package installation.

### 2. Update user documentation for the Web wizard

The current Docker documentation still tells users to exec into the container, run `plex2jellyfin config init`, edit TOML, and restart. Replace that flow with password creation and `/setup` once the release containing this work is published.

Update at least:

- `docs-site/content/docs/getting-started/docker.md`
- package installation walkthroughs;
- README installation section;
- configuration reference for `[setup]` metadata;
- screenshots or GIFs after the final UI copy settles.

### 3. Native Windows and macOS remain deferred

The current activation path targets Linux services and the packaged/container binaries. The owner agreed to address native Windows and macOS later.

### 4. First-run unauthenticated API posture remains unchanged

When no password exists, middleware permits API access. The owner accepted this for the current self-hosted first-run model. Revisit one-time claims or loopback-only bootstrap if the threat model changes.

Reference: `internal/api/server.go:325`.

### 5. Web server startup prints the wrong auth status

`cmd/plex2jellyfin-web/main.go:105` checks `cfg.Password`, but normal installs store `PasswordHash`. The service prints `No password set` after restart even though authentication works. Change the message to check `PasswordHash` or the server's auth state.

### 6. Legacy bypass checks structure, not live path access

Versionless configs bypass setup when they contain a non-empty incoming/library pair. `GET /setup/status` does not preflight those legacy paths. This avoids forcing existing users through setup after an upgrade, but stale legacy paths still reach the dashboard.

### 7. Unsaved wizard progress lives only in browser memory

The wizard writes nothing until Review. Refreshing the page before Apply discards the draft. The atomic transaction stays simple, but long setup sessions cannot resume mid-form.

### 8. Application CI is still local-only

The repository currently exposes the documentation deployment workflow. The session verified the application locally before pushing, but no general application test workflow ran for `8125d62b`.

## Suggested Next Session

1. Fix the misleading Web auth startup message.
2. Design the CLI wizard around `internal/setup`.
3. Write the CLI plan and implement it with TDD.
4. Update Docker/package documentation to use the Web wizard.
5. Add an application CI workflow for Go tests, frontend tests/typecheck/build, and `make check-frontend`.
6. Run a real Docker first-start test with mounted `/config`, `/watch`, and `/library` volumes.

## Commands for a Fresh Verification

```bash
cd /home/nomadx/Documents/plex2jellyfin

cd web
npm test -- --run
npm run typecheck
npm run build
cd ..

make frontend
diff -qr web/out embedded/frontend
make check-frontend
go test ./...
git diff --check
git status --short --branch
```

## Constraints and Preferences Carried Forward

- Keep the documentation and Web UI dark-only.
- Preserve the operator-console visual language.
- Treat TV and Movies as independent optional media types.
- Keep the dashboard unavailable until fresh-install setup completes.
- Keep initial scanning optional.
- Prefer the smallest implementation that reuses standard library and installed dependencies.
- Do not run token-intensive external review tools without asking.
- Do not modify the old live JellyWatch repository.
