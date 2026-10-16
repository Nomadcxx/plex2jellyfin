# Seeding-Safe Torrent Ingestion Handover

**Date:** 2026-08-27
**Branch:** `feat/torrent-ingestion`
**Worktree:** `/home/nomadx/Documents/plex2jellyfin/.worktrees/torrent-ingestion`
**Next task:** Task 8, standalone TUI installer

## Read first

Use these files as the requirements and execution record:

- `docs/superpowers/specs/2026-08-27-seeding-safe-torrent-ingestion-design.md`
- `docs/superpowers/plans/2026-08-27-seeding-safe-torrent-ingestion.md`
- this handover
- the repository `AGENTS.md` instructions supplied by the user

Run all work from the worktree above. Do not work from the repository's main checkout. The worktree was clean when this handover was written.

Follow the written plan task by task. Use test-driven development for Tasks 8 and 9. Commit each task under the message listed in the plan. Do not run CodeRabbit or another token-heavy external reviewer unless the user asks for it.

## Product boundaries

Plex2Jellyfin owns imports, canonical naming, placement, audit decisions, and its media database. Sonarr and Radarr may schedule downloads and receive scoped rescans. They must not import or rename completed downloads.

The torrent integration uses completed-download directories. Do not add downloader APIs, credentials, category management, ratio controls, or client-specific runtime branches.

Preserve these rules:

- ordinary watch roots move when `delete_source = true`;
- ordinary watch roots copy when `delete_source = false`;
- seeding roots hardlink when source and target share a filesystem, otherwise they copy;
- seeding sources remain in place;
- incomplete and unclassified downloads stay outside watched roots;
- retained staging paths do not become canonical database inventory;
- notifications and Jellyfin refreshes remain scoped;
- no library-wide Jellyfin refresh;
- no production changes before Tasks 10A and 11 pass;
- version bumps and AUR publication remain separate from this plan.

## Completed work

Tasks 1 through 7 are committed:

| Task | Commit | Result |
|---|---|---|
| 1 | `4464773d feat(config): add seeding watch roots` | Added `seeding_tv` and `seeding_movies`, helpers, defaults, TOML output, and compatibility tests. |
| 2 | `e4ac7b01 fix(config): reject conflicting watch roots` | Added boundary-safe overlap validation and daemon startup validation. |
| 3 | `6291c9dc feat(transfer): add hardlink-first copy` | Added atomic hardlink publication with copy fallback for supported link errors. |
| 4 | `a7ec680c refactor(organizer): resolve transfer mode` | Added per-source move, copy, and seed selection across organizer paths. |
| 5 | `a252eebb feat(daemon): preserve seeding sources` | Wired all roots, source hints, cleanup boundaries, and `delete_source` into the daemon. |
| 6 | `38e83d13 fix(daemon): reload watch policy atomically` | Reloads watcher, periodic scanner, handler hints, and source policy as one transaction. |
| 7 | `26f29a9a feat(setup): configure seeding paths` | Updated shared setup, CLI, API preflight, path settings, and OpenAPI. |

Task 7 added `setup.WatchDraft` with four arrays. `PathsDraft` still owns the two library arrays. Shared validation accepts seeding-only intake paired with its library and reports `config.ValidateWatchLayout` failures as a `watch` field error. API preflight requires read access for seeding roots and does not require delete access. CLI setup prompts for both seeding completion roots and labels them as retained in its review. Path settings accept `seeding_tv` and `seeding_movies` without changing ordinary roots.

Task 7 followed a RED/GREEN cycle. The final gate passed:

```text
go test ./internal/setup ./internal/api ./cmd/plex2jellyfin -count=1
ok github.com/Nomadcxx/plex2jellyfin/internal/setup
ok github.com/Nomadcxx/plex2jellyfin/internal/api
ok github.com/Nomadcxx/plex2jellyfin/cmd/plex2jellyfin
```

`api/openapi.yaml` now defines `SetupWatchPaths` and the two new path-setting kinds. `web/src/types/api.ts` has not been regenerated; Task 9 owns that generated change.

## Task 8: standalone TUI installer

No Task 8 files have been edited. Code discovery found these ownership points:

- `cmd/installer/types.go`: `WatchFolder` has `Label`, `Type`, and `Paths`. Add `Seeding bool`.
- `cmd/installer/main.go`: `newModel` creates two fixed ordinary folders. Create four fixed folders: ordinary TV, ordinary Movies, TV seeding completions, and Movie seeding completions.
- `cmd/installer/inputs.go`: path inputs follow `watchFolders`, with library fields starting at `len(watchFolders)`. Dynamic custom folders must set `Seeding: false`. `removeWatchFolder` currently protects two fixed entries; protect all four.
- `cmd/installer/validation.go`: `mediaPathsError` and `detectPathOverlap` flatten all folders as ordinary roots. Split on `WatchFolder.Seeding`, include seeding roots in media-pair checks, and call `config.ValidateWatchLayout` so conflicting media or policy roots block progress.
- `cmd/installer/tasks.go`: `generateConfigString` emits only `movies` and `tv`. Emit `seeding_movies` and `seeding_tv`, then load the generated TOML through the real config loader in tests.
- `cmd/installer/screens.go`: `renderPaths` can reuse the dynamic input loop. Add retention guidance for seeding fields. `renderConfirm` must distinguish ordinary folders from paths retained for seeding.
- `cmd/installer/update.go`: the existing path flow lets the user press Enter twice to bypass `pathOverlapWarning`. Make `mediaPathsError` return shared layout-validation errors so policy conflicts cannot reach that bypass.

Add RED tests before implementation. The minimum useful set is:

1. `newModel` exposes four fixed folder entries with correct type and seeding flags.
2. adding and removing a custom folder cannot remove the four fixed entries; the custom entry remains ordinary.
3. generated TOML routes all four arrays and survives `config.Load`.
4. seeding-only TV intake plus a TV library passes `mediaPathsError`.
5. ordinary/seeding overlap and watch/library overlap fail through shared layout validation.
6. confirmation output marks seeding paths as retained.

Run the RED gate from the worktree:

```bash
go test ./cmd/installer -run 'Seeding|GenerateConfig|Path' -count=1
```

Implement the fixed fields without adding client selection or credentials. Format only changed Go files if package-wide `gofmt` produces unrelated changes. Run:

```bash
go test ./cmd/installer -count=1
git diff --check
```

Commit:

```bash
git add cmd/installer
git commit -m "feat(installer): add seeding completion paths"
```

## Task 9: web setup, settings, types, and embedded frontend

Start with `web/src/components/setup/setupDraft.test.ts`.

Current ownership:

- `setupDraft.ts` hydrates only `watch.tv` and `watch.movies`. Default both seeding arrays to `[]` when an older setup response omits them.
- `mediaErrors` treats only ordinary arrays as intake. Count ordinary plus seeding intake for each media type.
- `pathCheckKey` uses `collection:media:path`. Add enough collection identity to keep ordinary TV, seeding TV, and library TV checks separate.
- `SetupWizard.tsx` has one incoming editor and one library editor per media type. Add a seeding completion editor with copy that says incomplete data must use another directory and completed sources remain available to the torrent client.
- ordinary incoming paths use `delete_source` to determine whether write/delete capability is required; seeding paths require read access; libraries require write access.
- `ReviewStep` reports only incoming and library counts. Report ordinary, seeding, and library counts.
- `web/src/app/settings/paths/page.tsx` creates TV and Movies hooks and panels. Add `seeding_tv` and `seeding_movies` panels through the existing settings hooks.
- `web/src/types/api.ts` comes from `npm run types`. Do not edit it by hand.
- `embedded/frontend` must match the final `web/out` build.

Write RED tests for hydration, seeding-only validation, unique check keys, and visibility of API overlap errors. Then run:

```bash
cd web
npm install
npm run types
npm test -- --run
```

After implementation:

```bash
cd web
npm test -- --run
npm run typecheck
npm run build
cd ..
rm -rf embedded/frontend
cp -r web/out embedded/frontend
make check-frontend
```

Commit:

```bash
git add web/src embedded/frontend
git commit -m "feat(web): expose seeding watch paths"
```

## Task 10: documentation

Apply the `stop-slop` skill before editing public documentation. Update:

- `README.md`
- `docs-site/content/docs/reference/configuration.md`
- `docs-site/content/docs/getting-started/docker.md`
- a new `docs-site/content/docs/getting-started/torrent-clients.md`
- the docs navigation
- `docker-compose.example.yml`

Lead with the client-neutral filesystem contract. Cover qBittorrent categories, Transmission final directories, and Deluge Label `move_completed` paths. Document incomplete roots, unclassified safety, shared group and umask, hardlink versus cross-volume copy, unsupported final-name preallocation, and the ban on direct library writes.

Run the documentation gates from the plan and commit as:

```bash
git commit -m "docs: add torrent client setup contract"
```

## Task 10A: Q/A and specification audit

The user added this gate after approving the implementation plan. Run it after Task 10 and before final verification or deployment.

The audit file does not exist. Create:

`docs/superpowers/plans/2026-08-27-seeding-safe-torrent-ingestion-qa-audit.md`

Use temporary directories and local test servers. Do not touch production paths. Exercise legacy config compatibility, all setup surfaces, move/copy/seed transfers, cross-filesystem fallback, single-file and multi-file arrivals, season packs, temporary extensions, source removal, reload changes, and rollback after a forced reload failure.

Build the requirement table specified in the execution plan:

```text
Requirement | Evidence (file:test/command) | Status | Finding
```

Resolve code findings with a RED regression test. The gate requires zero open critical, high, or medium findings and no unexplained design mismatch. The audit must conclude `ready for final verification` before Task 11 starts.

## Task 11: full verification

Use `superpowers:verification-before-completion`. Run the exact gates in the implementation plan:

- format changed Go files and regenerate API types;
- `go test ./...`;
- race tests for daemon, reload, scanner, transfer, and organizer;
- frontend tests, typecheck, build, and embedded-frontend check;
- Go builds for all four binaries;
- temporary-directory filesystem acceptance;
- scope review against `origin/main`.

Do not absorb unrelated pre-existing formatting into the task diff. SQLite compiler warnings are acceptable when commands exit successfully.

## Task 12: deployment and Stargirl repair

Begin only after Tasks 10A and 11 pass. Capture rollback state before changing the installed package, config, services, container mounts, qBittorrent paths, or Stargirl data.

Production state named by the approved plan:

- torrent staging base: `/mnt/STORAGE5/.downloads/torrents`
- config: `~/.config/plex2jellyfin/config.toml`
- installed Plex2Jellyfin services
- qBittorrent mounts, categories, torrent hashes, and save paths
- Stargirl torrent, filesystem, database, and Jellyfin rows

Deploy through the same path used for `0.1.11`. Do not bump the version or publish AUR as part of this plan. Verify service health before changing qBittorrent.

Create incomplete, TV completion, movie completion, and unclassified staging directories with the existing media group. Give qBittorrent only the staging mount after verifying the replacement. Add the TV and Movies completion directories as Plex2Jellyfin seeding roots.

Prove the flow with an expendable completed fixture before relocating Stargirl. Verify retained source, link or copy choice, canonical target, database decision, scoped Jellyfin update, and continued seeding.

For Stargirl seasons 1 and 3, use qBittorrent `Set location` so the client retains torrent state. Let Plex2Jellyfin import them into the canonical series. Confirm all 39 episodes share one provider identity and canonical layout before removing obsolete direct-library directories. Preserve season 2. Do not trigger a library-wide refresh.

Report service health, journals, disk use, parse decisions, seed state, Jellyfin paths, rollback artifacts, and cross-volume duplicate-space exposure.

## Resume command

```bash
cd /home/nomadx/Documents/plex2jellyfin/.worktrees/torrent-ingestion
git status --short --branch
git log --oneline -8
go test ./cmd/installer -run 'Seeding|GenerateConfig|Path' -count=1
```

The last command should pass before RED tests are added. After adding the Task 8 tests, it must fail for the missing seeding behavior before production code changes.
