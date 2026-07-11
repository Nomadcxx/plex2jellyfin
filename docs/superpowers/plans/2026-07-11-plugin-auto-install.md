# Wizard-Driven Jellyfin Plugin Installation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The setup wizard (and a standalone `plex2jellyfin plugin` command) installs, configures, and verifies the companion Jellyfin plugin through Jellyfin's own package API.

**Architecture:** Phase A prepares the plugin repo (real GUID, plugin 1.2.0 with a `test-webhook` endpoint and quiet-when-unconfigured forwarding, release CI that publishes a zip and stamps a repo-level manifest). Phase B adds a six-stage engine (`internal/jellyfin/plugininstall`) to the main repo, a `plex2jellyfin plugin` command, and two wizard steps: install/restart inside the Jellyfin step, configure/verify after daemon activation.

**Tech Stack:** Go (stdlib + httptest fakes), C# net9.0 (Jellyfin.Controller 10.11.x), GitHub Actions.

Spec: `docs/superpowers/specs/2026-07-11-plugin-auto-install-design.md`.

## Global Constraints

- Plugin GUID everywhere (C#, manifests, Go engine): `f4eda3a1-c062-49b3-a958-7cf9ca80c269`
- Manifest URL: `https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json`
- Plugin version this slice: `1.2.0`; `targetAbi 10.11.0.0`; engine ABI gate: server version must start with `10.11.`
- Restart prompt defaults to **No**; restart readiness poll: 60s deadline, 2s interval
- Setup completion (`setup.completed = true`) must never depend on any plugin stage
- Plugin repo working copy: `/home/nomadx/Documents/plex2jellyfin-plugin` (clone in Task 1). Main repo: `/home/nomadx/Documents/plex2jellyfin`
- Commit messages: NEVER add AI/agent attribution or Co-Authored-By trailers. The global commit-msg hook rejects any message containing the substring `bot` — this includes the word "both"; avoid it.
- CI commits in the plugin repo must be authored as `Nomadcxx <noovie@gmail.com>` — never `github-actions[bot]` (a `[bot]` contributor is exactly what this project purged from its history).
- Never touch the live Jellyfin (`:8096`) or the running JellyWatch daemon (`:8686`) on this host. E2E uses a throwaway container on `:18096`.
- Version upgrades: once the repository is registered, Jellyfin's own plugin-update mechanism handles newer manifest versions; re-running `plex2jellyfin plugin install` also installs the newest version. The spec's "offer upgrade" degradation row is satisfied this way — the engine does not track a LatestVersion.

---

## Phase A — plugin repo (`plex2jellyfin-plugin`)

The plugin repo has no automated test project; each task's cycle is `dotnet build -c Release` (compile gate) → commit. Do not add a C# test project in this slice (YAGNI).

### Task 1: Clone, real GUID, version 1.2.0

**Files:**
- Create: `/home/nomadx/Documents/plex2jellyfin-plugin` (git clone)
- Modify: `Plex2JellyfinPlugin.cs:19-21` (GUID doc comment + `Guid.Parse`)
- Modify: `manifest.json` (guid field — full rewrite comes in Task 4; just the guid here)
- Modify: `Plex2Jellyfin.Plugin.csproj` (`<Version>`)

**Interfaces:**
- Produces: plugin assembly GUID `f4eda3a1-c062-49b3-a958-7cf9ca80c269`, version `1.2.0` — Tasks 3, 4, 7 depend on these exact values.

- [ ] **Step 1: Clone the repo**

```bash
git clone https://github.com/Nomadcxx/plex2jellyfin-plugin.git /home/nomadx/Documents/plex2jellyfin-plugin
cd /home/nomadx/Documents/plex2jellyfin-plugin
```

- [ ] **Step 2: Replace the placeholder GUID**

In `Plex2JellyfinPlugin.cs`, replace lines 17–21 with:

```csharp
    /// <summary>
    /// Unique identifier for this plugin.
    /// Guid: f4eda3a1-c062-49b3-a958-7cf9ca80c269
    /// </summary>
    public override Guid Id => Guid.Parse("f4eda3a1-c062-49b3-a958-7cf9ca80c269");
```

In `manifest.json`, change the `guid` value from `a1b2c3d4-e5f6-7890-abcd-ef1234567890` to `f4eda3a1-c062-49b3-a958-7cf9ca80c269`.

Confirm no other file carries the old GUID:

```bash
grep -rn 'a1b2c3d4' --include='*.cs' --include='*.json' --include='*.html' .
```

Expected: no output (fix any hit the same way).

- [ ] **Step 3: Bump the version to 1.2.0**

In `Plex2Jellyfin.Plugin.csproj`, set `<Version>1.2.0</Version>` (and `<AssemblyVersion>`/`<FileVersion>` if present to `1.2.0.0`). In `manifest.json`, set `"version": "1.2.0"`.

- [ ] **Step 4: Compile gate**

```bash
dotnet build -c Release
```

Expected: `Build succeeded. 0 Error(s)`.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat!: assign the permanent plugin GUID and start 1.2.0

The previous GUID was a placeholder. Changing it later would break
upgrade continuity for installed copies, so it becomes permanent now,
before the first repository-hosted release."
git push
```

### Task 2: Quiet-when-unconfigured event forwarding

**Files:**
- Modify: `EventHandlers/EventForwarder.cs:232-246` (`ForwardEvent` early-out)

**Interfaces:**
- Consumes: `PluginConfiguration.SharedSecret`, `PluginConfiguration.Plex2JellyfinUrl` (existing).
- Produces: an installed-but-unconfigured plugin sends no HTTP requests (Task 10's wizard relies on this being safe mid-abort).

- [ ] **Step 1: Add the early-out**

In `ForwardEvent`, directly after the existing `if (config == null) return;` line, insert:

```csharp
        if (string.IsNullOrWhiteSpace(config.SharedSecret) ||
            string.IsNullOrWhiteSpace(config.Plex2JellyfinUrl))
        {
            _logger.LogDebug(
                "Skipping {EventType}: plugin is not configured yet (missing daemon URL or shared secret)",
                eventType);
            return;
        }
```

- [ ] **Step 2: Compile gate**

```bash
dotnet build -c Release
```

Expected: `Build succeeded. 0 Error(s)`.

- [ ] **Step 3: Commit**

```bash
git add EventHandlers/EventForwarder.cs
git commit -m "fix: send nothing until a daemon URL and shared secret are configured

An installed-but-unconfigured plugin used to retry-spam the default
localhost:3000 with unauthenticated events."
git push
```

### Task 3: `POST /plex2jellyfin/test-webhook` endpoint

**Files:**
- Modify: `Api/Plex2JellyfinController.cs` (constructor + new action)

**Interfaces:**
- Consumes: daemon webhook contract — `POST {Plex2JellyfinUrl}/api/v1/webhooks/jellyfin` with headers `X-Plex2Jellyfin-Webhook-Secret`, `X-Jellyfin-Event` (same contract `ForwardEvent` uses).
- Produces: `POST /plex2jellyfin/test-webhook` (admin-authorized) returning PascalCase JSON `{ Sent, DaemonUrl, DaemonStatusCode, Authenticated }` or `{ Sent: false, DaemonUrl, Error }`; `412` when unconfigured. Task 7's `Engine.Verify` parses exactly this shape.

- [ ] **Step 1: Inject IHttpClientFactory into the controller**

Add `using System.Text;` and `using System.Text.Json;` to the controller's using block. Extend the constructor:

```csharp
    private readonly IHttpClientFactory _httpClientFactory;

    public Plex2JellyfinController(
        ILibraryManager libraryManager,
        ISessionManager sessionManager,
        IHttpClientFactory httpClientFactory,
        ILogger<Plex2JellyfinController> logger)
    {
        _libraryManager = libraryManager;
        _sessionManager = sessionManager;
        _httpClientFactory = httpClientFactory;
        _logger = logger;
    }
```

(Keep the existing field assignments; only `_httpClientFactory` is new. Jellyfin's DI provides `IHttpClientFactory` automatically.)

- [ ] **Step 2: Add the action**

Add after the `Health()` action:

```csharp
    /// <summary>
    /// Sends one signed synthetic event to the configured plex2jellyfin
    /// daemon so an installer can verify the reverse path end to end.
    /// </summary>
    [HttpPost("test-webhook")]
    [ProducesResponseType(StatusCodes.Status200OK)]
    [ProducesResponseType(StatusCodes.Status412PreconditionFailed)]
    public async Task<IActionResult> TestWebhook()
    {
        var config = Plex2JellyfinPlugin.Instance?.Configuration;
        if (config == null ||
            string.IsNullOrWhiteSpace(config.SharedSecret) ||
            string.IsNullOrWhiteSpace(config.Plex2JellyfinUrl))
        {
            return StatusCode(StatusCodes.Status412PreconditionFailed,
                new { Error = "Plugin is not configured (daemon URL or shared secret missing)" });
        }

        var url = $"{config.Plex2JellyfinUrl.TrimEnd('/')}/api/v1/webhooks/jellyfin";
        try
        {
            using var client = _httpClientFactory.CreateClient();
            client.Timeout = TimeSpan.FromSeconds(10);

            var json = JsonSerializer.Serialize(new
            {
                NotificationType = "TestNotification",
                Timestamp = DateTime.UtcNow.ToString("O")
            });
            var request = new HttpRequestMessage(HttpMethod.Post, url)
            {
                Content = new StringContent(json, Encoding.UTF8, "application/json")
            };
            request.Headers.Add("X-Plex2Jellyfin-Webhook-Secret", config.SharedSecret);
            request.Headers.Add("X-Jellyfin-Event", "TestNotification");

            var response = await client.SendAsync(request);
            return Ok(new
            {
                Sent = true,
                DaemonUrl = url,
                DaemonStatusCode = (int)response.StatusCode,
                Authenticated = response.IsSuccessStatusCode
            });
        }
        catch (Exception ex)
        {
            return Ok(new { Sent = false, DaemonUrl = url, Error = ex.Message });
        }
    }
```

- [ ] **Step 3: Compile gate**

```bash
dotnet build -c Release
```

Expected: `Build succeeded. 0 Error(s)`.

- [ ] **Step 4: Commit**

```bash
git add Api/Plex2JellyfinController.cs
git commit -m "feat: test-webhook endpoint for end-to-end verification

Fires one signed synthetic event at the configured daemon and reports
the status code it got back, so an installer can prove the reverse
path (URL reachability, secret match) without waiting for real events."
git push
```

### Task 4: Repo-level manifest + release workflow + v1.2.0 release

**Files:**
- Rewrite: `manifest.json` (single-object → Jellyfin repo array format)
- Create: `scripts/update-manifest.py`
- Create: `.github/workflows/release.yml`
- Modify: `build.sh` (stop embedding manifest.json in the zip)
- Modify: `README.md` (repository install instructions)

**Interfaces:**
- Consumes: version and GUID from Task 1.
- Produces: `https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json` serving a Jellyfin repository manifest whose `versions[0].sourceUrl` is a GitHub Release zip with a matching md5 `checksum`. Tasks 7 and 12 depend on this URL being live.

- [ ] **Step 1: Simplify build.sh**

Delete from `build.sh`: the `cp -f "$PROJECT_DIR/manifest.json" "$BUILD_DIR/manifest.json"` line, the `"manifest.json"` entry in `required_files`, the entire md5/python checksum-injection block (lines 45–57), and `"manifest.json"` from the `zip -r` argument list. The zip now contains only `Plex2Jellyfin.Plugin.dll` and `Plex2Jellyfin.Plugin.pdb` — Jellyfin writes its own `meta.json` on repository installs, and the repo-level manifest (next step) owns the checksum.

- [ ] **Step 2: Rewrite manifest.json in repository format**

Replace the entire file with:

```json
[
  {
    "guid": "f4eda3a1-c062-49b3-a958-7cf9ca80c269",
    "name": "Plex2Jellyfin",
    "description": "Companion plugin for the Plex2Jellyfin media organizer - event forwarding and custom endpoints",
    "overview": "Forwards item and playback events to plex2jellyfin and exposes helper endpoints",
    "owner": "Nomadcxx",
    "category": "General",
    "imageUrl": "",
    "versions": []
  }
]
```

(`versions` stays empty here; the release workflow fills it.)

- [ ] **Step 3: Create scripts/update-manifest.py**

```python
#!/usr/bin/env python3
"""Prepend a release entry to the repo-level manifest.json.

Usage: update-manifest.py <version> <zip-path> <download-url> <changelog>
Jellyfin verifies the md5 of the downloaded zip against `checksum`.
"""
import hashlib
import json
import sys
from datetime import datetime, timezone

GUID = "f4eda3a1-c062-49b3-a958-7cf9ca80c269"


def main() -> None:
    version, zip_path, url, changelog = sys.argv[1:5]
    if version.count(".") == 2:
        version += ".0"

    with open(zip_path, "rb") as f:
        checksum = hashlib.md5(f.read()).hexdigest()

    with open("manifest.json") as f:
        manifest = json.load(f)

    entry = next(pkg for pkg in manifest if pkg["guid"] == GUID)
    versions = [v for v in entry.get("versions", []) if v.get("version") != version]
    versions.insert(0, {
        "version": version,
        "changelog": changelog,
        "targetAbi": "10.11.0.0",
        "sourceUrl": url,
        "checksum": checksum,
        "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    })
    entry["versions"] = versions

    with open("manifest.json", "w") as f:
        json.dump(manifest, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
```

```bash
chmod +x scripts/update-manifest.py
```

- [ ] **Step 4: Create .github/workflows/release.yml**

```yaml
name: release

on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: main
          fetch-depth: 0

      - uses: actions/setup-dotnet@v4
        with:
          dotnet-version: '9.0.x'

      - name: Build at the tagged commit
        run: |
          git checkout "${GITHUB_REF_NAME}"
          ./build.sh

      - name: Create GitHub release with the zip
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          gh release create "${GITHUB_REF_NAME}" \
            "artifacts/plex2jellyfin_${VERSION}.zip" \
            --title "${GITHUB_REF_NAME}" \
            --notes "Install through a Jellyfin plugin repository - see README."

      - name: Update the repo manifest on main
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          URL="https://github.com/Nomadcxx/plex2jellyfin-plugin/releases/download/${GITHUB_REF_NAME}/plex2jellyfin_${VERSION}.zip"
          CHANGELOG="$(git tag -l --format='%(contents:subject)' "${GITHUB_REF_NAME}")"
          git checkout main
          python3 scripts/update-manifest.py "${VERSION}" \
            "artifacts/plex2jellyfin_${VERSION}.zip" "${URL}" \
            "${CHANGELOG:-Release ${VERSION}}"
          git config user.name "Nomadcxx"
          git config user.email "noovie@gmail.com"
          git add manifest.json
          git commit -m "release: manifest entry for ${GITHUB_REF_NAME}"
          git push origin main
```

(The commit identity is deliberately Nomadcxx, not a workflow identity — see Global Constraints.)

- [ ] **Step 5: Update README install section**

In `README.md`, replace the build-and-copy install instructions with a "Install from the plugin repository" section as the primary path:

```markdown
## Install

### From the plugin repository (recommended)

1. Jellyfin Dashboard → Plugins → Repositories → **+** and add:
   `https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json`
2. Catalog → **Plex2Jellyfin** → Install, then restart Jellyfin.

The plex2jellyfin setup wizard can do all of this for you (including
configuration) - run `plex2jellyfin setup` or use the web UI.

### Manual build (development)
```

Keep the existing `./build.sh` / `./install.sh` instructions under the manual heading.

- [ ] **Step 6: Verify build.sh still produces the zip**

```bash
./build.sh && unzip -l artifacts/plex2jellyfin_1.2.0.zip
```

Expected: zip listing shows exactly `Plex2Jellyfin.Plugin.dll` and `Plex2Jellyfin.Plugin.pdb`.

- [ ] **Step 7: Sanity-check the manifest script locally**

```bash
python3 scripts/update-manifest.py 1.2.0 artifacts/plex2jellyfin_1.2.0.zip https://example.invalid/test.zip "local test" \
  && python3 -m json.tool manifest.json \
  && git checkout manifest.json
```

Expected: valid JSON with one `versions` entry, then restored to the committed empty-versions state.

- [ ] **Step 8: Commit and tag the release**

```bash
git add -A
git commit -m "ci: release workflow publishing the zip and repo manifest

Tag push builds the plugin, attaches the zip to a GitHub release, and
stamps manifest.json on main with the zip md5 so Jellyfin servers can
install and update from the raw manifest URL."
git push
git tag -a v1.2.0 -m "Repository-hosted install: permanent GUID, test-webhook endpoint, quiet-when-unconfigured forwarding"
git push origin v1.2.0
```

- [ ] **Step 9: Verify the release pipeline end to end**

```bash
gh run watch --repo Nomadcxx/plex2jellyfin-plugin --exit-status $(gh run list --repo Nomadcxx/plex2jellyfin-plugin --workflow release --limit 1 --json databaseId --jq '.[0].databaseId')
curl -s https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json | python3 -m json.tool | head -30
```

Expected: workflow succeeds; manifest shows `versions[0].version == "1.2.0.0"` with a non-empty `checksum` and a `sourceUrl` under `releases/download/v1.2.0/`. Download the sourceUrl and confirm `md5sum` matches `checksum`.

---

## Phase B — main repo (`/home/nomadx/Documents/plex2jellyfin`)

All commands run from the repo root. Test cycle per task: write failing test → run (expect FAIL) → implement → run (expect PASS) → commit.

### Task 5: Config field `plugin_daemon_url`

**Files:**
- Modify: `internal/config/config.go` (JellyfinConfig struct, ~line 267, and the `ToTOML` jellyfin section)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.JellyfinConfig.PluginDaemonURL string` (`mapstructure:"plugin_daemon_url"`) — the base URL Jellyfin's plugin uses to reach plex2jellyfin (e.g. `http://192.168.1.20:5522`; the plugin appends `/api/v1/webhooks/jellyfin` itself). Tasks 9 and 10 read/write it.

- [ ] **Step 1: Write the failing round-trip test**

Add to `internal/config/config_test.go` (match the file's existing test style for Save/Load round-trips — there is an existing regression test for top-level `password_hash` placement to crib from):

```go
func TestPluginDaemonURLSurvivesRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")

	cfg := DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.PluginDaemonURL = "http://192.168.1.20:5522"
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Jellyfin.PluginDaemonURL != "http://192.168.1.20:5522" {
		t.Fatalf("plugin_daemon_url lost on reload: %q", loaded.Jellyfin.PluginDaemonURL)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/config/ -run TestPluginDaemonURLSurvivesRoundTrip -v
```

Expected: FAIL (compile error: unknown field `PluginDaemonURL`).

- [ ] **Step 3: Add the field and serialization**

In `JellyfinConfig` after `PluginVerifyInterval`:

```go
	// PluginDaemonURL is the base URL Jellyfin's companion plugin calls
	// back to (the plugin appends /api/v1/webhooks/jellyfin). From
	// Jellyfin's point of view, so never localhost when either side is
	// containerized.
	PluginDaemonURL string `mapstructure:"plugin_daemon_url"`
```

Then find the `[jellyfin]` section in `ToTOML()` and add a `plugin_daemon_url = "…"` line alongside the other plugin_* keys, following the exact quoting/formatting pattern of the neighboring lines. **This file writes TOML by hand — a field without a ToTOML line silently vanishes on the next Save (this exact class of bug previously ate `password_hash`).**

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/config/ -v -run TestPluginDaemonURLSurvivesRoundTrip && go test ./internal/config/
```

Expected: PASS, and the full config package stays green.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): jellyfin plugin_daemon_url for the callback direction"
```

### Task 6: Daemon logs the plugin's test event

**Files:**
- Modify: `internal/jellyfin/webhook_types.go:42` area (event constants)
- Modify: `internal/api/webhooks.go:31-48` (switch)
- Test: `internal/api/webhooks_test.go`

**Interfaces:**
- Consumes: plugin sends `NotificationType: "TestNotification"` (Task 3).
- Produces: activity entry `jellyfin_test_event`, so the verify round-trip is visible in the Activity page. (The 200 response already happens today via the default case — this task is observability only.)

- [ ] **Step 1: Write the failing test**

Add to `internal/api/webhooks_test.go`, reusing the file's existing webhook test harness (server construction with a configured `WebhookSecret` and a capturing activity logger — mirror whichever existing test posts a valid-secret event and asserts on logged activity):

```go
func TestWebhookTestNotificationIsLoggedAndAccepted(t *testing.T) {
	srv, activity := newWebhookTestServer(t) // use the file's existing helper/fixture pattern

	body := `{"NotificationType":"TestNotification","Timestamp":"2026-07-11T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", strings.NewReader(body))
	req.Header.Set("X-Plex2Jellyfin-Webhook-Secret", testWebhookSecret)
	rec := httptest.NewRecorder()
	srv.HandleJellyfinWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !activity.hasAction("jellyfin_test_event") {
		t.Fatal("expected a jellyfin_test_event activity entry")
	}
}
```

(Adapt helper names to what `webhooks_test.go` actually provides; the assertions — 200 status and a `jellyfin_test_event` entry — are the contract.)

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/api/ -run TestWebhookTestNotificationIsLoggedAndAccepted -v
```

Expected: FAIL on the missing activity entry (the 200 already passes).

- [ ] **Step 3: Implement**

In `internal/jellyfin/webhook_types.go`, add to the event constants block:

```go
	EventTestNotification = "TestNotification"
```

In `internal/api/webhooks.go`, add a case before `default`:

```go
	case jellyfin.EventTestNotification:
		s.logJellyfinActivity("jellyfin_test_event", "companion plugin", "", true, "")
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/api/ ./internal/jellyfin/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/ internal/jellyfin/
git commit -m "feat(webhooks): log the plugin's synthetic test event as activity"
```

### Task 7: Install engine `internal/jellyfin/plugininstall`

**Files:**
- Create: `internal/jellyfin/plugininstall/plugininstall.go`
- Test: `internal/jellyfin/plugininstall/plugininstall_test.go`

**Interfaces:**
- Consumes: Jellyfin admin API (verified contract: `GET /System/Info`, `GET|POST /Repositories` where POST replaces the whole list, `GET /Plugins`, `POST /Packages/Installed/{name}?assemblyGuid=`, `POST /System/Restart`, `POST /Plugins/{guid}/Configuration`), plus the plugin's `GET /plex2jellyfin/health` and `POST /plex2jellyfin/test-webhook` (Task 3 shape).
- Produces (Tasks 9/10 call these):
  - `plugininstall.New(baseURL, apiKey string, client *http.Client) *Engine`
  - `(*Engine) Inspect(ctx) (*Inspection, error)` — `Inspection{ServerVersion string; ABISupported, RepoRegistered bool; InstalledVersion string; PluginResponding bool}`
  - `(*Engine) RegisterRepo(ctx) (added bool, err error)`
  - `(*Engine) Install(ctx) error`
  - `(*Engine) Restart(ctx) error`
  - `(*Engine) WaitReady(ctx, timeout time.Duration) error`
  - `(*Engine) Configure(ctx, daemonBaseURL, sharedSecret string) error`
  - `(*Engine) Verify(ctx) (*VerifyResult, error)` — `VerifyResult{Sent bool; DaemonURL string; DaemonStatusCode int; Authenticated bool; Error string}`
  - Constants `PluginGUID`, `PluginName`, `RepoName`, `ManifestURL`

- [ ] **Step 1: Write the failing tests**

Create `plugininstall_test.go` with a scripted fake Jellyfin:

```go
package plugininstall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeJellyfin serves just enough of the admin API for the engine.
type fakeJellyfin struct {
	version       string
	repos         []map[string]any
	plugins       []map[string]any
	pluginHealthy bool

	repoPosts    [][]map[string]any
	installCalls []string
	restartCalls int
	configPosts  []map[string]any
}

func (f *fakeJellyfin) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /System/Info", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"Version": f.version})
	})
	mux.HandleFunc("GET /Repositories", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(f.repos)
	})
	mux.HandleFunc("POST /Repositories", func(w http.ResponseWriter, r *http.Request) {
		var posted []map[string]any
		json.NewDecoder(r.Body).Decode(&posted)
		f.repoPosts = append(f.repoPosts, posted)
		f.repos = posted
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /Plugins", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(f.plugins)
	})
	mux.HandleFunc("POST /Packages/Installed/", func(w http.ResponseWriter, r *http.Request) {
		f.installCalls = append(f.installCalls, r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /System/Restart", func(w http.ResponseWriter, r *http.Request) {
		f.restartCalls++
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /Plugins/", func(w http.ResponseWriter, r *http.Request) {
		var cfg map[string]any
		json.NewDecoder(r.Body).Decode(&cfg)
		f.configPosts = append(f.configPosts, cfg)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /plex2jellyfin/health", func(w http.ResponseWriter, r *http.Request) {
		if !f.pluginHealthy {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"Status": "healthy"})
	})
	mux.HandleFunc("POST /plex2jellyfin/test-webhook", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Sent": true, "DaemonUrl": "http://10.0.0.5:5522/api/v1/webhooks/jellyfin",
			"DaemonStatusCode": 200, "Authenticated": true,
		})
	})
	return mux
}

func newTestEngine(t *testing.T, f *fakeJellyfin) *Engine {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	e := New(srv.URL, "test-key", srv.Client())
	e.pollInterval = 10 * time.Millisecond
	return e
}

func TestInspectReportsServerAndPluginState(t *testing.T) {
	f := &fakeJellyfin{
		version: "10.11.6",
		repos:   []map[string]any{{"Name": "Existing", "Url": "https://example.org/m.json", "Enabled": true}},
		plugins: []map[string]any{{"Id": strings.ReplaceAll(PluginGUID, "-", ""), "Name": "Plex2Jellyfin", "Version": "1.2.0.0"}},
		pluginHealthy: true,
	}
	insp, err := newTestEngine(t, f).Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !insp.ABISupported || insp.ServerVersion != "10.11.6" {
		t.Errorf("ABI gate wrong: %+v", insp)
	}
	if insp.RepoRegistered {
		t.Error("repo should not be registered yet")
	}
	if insp.InstalledVersion != "1.2.0.0" || !insp.PluginResponding {
		t.Errorf("plugin detection wrong: %+v", insp)
	}
}

func TestInspectGatesOldServers(t *testing.T) {
	f := &fakeJellyfin{version: "10.10.7"}
	insp, err := newTestEngine(t, f).Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if insp.ABISupported {
		t.Error("10.10.x must not pass the ABI gate")
	}
}

func TestRegisterRepoPreservesExistingRepositories(t *testing.T) {
	f := &fakeJellyfin{
		version: "10.11.6",
		repos:   []map[string]any{{"Name": "Existing", "Url": "https://example.org/m.json", "Enabled": true}},
	}
	added, err := newTestEngine(t, f).RegisterRepo(context.Background())
	if err != nil || !added {
		t.Fatalf("added=%v err=%v", added, err)
	}
	if len(f.repoPosts) != 1 || len(f.repoPosts[0]) != 2 {
		t.Fatalf("expected one POST with 2 repos, got %+v", f.repoPosts)
	}
	if f.repoPosts[0][0]["Url"] != "https://example.org/m.json" {
		t.Error("existing repository was clobbered")
	}
	if f.repoPosts[0][1]["Url"] != ManifestURL {
		t.Error("our manifest URL missing from the posted list")
	}
}

func TestRegisterRepoIsIdempotent(t *testing.T) {
	f := &fakeJellyfin{
		version: "10.11.6",
		repos:   []map[string]any{{"Name": RepoName, "Url": ManifestURL, "Enabled": true}},
	}
	added, err := newTestEngine(t, f).RegisterRepo(context.Background())
	if err != nil || added {
		t.Fatalf("added=%v err=%v; want no-op", added, err)
	}
	if len(f.repoPosts) != 0 {
		t.Error("repository list must not be re-posted when already registered")
	}
}

func TestInstallTargetsThePackageByGUID(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6"}
	if err := newTestEngine(t, f).Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.installCalls) != 1 ||
		!strings.Contains(f.installCalls[0], "/Packages/Installed/Plex2Jellyfin") ||
		!strings.Contains(f.installCalls[0], "assemblyGuid="+PluginGUID) {
		t.Fatalf("install call wrong: %v", f.installCalls)
	}
}

func TestConfigurePushesSecretAndDaemonURL(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6"}
	err := newTestEngine(t, f).Configure(context.Background(), "http://10.0.0.5:5522/", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.configPosts) != 1 {
		t.Fatalf("config posts: %+v", f.configPosts)
	}
	got := f.configPosts[0]
	if got["Plex2JellyfinUrl"] != "http://10.0.0.5:5522" {
		t.Errorf("daemon URL not normalized: %v", got["Plex2JellyfinUrl"])
	}
	if got["SharedSecret"] != "s3cret" || got["EnableEventForwarding"] != true {
		t.Errorf("config body wrong: %+v", got)
	}
}

func TestWaitReadySucceedsOnceHealthy(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6", pluginHealthy: true}
	if err := newTestEngine(t, f).WaitReady(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestWaitReadyTimesOut(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6", pluginHealthy: false}
	err := newTestEngine(t, f).WaitReady(context.Background(), 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestVerifyParsesThePluginResponse(t *testing.T) {
	f := &fakeJellyfin{version: "10.11.6", pluginHealthy: true}
	res, err := newTestEngine(t, f).Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Sent || !res.Authenticated || res.DaemonStatusCode != 200 {
		t.Errorf("verify result wrong: %+v", res)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/jellyfin/plugininstall/ -v 2>&1 | head -5
```

Expected: FAIL (package does not exist / does not compile).

- [ ] **Step 3: Implement plugininstall.go**

```go
// Package plugininstall drives installation, configuration, and
// verification of the companion Jellyfin plugin through Jellyfin's own
// package API, so it works identically for native and containerized
// Jellyfin servers.
package plugininstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	PluginGUID  = "f4eda3a1-c062-49b3-a958-7cf9ca80c269"
	PluginName  = "Plex2Jellyfin"
	RepoName    = "Plex2Jellyfin (official)"
	ManifestURL = "https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin-plugin/main/manifest.json"

	// The plugin targets ABI 10.11.0.0; older servers cannot load it and
	// newer major ABIs are unknown until tested.
	requiredVersionPrefix = "10.11."
)

// Engine drives plugin installation against one Jellyfin server.
type Engine struct {
	baseURL      string
	apiKey       string
	http         *http.Client
	pollInterval time.Duration
}

func New(baseURL, apiKey string, client *http.Client) *Engine {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Engine{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		http:         client,
		pollInterval: 2 * time.Second,
	}
}

// Inspection is the read-only state the wizard renders before acting.
type Inspection struct {
	ServerVersion    string
	ABISupported     bool
	RepoRegistered   bool
	InstalledVersion string // empty when not installed
	PluginResponding bool   // /plex2jellyfin/health answered 200
}

// VerifyResult mirrors the plugin's test-webhook response.
type VerifyResult struct {
	Sent             bool   `json:"Sent"`
	DaemonURL        string `json:"DaemonUrl"`
	DaemonStatusCode int    `json:"DaemonStatusCode"`
	Authenticated    bool   `json:"Authenticated"`
	Error            string `json:"Error"`
}

type repositoryInfo struct {
	Name    string `json:"Name"`
	URL     string `json:"Url"`
	Enabled bool   `json:"Enabled"`
}

func (e *Engine) do(ctx context.Context, method, path string, body, result any) (int, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.baseURL+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Emby-Token", e.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("%s %s: %s", method, path, resp.Status)
	}
	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

func guidsEqual(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, "-", "")) }
	return norm(a) == norm(b)
}

func (e *Engine) Inspect(ctx context.Context) (*Inspection, error) {
	var info struct {
		Version string `json:"Version"`
	}
	if _, err := e.do(ctx, http.MethodGet, "/System/Info", nil, &info); err != nil {
		return nil, fmt.Errorf("jellyfin system info: %w", err)
	}
	insp := &Inspection{
		ServerVersion: info.Version,
		ABISupported:  strings.HasPrefix(info.Version, requiredVersionPrefix),
	}

	var repos []repositoryInfo
	if _, err := e.do(ctx, http.MethodGet, "/Repositories", nil, &repos); err != nil {
		return nil, fmt.Errorf("list plugin repositories: %w", err)
	}
	for _, r := range repos {
		if r.URL == ManifestURL {
			insp.RepoRegistered = true
		}
	}

	var plugins []struct {
		ID      string `json:"Id"`
		Version string `json:"Version"`
	}
	if _, err := e.do(ctx, http.MethodGet, "/Plugins", nil, &plugins); err != nil {
		return nil, fmt.Errorf("list installed plugins: %w", err)
	}
	for _, p := range plugins {
		if guidsEqual(p.ID, PluginGUID) {
			insp.InstalledVersion = p.Version
		}
	}

	status, err := e.do(ctx, http.MethodGet, "/plex2jellyfin/health", nil, nil)
	insp.PluginResponding = err == nil && status == http.StatusOK
	return insp, nil
}

// RegisterRepo adds our manifest URL to the server's repository list.
// POST /Repositories replaces the entire list, so the existing entries
// are always carried over.
func (e *Engine) RegisterRepo(ctx context.Context) (bool, error) {
	var repos []repositoryInfo
	if _, err := e.do(ctx, http.MethodGet, "/Repositories", nil, &repos); err != nil {
		return false, err
	}
	for _, r := range repos {
		if r.URL == ManifestURL {
			return false, nil
		}
	}
	repos = append(repos, repositoryInfo{Name: RepoName, URL: ManifestURL, Enabled: true})
	if _, err := e.do(ctx, http.MethodPost, "/Repositories", repos, nil); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Engine) Install(ctx context.Context) error {
	path := "/Packages/Installed/" + PluginName + "?assemblyGuid=" + PluginGUID
	_, err := e.do(ctx, http.MethodPost, path, nil, nil)
	return err
}

func (e *Engine) Restart(ctx context.Context) error {
	_, err := e.do(ctx, http.MethodPost, "/System/Restart", nil, nil)
	return err
}

// WaitReady polls the plugin's anonymous health endpoint until it
// answers, which proves the server is back AND the plugin loaded.
func (e *Engine) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()
	for {
		if status, err := e.do(deadline, http.MethodGet, "/plex2jellyfin/health", nil, nil); err == nil && status == http.StatusOK {
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("plugin did not respond within %s", timeout)
		case <-ticker.C:
		}
	}
}

func (e *Engine) Configure(ctx context.Context, daemonBaseURL, sharedSecret string) error {
	cfg := map[string]any{
		"Plex2JellyfinUrl":      strings.TrimRight(daemonBaseURL, "/"),
		"SharedSecret":          sharedSecret,
		"EnableEventForwarding": true,
		"EnableCustomEndpoints": true,
		"ForwardLibraryEvents":  true,
		"ForwardPlaybackEvents": true,
		"RequestTimeoutSeconds": 30,
		"RetryCount":            3,
	}
	_, err := e.do(ctx, http.MethodPost, "/Plugins/"+PluginGUID+"/Configuration", cfg, nil)
	return err
}

func (e *Engine) Verify(ctx context.Context) (*VerifyResult, error) {
	var result VerifyResult
	if _, err := e.do(ctx, http.MethodPost, "/plex2jellyfin/test-webhook", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
go test ./internal/jellyfin/plugininstall/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jellyfin/plugininstall/
git commit -m "feat(jellyfin): plugin install engine over the package API

Six idempotent stages - inspect, register repo, install, restart+wait,
configure, verify - against Jellyfin's admin API, so the companion
plugin installs identically for native and containerized servers. The
repository list is read-modify-written because POST /Repositories
replaces the entire list."
```

### Task 8: `DetectAdvertiseIP` helper

**Files:**
- Create: `internal/setup/network.go`
- Test: `internal/setup/network_test.go`

**Interfaces:**
- Produces: `setupdomain.DetectAdvertiseIP() string` — primary outbound IPv4 or `""`. Tasks 9/10 use it as the webhook-URL default.

- [ ] **Step 1: Write the failing test**

```go
package setup

import (
	"net"
	"testing"
)

func TestDetectAdvertiseIPReturnsEmptyOrRoutableIP(t *testing.T) {
	ip := DetectAdvertiseIP()
	if ip == "" {
		t.Skip("no outbound route in this environment")
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		t.Fatalf("not an IP: %q", ip)
	}
	if parsed.IsLoopback() {
		t.Fatalf("loopback is never a useful advertise address: %q", ip)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./internal/setup/ -run TestDetectAdvertiseIP -v
```

Expected: FAIL (undefined: DetectAdvertiseIP).

- [ ] **Step 3: Implement network.go**

```go
package setup

import "net"

// DetectAdvertiseIP returns the host's primary outbound IPv4 address,
// or "" when it cannot be determined. Dialing UDP sends no packets -
// it only asks the kernel which source address routes toward a public
// destination (192.0.2.1 is TEST-NET-1, never actually contacted).
func DetectAdvertiseIP() string {
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil || addr.IP.IsLoopback() || addr.IP.IsUnspecified() {
		return ""
	}
	return addr.IP.String()
}
```

- [ ] **Step 4: Run to verify it passes**

```bash
go test ./internal/setup/ -v -run TestDetectAdvertiseIP
```

Expected: PASS (or SKIP on a host with no route — both acceptable).

- [ ] **Step 5: Commit**

```bash
git add internal/setup/network.go internal/setup/network_test.go
git commit -m "feat(setup): detect the host's primary outbound IP for URL defaults"
```

### Task 9: `plex2jellyfin plugin` command

**Files:**
- Create: `cmd/plex2jellyfin/plugin_cmd.go`
- Test: `cmd/plex2jellyfin/plugin_cmd_test.go`
- Modify: `cmd/plex2jellyfin/main.go` (register `newPluginCmd()` in `newRootCmd`, next to the `newSetupCmd()` registration)
- Modify: `cmd/plex2jellyfin/root_cmd_test.go:35` (visible-commands list)

**Interfaces:**
- Consumes: `plugininstall` engine (Task 7 signatures), `config.JellyfinConfig.PluginDaemonURL` (Task 5), `setupdomain.DetectAdvertiseIP` (Task 8), the `prompter` type from `setup_cmd.go` (same package).
- Produces: `newPluginCmd() *cobra.Command` with `install`, `verify`, `status` subcommands; `pluginEngine` interface + `pluginDeps` struct that Task 10 reuses for the wizard.

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
)

// fakeEngine scripts the engine for command tests.
type fakeEngine struct {
	inspection   plugininstall.Inspection
	inspectErr   error
	repoAdded    bool
	installed    bool
	restarted    bool
	waited       bool
	configured   []string // daemonURL, secret
	verifyResult plugininstall.VerifyResult
	verifyErr    error
}

func (f *fakeEngine) Inspect(ctx context.Context) (*plugininstall.Inspection, error) {
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	insp := f.inspection
	return &insp, nil
}
func (f *fakeEngine) RegisterRepo(ctx context.Context) (bool, error) { f.repoAdded = true; return true, nil }
func (f *fakeEngine) Install(ctx context.Context) error              { f.installed = true; return nil }
func (f *fakeEngine) Restart(ctx context.Context) error              { f.restarted = true; return nil }
func (f *fakeEngine) WaitReady(ctx context.Context, timeout time.Duration) error {
	f.waited = true
	return nil
}
func (f *fakeEngine) Configure(ctx context.Context, daemonURL, secret string) error {
	f.configured = []string{daemonURL, secret}
	return nil
}
func (f *fakeEngine) Verify(ctx context.Context) (*plugininstall.VerifyResult, error) {
	if f.verifyErr != nil {
		return nil, f.verifyErr
	}
	res := f.verifyResult
	return &res, nil
}

func pluginTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.URL = "http://jellyfin.local:8096"
	cfg.Jellyfin.APIKey = "key"
	cfg.Jellyfin.WebhookSecret = "s3cret"
	cfg.Jellyfin.PluginDaemonURL = "http://10.0.0.5:5522"
	return cfg
}

func pluginTestDeps(engine *fakeEngine, cfg *config.Config) pluginDeps {
	return pluginDeps{
		loadConfig: func() (*config.Config, error) { return cfg, nil },
		saveConfig: func(c *config.Config) error { return nil },
		newEngine:  func(baseURL, apiKey string) pluginEngine { return engine },
		advertiseIP: func() string { return "10.0.0.5" },
	}
}

func TestPluginInstallHappyPathWithRestartConsent(t *testing.T) {
	engine := &fakeEngine{
		inspection:   plugininstall.Inspection{ServerVersion: "10.11.6", ABISupported: true},
		verifyResult: plugininstall.VerifyResult{Sent: true, DaemonStatusCode: 200, Authenticated: true},
	}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, pluginTestConfig()),
		strings.NewReader("y\n"), &out) // single prompt: restart consent
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	if !engine.repoAdded || !engine.installed || !engine.restarted || !engine.waited {
		t.Errorf("stage skipped: %+v", engine)
	}
	if len(engine.configured) != 2 || engine.configured[0] != "http://10.0.0.5:5522" || engine.configured[1] != "s3cret" {
		t.Errorf("configure args: %v", engine.configured)
	}
	if !strings.Contains(out.String(), "verified") {
		t.Errorf("expected verified line:\n%s", out.String())
	}
}

func TestPluginInstallDecliningRestartPrintsRecovery(t *testing.T) {
	engine := &fakeEngine{
		inspection: plugininstall.Inspection{ServerVersion: "10.11.6", ABISupported: true},
	}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, pluginTestConfig()),
		strings.NewReader("n\n"), &out)
	if err != nil {
		t.Fatalf("declining restart must not error: %v", err)
	}
	if engine.restarted {
		t.Error("restarted without consent")
	}
	if !strings.Contains(out.String(), "plex2jellyfin plugin verify") {
		t.Errorf("expected recovery pointer:\n%s", out.String())
	}
}

func TestPluginInstallRefusesUnsupportedABI(t *testing.T) {
	engine := &fakeEngine{
		inspection: plugininstall.Inspection{ServerVersion: "10.10.7", ABISupported: false},
	}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, pluginTestConfig()),
		strings.NewReader(""), &out)
	if err == nil {
		t.Fatal("expected ABI gate error")
	}
	if engine.installed {
		t.Error("must not install on unsupported servers")
	}
}

func TestPluginVerifyFailsOnUnauthenticated(t *testing.T) {
	engine := &fakeEngine{
		inspection:   plugininstall.Inspection{ServerVersion: "10.11.6", ABISupported: true, PluginResponding: true},
		verifyResult: plugininstall.VerifyResult{Sent: true, DaemonStatusCode: 401, Authenticated: false},
	}
	var out bytes.Buffer
	err := runPluginVerify(context.Background(), pluginTestDeps(engine, pluginTestConfig()), &out)
	if err == nil {
		t.Fatal("verify must fail when the daemon rejects the secret")
	}
}

func TestPluginInstallRequiresJellyfinConfig(t *testing.T) {
	cfg := config.DefaultConfig() // jellyfin disabled
	engine := &fakeEngine{}
	var out bytes.Buffer
	err := runPluginInstall(context.Background(), pluginTestDeps(engine, cfg), strings.NewReader(""), &out)
	if err == nil || !strings.Contains(err.Error(), "setup") {
		t.Fatalf("expected pointer to setup, got %v", err)
	}
	_ = errors.Is // keep errors import if unused otherwise
}
```

Also update `root_cmd_test.go:35` `want` list to:

```go
	want := []string{"config", "consolidate", "duplicates", "plugin", "scan", "setup", "status", "trace", "version"}
```

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./cmd/plex2jellyfin/ -run 'TestPlugin|TestNewRootCmdShowsCoreManualCommandsOnly' 2>&1 | head -5
```

Expected: FAIL (undefined types/functions; root command list mismatch).

- [ ] **Step 3: Implement plugin_cmd.go**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"bufio"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
	"github.com/spf13/cobra"
)

// pluginEngine is the slice of plugininstall.Engine the commands use,
// injectable for tests.
type pluginEngine interface {
	Inspect(ctx context.Context) (*plugininstall.Inspection, error)
	RegisterRepo(ctx context.Context) (bool, error)
	Install(ctx context.Context) error
	Restart(ctx context.Context) error
	WaitReady(ctx context.Context, timeout time.Duration) error
	Configure(ctx context.Context, daemonBaseURL, sharedSecret string) error
	Verify(ctx context.Context) (*plugininstall.VerifyResult, error)
}

type pluginDeps struct {
	loadConfig  func() (*config.Config, error)
	saveConfig  func(*config.Config) error
	newEngine   func(baseURL, apiKey string) pluginEngine
	advertiseIP func() string
}

func defaultPluginDeps() pluginDeps {
	return pluginDeps{
		loadConfig: config.Load,
		saveConfig: func(c *config.Config) error { return c.Save() },
		newEngine: func(baseURL, apiKey string) pluginEngine {
			return plugininstall.New(baseURL, apiKey, &http.Client{Timeout: 15 * time.Second})
		},
		advertiseIP: setupdomain.DetectAdvertiseIP,
	}
}

const restartWaitTimeout = 60 * time.Second

func newPluginCmd() *cobra.Command {
	deps := defaultPluginDeps()
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Install and verify the companion Jellyfin plugin",
		Long: `Manages the companion Jellyfin plugin through Jellyfin's own plugin
system: registers the plugin repository, installs the package, restarts
Jellyfin (with consent), pushes the webhook configuration, and verifies
the feedback loop with a signed test event.`,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install, configure, and verify the plugin",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginInstall(cmd.Context(), deps, os.Stdin, os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Send a signed test event through Jellyfin and confirm receipt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginVerify(cmd.Context(), deps, os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show plugin installation state on the configured Jellyfin",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPluginStatus(cmd.Context(), deps, os.Stdout)
		},
	})
	return cmd
}

func pluginEngineFromConfig(deps pluginDeps) (*config.Config, pluginEngine, error) {
	cfg, err := deps.loadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	if !cfg.Jellyfin.Enabled || cfg.Jellyfin.URL == "" || cfg.Jellyfin.APIKey == "" {
		return nil, nil, errors.New("Jellyfin is not configured - run 'plex2jellyfin setup' first")
	}
	return cfg, deps.newEngine(cfg.Jellyfin.URL, cfg.Jellyfin.APIKey), nil
}

func runPluginInstall(ctx context.Context, deps pluginDeps, stdin io.Reader, out io.Writer) error {
	cfg, engine, err := pluginEngineFromConfig(deps)
	if err != nil {
		return err
	}
	p := &prompter{in: bufio.NewScanner(stdin), out: out}

	insp, err := engine.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Jellyfin: %w", err)
	}
	if !insp.ABISupported {
		return fmt.Errorf("Jellyfin %s is not supported: the plugin needs 10.11.x", insp.ServerVersion)
	}

	if insp.InstalledVersion != "" && insp.PluginResponding {
		fmt.Fprintf(out, "Plugin %s is already installed and responding.\n", insp.InstalledVersion)
	} else {
		if added, err := engine.RegisterRepo(ctx); err != nil {
			return fmt.Errorf("register plugin repository: %w", err)
		} else if added {
			fmt.Fprintln(out, "  plugin repository registered (existing repositories kept)")
		} else {
			fmt.Fprintln(out, "  plugin repository already registered")
		}
		if err := engine.Install(ctx); err != nil {
			return fmt.Errorf("install plugin package: %w", err)
		}
		fmt.Fprintln(out, "  Jellyfin downloaded the plugin package")

		restart, err := p.askBool("Restart Jellyfin now to load it?", false)
		if err != nil {
			return err
		}
		if !restart {
			fmt.Fprintln(out, "Restart Jellyfin when convenient, then run: plex2jellyfin plugin verify")
			return nil
		}
		if err := engine.Restart(ctx); err != nil {
			return fmt.Errorf("restart Jellyfin: %w", err)
		}
		fmt.Fprintln(out, "  restart requested, waiting for the plugin to come back…")
		if err := engine.WaitReady(ctx, restartWaitTimeout); err != nil {
			fmt.Fprintf(out, "Jellyfin did not come back in time (%v).\n", err)
			fmt.Fprintln(out, "Once it is up again, run: plex2jellyfin plugin verify")
			return nil
		}
		fmt.Fprintln(out, "  plugin loaded")
	}

	return configureAndVerify(ctx, deps, cfg, engine, p, out)
}

// configureAndVerify pushes the webhook settings into the plugin and
// proves the reverse path with a signed test event. Shared by the
// standalone command and the setup wizard's feedback-loop step.
func configureAndVerify(ctx context.Context, deps pluginDeps, cfg *config.Config, engine pluginEngine, p *prompter, out io.Writer) error {
	secret := firstNonEmpty(cfg.Jellyfin.PluginSharedSecret, cfg.Jellyfin.WebhookSecret)
	if secret == "" {
		fmt.Fprintln(out, "No webhook secret in the config - run 'plex2jellyfin setup' to generate one.")
		return nil
	}

	daemonURL := cfg.Jellyfin.PluginDaemonURL
	if daemonURL == "" {
		def := "http://localhost:5522"
		if ip := deps.advertiseIP(); ip != "" {
			def = "http://" + ip + ":5522"
		}
		var err error
		if daemonURL, err = p.ask("Web UI URL reachable from Jellyfin", def); err != nil {
			return err
		}
		cfg.Jellyfin.PluginDaemonURL = strings.TrimRight(daemonURL, "/")
		cfg.Jellyfin.PluginEnabled = true
		if err := deps.saveConfig(cfg); err != nil {
			return fmt.Errorf("save plugin daemon URL: %w", err)
		}
	}

	if err := engine.Configure(ctx, daemonURL, secret); err != nil {
		fmt.Fprintf(out, "Pushing the plugin configuration failed: %v\n", err)
		fmt.Fprintln(out, "Retry later with: plex2jellyfin plugin verify")
		return nil
	}
	fmt.Fprintln(out, "  plugin configured (webhook URL + secret pushed)")

	res, err := engine.Verify(ctx)
	if err != nil {
		fmt.Fprintf(out, "Verification call failed: %v\n", err)
		fmt.Fprintln(out, "Retry later with: plex2jellyfin plugin verify")
		return nil
	}
	printVerifyResult(out, res)
	return nil
}

func printVerifyResult(out io.Writer, res *plugininstall.VerifyResult) {
	switch {
	case res.Sent && res.Authenticated:
		fmt.Fprintln(out, "  verified - a signed test event round-tripped through Jellyfin")
	case res.Sent:
		fmt.Fprintf(out, "  test event reached %s but got HTTP %d - check that the webhook secret matches\n",
			res.DaemonURL, res.DaemonStatusCode)
	default:
		fmt.Fprintf(out, "  Jellyfin could not reach %s: %s\n", res.DaemonURL, res.Error)
		fmt.Fprintln(out, "  (wrong URL for Jellyfin's network? containers cannot reach the host's localhost)")
	}
}

func runPluginVerify(ctx context.Context, deps pluginDeps, out io.Writer) error {
	_, engine, err := pluginEngineFromConfig(deps)
	if err != nil {
		return err
	}
	insp, err := engine.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Jellyfin: %w", err)
	}
	if !insp.PluginResponding {
		return errors.New("the plugin is not responding - is it installed and Jellyfin restarted? Run: plex2jellyfin plugin install")
	}
	res, err := engine.Verify(ctx)
	if err != nil {
		return fmt.Errorf("trigger test event: %w", err)
	}
	printVerifyResult(out, res)
	if !res.Sent || !res.Authenticated {
		return errors.New("feedback loop verification failed")
	}
	return nil
}

func runPluginStatus(ctx context.Context, deps pluginDeps, out io.Writer) error {
	_, engine, err := pluginEngineFromConfig(deps)
	if err != nil {
		return err
	}
	insp, err := engine.Inspect(ctx)
	if err != nil {
		return fmt.Errorf("inspect Jellyfin: %w", err)
	}
	fmt.Fprintf(out, "Jellyfin server   %s (supported: %v)\n", insp.ServerVersion, insp.ABISupported)
	fmt.Fprintf(out, "Plugin repository %v\n", insp.RepoRegistered)
	if insp.InstalledVersion == "" {
		fmt.Fprintln(out, "Plugin            not installed - run: plex2jellyfin plugin install")
	} else {
		fmt.Fprintf(out, "Plugin            %s (responding: %v)\n", insp.InstalledVersion, insp.PluginResponding)
	}
	return nil
}
```

Register in `newRootCmd` (in `main.go`, next to the `newSetupCmd()` line): `rootCmd.AddCommand(newPluginCmd())` — visible (not hidden).

- [ ] **Step 4: Run to verify everything passes**

```bash
go test ./cmd/plex2jellyfin/ && go vet ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/plex2jellyfin/
git commit -m "feat(cli): plugin install/verify/status over Jellyfin's package API

Installs the companion plugin the way Jellyfin wants plugins installed
(repository + package endpoints), restarts only with explicit consent,
pushes the webhook configuration, and proves the reverse path with a
signed test event. Works identically for containerized Jellyfin."
```

### Task 10: Wizard integration (install in the Jellyfin step, configure/verify after activation)

**Files:**
- Modify: `cmd/plex2jellyfin/setup_cmd.go` (setupDeps, `runSetupWizard` after line 298 and after the completed-save at line 400-403)
- Test: `cmd/plex2jellyfin/setup_cmd_test.go`

**Interfaces:**
- Consumes: `pluginEngine`/`pluginDeps`/`configureAndVerify`/`printVerifyResult` (Task 9), `setupdomain.DetectAdvertiseIP` (Task 8).
- Produces: wizard flow per spec. Setup completion stays independent of every plugin stage: the feedback-loop step runs strictly AFTER `Setup.Completed = true` is saved.

- [ ] **Step 1: Extend setupDeps and wire defaults**

Add to the `setupDeps` struct:

```go
	pluginEngine func(url, apiKey string) pluginEngine
	advertiseIP  func() string
	saveConfig   func(*config.Config) error
```

In `defaultSetupDeps()` add:

```go
		pluginEngine: func(url, apiKey string) pluginEngine {
			return plugininstall.New(url, apiKey, &http.Client{Timeout: 15 * time.Second})
		},
		advertiseIP: setupdomain.DetectAdvertiseIP,
		saveConfig:  func(c *config.Config) error { return c.Save() },
```

(Imports: `net/http`, `github.com/Nomadcxx/plex2jellyfin/internal/jellyfin/plugininstall`.)

- [ ] **Step 2: Write the failing tests**

In `setup_cmd_test.go`: extend `wizardTestDeps` with a scripted engine and update the existing happy path.

```go
// add to the state struct fields:
	pluginInstalled  bool
	pluginRestarted  bool
	pluginConfigured []string

// add to wizardTestDeps deps literal:
		pluginEngine: func(url, apiKey string) pluginEngine {
			return &wizardFakeEngine{state: state}
		},
		advertiseIP: func() string { return "10.0.0.5" },
		saveConfig:  func(c *config.Config) error { return c.Save() },
```

Add the fake engine (top level in the test file):

```go
type wizardFakeEngine struct {
	state *wizardState // rename the anonymous state struct to a named type wizardState
}

func (w *wizardFakeEngine) Inspect(ctx context.Context) (*plugininstall.Inspection, error) {
	return &plugininstall.Inspection{
		ServerVersion:    "10.11.6",
		ABISupported:     true,
		InstalledVersion: "",
		PluginResponding: w.state.pluginRestarted,
	}, nil
}
func (w *wizardFakeEngine) RegisterRepo(ctx context.Context) (bool, error) { return true, nil }
func (w *wizardFakeEngine) Install(ctx context.Context) error {
	w.state.pluginInstalled = true
	return nil
}
func (w *wizardFakeEngine) Restart(ctx context.Context) error {
	w.state.pluginRestarted = true
	return nil
}
func (w *wizardFakeEngine) WaitReady(ctx context.Context, d time.Duration) error { return nil }
func (w *wizardFakeEngine) Configure(ctx context.Context, daemonURL, secret string) error {
	w.state.pluginConfigured = []string{daemonURL, secret}
	return nil
}
func (w *wizardFakeEngine) Verify(ctx context.Context) (*plugininstall.VerifyResult, error) {
	return &plugininstall.VerifyResult{Sent: true, DaemonStatusCode: 200, Authenticated: true}, nil
}
```

(Refactor note: the anonymous state struct returned by `wizardTestDeps` becomes a named `wizardState` type so the fake engine can hold a pointer to it. Update the two existing usages mechanically.)

Update `TestSetupWizardTVOnlyHappyPath` answers — after `"jf-key"` insert the plugin prompts, and after `"y"` (confirm write) append the feedback-loop answer:

```go
	answers := []string{
		"/downloads/tv",  // TV incoming
		"/media/tv",      // TV library
		"",               // Movie incoming (skip)
		"",               // Movie library (skip)
		"y",              // connect Sonarr?
		"",               // Sonarr URL (default)
		"sonarr-key",     // Sonarr API key
		"y",              // apply sonarr fixes?
		"n",              // connect Radarr?
		"y",              // connect Jellyfin?
		"",               // Jellyfin URL (default)
		"jf-key",         // Jellyfin API key
		"y",              // install companion plugin?
		"y",              // restart Jellyfin?
		"n",              // use Ollama?
		"10m",            // scan frequency
		"y",              // move files?
		"n",              // verify checksums?
		"",               // chown user (skip)
		"y",              // confirm write
		"",               // webhook URL (accept detected default)
	}
```

New assertions at the end of the happy path test:

```go
	if !state.pluginInstalled || !state.pluginRestarted {
		t.Error("plugin was not installed+restarted during the Jellyfin step")
	}
	if len(state.pluginConfigured) != 2 ||
		state.pluginConfigured[0] != "http://10.0.0.5:5522" ||
		state.pluginConfigured[1] != "generated-secret" {
		t.Errorf("plugin configure args: %v", state.pluginConfigured)
	}
	if saved.Jellyfin.PluginDaemonURL != "http://10.0.0.5:5522" || !saved.Jellyfin.PluginEnabled {
		t.Errorf("plugin daemon URL not persisted: %+v", saved.Jellyfin)
	}
```

Add two new tests:

```go
func TestSetupWizardDeclinedPluginRestartStillCompletesSetup(t *testing.T) {
	deps, state := wizardTestDeps(t)

	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n", // sonarr, radarr off
		"y", "", "jf-key", // jellyfin
		"y",            // install plugin?
		"n",            // restart Jellyfin? (declined)
		"n",            // no ollama
		"5m", "n", "n", // runtime
		"",  // chown skip
		"y", // confirm
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("wizard error: %v\n---\n%s", err, out)
	}
	if state.pluginRestarted {
		t.Error("restarted Jellyfin without consent")
	}
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !saved.Setup.Completed {
		t.Error("setup must complete regardless of the plugin step")
	}
	if !strings.Contains(out, "plex2jellyfin plugin verify") {
		t.Errorf("expected recovery pointer in output:\n%s", out)
	}
}

func TestSetupWizardPluginFailureDoesNotFailSetup(t *testing.T) {
	deps, state := wizardTestDeps(t)
	deps.pluginEngine = func(url, apiKey string) pluginEngine {
		return &failingEngine{}
	}

	answers := []string{
		"/downloads/tv", "/media/tv", "", "",
		"n", "n",
		"y", "", "jf-key",
		"n",            // no ollama
		"5m", "n", "n", // runtime
		"",
		"y",
	}
	out, err := runWizard(t, deps, answers)
	if err != nil {
		t.Fatalf("plugin failure must not fail setup: %v\n%s", err, out)
	}
	_ = state
	saved, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !saved.Setup.Completed {
		t.Error("setup must complete when the plugin step errors")
	}
	if !strings.Contains(out, "plex2jellyfin plugin install") {
		t.Errorf("expected install recovery pointer:\n%s", out)
	}
}

type failingEngine struct{}

func (f *failingEngine) Inspect(ctx context.Context) (*plugininstall.Inspection, error) {
	return nil, errors.New("jellyfin exploded")
}
func (f *failingEngine) RegisterRepo(ctx context.Context) (bool, error) { return false, errors.New("no") }
func (f *failingEngine) Install(ctx context.Context) error              { return errors.New("no") }
func (f *failingEngine) Restart(ctx context.Context) error              { return errors.New("no") }
func (f *failingEngine) WaitReady(ctx context.Context, d time.Duration) error {
	return errors.New("no")
}
func (f *failingEngine) Configure(ctx context.Context, u, s string) error { return errors.New("no") }
func (f *failingEngine) Verify(ctx context.Context) (*plugininstall.VerifyResult, error) {
	return nil, errors.New("no")
}
```

(Note: when `Inspect` fails, the wizard prints the skip message and asks NO plugin questions — hence the failing-engine test has no plugin answers.)

- [ ] **Step 3: Run to verify they fail**

```bash
go test ./cmd/plex2jellyfin/ -run TestSetupWizard 2>&1 | head -10
```

Expected: FAIL (setupDeps fields missing; happy-path answers now misaligned).

- [ ] **Step 4: Implement the two wizard steps**

In `runSetupWizard`, after the Jellyfin draft assignment (after current line 298), add:

```go
	pluginState := wizardPluginStep(ctx, p, stdout, deps, jellyfinDraft)
```

and above `promptService` (top level), add:

```go
// wizardPluginState carries what the Jellyfin-step plugin actions
// accomplished into the post-activation feedback-loop step.
type wizardPluginState struct {
	attempted bool // user said yes to installing
	loaded    bool // plugin responded after install(+restart)
	skipped   string // non-empty: reason the step was skipped, for the summary
}

// wizardPluginStep runs install/restart (engine stages 1-4) inside the
// Jellyfin step. Every failure degrades to a printed recovery command;
// nothing here can abort the wizard.
func wizardPluginStep(ctx context.Context, p *prompter, out io.Writer, deps setupDeps, jf setupdomain.ServiceDraft) wizardPluginState {
	if !jf.Enabled {
		return wizardPluginState{skipped: "jellyfin disabled"}
	}
	engine := deps.pluginEngine(jf.URL, jf.APIKey)
	insp, err := engine.Inspect(ctx)
	if err != nil {
		fmt.Fprintf(out, "  skipping the companion plugin step (%v)\n", err)
		fmt.Fprintln(out, "  install it later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "jellyfin unreachable"}
	}
	if !insp.ABISupported {
		fmt.Fprintf(out, "  Jellyfin %s cannot load the companion plugin (needs 10.11.x); see the docs for manual options\n", insp.ServerVersion)
		return wizardPluginState{skipped: "unsupported server"}
	}
	if insp.InstalledVersion != "" && insp.PluginResponding {
		fmt.Fprintf(out, "  companion plugin %s already installed\n", insp.InstalledVersion)
		return wizardPluginState{attempted: true, loaded: true}
	}

	fmt.Fprintln(out, "\nThe companion plugin is required for the feedback loop: it confirms")
	fmt.Fprintln(out, "organized files against real Jellyfin items and powers orphan detection.")
	install, err := p.askBool("Install it through Jellyfin's plugin system now?", true)
	if err != nil || !install {
		fmt.Fprintln(out, "  install it later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "declined"}
	}

	if added, err := engine.RegisterRepo(ctx); err != nil {
		fmt.Fprintf(out, "  registering the plugin repository failed: %v\n", err)
		fmt.Fprintln(out, "  retry later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "repository registration failed"}
	} else if added {
		fmt.Fprintln(out, "  plugin repository registered (existing repositories kept)")
	}
	if err := engine.Install(ctx); err != nil {
		fmt.Fprintf(out, "  plugin install failed: %v\n", err)
		fmt.Fprintln(out, "  retry later with: plex2jellyfin plugin install")
		return wizardPluginState{skipped: "install failed"}
	}
	fmt.Fprintln(out, "  Jellyfin downloaded the plugin; a restart is needed before it loads")

	restart, err := p.askBool("Restart Jellyfin now?", false)
	if err != nil || !restart {
		fmt.Fprintln(out, "  after restarting Jellyfin, run: plex2jellyfin plugin verify")
		return wizardPluginState{attempted: true}
	}
	if err := engine.Restart(ctx); err != nil {
		fmt.Fprintf(out, "  restart request failed: %v\n", err)
		fmt.Fprintln(out, "  restart Jellyfin manually, then run: plex2jellyfin plugin verify")
		return wizardPluginState{attempted: true}
	}
	fmt.Fprintln(out, "  restart requested, waiting for the plugin to come back…")
	if err := engine.WaitReady(ctx, restartWaitTimeout); err != nil {
		fmt.Fprintf(out, "  Jellyfin did not come back in time (%v)\n", err)
		fmt.Fprintln(out, "  once it is up, run: plex2jellyfin plugin verify")
		return wizardPluginState{attempted: true}
	}
	fmt.Fprintln(out, "  plugin loaded")
	return wizardPluginState{attempted: true, loaded: true}
}
```

Then, AFTER the `candidate.Setup.Completed = true` save (current lines 400–403) and before the final "Setup complete" prints, add:

```go
	if pluginState.loaded {
		fmt.Fprintln(stdout, "\n— Feedback loop —")
		if deps.runtime.Kind == setupdomain.RuntimeContainer {
			fmt.Fprintln(stdout, "plex2jellyfin runs in a container: Jellyfin most likely reaches it via")
			fmt.Fprintln(stdout, "the Docker gateway IP (often 172.17.0.1) or a compose service name,")
			fmt.Fprintln(stdout, "not the LAN IP suggested below.")
		}
		engine := deps.pluginEngine(candidate.Jellyfin.URL, candidate.Jellyfin.APIKey)
		pd := pluginDeps{
			loadConfig:  deps.loadConfig,
			saveConfig:  deps.saveConfig,
			newEngine:   func(string, string) pluginEngine { return engine },
			advertiseIP: deps.advertiseIP,
		}
		if err := configureAndVerify(ctx, pd, candidate, engine, p, stdout); err != nil {
			fmt.Fprintf(stdout, "  feedback-loop setup incomplete: %v\n", err)
			fmt.Fprintln(stdout, "  finish it later with: plex2jellyfin plugin verify")
		}
	} else if pluginState.attempted {
		fmt.Fprintln(stdout, "\nCompanion plugin: restart Jellyfin, then run: plex2jellyfin plugin verify")
	} else if candidate.Jellyfin.Enabled && pluginState.skipped != "" && pluginState.skipped != "jellyfin disabled" {
		fmt.Fprintln(stdout, "\nCompanion plugin not installed - run: plex2jellyfin plugin install")
	}
```

(`configureAndVerify` prompts for the webhook URL, saves `PluginDaemonURL` + `PluginEnabled` through `pd.saveConfig`, pushes config, verifies — Task 9's shared function. It never returns a setup-fatal error; the error branch only covers prompt-I/O failure.)

- [ ] **Step 5: Run the full package**

```bash
go test ./cmd/plex2jellyfin/ && go build ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/plex2jellyfin/
git commit -m "feat(setup): companion plugin install and verification in the wizard

Install and consented restart happen inside the Jellyfin step; webhook
configuration and the signed test-event verification run after daemon
activation, because the secret is generated at apply time and the
daemon must be running to receive the event. Setup completion never
depends on any plugin stage - every failure degrades to a printed
recovery command."
```

### Task 11: Documentation

**Files:**
- Modify: `docs-site/content/docs/getting-started/jellyfin-plugin.md`
- Modify: `docs-site/content/docs/reference/configuration.md` (`[jellyfin]` section)
- Modify: `README.md` (plugin section)

**Interfaces:** none (prose).

- [ ] **Step 1: Rewrite the plugin guide around automatic install**

In `jellyfin-plugin.md`, insert before the "Build and install" section (and retitle that section "Manual build and install (fallback)"):

```markdown
## Automatic install (recommended)

The setup wizard installs the plugin for you: when you connect Jellyfin
during `plex2jellyfin setup`, the wizard registers the plugin
repository, has Jellyfin download the plugin, asks before restarting
Jellyfin, then pushes the webhook secret and URL into the plugin and
verifies the loop with a signed test event.

On an existing install:

```bash
plex2jellyfin plugin install   # install + configure + verify
plex2jellyfin plugin verify    # re-check the feedback loop any time
plex2jellyfin plugin status    # what Jellyfin reports about the plugin
```

This works for Jellyfin in Docker too - Jellyfin downloads the plugin
itself, so no filesystem access is needed. The one URL you may need to
adjust is the callback URL: it is *Jellyfin's* view of plex2jellyfin,
so never `localhost` when either side runs in a container.
```

Update the "Configure the webhook secret" section to note it is only needed for manual installs, since the wizard and `plugin install` push the configuration automatically.

- [ ] **Step 2: Document plugin_daemon_url**

In `configuration.md`'s `[jellyfin]` section, add a row/entry:

```markdown
- `plugin_daemon_url` — base URL the companion plugin calls back to
  (the plugin appends `/api/v1/webhooks/jellyfin`). Set by the wizard;
  must be reachable *from Jellyfin's network*, so never `localhost`
  when either side is containerized.
```

- [ ] **Step 3: Update the README plugin section**

In `README.md`, in the "Jellyfin Plugin — install this too" section, replace the clone-and-build code block with:

```markdown
The setup wizard installs and configures it automatically when you
connect Jellyfin (or run `plex2jellyfin plugin install` on an existing
setup). Manual build instructions live in the
[plugin repository](https://github.com/Nomadcxx/plex2jellyfin-plugin).
```

- [ ] **Step 4: Commit**

```bash
git add docs-site/ README.md
git commit -m "docs: automatic plugin install is the primary path"
```

### Task 12: End-to-end verification against a throwaway Jellyfin

**Files:** none committed (verification only; requires Task 4's release to be live).

**Interfaces:** consumes everything.

- [ ] **Step 1: Start a fresh Jellyfin 10.11 container**

```bash
D=$(mktemp -d /tmp/p2j-jf-e2e-XXXX)
docker run -d --name p2j-jf-e2e -p 18096:8096 \
  -v "$D/config:/config" -v "$D/cache:/cache" jellyfin/jellyfin:10.11
```

(Port 18096 — the live server on 8096 and JellyWatch on 8686 must not be touched.)

- [ ] **Step 2: Complete Jellyfin's startup wizard and mint an API key**

Open `http://localhost:18096`, finish the startup wizard (any admin user, no libraries needed), then Dashboard → API Keys → new key.

- [ ] **Step 3: Point a scratch plex2jellyfin config at it and run the flow**

Use a scratch HOME so the real config is untouched, with a spare health port (host holds :8686):

```bash
export HOME=/tmp/p2jwiz-e2e && mkdir -p "$HOME"
go build -o /tmp/p2j-e2e/plex2jellyfin ./cmd/plex2jellyfin
go build -o /tmp/p2j-e2e/plex2jellyfin-daemon ./cmd/plex2jellyfin-daemon
# run setup with TV paths under $HOME, Jellyfin http://localhost:18096 + the API key,
# accepting the plugin install and restart prompts; health_addr must be overridden
# in the pre-seeded config or the daemon will collide with :8686
/tmp/p2j-e2e/plex2jellyfin setup
```

Expected at the Jellyfin step: repository registered → plugin downloaded → restart consented → "plugin loaded". Expected at the feedback-loop step: webhook URL default accepted (host LAN IP — for a Docker Jellyfin reaching the host, use `http://172.17.0.1:5522`) → "plugin configured" → "verified".

- [ ] **Step 4: Confirm from the Jellyfin side**

```bash
curl -s http://localhost:18096/plex2jellyfin/status | python3 -m json.tool
/tmp/p2j-e2e/plex2jellyfin plugin status
/tmp/p2j-e2e/plex2jellyfin plugin verify
```

Expected: status shows `EventForwardingEnabled: true` with the pushed URL; `plugin verify` prints the verified line and exits 0. Also check the Jellyfin dashboard shows the repository under Plugins → Repositories and the plugin as Active.

- [ ] **Step 5: Clean up**

```bash
docker rm -f p2j-jf-e2e && rm -rf "$D" /tmp/p2jwiz-e2e /tmp/p2j-e2e
```

- [ ] **Step 6: Push everything**

```bash
git push
```
