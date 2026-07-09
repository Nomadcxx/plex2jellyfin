# Plex2Jellyfin Public Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the JellyWatch→Plex2Jellyfin rebrand (installer TUI, web UI, embedded frontend), publish the repo to github.com/Nomadcxx/plex2jellyfin, and ship first-release packaging (deb, rpm, Docker).

**Architecture:** The mechanical rename is already committed (`cf951ebb`). What remains is visual branding (block-art wordmarks and PNG logos still show the old JellyWatch artwork), a frontend rebuild so `embedded/` is regenerated from source rather than sed-patched, publishing, and packaging. Packaging uses GoReleaser+nfpm for deb/rpm and a multi-stage Dockerfile for containers; both are possible because the active SQLite driver is pure-Go (`modernc.org/sqlite`), so all binaries build with `CGO_ENABLED=0`.

**Tech Stack:** Go 1.24, Next.js (static export in `web/`, embedded via `go:embed`), lipgloss/bubbletea (installer TUI), GoReleaser + nfpm, Docker.

## Global Constraints

- Repo root: `/home/nomadx/Documents/plex2jellyfin` (NOT `~/Documents/jellywatch` — that is the old repo with a live deployment; never modify it).
- Remote: `https://github.com/Nomadcxx/plex2jellyfin.git` (already set as `origin`).
- Commit messages MUST NOT contain AI/agent attribution lines — a commit-msg hook rejects them.
- The string `jellywatch` (any case) must not appear in any tracked file after Task 5. Git history keeps the old name; that is intentional (shows project age).
- Canonical wordmark ASCII: `cmd/plex2jellyfin/assets/header.txt` (6 lines, 80 cols). Every other rendering (installer, PNGs) derives from it.
- Brand color: Jellyfin purple `#AA5CC3`.
- License: GPL-3.0-or-later (README already claims it; Task 1 adds the missing file).
- Gate for every code task: `go build ./... && go vet ./...` exit 0, plus the task's own test command.

## Delegation Map

| Task | Who | Why |
|---|---|---|
| 1 LICENSE | any cheap agent | mechanical download+commit |
| 2 Installer wordmark | cheap agent (Sonnet/Haiku class) | single-file edit, hard build gate |
| 3 Interim brand PNGs | cheap agent | script run + copy, visual gate |
| 4 Frontend rebuild | mid agent | npm env handling, embed gates |
| 5 Residual sweep | cheap agent | grep sweep with explicit allowlist |
| 6 Publish | main session + user | outward-facing push |
| 7 Docker | mid agent, main session reviews | new files, needs design judgment on entrypoint |
| 8 deb/rpm packaging | mid agent, main session reviews | new files, release wiring |
| 9 Live deployment migration | main session + user | touches the running system |
| 10 Final art swap | main session | BLOCKED on user-provided art |

---

### Task 1: Add the LICENSE file

**Files:**
- Create: `LICENSE`

**Interfaces:**
- Produces: `LICENSE` at repo root; Task 8's nfpm config references `license: GPL-3.0-or-later`.

- [ ] **Step 1: Download the canonical GPL-3.0 text**

```bash
cd /home/nomadx/Documents/plex2jellyfin
curl -fsSL https://www.gnu.org/licenses/gpl-3.0.txt -o LICENSE
```

- [ ] **Step 2: Verify it is the real license, not an error page**

Run: `head -2 LICENSE && wc -l LICENSE`
Expected: `GNU GENERAL PUBLIC LICENSE` / `Version 3, 29 June 2007`, ~675 lines.

- [ ] **Step 3: Commit**

```bash
git add LICENSE && git commit -m "chore: add GPL-3.0 license text"
```

---

### Task 2: Installer TUI wordmark

**Files:**
- Modify: `cmd/installer/theme.go:29-39`

**Interfaces:**
- Consumes: `cmd/plex2jellyfin/assets/header.txt` (canonical art).
- Produces: `asciiHeader` (string const) and `asciiHeaderLines` (`[]string`) — consumed by `cmd/installer/view.go:39,51` and `cmd/installer/update.go:18` (`NewBeamsTextEffect(msg.Width, headerHeight, asciiHeader)`). `headerHeight` in `update.go:16` is `6`; the new art is exactly 6 lines, so no change there.

- [ ] **Step 1: Replace both art constants in `cmd/installer/theme.go`**

Replace lines 29-39 (the `asciiHeaderLines` var and `asciiHeader` const containing the old JELLYWATCH block art) with:

```go
const asciiHeader = `                                  ██                                  ██        
▄▄▄▄▄   ██    ▄▄▄▄ ▄▄  ▄▄ ████▄   ▄▄    ▄▄▄▄  ██   ██  ▄▄   ▄▄  ▄████ ▄▄ ▄▄▄▄▄  
██▀▀██▄ ██  ▄█████ ▀████▀ ▄▄▄███  ██  ▄█████  ██   ██  ██   ██ ██▀    ██ ██▀▀██▄
██▄▄▄██ ██▄ ██▄▄▄▄ ▄████▄ ██▄▄▄▄  ██  ██▄▄▄▄  ██▄  ██▄ ▀██▄▄██ █████  ██ ██   ██
██▀▀▀▀▀ ▀▀▀ ▀▀▀▀▀▀ ▀▀  ▀▀ ▀▀▀▀▀▀ ▄██  ▀▀▀▀▀▀  ▀▀▀  ▀▀▀   ▀▀▀██ ▀▀     ▀▀ ▀▀   ▀▀
▀▀                               ▀▀                         ▀▀`

var asciiHeaderLines = strings.Split(asciiHeader, "\n")
```

Add `"strings"` to the import block of `theme.go` if not present. Note the art must match `cmd/plex2jellyfin/assets/header.txt` byte-for-byte except the trailing newline; copy from that file, do not retype.

- [ ] **Step 2: Verify build and consistency**

```bash
go build ./cmd/installer && \
diff <(sed -e '$a\' cmd/plex2jellyfin/assets/header.txt) <(go run ./scripts/print_header 2>/dev/null || true)
```

The `diff` half is optional tooling; the required check is: `go build ./cmd/installer` exits 0 and
`grep -c "██▄▄▄██" cmd/installer/theme.go` prints `1` (a line unique to the new art).

- [ ] **Step 3: Run installer package tests**

Run: `go test ./cmd/installer/`
Expected: PASS (or the same skips as `git stash; go test ./cmd/installer/; git stash pop` shows pre-change — no new failures).

- [ ] **Step 4: Commit**

```bash
git add cmd/installer/theme.go
git commit -m "brand(installer): plex2jellyfin wordmark in TUI header"
```

---

### Task 3: Interim brand PNGs for web UI and README assets

The renamed `plex2jellyfin_brand.png` files still contain the old JellyWatch artwork pixels. Until the user delivers final art (Task 10), regenerate them from the canonical ASCII so nothing user-visible says JellyWatch.

**Files:**
- Modify (binary overwrite): `web/public/plex2jellyfin_brand.png`, `assets/plex2jellyfin_brand.png`, `embedded/frontend/plex2jellyfin_brand.png`
- Reference (already correct): `web/src/app/page.tsx:28`, `web/src/components/auth/LoginForm.tsx:40` load `/plex2jellyfin_brand.png`.

- [ ] **Step 1: Generate the brand PNG from the canonical art**

```bash
cd /home/nomadx/Documents/plex2jellyfin
python3 scripts/ascii_to_png.py assets/plex2jellyfin_brand.png
cp assets/plex2jellyfin_brand.png web/public/plex2jellyfin_brand.png
```

Do NOT copy into `embedded/frontend/` by hand — Task 4's `make frontend` regenerates that directory from `web/`.

- [ ] **Step 2: Verify the pixels changed**

Run: `sha256sum assets/plex2jellyfin_brand.png web/public/plex2jellyfin_brand.png`
Expected: both hashes identical to each other and different from `git show HEAD:assets/plex2jellyfin_brand.png | sha256sum`.

- [ ] **Step 3: Commit**

```bash
git add assets/plex2jellyfin_brand.png web/public/plex2jellyfin_brand.png
git commit -m "brand(web): interim plex2jellyfin brand image generated from wordmark"
```

---

### Task 4: Rebuild the frontend and regenerate embedded assets

The `embedded/frontend/` tree currently contains sed-renamed build artifacts. Rebuild from source so build output and source agree.

**Files:**
- Regenerate: `embedded/frontend/` (entire tree, via Makefile)
- Test: `embed_test.go` (exists at repo root)

**Interfaces:**
- Consumes: `web/` Next.js app (already string-renamed), `make frontend` target (`Makefile:9-12`: `npm run build`, `rm -rf embedded/frontend`, `cp -r web/out embedded/frontend`).
- Produces: regenerated `embedded/frontend` consumed by `embed.go` (`//go:embed all:embedded/frontend`).

- [ ] **Step 1: Install web deps and rebuild**

```bash
cd /home/nomadx/Documents/plex2jellyfin/web && npm ci && cd .. && make frontend
```

Expected: Next.js build completes; `embedded/frontend/index.html` regenerated.

- [ ] **Step 2: Verify no stale branding in the build output**

```bash
grep -rIl "jellywatch\|JellyWatch" embedded/frontend/ | wc -l
```

Expected: `0`.

- [ ] **Step 3: Run embed test and full build**

```bash
go test . && go build ./... && make check-frontend
```

Expected: PASS, exit 0.

- [ ] **Step 4: Commit**

```bash
git add embedded/frontend
git commit -m "build(embed): regenerate frontend from renamed web source"
```

---

### Task 5: Residual branding sweep

**Files:**
- Possibly modify: whatever the sweep finds.

- [ ] **Step 1: Sweep tracked files**

```bash
cd /home/nomadx/Documents/plex2jellyfin
git grep -Iin "jellywatch" || echo CLEAN
```

Expected: `CLEAN`. If hits appear, fix each with a contextual replacement (not blind sed), re-run, and include the fixes in this task's commit.

- [ ] **Step 2: Sweep rendered help output**

```bash
go run ./cmd/plex2jellyfin --help 2>&1 | grep -ci jellywatch; \
go run ./cmd/plex2jellyfin-daemon --help 2>&1 | grep -ci jellywatch
```

Expected: `0` and `0`.

- [ ] **Step 3: Full gate**

```bash
gofmt -l . | grep -v '^web/' ; go build ./... && go vet ./... && go test ./...
```

Expected: no gofmt output, all green.

- [ ] **Step 4: Commit (only if Step 1/2 found fixes)**

```bash
git add -A && git commit -m "brand: sweep residual jellywatch references"
```

---

### Task 6: Publish to GitHub

Main session with user present — this makes the code public.

- [ ] **Step 1: Push**

```bash
cd /home/nomadx/Documents/plex2jellyfin && git push -u origin main
```

- [ ] **Step 2: Set repo metadata**

```bash
gh repo edit Nomadcxx/plex2jellyfin \
  --description "Migrate your Plex library to Jellyfin and keep it clean forever. Mass rename, dedupe, consolidate, then a daemon guards new downloads." \
  --add-topic jellyfin --add-topic plex --add-topic media-server --add-topic sonarr --add-topic radarr --add-topic go
```

- [ ] **Step 3: Verify public artifacts**

```bash
curl -fsSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/install.sh | head -3
GOPROXY=direct go install github.com/Nomadcxx/plex2jellyfin/cmd/plex2jellyfin@main && ~/go/bin/plex2jellyfin version
```

Expected: install.sh shebang appears; `go install` resolves and the binary prints a version.

- [ ] **Step 4: Confirm README renders** — open https://github.com/Nomadcxx/plex2jellyfin and check the header PNG displays and the mermaid diagram renders.

---

### Task 7: Docker support

Homeserver users expect a single container, linuxserver.io-style PUID/PGID, and volume mounts for config/watch/library.

**Files:**
- Create: `Dockerfile`, `.dockerignore`, `docker/entrypoint.sh`, `docker-compose.example.yml`
- Modify: `README.md` (add Docker section after "Install")

**Interfaces:**
- Consumes: `make frontend` layout (web build copied into `embedded/frontend` before `go build`); config path resolution in `internal/config` (verify step 1).
- Produces: image entrypoint that runs `plex2jellyfin-daemon` and `plex2jellyfin-web` in one container.

- [ ] **Step 1: Verify config dir honors `$HOME` (containers set it)**

```bash
git grep -n "UserConfigDir\|\.config/plex2jellyfin\|UserHomeDir" internal/config/config.go | head
```

If it uses `os.UserConfigDir()` or `os.UserHomeDir()`, `HOME=/config` in the entrypoint is sufficient. If a path is hardcoded, change it to `os.UserConfigDir()` first (with a unit test asserting `XDG_CONFIG_HOME` is honored) — that is a prerequisite commit inside this task.

- [ ] **Step 2: Write `Dockerfile`**

```dockerfile
# ---- frontend build ----
FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- go build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf embedded/frontend
COPY --from=frontend /src/web/out ./embedded/frontend
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/plex2jellyfin ./cmd/plex2jellyfin && \
    go build -trimpath -ldflags="-s -w" -o /out/plex2jellyfin-daemon ./cmd/plex2jellyfin-daemon && \
    go build -trimpath -ldflags="-s -w" -o /out/plex2jellyfin-web ./cmd/plex2jellyfin-web

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache tini su-exec
COPY --from=build /out/ /usr/local/bin/
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENV PUID=1000 PGID=1000 HOME=/config
VOLUME ["/config", "/watch", "/library"]
EXPOSE 5522
ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]
```

- [ ] **Step 3: Write `docker/entrypoint.sh`**

```sh
#!/bin/sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if ! getent group p2j >/dev/null 2>&1; then
    addgroup -g "$PGID" p2j
fi
if ! getent passwd p2j >/dev/null 2>&1; then
    adduser -D -H -u "$PUID" -G p2j -h /config p2j
fi

mkdir -p /config
chown -R "$PUID:$PGID" /config

su-exec p2j plex2jellyfin-daemon &
DAEMON_PID=$!

trap 'kill $DAEMON_PID 2>/dev/null' TERM INT
exec su-exec p2j plex2jellyfin-web
```

Note: running as non-root means the `[permissions]` chown feature is unavailable in-container; users control ownership with PUID/PGID instead. Say so in the README Docker section.

- [ ] **Step 4: Write `.dockerignore`**

```
bin/
build/
web/node_modules/
web/.next/
docs/
*.test
media.db
.git/
```

- [ ] **Step 5: Write `docker-compose.example.yml`**

```yaml
services:
  plex2jellyfin:
    image: ghcr.io/nomadcxx/plex2jellyfin:latest
    container_name: plex2jellyfin
    environment:
      - PUID=1000
      - PGID=1000
    volumes:
      - ./config:/config
      - /path/to/downloads:/watch
      - /path/to/media:/library
    ports:
      - "5522:5522"
    restart: unless-stopped
```

- [ ] **Step 6: Build and smoke-test the image**

```bash
docker build -t plex2jellyfin:dev . && \
docker run --rm plex2jellyfin:dev plex2jellyfin version && \
docker run --rm -d --name p2j-smoke -p 5523:5522 plex2jellyfin:dev && sleep 3 && \
curl -fsS http://localhost:5523/ >/dev/null && echo WEB_OK; docker rm -f p2j-smoke
```

Expected: version prints; `WEB_OK`.

- [ ] **Step 7: Add README "Docker" subsection** under Install, showing the compose file and the PUID/PGID note from Step 3.

- [ ] **Step 8: Commit**

```bash
git add Dockerfile .dockerignore docker/ docker-compose.example.yml README.md
git commit -m "feat(docker): single-container image with PUID/PGID and compose example"
```

---

### Task 8: Debian/Fedora packages via GoReleaser + nfpm

Same approach as sysc-greet and gslapper: native packages for the first release.

**Files:**
- Create: `.goreleaser.yaml`, `packaging/postinstall.sh`, `packaging/preremove.sh`

**Interfaces:**
- Consumes: `LICENSE` (Task 1), systemd units `systemd/plex2jellyfin-daemon.service`, `systemd/plex2jellyfin-web.service`.
- Produces: `.deb` + `.rpm` on `git tag` via `goreleaser release`; GitHub release with binaries.

- [ ] **Step 1: Write `.goreleaser.yaml`**

```yaml
version: 2
project_name: plex2jellyfin

before:
  hooks:
    - make frontend

builds:
  - id: cli
    main: ./cmd/plex2jellyfin
    binary: plex2jellyfin
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64]
  - id: daemon
    main: ./cmd/plex2jellyfin-daemon
    binary: plex2jellyfin-daemon
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64]
  - id: web
    main: ./cmd/plex2jellyfin-web
    binary: plex2jellyfin-web
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64]
  - id: installer
    main: ./cmd/installer
    binary: plex2jellyfin-installer
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64]

nfpms:
  - id: packages
    package_name: plex2jellyfin
    formats: [deb, rpm]
    license: GPL-3.0-or-later
    maintainer: "Nomadcxx <nomadcxx@users.noreply.github.com>"
    description: |-
      Migrate your Plex library to Jellyfin and keep it clean forever.
      Mass rename, dedupe, and consolidate an existing library, then a
      daemon guards new downloads as they arrive.
    homepage: https://github.com/Nomadcxx/plex2jellyfin
    builds: [cli, daemon, web]
    contents:
      - src: systemd/plex2jellyfin-daemon.service
        dst: /usr/lib/systemd/system/plex2jellyfin-daemon.service
      - src: systemd/plex2jellyfin-web.service
        dst: /usr/lib/systemd/system/plex2jellyfin-web.service
      - src: config.toml.example
        dst: /usr/share/doc/plex2jellyfin/config.toml.example
    scripts:
      postinstall: packaging/postinstall.sh
      preremove: packaging/preremove.sh

archives:
  - id: tarballs
    builds: [cli, daemon, web, installer]

release:
  github:
    owner: Nomadcxx
    name: plex2jellyfin
  draft: true
```

- [ ] **Step 2: Write `packaging/postinstall.sh`**

```sh
#!/bin/sh
set -e
systemctl daemon-reload || true
echo "plex2jellyfin installed."
echo "  1. Copy /usr/share/doc/plex2jellyfin/config.toml.example to ~/.config/plex2jellyfin/config.toml and edit it"
echo "  2. systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web"
```

- [ ] **Step 3: Write `packaging/preremove.sh`**

```sh
#!/bin/sh
set -e
systemctl stop plex2jellyfin-web plex2jellyfin-daemon 2>/dev/null || true
systemctl disable plex2jellyfin-web plex2jellyfin-daemon 2>/dev/null || true
```

- [ ] **Step 4: Verify unit files reference packaged paths**

```bash
grep -n "ExecStart" systemd/plex2jellyfin-daemon.service systemd/plex2jellyfin-web.service
```

Expected: `ExecStart=/usr/local/bin/...`. Packages install to `/usr/bin`; change ExecStart lines to `/usr/bin/plex2jellyfin-daemon` and `/usr/bin/plex2jellyfin-web` OR keep `/usr/local/bin` and add nfpm `contents` symlinks. Pick `/usr/bin` and update `install.sh` to match so the two install paths agree.

- [ ] **Step 5: Dry-run the release**

```bash
go install github.com/goreleaser/goreleaser/v2@latest 2>/dev/null || sudo pacman -S --noconfirm goreleaser
goreleaser release --snapshot --clean
ls dist/*.deb dist/*.rpm
```

Expected: deb and rpm for amd64+arm64 in `dist/`.

- [ ] **Step 6: Install-test the deb in a container**

```bash
docker run --rm -v "$PWD/dist:/dist" debian:bookworm sh -c \
  "apt-get update -qq && apt-get install -y -qq /dist/plex2jellyfin_*_amd64.deb && plex2jellyfin version"
```

Expected: version string prints.

- [ ] **Step 7: Commit**

```bash
git add .goreleaser.yaml packaging/ systemd/ install.sh
git commit -m "feat(packaging): deb/rpm via goreleaser+nfpm, aligned install paths"
```

---

### Task 9: Migrate the author's live deployment (this machine)

Main session, with user. The old jellywatch daemon is running from `/usr/local/bin` with config in `~/.config/jellywatch`.

- [ ] **Step 1: Copy config and database**

```bash
cp -r ~/.config/jellywatch ~/.config/plex2jellyfin
```

- [ ] **Step 2: Build and install new binaries**

```bash
cd /home/nomadx/Documents/plex2jellyfin && make && \
sudo install bin/plex2jellyfin bin/plex2jellyfin-daemon bin/plex2jellyfin-web /usr/local/bin/
```

- [ ] **Step 3: Install and start new units, stop old ones**

```bash
sudo systemctl disable --now jellywatchd jellyweb 2>/dev/null || true
sudo cp systemd/plex2jellyfin-daemon.service systemd/plex2jellyfin-web.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now plex2jellyfin-daemon plex2jellyfin-web
systemctl --user disable --now jellywatch-postmortem.timer 2>/dev/null || true
cp systemd/user/plex2jellyfin-postmortem.* ~/.config/systemd/user/ && \
systemctl --user daemon-reload && systemctl --user enable --now plex2jellyfin-postmortem.timer
```

- [ ] **Step 4: Verify**

```bash
systemctl is-active plex2jellyfin-daemon plex2jellyfin-web && \
plex2jellyfin status && curl -fsS http://localhost:5522/ >/dev/null && echo WEB_OK
```

Expected: `active` twice, status prints DB stats, `WEB_OK`. Watch `journalctl -u plex2jellyfin-daemon -f` through one download cycle before deleting anything old.

---

### Task 10: Final art swap-in (BLOCKED: waiting on user-provided art)

When the user delivers final ASCII/PNG art:

- [ ] **Step 1:** Overwrite `cmd/plex2jellyfin/assets/header.txt` with the new ASCII (if changed).
- [ ] **Step 2:** Re-run Task 2 Step 1 (installer const must match the file byte-for-byte) if the ASCII changed.
- [ ] **Step 3:** If the user supplies a PNG, copy it over `assets/plex2jellyfin-header.png` and/or `assets/plex2jellyfin_brand.png` + `web/public/plex2jellyfin_brand.png`; otherwise regenerate with `python3 scripts/ascii_to_png.py <target>`.
- [ ] **Step 4:** `cd web && npm run build && cd .. && make frontend` to refresh embedded assets; `go build ./... && go test .`.
- [ ] **Step 5:** Commit: `git add -A && git commit -m "brand: final artwork"` and push.

---

## Launch checklist (after all tasks)

- [ ] Tag `v0.1.0-beta.1` and run `goreleaser release --clean` (publishes draft GitHub release; user reviews then publishes).
- [ ] GHCR image push (`docker build -t ghcr.io/nomadcxx/plex2jellyfin:v0.1.0-beta.1 . && docker push ...`) — needs `gh auth token` with `write:packages`.
- [ ] Submit to awesome-jellyfin list (PR adding plex2jellyfin under Media Organization).
- [ ] AUR package (`plex2jellyfin-bin`) — follow-up, same pattern as sysc-greet/gslapper.
- [ ] Announcement post drafts for r/jellyfin, r/selfhosted (angle: "I migrated my Plex library to Jellyfin and wrote the tool that fixed the naming chaos").
