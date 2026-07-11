# Setup-State Adoption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Isolation & delivery:** Execute this plan in a dedicated git worktree (use the superpowers:using-git-worktrees skill) on branch `setup-state-adoption` off `main`. The final task pushes the branch and opens a PR — this work is delivered for review, NOT merged directly to main.

**Goal:** Every surface (CLI wizard, web wizard, TUI installer, and configs already deployed in the field) agrees on one explicit "setup already done" signal, so the web UI never re-runs its guided setup for a configured install — users with completed setup only ever create a web password.

**Architecture:** Setup completion is decided by `setup.NeedsSetup()` (`internal/setup/setup.go:73`): an explicit `[setup] version/completed` marker when present, else the `HasMediaPair` heuristic for configs that predate the marker. All wizards already write the marker (the TUI installer gains it in the concurrently-executing TUI plugin plan — see Coordination below). The one remaining hole is **legacy configs**: they pass only via the heuristic, and stay implicit forever — a later hand-edit to paths can flip the heuristic and re-wizard a long-configured user. This plan adds a one-time **adoption** step: when the web server loads a legacy-complete config, it stamps the explicit marker and persists it.

**Tech Stack:** Go, `internal/setup`, `internal/config`, `cmd/plex2jellyfin-web`.

## Current state (verified 2026-07-12, do not re-derive)

- `setup.NeedsSetup(cfg)`: `version > 0` → `!completed`; else `!HasMediaPair(cfg)`. Table-tested in `internal/setup/setup_test.go:10` with helpers `configWithPaths(...)` (line 108) and `versionedConfig(completed bool)` (line 117).
- CLI wizard: writes marker (`cmd/plex2jellyfin/setup_cmd.go:412`) AND guards re-runs (`setup_cmd.go:255`: "This install is already configured. Re-run setup anyway?" default No).
- Web wizard: writes marker after daemon activation (`internal/api/setup_handlers.go:91`).
- Web frontend `AuthGuard` (`web/src/components/auth/AuthGuard.tsx`): no password → `SetupForm` (create password); password + unauthenticated → `LoginForm`; authenticated → fetches `/setup/status` and redirects to the wizard only when `required`. **This sequencing is already correct — no frontend changes in this plan.**
- API contract pinned by `internal/api/setup_handlers_test.go` incl. `TestSetupStatusBypassesLegacyConfiguredInstall`.

## Coordination with the TUI plugin plan

The TUI installer's missing `[setup]` emission is Task 1 of `docs/superpowers/plans/2026-07-12-tui-plugin-install.md`, being executed by another agent on `cmd/installer/*`. This plan MUST NOT touch `cmd/installer/` — zero file overlap, zero merge conflicts. Files this plan touches: `internal/setup/setup.go` + test, `cmd/plex2jellyfin-web/main.go`, `docs-site/content/docs/reference/configuration.md`.

## Global Constraints

- NEVER add AI/agent attribution anywhere — no Co-Authored-By trailers in commits, no "Generated with" lines in the PR body.
- The commit-msg hook rejects any message containing the substring `bot` — this includes the word "both".
- `setup.CurrentVersion` is the only source for the version value — never hardcode the literal.
- Adoption must be conservative: mutate only configs with `Setup.Version == 0` AND a complete media pair. Never touch versioned configs (completed or not), never un-complete anything.
- A failed persist of the adopted marker must not prevent the web server from starting (the heuristic still covers that session) — warn and continue.
- Docs keep the dark operator-console tone.
- Test commands: `go test ./internal/setup/ -count=1` and `go build ./...`.

---

### Task 1: `AdoptLegacyCompletion` in internal/setup

**Files:**
- Modify: `internal/setup/setup.go` (below `HasMediaPair`, ~line 90)
- Test: `internal/setup/setup_test.go`

**Interfaces:**
- Consumes: existing `HasMediaPair(cfg *config.Config) bool`, `CurrentVersion` const, test helpers `configWithPaths`, `versionedConfig`.
- Produces: `AdoptLegacyCompletion(cfg *config.Config) bool` — returns true when it stamped the marker (caller must persist). Task 2 consumes this exact signature.

- [ ] **Step 1: Write the failing tests**

Append to `internal/setup/setup_test.go`:

```go
func TestAdoptLegacyCompletion(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantAdopted bool
	}{
		{name: "legacy complete adopts", cfg: configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil), wantAdopted: true},
		{name: "legacy incomplete untouched", cfg: configWithPaths([]string{"/watch/tv"}, nil, nil, nil), wantAdopted: false},
		{name: "blank untouched", cfg: config.DefaultConfig(), wantAdopted: false},
		{name: "versioned complete untouched", cfg: versionedConfig(true), wantAdopted: false},
		{name: "versioned incomplete untouched", cfg: versionedConfig(false), wantAdopted: false},
		{name: "nil untouched", cfg: nil, wantAdopted: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var beforeVersion int
			var beforeCompleted bool
			if tt.cfg != nil {
				beforeVersion = tt.cfg.Setup.Version
				beforeCompleted = tt.cfg.Setup.Completed
			}

			if got := AdoptLegacyCompletion(tt.cfg); got != tt.wantAdopted {
				t.Fatalf("AdoptLegacyCompletion() = %v, want %v", got, tt.wantAdopted)
			}

			if tt.cfg == nil {
				return
			}
			if tt.wantAdopted {
				if tt.cfg.Setup.Version != CurrentVersion || !tt.cfg.Setup.Completed {
					t.Fatalf("adopted config not stamped: %+v", tt.cfg.Setup)
				}
				if NeedsSetup(tt.cfg) {
					t.Fatal("adopted config must not need setup")
				}
			} else {
				if tt.cfg.Setup.Version != beforeVersion || tt.cfg.Setup.Completed != beforeCompleted {
					t.Fatalf("non-adopted config was mutated: %+v", tt.cfg.Setup)
				}
			}
		})
	}
}

// The stamp must survive a real Save/Load cycle - this is what protects a
// legacy user whose config paths are later hand-edited (the heuristic would
// flip, the explicit marker must not).
func TestAdoptLegacyCompletionPersistsThroughSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp+"/.config")
	t.Setenv("SUDO_USER", "")

	cfg := configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	if !AdoptLegacyCompletion(cfg) {
		t.Fatal("expected adoption")
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save adopted config: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload adopted config: %v", err)
	}
	if loaded.Setup.Version != CurrentVersion || !loaded.Setup.Completed {
		t.Fatalf("marker did not survive round-trip: %+v", loaded.Setup)
	}

	// The heuristic-breaking edit: wipe the media pair. Explicit marker wins.
	loaded.Watch.TV = nil
	loaded.Libraries.TV = nil
	if NeedsSetup(loaded) {
		t.Fatal("explicit marker must keep setup complete after path edits")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/setup/ -run TestAdoptLegacyCompletion -count=1 -v`
Expected: compile error — `AdoptLegacyCompletion` undefined.

- [ ] **Step 3: Implement**

`internal/setup/setup.go`, directly below `HasMediaPair`:

```go
// AdoptLegacyCompletion stamps the explicit setup marker onto configs that
// predate it: version 0 but recognizably configured (a complete media pair).
// This converts the HasMediaPair heuristic into durable state once, so later
// hand-edits to paths can never re-trigger a wizard for a configured install.
// Returns true when the config was mutated; the caller persists it.
func AdoptLegacyCompletion(cfg *config.Config) bool {
	if cfg == nil || cfg.Setup.Version > 0 {
		return false
	}
	if !HasMediaPair(cfg) {
		return false
	}
	cfg.Setup.Version = CurrentVersion
	cfg.Setup.Completed = true
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/setup/ -count=1`
Expected: PASS (including the pre-existing `TestNeedsSetupState`).

- [ ] **Step 5: Commit**

```bash
git add internal/setup/setup.go internal/setup/setup_test.go
git commit -m "feat(setup): adopt legacy configs into the explicit completion marker"
```

---

### Task 2: Stamp at web server startup

**Files:**
- Modify: `cmd/plex2jellyfin-web/main.go` (`runServer`, after `config.Load()` ~line 56; imports)

**Interfaces:**
- Consumes: `setup.AdoptLegacyCompletion(cfg) bool` (Task 1); existing `cfg.Save() error`.
- Produces: nothing downstream — the loaded `cfg` (already passed to `api.NewServer`) carries the stamp for this process either way; the Save makes it durable.

Why the web server and not `config.Load` itself: `Load` runs in every binary including read-only contexts (daemon status calls, CLI one-shots) — auto-writing from a loader is a side effect nobody expects and invites concurrent-write clobbers. The web server is the single surface where wizard duplication hurts, it loads the config exactly once at startup, and it already runs with write access to the config dir.

- [ ] **Step 1: Wire the adoption**

`cmd/plex2jellyfin-web/main.go` — add import:

```go
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
```

In `runServer`, directly after the `config.Load()` error check:

```go
	// Configs written before the [setup] marker existed are recognized as
	// configured only by heuristic. Stamp them explicitly once so the web
	// wizard can never re-trigger for an already-configured install.
	if setupdomain.AdoptLegacyCompletion(cfg) {
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not persist setup completion marker: %v\n", err)
		}
	}
```

(No new test file here: the adoption semantics and persistence are fully covered by Task 1's tests; this is four lines of wiring whose only failure mode — unwritable config — is deliberately non-fatal per Global Constraints.)

- [ ] **Step 2: Build and run the full affected suites**

Run: `go build ./... && go test ./internal/setup/ ./internal/api/ -count=1`
Expected: clean build; PASS including `TestSetupStatusBypassesLegacyConfiguredInstall` (heuristic path still works for the session even without a persisted stamp).

- [ ] **Step 3: Commit**

```bash
git add cmd/plex2jellyfin-web/main.go
git commit -m "feat(web): stamp legacy setup completion at server startup"
```

---

### Task 3: Document the [setup] contract

**Files:**
- Modify: `docs-site/content/docs/reference/configuration.md` (add a `[setup]` section entry; place it in the same order sections appear in the emitted config — before `[watch]`)

**Interfaces:** none — docs only.

- [ ] **Step 1: Add the reference entry**

Insert a section documenting, in the file's existing key-list style and tone:

```markdown
## [setup]

Wizard completion state. Written automatically — you normally never edit this.

| Key | Type | Description |
|-----|------|-------------|
| `version` | int | Setup schema version the completing wizard wrote. |
| `completed` | bool | `true` once any setup surface finished: CLI wizard, web wizard, or the TUI installer. The web UI skips its guided setup when this is `true` and only asks you to create a password. |

Every setup surface writes the same marker, so completing setup once — from
any of them — completes it everywhere. Configs that predate this block are
adopted automatically the first time the web server starts.

To deliberately re-run the web guided setup on a configured install, set
`completed = false` and restart `plex2jellyfin-web`. The CLI wizard instead
asks for confirmation before re-running on a configured install.
```

Adapt table formatting to match how the rest of `configuration.md` documents keys (if it uses bullet lists rather than tables, use its list style — match the file, not this plan).

- [ ] **Step 2: Verify rendering**

Run: `grep -n "## \[setup\]" docs-site/content/docs/reference/configuration.md`
Expected: one hit, positioned before the `[watch]` section's documentation.

- [ ] **Step 3: Commit**

```bash
git add docs-site/content/docs/reference/configuration.md
git commit -m "docs(reference): the setup block and cross-surface completion"
```

---

### Task 4: Push branch and open the PR

**Files:** none (delivery only).

- [ ] **Step 1: Final verification**

Run: `go build ./... && go test ./internal/setup/ ./internal/api/ ./cmd/plex2jellyfin/ -count=1`
Expected: clean build, all PASS.

- [ ] **Step 2: Push and create the PR**

```bash
git push -u origin setup-state-adoption
gh pr create \
  --title "Explicit cross-surface setup completion state" \
  --body "$(cat <<'EOF'
## Summary

- internal/setup: AdoptLegacyCompletion stamps the explicit [setup] marker onto pre-marker configs that are recognizably configured (complete media pair), converting the HasMediaPair heuristic into durable state
- plex2jellyfin-web: adopts at startup and persists, so the web guided setup can never re-trigger for an already-configured install; a failed persist degrades to the heuristic with a warning
- docs: [setup] reference entry documenting the cross-surface completion contract and the deliberate re-run escape hatch

## Context

All setup surfaces (CLI wizard, web wizard, TUI installer as of the TUI plugin slice) write [setup] version/completed. Legacy configs relied on a path heuristic that a later hand-edit could flip, re-wizarding configured users. Adoption closes that permanently. No frontend changes: AuthGuard already sequences password -> login -> wizard-only-if-required.

## Testing

- internal/setup: adoption table test (legacy complete/incomplete, versioned, blank, nil) plus a Save/Load round-trip proving the stamp survives the exact heuristic-breaking edit it guards against
- go build ./... and internal/setup + internal/api + cmd/plex2jellyfin suites green
EOF
)"
```

The PR body must contain NO attribution lines. Verify the title and body contain no `bot` substring (they don't — checked).

- [ ] **Step 3: Report the PR URL**

Print the URL from `gh pr create` output as the final deliverable.
