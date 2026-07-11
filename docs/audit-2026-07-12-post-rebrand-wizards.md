# Audit: Post-rebrand wizards & TUI installer

**Date:** 2026-07-12  
**Scope:** jellywatch → plex2jellyfin rebrand through TUI plugin install + setup-state adoption  
**Trigger:** First real plex2jellyfin install showed success while systemd units were missing, libraries empty, plugin verify returned 412, and TUI styling looked muted/holed  
**Branch for remediation:** `fix/post-rebrand-wizard-remediation`  
**Worktree:** `.worktrees/fix-post-rebrand-wizard-remediation`

---

## Executive summary

The mechanical rebrand did not swap watch/library fields or break systemd unit paths. The first-install failure is a **TUI installer control-flow bug** (systemd gated on non-empty libraries) combined with **missing path validation** and a **new** early `[setup] completed = true` stamp. CLI and web wizards share `internal/setup` and are largely correct; the TUI hand-rolls TOML and diverges. TUI “muted / background holes” are mostly pre-existing lipgloss nesting (FG-only styles reset BG), made more visible by denser plugin UI and a gray monochrome palette — not a lost pure-black theme in git.

---

## 1. Incident reconstruction (this machine)

| Observation | Evidence |
|---|---|
| Binaries installed | `/usr/bin/plex2jellyfin{,-daemon,-web,-installer}` present |
| No systemd units | `systemctl` → unit not found; 0 unit files |
| Installer log | `/tmp/plex2jellyfin-installer-*.log` only build + `install` |
| Config at 05:20 | `[libraries] movies/tv = []`; `[watch]` filled with STORAGE library paths |
| jellywatch config | Still correct: SAB under `[watch]`, STORAGE under `[libraries]` |
| Plugin callback | `plugin_daemon_url = "http://192.168.0.10:5522"` with nothing listening |
| Scan | `Scanning 0 TV/movie libraries` — reads empty `[libraries]` |
| Plugin verify | `POST /plex2jellyfin/test-webhook: 412` — callback unreachable / not ready |

Causal chain:

```
empty library fields in TUI
  → handleTaskComplete skips postScanTasks (systemd + start + plugin configure)
  → complete screen still advertises systemctl start plex2jellyfin-web
  → [setup] completed=true already written → web wizard will not recover
  → plugin points at :5522 → verify 412
  → scan sees 0 libraries
```

---

## 2. What changed vs jellywatch installer

### Rebrand-only (low risk)

| Change | Commits |
|---|---|
| Mechanical rename | `9e5573e1` / `cf951ebb` |
| Wordmark 4×85 → 6×80 | `9f11c865` |
| `/usr/local/bin` → `/usr/bin` | `9814738d` (binaries on this host confirm path works) |
| Packaging / docs / handover | `2f258042`, docs commits |

Paths step, `savePathsInputs`, watch→`[watch]` / libraries→`[libraries]` format args, and the library-gated `postScanTasks` check are **the same shape as jellywatch**.

### Real new installer logic (where bugs live)

| Change | Commits |
|---|---|
| `[setup] completed = true` in generated TOML | `87934339` |
| Plugin consent + callback URL | `5fcd7201`, `185bc7f9` |
| Plugin install/restart/configure tasks | `ed411853`, `2e3921e4` |
| Complete-screen plugin status | `81ab43bb` |

---

## 3. TUI installer defects

### P0 — Systemd coupled to “have libraries for scan”

```go
// cmd/installer/validation.go — handleTaskComplete
if !m.uninstallMode && !m.updateMode &&
    (m.tvLibraryPaths != "" || m.movieLibraryPaths != "") &&
    len(m.postScanTasks) > 0 {
    // arr validate → scan → THEN postScanTasks (systemd)
}
// else: stepComplete — postScanTasks never run
```

`postScanTasks` includes `setupSystemd`, `startService`, optional web units, and plugin configure. Empty libraries → **no units written**, but complete UI still says start them.

**Fix:** Always run systemd/listener tasks after config write. Gate *only* the initial library scan on non-empty libraries.

### P0 — Early `setup.completed = true`

`generateConfigString` writes `completed = true` in `writeConfig`, before systemd/start. CLI/web do the opposite: save with `completed=false` → activate → then stamp complete.

**Fix:** Mirror CLI/web two-phase completion. On start failure, leave incomplete so web re-enters the wizard.

### P0 — No media-pair / required-path validation

- Defaults: empty watch folders and empty libraries (`cmd/installer/main.go`)
- Paths step: overlap warning only; Enter always advances
- Does not call `setup.ValidateDraft` / `HasMediaPair` / `api.ValidateSetupPaths`

CLI rejects half-configured media (`TestSetupWizardRejectsHalfConfiguredMedia`). Web rejects via client + server `ValidateDraft`. TUI does not.

**Fix:** Share `ValidateDraft` (or thin wrapper) before leaving paths / before `writeConfig`.

### P1 — Complete screen lies

- Claims “daemon is running and watching…” unconditionally
- Always shows `sudo systemctl start plex2jellyfin-web`
- Plugin verify advice even when listeners never started

**Fix:** Advertise only units that were actually installed/started; reflect real `pluginState.outcome`.

### Soft — Watch vs library placement

No evidence of a swapped write after rebrand. This install put STORAGE paths under `[watch]` and left `[libraries]` empty — consistent with filling the first section (“TV Shows” / “Movies” watch folders) and skipping library fields. Stronger validation + clearer labels would prevent this.

---

## 4. CLI & web wizard audit

### Shared domain (`internal/setup`) — generally sound

- `Draft` / `ValidateDraft` / `ApplyDraft` / `NeedsSetup` / `HasMediaPair`
- `AdoptLegacyCompletion` for legacy configs without marker
- `DetectAdvertiseIP` for plugin callback defaults
- Two-phase completion in CLI (`setup_cmd.go`) and web (`setup_handlers.go`)

### CLI wizard (`plex2jellyfin setup`)

| Concern | Status |
|---|---|
| Media pair validation | Yes |
| Path preflight | Yes (`api.ValidateSetupPaths`) |
| Completion after activate | Yes |
| Plugin install / configure / verify | Yes (soft-fail on verify) |
| Tests | Happy path, half-pairs, failed activation, plugin soft-fail |

### Web wizard (`/setup` + API)

| Concern | Status |
|---|---|
| Media → services → AI → runtime → review | Yes |
| `ValidateDraft` on apply | Yes |
| Completion after activate | Yes |
| Legacy adoption at web startup | Yes (`AdoptLegacyCompletion`) |
| **Companion plugin install/callback/verify** | **Missing** |
| `Draft` plugin fields | **None** — web users never configure feedback loop |

### Parity matrix

| Concern | Domain | CLI | Web | TUI |
|---|---|---|---|---|
| Media pair validation | yes | yes | yes | **no** |
| Path preflight | via API | yes | yes | **no** |
| `completed` after activate | — | yes | yes | **no** (at write) |
| Config assembly | `ApplyDraft` | yes | yes | hand TOML |
| Plugin install | — | yes | **no** | yes |
| Plugin callback | `DetectAdvertiseIP` | prompt | **no** | field + default |
| Systemd units | — | no (direct/IPC) | no | yes (when libraries set) |
| Auth password | — | no | forced first | tip only |

### Other wizard risks

| Risk | Notes |
|---|---|
| Plugin 412 opacity | `Engine.do` drops body on ≥400; verify UX is status-only |
| Activation drift | TUI = systemd; CLI/web = IPC / direct exec — dual-launch risk on packaged hosts |
| Adoption cannot save TUI empty+completed | `version != 0` + `completed=true` with empty media skips wizard forever |

---

## 5. TUI styling audit

### What git did *not* change

- Theme colors identical to jellywatch: `BgBase=#1a1a1a`, white/`#cccccc`/`#666666` — **not** pure `#000` / only-white
- `view.go` byte-identical (full-screen BG + `WithWhitespaceBackground`)
- Beams cell painting unchanged; lipgloss still `v1.1.0`
- `styleTextInput` still sets Background on prompt/text/placeholder/cursor

### What did change

- Wordmark 4→6 lines (`9f11c865`); fills `headerHeight=6`, exactly 80 cols at minimum width
- Plugin UI added more `Foreground(...).Render(...)` spans without `Background` (~108 FG vs ~101 in jellywatch; **0** `Background` calls in `screens.go`)

### Root causes (ranked)

1. **High — FG-only nested styles punch holes.** Lipgloss `ESC[0m` after a FG-only render clears background; terminal default BG shows through. Outer `view.go` paint does not repair holes inside already-styled children. Same pattern in jellywatch; denser plugin copy makes it more obvious.
2. **High — Palette is gray monochrome by design.** `#1a1a1a` + `#666666` muted text + beams gradient will never read as “pure dark / pure white.”
3. **Medium — sudo / `COLORTERM`.** Installer escalates via sudo; env reset can drop TrueColor and downshift to ANSI256/16 (looks washed).
4. **Medium — wordmark** denser / edge-to-edge at 80 cols (header only).
5. **Ruled out — lipgloss version / theme constant rebrand edits.**

### Minimal styling fix

- `theme.go`: `fg(c)` helper = Foreground + `Background(BgBase)`; apply to marks / headerStyle
- `screens.go`: stop bare FG-only renders; use `fg(...)`
- `main.go`: spinner style includes `Background(BgBase)`
- Optional: `BgBase = "#000000"` if product wants literal pure black
- Optional: ensure `COLORTERM=truecolor` before `tea.NewProgram` when TERM is capable

---

## 6. Recommended remediation PR slices

Work on branch `fix/post-rebrand-wizard-remediation` in this worktree.

### Slice A — TUI correctness (ship first)

1. Decouple systemd `postScanTasks` from library-scan gate — always install/start selected units after config write.
2. Two-phase `[setup].completed` (false until listeners up).
3. Require media pairs via `setup.ValidateDraft` before leaving paths / writing config.
4. Complete screen: only show commands for units that exist; honest daemon/plugin status.
5. Tests: empty libraries still write units; empty paths blocked; completed false until start succeeds.

### Slice B — TUI styling

6. FG+BG helper; fix screens/spinner/marks; optional pure-black BgBase; optional COLORTERM harden.
7. Visual smoke: run installer in 80×24 TrueColor terminal; no terminal-default BG holes in content box.

### Slice C — Wizard parity / hardening

8. Web: plugin consent + callback + post-activate configure/verify (or clear “run CLI” deferral).
9. Surface 412/plugin bodies in `plugininstall.Engine`.
10. Document/align activation: prefer systemd when units exist.
11. Recovery: refuse to write empty+completed, **or** treat empty media as needing setup despite marker.

### Explicit non-goals

- Do not re-litigate AuthGuard password-before-wizard order.
- Do not change adoption semantics for genuine legacy configs with media pairs.
- No watch↔library “swap fix” in TOML format args (they are already correct).

---

## 7. Immediate host recovery (ops, not PR)

1. Fix `~/.config/plex2jellyfin/config.toml`: SAB paths → `[watch]`; STORAGE → `[libraries]` (copy from jellywatch).
2. Set `setup.completed = false` until services are confirmed, **or** leave true only after units are up.
3. Install/enable systemd units (re-run fixed installer, or copy from `systemd/` + `daemon-reload`).
4. `sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web`
5. `plex2jellyfin plugin verify` once listeners are up.
6. `plex2jellyfin scan`

---

## 8. Evidence index

| Item | Location |
|---|---|
| Library-gated postScan | `cmd/installer/validation.go` `handleTaskComplete` |
| Early completed stamp | `cmd/installer/tasks.go` `generateConfigString` |
| Empty path defaults | `cmd/installer/main.go` |
| Paths Enter (no validate) | `cmd/installer/update.go` `handlePathsKeys` |
| Complete screen systemctl | `cmd/installer/screens.go` `renderComplete` |
| CLI two-phase + plugin | `cmd/plex2jellyfin/setup_cmd.go` |
| Web apply | `internal/api/setup_handlers.go` |
| Domain validation | `internal/setup/setup.go` |
| Web UI (no plugin) | `web/src/components/setup/SetupWizard.tsx` |
| Theme / holes | `cmd/installer/theme.go`, `screens.go`, `view.go` |
| Plugin 412 | `internal/jellyfin/plugininstall/plugininstall.go` `do` / `Verify` |
| Daemon empty watch | `cmd/plex2jellyfin-daemon/main.go` |

---

## 9. Git arc (wizard-relevant)

```
9e5573e1  rebrand mechanical rename
9f11c865  brand installer wordmark
9814738d  packaging /usr/bin alignment
e36b84de  feat(setup): first-run config state
d569cd5c  feat(api): atomic setup bootstrap
fa203933  feat(web): first-run setup wizard
ec653aa4  feat(cli): interactive setup wizard
2793a274  DetectAdvertiseIP
72bc8e9c  companion plugin in CLI wizard
87934339  TUI setup marker + plugin fields in config
5fcd7201..2e3921e4  TUI plugin UI + pipeline + verify gate
74908132..f7783623  setup-state adoption
```
