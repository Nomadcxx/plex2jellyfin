# TUI Installer Plugin Auto-Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The TUI installer installs the companion Jellyfin plugin (consent-gated), restarts Jellyfin (consent-gated, default yes), verifies the feedback loop after services start, and writes a config the other wizards recognize as completed setup.

**Architecture:** The installer is a bubbletea TUI (`cmd/installer`) with a collect-then-execute shape: input screens gather state into a `model`, then `startInstallation` builds an `[]installTask` pipeline executed one task at a time. Plugin consent toggles live on the Jellyfin screen, the callback URL on the Web step, and the actual work runs as pipeline tasks via the existing `internal/jellyfin/plugininstall.Engine`. Verification runs in `postScanTasks` after `startService`/`startWebService`.

**Tech Stack:** Go, bubbletea/bubbles/lipgloss, `internal/jellyfin/plugininstall` (HTTP engine, already tested), `internal/setup` (`DetectAdvertiseIP`, `CurrentVersion`), `internal/config` (`GenerateWebhookSecret`).

**Spec:** `docs/superpowers/specs/2026-07-12-tui-plugin-install-design.md`

## Global Constraints

- NEVER add AI/agent attribution to commits — no Co-Authored-By trailers, no "Generated with" lines.
- The commit-msg hook rejects any message containing the substring `bot` — this includes the word "both".
- Plugin GUID, repo name, and manifest URL come only from `plugininstall` package constants — never redefine them.
- All plugin pipeline tasks are `optional: true` — a plugin failure NEVER aborts installation (`handleTaskComplete` degrades optional failures to statusSkipped + an entry in `m.errors`).
- Both consent toggles default **Yes** (`pluginInstall: true`, `pluginRestart: true`) — a rushed run gets the full correct process.
- `[setup] version` uses `setup.CurrentVersion` (import alias `setuppkg`) — never a hardcoded literal.
- User-visible TUI copy: dark operator-console tone, ASCII punctuation (plain `-`, no em dashes in TUI strings), no marketing language.
- Scope is the fresh-install path only: `updateMode` and `uninstallMode` task lists are untouched.
- Test command for every task: `go test ./cmd/installer/ -count=1` (plus focused `-run` during TDD).

## Critical Codebase Gotcha: model copies vs task goroutines

bubbletea copies the `model` value on every `Update`. `executeTaskCmd(index, &m)` gives the task goroutine a pointer to a *snapshot* — **string/bool fields written by an execute func are lost** to later Updates (only slice-backed data like `m.tasks` statuses survives, because copies share the backing array). Two rules in this plan follow from that:

1. Everything a task func *reads* (`webhookSecret`, `pluginDaemonURL`) is resolved in `startInstallation`, in the Update goroutine, **before** any copies diverge.
2. Everything a task func *writes* for later consumers (outcome, loaded flag) goes through the shared `*pluginRunState` pointer (all copies point at the same struct).

Do not "simplify" by writing outcome to a plain model field — it will silently read as zero later.

## File Structure

- `cmd/installer/types.go` — model fields (`pluginInstall`, `pluginRestart`, `pluginDaemonURL`, `callbackURLEdited`, `pluginState`), `pluginRunState` type
- `cmd/installer/main.go` — toggle defaults in `newModel`
- `cmd/installer/tasks.go` — `[setup]` + plugin keys in `generateConfigString`; pipeline task funcs; `startInstallation` wiring
- `cmd/installer/inputs.go` — callback URL input on the Web step; `defaultCallbackURL`
- `cmd/installer/screens.go` — Jellyfin toggles + guidance; Web URL row; Complete status line
- `cmd/installer/update.go` — Jellyfin/Web focus cycling and toggle key handling
- `cmd/installer/tasks_test.go`, `cmd/installer/inputs_test.go`, new `cmd/installer/screens_test.go` — tests

---

### Task 1: Setup marker and plugin fields in the generated config

**Files:**
- Modify: `cmd/installer/types.go` (model struct, after `webPort string` ~line 170)
- Modify: `cmd/installer/main.go` (`newModel`, after `webPort: "5522",` ~line 84)
- Modify: `cmd/installer/tasks.go` (`generateConfigString` ~lines 191-317, imports)
- Test: `cmd/installer/tasks_test.go`

**Interfaces:**
- Consumes: `setup.CurrentVersion` (int const, `internal/setup`), existing `generateConfigString() (string, error)`.
- Produces: model fields `pluginInstall bool`, `pluginRestart bool`, `pluginDaemonURL string`, `callbackURLEdited bool` (defaults: `pluginInstall: true`, `pluginRestart: true` set in `newModel`); generated TOML gains `[setup]` block and `plugin_enabled` / `plugin_daemon_url` in `[jellyfin]`. Tasks 2-5 rely on these exact field names.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/installer/tasks_test.go` (imports needed: add `path/filepath` and `os` are already imported; add `setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"`):

```go
func TestGenerateConfigString_IncludesSetupMarker(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if !strings.Contains(configStr, "[setup]") {
		t.Fatalf("expected [setup] block, got:\n%s", configStr)
	}
	if !strings.Contains(configStr, fmt.Sprintf("version = %d", setuppkg.CurrentVersion)) {
		t.Fatalf("expected setup version %d, got:\n%s", setuppkg.CurrentVersion, configStr)
	}
	if !strings.Contains(configStr, "completed = true") {
		t.Fatalf("expected completed = true, got:\n%s", configStr)
	}
}

func TestGenerateConfigString_IncludesPluginFields(t *testing.T) {
	m := &model{
		watchFolders:      []WatchFolder{{Type: "tv", Paths: "/watch/tv"}},
		tvLibraryPaths:    "/lib/tv",
		jellyfinEnabled:   true,
		jellyfinURL:       "http://localhost:8096",
		jellyfinAPIKey:    "jf-api",
		webhookSecret:     "secret-1",
		pluginInstall:     true,
		pluginDaemonURL:   "http://192.168.0.10:5522",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if !strings.Contains(configStr, "plugin_enabled = true") {
		t.Fatalf("expected plugin_enabled = true, got:\n%s", configStr)
	}
	if !strings.Contains(configStr, `plugin_daemon_url = "http://192.168.0.10:5522"`) {
		t.Fatalf("expected plugin_daemon_url, got:\n%s", configStr)
	}
}

// The TUI writes TOML by hand; this guards the silent-field-loss trap by
// parsing the emitted string through the real config loader.
func TestGenerateConfigString_RoundTripsThroughConfigLoad(t *testing.T) {
	m := &model{
		watchFolders:      []WatchFolder{{Type: "tv", Paths: "/watch/tv"}},
		tvLibraryPaths:    "/lib/tv",
		jellyfinEnabled:   true,
		jellyfinURL:       "http://localhost:8096",
		jellyfinAPIKey:    "jf-api",
		webhookSecret:     "secret-1",
		pluginInstall:     true,
		pluginDaemonURL:   "http://192.168.0.10:5522",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config"))
	t.Setenv("SUDO_USER", "")
	dir := filepath.Join(tmp, ".config", "plex2jellyfin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(configStr), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := configpkg.Load()
	if err != nil {
		t.Fatalf("config.Load() on generated config: %v", err)
	}
	if cfg.Setup.Version != setuppkg.CurrentVersion || !cfg.Setup.Completed {
		t.Errorf("setup marker did not round-trip: %+v", cfg.Setup)
	}
	if !cfg.Jellyfin.PluginEnabled {
		t.Error("plugin_enabled did not round-trip")
	}
	if cfg.Jellyfin.PluginDaemonURL != "http://192.168.0.10:5522" {
		t.Errorf("plugin_daemon_url did not round-trip: %q", cfg.Jellyfin.PluginDaemonURL)
	}
}
```

Note: `fmt` must be added to the test file's imports if not present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/installer/ -run 'TestGenerateConfigString_(IncludesSetupMarker|IncludesPluginFields|RoundTripsThroughConfigLoad)' -count=1 -v`
Expected: compile error — `m.pluginInstall`, `m.pluginDaemonURL` undefined.

- [ ] **Step 3: Add model fields and defaults**

`cmd/installer/types.go`, in the `model` struct directly after `webPort string`:

```go
	// Companion plugin (fresh install)
	pluginInstall     bool   // consent: install the plugin via Jellyfin's package API
	pluginRestart     bool   // consent: restart Jellyfin so the plugin loads
	pluginDaemonURL   string // callback URL pushed to the plugin
	callbackURLEdited bool   // user manually edited the callback URL field
```

`cmd/installer/main.go`, in the `model{...}` literal after `webPort: "5522",`:

```go
			// Companion plugin defaults: the full install+restart path unless
			// the user opts out on the Jellyfin screen.
			pluginInstall: true,
			pluginRestart: true,
```

- [ ] **Step 4: Emit the new TOML**

`cmd/installer/tasks.go`: add import `setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"`.

In `generateConfigString`, change the start of the template literal from `[watch]` to:

```go
	configStr := fmt.Sprintf(`[setup]
# Managed by the setup wizards. completed = true tells the web UI to skip
# its guided setup and only ask for a password.
version = %d
completed = true

[watch]
movies = [%s]
`+ /* ...rest of the existing literal unchanged... */
```

and add `setuppkg.CurrentVersion` as the **first** argument of the `fmt.Sprintf` call (before `formatPathList(watchMovies)`).

In the `if m.jellyfinEnabled` block, extend the `[jellyfin]` template: after the `plugin_shared_secret = "%s"` line and its comment, add:

```go
plugin_enabled = %t
# Base URL the companion plugin calls back to (the plugin appends
# /api/v1/webhooks/jellyfin). From Jellyfin's point of view - never
# localhost when Jellyfin runs in a container.
plugin_daemon_url = "%s"
```

and append `m.pluginInstall, m.pluginDaemonURL` to that Sprintf's arguments (after the second `webhookSecret`).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/installer/ -count=1`
Expected: PASS, including all pre-existing `TestGenerateConfigString_*` tests.

- [ ] **Step 6: Commit**

```bash
git add cmd/installer/types.go cmd/installer/main.go cmd/installer/tasks.go cmd/installer/tasks_test.go
git commit -m "feat(installer): setup marker and plugin fields in generated config"
```

---

### Task 2: Consent toggles on the Jellyfin screen

**Files:**
- Modify: `cmd/installer/update.go` (`handleJellyfinKeys` ~line 351, `nextJellyfinInput`/`prevJellyfinInput` ~lines 631-663)
- Modify: `cmd/installer/screens.go` (`renderJellyfin` ~lines 261-321)
- Test: create `cmd/installer/screens_test.go`

**Interfaces:**
- Consumes: `pluginInstall`, `pluginRestart` from Task 1; existing `jellyfinTested bool`, `boolToYesNo(bool) string` (in screens.go).
- Produces: `(m model) jellyfinPluginTogglesVisible() bool`; focus rows `len(m.inputs)+1` (install) and `len(m.inputs)+2` (restart) on the Jellyfin screen. Task 3+ do not depend on this task.

- [ ] **Step 1: Write the failing tests**

Create `cmd/installer/screens_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func jellyfinTestModel() model {
	m := model{
		step:            stepIntegrationsJellyfin,
		jellyfinEnabled: true,
		jellyfinTested:  true,
		jellyfinVersion: "10.11.11",
		pluginInstall:   true,
		pluginRestart:   true,
	}
	m.initJellyfinInputs()
	return m
}

func TestRenderJellyfin_PluginTogglesVisibleAfterSuccessfulTest(t *testing.T) {
	m := jellyfinTestModel()

	out := m.renderJellyfin()
	for _, want := range []string{
		"Install companion plugin: Yes",
		"Restart Jellyfin after install: Yes",
		"(recommended)",
		"closes the feedback loop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in Jellyfin screen, got:\n%s", want, out)
		}
	}
}

func TestRenderJellyfin_TogglesHiddenBeforeTest(t *testing.T) {
	m := jellyfinTestModel()
	m.jellyfinTested = false

	if out := m.renderJellyfin(); strings.Contains(out, "Install companion plugin") {
		t.Fatalf("toggles should not render before a successful connection test:\n%s", out)
	}
}

func TestHandleJellyfinKeys_TogglesPluginConsent(t *testing.T) {
	m := jellyfinTestModel()
	m.focusedInput = len(m.inputs) + 1 // install toggle row

	next, _ := m.handleJellyfinKeys("up")
	got := next.(model)
	if got.pluginInstall {
		t.Error("expected up on install row to toggle pluginInstall off")
	}

	got.focusedInput = len(got.inputs) + 2 // restart toggle row
	next, _ = got.handleJellyfinKeys("down")
	got = next.(model)
	if got.pluginRestart {
		t.Error("expected down on restart row to toggle pluginRestart off")
	}
}

func TestNextJellyfinInput_CyclesThroughToggleRows(t *testing.T) {
	m := jellyfinTestModel()
	// rows: 0 enable, 1-3 inputs, 4 install toggle, 5 restart toggle
	m.focusedInput = 5
	next, _ := m.nextJellyfinInput()
	if got := next.(model); got.focusedInput != 0 {
		t.Errorf("expected focus to wrap 5 -> 0, got %d", got.focusedInput)
	}

	m.jellyfinTested = false // toggles hidden: old arithmetic
	m.focusedInput = 3
	next, _ = m.nextJellyfinInput()
	if got := next.(model); got.focusedInput != 0 {
		t.Errorf("expected focus to wrap 3 -> 0 when toggles hidden, got %d", got.focusedInput)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/installer/ -run 'TestRenderJellyfin|TestHandleJellyfinKeys_TogglesPluginConsent|TestNextJellyfinInput_CyclesThroughToggleRows' -count=1 -v`
Expected: FAIL — toggles neither rendered nor handled; focus wraps at 4 rows.

- [ ] **Step 3: Implement visibility helper and key handling**

`cmd/installer/update.go` — add above `handleJellyfinKeys`:

```go
// jellyfinPluginTogglesVisible reports whether the plugin consent rows render.
// Consent is only meaningful against a server we actually reached.
func (m model) jellyfinPluginTogglesVisible() bool {
	return m.jellyfinEnabled && m.jellyfinTested
}
```

In `handleJellyfinKeys`, replace the `"up", "k"` and `"down", "j"` cases:

```go
	case "up", "k", "down", "j":
		switch {
		case m.focusedInput == 0:
			m.jellyfinEnabled = !m.jellyfinEnabled
		case m.jellyfinPluginTogglesVisible() && m.focusedInput == len(m.inputs)+1:
			m.pluginInstall = !m.pluginInstall
		case m.jellyfinPluginTogglesVisible() && m.focusedInput == len(m.inputs)+2:
			m.pluginRestart = !m.pluginRestart
		}
```

In `nextJellyfinInput` AND `prevJellyfinInput`, replace the total computation:

```go
	total := len(m.inputs) + 1 // "Enable" is focusable row 0
	if m.jellyfinPluginTogglesVisible() {
		total += 2 // install + restart toggle rows
	}
```

(The blur/focus index mapping below is unchanged — toggle rows have no textinput, and `newInputIdx < len(m.inputs)` already guards them.)

- [ ] **Step 4: Render the toggles and guidance**

`cmd/installer/screens.go`, in `renderJellyfin`, inside the `if m.jellyfinEnabled {` block, after the connection-test status lines (after the `else if m.jellyfinVersion != ""` block closes):

```go
		if m.jellyfinPluginTogglesVisible() {
			b.WriteString("\n")
			installPrefix := "  "
			if m.focusedInput == len(m.inputs)+1 {
				installPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
			}
			b.WriteString(fmt.Sprintf("%sInstall companion plugin: %s\n", installPrefix, boolToYesNo(m.pluginInstall)))

			restartPrefix := "  "
			if m.focusedInput == len(m.inputs)+2 {
				restartPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
			}
			b.WriteString(fmt.Sprintf("%sRestart Jellyfin after install: %s   %s\n",
				restartPrefix, boolToYesNo(m.pluginRestart),
				lipgloss.NewStyle().Foreground(FgMuted).Render("(recommended)")))

			b.WriteString("\n" + lipgloss.NewStyle().Foreground(FgMuted).Render(
				"  The companion plugin closes the feedback loop: it confirms organized\n"+
					"  files against real Jellyfin items and powers orphan detection. It only\n"+
					"  loads after Jellyfin restarts - without a restart the daemon runs\n"+
					"  degraded until you restart Jellyfin yourself.") + "\n")
		}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/installer/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/installer/update.go cmd/installer/screens.go cmd/installer/screens_test.go
git commit -m "feat(installer): plugin consent toggles on the Jellyfin screen"
```

---

### Task 3: Callback URL field on the Web step

**Files:**
- Modify: `cmd/installer/inputs.go` (`initWebInputs`/`saveWebInputs` ~lines 220-235, imports)
- Modify: `cmd/installer/update.go` (`handleWebServiceKeys` ~line 502, stepSystemWeb input routing in `Update` ~line 90)
- Modify: `cmd/installer/screens.go` (`renderWebService` ~lines 500-532)
- Test: `cmd/installer/inputs_test.go`

**Interfaces:**
- Consumes: `pluginInstall`, `pluginDaemonURL`, `callbackURLEdited` (Task 1); `setup.DetectAdvertiseIP() string` (returns "" on failure); existing `normalizedWebPort(string) string` (tasks.go).
- Produces: `(m *model) defaultCallbackURL() string`; Web step input index 1 = callback URL (focus row 3); `saveWebInputs` writes `m.pluginDaemonURL`. Task 4 consumes `defaultCallbackURL` and `m.pluginDaemonURL`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/installer/inputs_test.go` (add `strings` to its imports if missing):

```go
func TestDefaultCallbackURL(t *testing.T) {
	m := &model{webEnabled: true, webPort: "5522"}
	url := m.defaultCallbackURL()
	if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, ":5522") {
		t.Errorf("web enabled: got %q, want http://<host>:5522", url)
	}

	m.webPort = "18080"
	if url := m.defaultCallbackURL(); !strings.HasSuffix(url, ":18080") {
		t.Errorf("custom port: got %q, want suffix :18080", url)
	}

	m.webEnabled = false
	if url := m.defaultCallbackURL(); !strings.HasSuffix(url, ":8686") {
		t.Errorf("web disabled: got %q, want daemon health port :8686", url)
	}
}

func TestInitWebInputs_CallbackFieldGatedOnPluginConsent(t *testing.T) {
	m := &model{webEnabled: true, webPort: "5522", jellyfinEnabled: true, pluginInstall: true}
	m.initWebInputs()
	if len(m.inputs) != 2 {
		t.Fatalf("expected port + callback URL inputs, got %d", len(m.inputs))
	}
	if v := m.inputs[1].Value(); !strings.HasSuffix(v, ":5522") {
		t.Errorf("callback input should pre-fill derived default, got %q", v)
	}

	m2 := &model{webEnabled: true, webPort: "5522", jellyfinEnabled: true, pluginInstall: false}
	m2.initWebInputs()
	if len(m2.inputs) != 1 {
		t.Fatalf("expected only port input when plugin install declined, got %d", len(m2.inputs))
	}
}

func TestSaveWebInputs_CapturesCallbackURL(t *testing.T) {
	m := &model{webEnabled: true, webPort: "5522", jellyfinEnabled: true, pluginInstall: true}
	m.initWebInputs()
	m.inputs[1].SetValue("  http://10.0.0.5:9000  ")
	m.saveWebInputs()
	if m.pluginDaemonURL != "http://10.0.0.5:9000" {
		t.Errorf("pluginDaemonURL = %q, want trimmed URL", m.pluginDaemonURL)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/installer/ -run 'TestDefaultCallbackURL|TestInitWebInputs_CallbackFieldGatedOnPluginConsent|TestSaveWebInputs_CapturesCallbackURL' -count=1 -v`
Expected: compile error — `defaultCallbackURL` undefined.

- [ ] **Step 3: Implement derivation, inputs, and save**

`cmd/installer/inputs.go` — add imports `strings` and `setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"`, then:

```go
// defaultCallbackURL derives where Jellyfin's companion plugin posts events:
// the web UI when it will exist, otherwise the daemon's health endpoint
// (which mounts the same /api/v1/webhooks/jellyfin route).
func (m *model) defaultCallbackURL() string {
	host := setuppkg.DetectAdvertiseIP()
	if host == "" {
		host = "localhost"
	}
	port := normalizedWebPort(m.webPort)
	if !m.webEnabled {
		port = "8686"
	}
	return "http://" + host + ":" + port
}
```

Replace `initWebInputs` and `saveWebInputs`:

```go
func (m *model) initWebInputs() {
	m.inputs = make([]textinput.Model, 1)

	m.inputs[0] = textinput.New()
	m.inputs[0].Placeholder = "5522"
	m.inputs[0].Width = 10
	m.inputs[0].CharLimit = 5
	m.inputs[0].SetValue(m.webPort)
	styleTextInput(&m.inputs[0])

	if m.jellyfinEnabled && m.pluginInstall {
		ti := textinput.New()
		ti.Placeholder = "http://<lan-ip>:<port>"
		ti.Width = 40
		ti.CharLimit = 200
		if m.pluginDaemonURL != "" {
			ti.SetValue(m.pluginDaemonURL)
		} else {
			ti.SetValue(m.defaultCallbackURL())
		}
		styleTextInput(&ti)
		m.inputs = append(m.inputs, ti)
	}
}

func (m *model) saveWebInputs() {
	if len(m.inputs) >= 1 {
		m.webPort = m.inputs[0].Value()
	}
	if len(m.inputs) >= 2 {
		m.pluginDaemonURL = strings.TrimSpace(m.inputs[1].Value())
	}
}
```

- [ ] **Step 4: Wire focus, live re-derivation, and rendering**

`cmd/installer/update.go`, `handleWebServiceKeys` — replace the tab cases and extend the toggle cases so flipping Enable re-derives an unedited URL:

```go
	rows := 3
	if len(m.inputs) >= 2 {
		rows = 4
	}
	switch key {
	case "tab":
		m.focusedInput = (m.focusedInput + 1) % rows
	case "shift+tab":
		m.focusedInput = (m.focusedInput + rows - 1) % rows
	case "up", "k", "down", "j":
		switch m.focusedInput {
		case 0:
			m.webEnabled = !m.webEnabled
			if len(m.inputs) >= 2 && !m.callbackURLEdited {
				m.inputs[1].SetValue(m.defaultCallbackURL())
			}
		case 1:
			m.webStartNow = !m.webStartNow
		}
	case "e", "E":
		m.webEnabled = !m.webEnabled
		if len(m.inputs) >= 2 && !m.callbackURLEdited {
			m.inputs[1].SetValue(m.defaultCallbackURL())
		}
	case "enter":
		m.saveWebInputs()
		return m.nextStep()
	case "esc":
		return m.prevStep()
	}
	return m, nil
```

In `Update` (the `tea.KeyMsg` branch), replace the stepSystemWeb routing block (currently `if mdl.step == stepSystemWeb && mdl.focusedInput == 2 { ... }`):

```go
				if mdl.step == stepSystemWeb && mdl.focusedInput == 2 {
					var inputCmd tea.Cmd
					mdl.inputs[0], inputCmd = mdl.inputs[0].Update(msg)
					// Port edits re-derive an untouched callback URL live.
					if len(mdl.inputs) >= 2 && !mdl.callbackURLEdited {
						mdl.webPort = mdl.inputs[0].Value()
						mdl.inputs[1].SetValue(mdl.defaultCallbackURL())
					}
					return mdl, inputCmd
				}
				if mdl.step == stepSystemWeb && mdl.focusedInput == 3 && len(mdl.inputs) >= 2 {
					var inputCmd tea.Cmd
					mdl.inputs[1], inputCmd = mdl.inputs[1].Update(msg)
					mdl.callbackURLEdited = true
					return mdl, inputCmd
				}
```

`cmd/installer/screens.go`, `renderWebService` — after the Port row, before the muted footer:

```go
	if len(m.inputs) >= 2 {
		urlPrefix := "  "
		if m.focusedInput == 3 {
			urlPrefix = lipgloss.NewStyle().Foreground(Primary).Render("▸ ")
		}
		b.WriteString(fmt.Sprintf("%sPlugin callback URL: %s\n", urlPrefix, m.inputs[1].View()))
		b.WriteString(lipgloss.NewStyle().Foreground(FgMuted).Render(
			"  Where Jellyfin's companion plugin posts events back to this machine.\n"+
				"  Never localhost when Jellyfin runs in a container.") + "\n")
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/installer/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/installer/inputs.go cmd/installer/update.go cmd/installer/screens.go cmd/installer/inputs_test.go
git commit -m "feat(installer): plugin callback URL field on the web step"
```

---

### Task 4: Pipeline tasks — install, restart, configure+verify

**Files:**
- Modify: `cmd/installer/types.go` (add `pluginRunState` type + `pluginState` model field)
- Modify: `cmd/installer/tasks.go` (`startInstallation` fresh-install branch; new task funcs; imports)
- Test: `cmd/installer/tasks_test.go`

**Interfaces:**
- Consumes: `plugininstall.New(baseURL, apiKey string, client *http.Client) *Engine` with `Inspect(ctx) (*Inspection, error)` (`Inspection{ServerVersion string, ABISupported bool, RepoRegistered bool, InstalledVersion string, PluginResponding bool}`), `RegisterRepo(ctx) (bool, error)`, `Install(ctx) error`, `Restart(ctx) error`, `WaitReady(ctx, timeout) error`, `Configure(ctx, daemonBaseURL, sharedSecret string) error`, `Verify(ctx) (*VerifyResult, error)` (`VerifyResult{Sent bool, DaemonURL string, DaemonStatusCode int, Authenticated bool, Error string}`); `configpkg.GenerateWebhookSecret()`; `defaultCallbackURL` (Task 3); model fields (Task 1).
- Produces: `pluginRunState{loaded bool, outcome string}` with outcome values `"skipped"`, `"needs-restart"`, `"unverified"`, `"failed"`, `"verified"`; `m.pluginState *pluginRunState` (always non-nil after fresh-install `startInstallation`). Task 5 consumes `m.pluginState.outcome`.

Outcome semantics (Task 5 renders from these):
- `skipped` — Jellyfin disabled or install consent off; no Complete line.
- `needs-restart` — install ran (or will run) but the plugin never loaded (restart declined or failed).
- `unverified` — plugin loaded but configure/verify could not run (no listener started).
- `failed` — a plugin task errored.
- `verified` — signed test event round-tripped.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/installer/tasks_test.go` (add imports `net/http`, `net/http/httptest`):

```go
func taskNames(tasks []installTask) []string {
	names := make([]string, 0, len(tasks))
	for _, t := range tasks {
		names = append(names, t.name)
	}
	return names
}

func hasTask(tasks []installTask, name string) bool {
	for _, t := range tasks {
		if t.name == name {
			return true
		}
	}
	return false
}

func TestStartInstallation_PluginTasksGatedByConsent(t *testing.T) {
	cases := []struct {
		name            string
		jellyfin        bool
		install         bool
		restart         bool
		serviceStartNow bool
		wantInstall     bool
		wantRestart     bool
		wantConfigure   bool
	}{
		{"full consent", true, true, true, true, true, true, true},
		{"jellyfin disabled", false, true, true, true, false, false, false},
		{"install declined", true, false, true, true, false, false, false},
		{"restart declined", true, true, false, true, true, false, false},
		{"no listener started", true, true, true, false, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model{
				serviceEnabled:  true,
				serviceStartNow: tc.serviceStartNow,
				webEnabled:      false,
				jellyfinEnabled: tc.jellyfin,
				jellyfinURL:     "http://localhost:8096",
				jellyfinAPIKey:  "k",
				webhookSecret:   "s",
				pluginInstall:   tc.install,
				pluginRestart:   tc.restart,
				pluginDaemonURL: "http://10.0.0.5:5522",
			}

			next, _ := m.startInstallation()
			got := next.(model)

			if hasTask(got.tasks, "Install Jellyfin plugin") != tc.wantInstall {
				t.Errorf("install task presence = %v, want %v (tasks: %v)",
					!tc.wantInstall, tc.wantInstall, taskNames(got.tasks))
			}
			if hasTask(got.tasks, "Restart Jellyfin") != tc.wantRestart {
				t.Errorf("restart task presence = %v, want %v", !tc.wantRestart, tc.wantRestart)
			}
			if hasTask(got.postScanTasks, "Configure plugin feedback loop") != tc.wantConfigure {
				t.Errorf("configure task presence = %v, want %v (postScan: %v)",
					!tc.wantConfigure, tc.wantConfigure, taskNames(got.postScanTasks))
			}
			if got.pluginState == nil {
				t.Fatal("pluginState must always be initialized on fresh install")
			}
		})
	}
}

func TestStartInstallation_ResolvesPluginPrerequisitesUpFront(t *testing.T) {
	m := model{
		serviceEnabled:  true,
		jellyfinEnabled: true,
		jellyfinURL:     "http://localhost:8096",
		jellyfinAPIKey:  "k",
		pluginInstall:   true,
		pluginRestart:   true,
	}

	next, _ := m.startInstallation()
	got := next.(model)

	if got.webhookSecret == "" {
		t.Error("webhook secret must be generated before the pipeline forks goroutines")
	}
	if got.pluginDaemonURL == "" {
		t.Error("pluginDaemonURL must be derived before the pipeline forks goroutines")
	}
	if got.pluginState.outcome != "needs-restart" {
		t.Errorf("initial outcome = %q, want needs-restart", got.pluginState.outcome)
	}
}

func TestStartInstallation_PluginSkippedOutcomeWhenDeclined(t *testing.T) {
	m := model{serviceEnabled: true, jellyfinEnabled: true, pluginInstall: false}
	next, _ := m.startInstallation()
	if got := next.(model); got.pluginState.outcome != "skipped" {
		t.Errorf("outcome = %q, want skipped", got.pluginState.outcome)
	}
}

func TestInstallJellyfinPlugin_ServerErrorMarksFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := &model{
		jellyfinURL:    srv.URL,
		jellyfinAPIKey: "k",
		pluginState:    &pluginRunState{outcome: "needs-restart"},
	}

	if err := installJellyfinPlugin(m); err == nil {
		t.Fatal("expected error from erroring server")
	}
	if m.pluginState.outcome != "failed" {
		t.Errorf("outcome = %q, want failed", m.pluginState.outcome)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/installer/ -run 'TestStartInstallation_Plugin|TestStartInstallation_Resolves|TestInstallJellyfinPlugin' -count=1 -v`
Expected: compile error — `pluginRunState`, `installJellyfinPlugin` undefined.

- [ ] **Step 3: Add the shared state type**

`cmd/installer/types.go`, below the `model` struct:

```go
// pluginRunState is shared by pointer across bubbletea model copies so async
// task goroutines and later Update calls see one source of truth. Do NOT move
// these onto plain model fields: writes from executeTaskCmd goroutines to
// value-copied fields are lost (see stopRunningServices/daemonWasRunning for
// the failure mode this avoids).
type pluginRunState struct {
	loaded  bool   // plugin responded after install/restart
	outcome string // "skipped", "needs-restart", "unverified", "failed", "verified"
}
```

and in the `model` struct, after `callbackURLEdited bool`:

```go
	pluginState *pluginRunState
```

- [ ] **Step 4: Wire startInstallation and implement the task funcs**

`cmd/installer/tasks.go` — add imports:

```go
	"net/http"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
```

In `startInstallation`, in the fresh-install `else` branch, immediately after the `m.postScanTasks = []installTask{...}` assignment and the `if m.webEnabled {...}` append (i.e., before `m.currentTaskIndex = 0`):

```go
		// Companion plugin: resolve everything the async pipeline reads up
		// front, in this goroutine, before model copies diverge (see the
		// pluginRunState comment in types.go).
		m.pluginState = &pluginRunState{outcome: "skipped"}
		if m.jellyfinEnabled && m.pluginInstall {
			m.pluginState.outcome = "needs-restart" // upgraded as tasks succeed
			if strings.TrimSpace(m.webhookSecret) == "" {
				if generated, err := configpkg.GenerateWebhookSecret(); err == nil {
					m.webhookSecret = generated
				}
			}
			if strings.TrimSpace(m.pluginDaemonURL) == "" {
				m.pluginDaemonURL = m.defaultCallbackURL()
			}

			m.tasks = append(m.tasks, installTask{
				name:        "Install Jellyfin plugin",
				description: "Registering the plugin repository and installing the companion plugin",
				execute:     installJellyfinPlugin,
				optional:    true,
				status:      statusPending,
			})
			if m.pluginRestart {
				m.tasks = append(m.tasks, installTask{
					name:        "Restart Jellyfin",
					description: "Restarting Jellyfin and waiting for the plugin to load (up to 60s)",
					execute:     restartJellyfinForPlugin,
					optional:    true,
					status:      statusPending,
				})
				listenerStarts := (m.serviceEnabled && m.serviceStartNow) || (m.webEnabled && m.webStartNow)
				if listenerStarts {
					m.postScanTasks = append(m.postScanTasks, installTask{
						name:        "Configure plugin feedback loop",
						description: "Pushing the callback URL and secret to the plugin, then verifying a signed test event",
						execute:     configurePluginFeedback,
						optional:    true,
						status:      statusPending,
					})
				}
			}
		}
```

New task funcs at the bottom of `tasks.go`:

```go
const pluginRestartWaitTimeout = 60 * time.Second

func newPluginEngine(m *model) *plugininstall.Engine {
	return plugininstall.New(m.jellyfinURL, m.jellyfinAPIKey, &http.Client{Timeout: 15 * time.Second})
}

func pluginTaskContext(m *model) context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func installJellyfinPlugin(m *model) error {
	st := m.pluginState
	if st == nil {
		return fmt.Errorf("plugin state missing")
	}
	engine := newPluginEngine(m)
	ctx := pluginTaskContext(m)

	insp, err := engine.Inspect(ctx)
	if err != nil {
		st.outcome = "failed"
		return fmt.Errorf("inspect jellyfin: %v", err)
	}
	if !insp.ABISupported {
		st.outcome = "failed"
		return fmt.Errorf("jellyfin %s does not support the plugin (needs 10.11.x)", insp.ServerVersion)
	}
	if insp.InstalledVersion != "" && insp.PluginResponding {
		st.loaded = true
		st.outcome = "unverified"
		return nil
	}
	if _, err := engine.RegisterRepo(ctx); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("register plugin repository: %v", err)
	}
	if insp.InstalledVersion == "" {
		if err := engine.Install(ctx); err != nil {
			st.outcome = "failed"
			return fmt.Errorf("install plugin: %v", err)
		}
	}
	return nil
}

func restartJellyfinForPlugin(m *model) error {
	st := m.pluginState
	if st == nil {
		return fmt.Errorf("plugin state missing")
	}
	if st.outcome == "failed" {
		return fmt.Errorf("skipping restart: plugin install did not succeed")
	}
	if st.loaded {
		return nil // already responding; nothing to restart for
	}
	engine := newPluginEngine(m)
	ctx := pluginTaskContext(m)

	if err := engine.Restart(ctx); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("restart jellyfin: %v", err)
	}
	if err := engine.WaitReady(ctx, pluginRestartWaitTimeout); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("plugin did not come back after restart: %v", err)
	}
	st.loaded = true
	st.outcome = "unverified"
	return nil
}

func configurePluginFeedback(m *model) error {
	st := m.pluginState
	if st == nil || !st.loaded {
		return fmt.Errorf("plugin not loaded; restart Jellyfin, then run: plex2jellyfin plugin verify")
	}
	engine := newPluginEngine(m)
	ctx := pluginTaskContext(m)

	if err := engine.Configure(ctx, m.pluginDaemonURL, m.webhookSecret); err != nil {
		st.outcome = "failed"
		return fmt.Errorf("configure plugin: %v", err)
	}
	res, err := engine.Verify(ctx)
	if err != nil {
		st.outcome = "failed"
		return fmt.Errorf("verify feedback loop: %v", err)
	}
	if !res.Sent || !res.Authenticated {
		st.outcome = "failed"
		detail := res.Error
		if detail == "" {
			detail = fmt.Sprintf("daemon responded %d", res.DaemonStatusCode)
		}
		return fmt.Errorf("test event did not round-trip: %s", detail)
	}
	st.outcome = "verified"
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/installer/ -count=1`
Expected: PASS (including `TestStartInstallation_WebUIDisabledSkipsWebTasks` — the configure task's name contains "feedback loop", not "web service", so it doesn't trip that test).

- [ ] **Step 6: Commit**

```bash
git add cmd/installer/types.go cmd/installer/tasks.go cmd/installer/tasks_test.go
git commit -m "feat(installer): plugin install pipeline tasks"
```

---

### Task 5: Plugin status on the Complete screen

**Files:**
- Modify: `cmd/installer/screens.go` (`renderComplete` ~line 715, fresh-install branch)
- Test: `cmd/installer/screens_test.go`

**Interfaces:**
- Consumes: `m.pluginState.outcome` (Task 4); existing marks `checkMark`, `failMark`, `skipMark` and styles in `renderComplete`.
- Produces: user-facing status line; nothing downstream.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/installer/screens_test.go`:

```go
func TestRenderComplete_PluginOutcomeLines(t *testing.T) {
	cases := []struct {
		outcome string
		want    string
	}{
		{"verified", "feedback loop active"},
		{"needs-restart", "restart Jellyfin to load it"},
		{"unverified", "plex2jellyfin plugin verify"},
		{"failed", "plex2jellyfin plugin install"},
	}
	for _, tc := range cases {
		m := model{pluginState: &pluginRunState{outcome: tc.outcome}}
		if out := m.renderComplete(); !strings.Contains(out, tc.want) {
			t.Errorf("outcome %q: expected %q in Complete screen, got:\n%s", tc.outcome, tc.want, out)
		}
	}
}

func TestRenderComplete_NoPluginLineWhenSkipped(t *testing.T) {
	for _, st := range []*pluginRunState{nil, {outcome: "skipped"}} {
		m := model{pluginState: st}
		if out := m.renderComplete(); strings.Contains(out, "Companion plugin") {
			t.Errorf("expected no plugin line for state %+v, got:\n%s", st, out)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/installer/ -run 'TestRenderComplete_PluginOutcomeLines|TestRenderComplete_NoPluginLineWhenSkipped' -count=1 -v`
Expected: FAIL — no plugin lines rendered.

- [ ] **Step 3: Render the status line**

`cmd/installer/screens.go`, in `renderComplete`, in the fresh-install branch after the scan-stats block (right before the `// ── What's Next ...` section):

```go
	// ── Companion plugin ──────────────────────────────────────────────────
	if m.pluginState != nil {
		switch m.pluginState.outcome {
		case "verified":
			b.WriteString(checkMark.String() + " " + muted.Render("Companion plugin verified - feedback loop active") + "\n\n")
		case "needs-restart":
			b.WriteString(skipMark.String() + " " + muted.Render("Companion plugin downloaded; restart Jellyfin to load it, then run: ") +
				cmd.Render("plex2jellyfin plugin verify") + "\n\n")
		case "unverified":
			b.WriteString(skipMark.String() + " " + muted.Render("Companion plugin loaded but unverified (services not started) - run: ") +
				cmd.Render("plex2jellyfin plugin verify") + "\n\n")
		case "failed":
			b.WriteString(failMark.String() + " " + muted.Render("Companion plugin step failed - run: ") +
				cmd.Render("plex2jellyfin plugin install") + "\n\n")
		}
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/installer/ -count=1`
Expected: PASS.

- [ ] **Step 5: Build everything and run the full suite**

Run: `go build ./... && go test ./cmd/installer/ -count=1`
Expected: clean build, all installer tests PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/installer/screens.go cmd/installer/screens_test.go
git commit -m "feat(installer): companion plugin status on the complete screen"
```

---

## Deviations from spec (recorded deliberately)

- Outcome set gains `"unverified"` (plugin loaded, configure/verify never ran because no listener started). The spec's §6 "verify-impossible situations skip quietly with the recovery pointer" requires distinguishing this from `"needs-restart"` — a loaded plugin must not be told "restart Jellyfin to load it".
- The spec's `pluginVerified bool` model field is dropped as redundant: it is exactly `outcome == "verified"`. Single source of truth in `pluginRunState.outcome`.

## Post-plan note (not a task)

`stopRunningServices` writes `m.daemonWasRunning` from a task goroutine through the `executeTaskCmd` pointer — per the model-copy gotcha above, that write is likely lost before `restartServices` reads it, meaning update mode may never restart services. Pre-existing, out of scope here; flag to the user.
