# Handover: Rebrand & Public-Release Review — 2026-07-10

**Audience:** a fresh review agent. Your job is to audit everything done in the 2026-07-10 session that renamed JellyWatch to Plex2Jellyfin and published it. Nothing in this document has been independently reviewed unless stated; treat all of it as claims to verify.

## The Two Repos

| Repo | Path | State |
|---|---|---|
| Old (JellyWatch) | `/home/nomadx/Documents/jellywatch` | Retains the LIVE deployment (`jellywatchd` running from `/usr/local/bin`, config in `~/.config/jellywatch`). 10 commits were added this session BEFORE the rename. Do not modify this repo. |
| New (Plex2Jellyfin) | `/home/nomadx/Documents/plex2jellyfin` | `git clone` of the old repo (full 419-commit history), plus ~25 rebrand/release commits. Pushed to `https://github.com/Nomadcxx/plex2jellyfin` (public). |

## What the Session Did, In Order

### Phase 1 — old repo: committed two weeks of in-flight work (10 commits, `0116d9e7..90435c68`)

Grouped commits closing out the June correctness audit plus new features. These were carried into the new repo via the clone. Review targets:

- `7e7ba04d` fix(config): chmod 0600 on load (A6-H2)
- `f253e46d` fix(naming): multi-episode range rejection, release-tag coverage
- `452af361` feat(identity): series identity guard + season-pack rollback (A1-H2) — new `internal/identity` package
- `9f375977` fix(consolidate): identity safety + stale conflict resolution
- `3f5dbe70` feat(jellyfin): unknown-season classification/repair + daemon.enabled honored
- `b4b59485` fix(daemon): circuit breaker wiring (A4-H1), timer shutdown (A3-8), webhook errors (A5-H1), scheduler panic recovery, privilege escalation, transfer Move errors
- `37fba3ad` feat(cli): plan path-root validation before privilege escalation (A6-H1)
- `ee41edb5` feat(scripts): SABnzbd deobfuscation post-processor
- `90435c68` feat(postmortem): unknown-season evidence + parser-drift classification

These were written across earlier sessions and committed as-is with tests passing. **They have never been code-reviewed.** This is the highest-value review surface for correctness bugs.

### Phase 2 — new repo: mechanical rename (`cf951ebb`)

374 files. Method: `git mv` of branded dirs/files, then ordered global sed over tracked text files (`jellywatchd`→`plex2jellyfin-daemon` before `jellywatch`→`plex2jellyfin`; case variants; `jellyweb`→`plex2jellyfin-web`), `internal/jellyweb/daemonctl` hoisted to `internal/daemonctl`. Gates passed: gofmt, `go build ./...`, `go vet ./...`, full test suite.

Sed-rename hazards to check:
- Awkward phrasings from mechanical case replacement (e.g. "Plex2Jellyfin daemon" mid-sentence, doubled words) in user-facing strings: Cobra `Short:`/`Long:` help, log messages, installer TUI copy, web UI strings.
- Renamed identifiers that now mislead (`plex2jellyfinRoasts` in `cmd/installer/roasts.go` contains Sonarr/Radarr jokes — content may reference the old name or no longer land).
- Env vars renamed to `PLEX2JELLYFIN_*` — verify every reader/writer pair still agrees (`git grep PLEX2JELLYFIN_`).
- Anything embedding the old config path `~/.config/jellywatch` that should now offer migration (there is deliberately NO config-migration code; the author migrates manually per plan Task 9).

### Phase 3 — release engineering (commits `81198cf3..6a472eb0` on the new repo)

Executed via subagent-driven development against `docs/superpowers/plans/2026-07-10-plex2jellyfin-public-release.md`. Progress ledger + per-task implementer reports: `.superpowers/sdd/` (untracked, local only).

| Commit | What | Review status at the time |
|---|---|---|
| `81198cf3` | LICENSE (GPL-3.0) | reviewed, clean |
| `22d515cd` | Installer TUI wordmark (theme.go) | reviewed, clean; Minor deferred: stale "4 lines" comment `cmd/installer/update.go:15` |
| `9d183037` | Brand PNGs regenerated from ASCII | controller-verified (visual + hash) only |
| `da0c756b` | Frontend rebuilt, `embedded/` regenerated via `make frontend` | controller-verified (gates + commit scope) only |
| `66e9f3a4` | gofmt sweep (27 files, pre-existing drift) | controller-verified whitespace/reflow-only |
| `752ece42` | fix(paths): `os.UserConfigDir()` honors XDG_CONFIG_HOME | reviewed; **verify no behavior change for existing bare-metal installs with `$XDG_CONFIG_HOME` set** — this changes where config resolves for those users |
| `04116c1e` | Docker: Dockerfile, entrypoint, compose, README section | reviewed; Critical SIGTERM bug found and fixed in follow-up |
| `4429378` | fix(docker): SIGTERM forwarding in entrypoint | written inline by controller, signal-tested (`docker stop` exits without SIGKILL), **not reviewed** |
| `68d6f8d8` | GoReleaser+nfpm deb/rpm; systemd + installer paths moved to `/usr/bin` | **not reviewed** (user waived reviews from here). deb+rpm install-tested in Debian 12/Fedora 40 containers |
| `e056ac4a` | plan doc: Task 11 added | n/a |
| `d20e1ab9` | MkDocs site (`docs-src/`, `mkdocs.yml`, `.github/workflows/docs.yml`) | **not reviewed**; `mkdocs build --strict` green |
| `46b7976c` | Prose pass on docs-src (2 files) | **not reviewed** |
| `6a472eb0` | README rewrite in house style | **not reviewed** |

## Known Open Issues (deferred deliberately)

1. `cmd/installer/update.go:15` — comment says "4 lines for ASCII art" but the art is 6 lines.
2. `cmd/plex2jellyfin/audit_cmd.go:811` — prints a hardcoded `$HOME/.config/plex2jellyfin/plans/audit.json` path instead of using `paths.PlansDir()`; wrong message (not wrong behavior) when `$XDG_CONFIG_HOME` is set.
3. **`ghcr.io/nomadcxx/plex2jellyfin:latest` does not exist yet** — README and compose reference it, but no image has been pushed to GHCR. Docker users following the README today get a pull error. Launch blocker for the Docker path; needs `docker build` + push with a `write:packages` token, or a GH Actions image workflow.
4. **Docs site not live** — README Quick Links point to `https://nomadcxx.github.io/plex2jellyfin/`; the workflow file exists (`.github/workflows/docs.yml`) but the user must enable Actions/Pages. 404 until then.
5. `plex2jellyfin version` prints `dev` — version stamping lands with the first tagged `goreleaser release` (`v0.1.0-beta.1` planned).
6. arm64 deb/rpm built but never install-tested (no emulation available).
7. `.gitignore` blanket `*.yml`/`*.yaml` rules keep silently swallowing new YAML files; three `!` exceptions were patched in ad hoc (`docker-compose.example.yml`, `mkdocs.yml`, `.github/workflows/*.yml`). Consider restructuring the rule.
8. Cobra help strings have had no prose pass (only mechanical rename).
9. SELinux/rootless-Podman guidance on the docs Docker page is unverified general knowledge.
10. Plan Tasks 9 (migrate author's live deployment) and 10 (final art swap) are outstanding, user-gated.

## Suggested Review Order

1. **Phase 1 commits** (never reviewed, touch data-integrity paths: transfers, rollback, deferred queues, privilege escalation). Diff: in either repo, `git diff 4b017f89..90435c68`.
2. **Docker + packaging** (`04116c1e`, `4429378`, `68d6f8d8`): entrypoint signal handling under `docker stop` with active transfers; nfpm contents vs systemd `ExecStart` paths; postinstall/preremove behavior on upgrade (not just install); `.goreleaser.yaml` `make frontend` hook interaction with a dirty tree.
3. **The XDG change** (`752ece42`) against every config-path consumer: `git grep -n "UserConfigDir\|ConfigDir()" internal/ cmd/`.
4. **Rename residue** in user-facing strings (see Phase 2 hazards).
5. **Docs accuracy**: every command and config key in README + docs-src exists and behaves as described (`go run ./cmd/plex2jellyfin --help`, `config.toml.example`).

## Verification Commands

```bash
cd /home/nomadx/Documents/plex2jellyfin
go build ./... && go vet ./... && go test ./...          # full gate (all green as of 6a472eb0)
git grep -Iin jellywatch                                  # only plan-doc historical refs expected
docker build -t plex2jellyfin:dev . && docker run --rm plex2jellyfin:dev plex2jellyfin version
~/go/bin/goreleaser release --snapshot --clean            # deb/rpm into dist/
~/.local/bin/mkdocs build --strict
```

## Constraints for Your Session

- Commit messages must NOT contain AI/agent attribution — a commit-msg hook rejects them.
- Never modify `/home/nomadx/Documents/jellywatch` (live deployment).
- The repo is public. Anything you commit and push is visible immediately.
- Findings format: the author wants a prioritized findings list (Critical/Important/Minor) with file:line references; fixes only on request unless a finding is a trivial one-liner.
