# Web Setup Wizard Design

**Date:** 2026-07-11

## Goal

Give fresh Docker and packaged Linux installations a complete first-run Web setup path before exposing the dashboard. The wizard configures Plex2Jellyfin; it does not install binaries, packages, or systemd units.

Existing installations with a usable TV or movie path pair bypass the wizard automatically.

## Decisions

- Password creation remains the first first-run screen.
- The dashboard is unavailable until setup completes.
- TV and Movies are both shown. Either may be left empty; any configured media type must have at least one incoming path and one library path.
- The initial library scan is optional and does not gate dashboard access.
- Service and Ollama checks use draft values and do not require saving partial configuration.
- Setup is one atomic configuration transaction followed by daemon activation and readiness verification.
- Docker/container installs do not expose ownership controls that cannot work after privilege dropping.
- Native Linux installs may configure file ownership and modes.
- The current unauthenticated first-run network posture is unchanged.

## Rejected Approaches

### Save each step through existing settings routes

Fresh installs have no daemon accepting IPC reloads. Existing settings writes therefore roll back, and a partially saved wizard would be difficult to recover safely.

### Run the TUI installer behind the Web UI

The installer owns source builds, privilege escalation, systemd installation, and terminal interaction. Those responsibilities do not belong in the Web process and do not apply consistently to containers or distribution packages.

## User Flow

1. **Account**: create the Web UI password using the existing auth setup.
2. **Media**: enter TV and/or movie incoming and library paths. Each path is preflighted for existence, directory type, access, and free space.
3. **Services**: optionally configure and test Sonarr, Radarr, and Jellyfin. Sonarr/Radarr compatibility findings can be repaired only after explicit confirmation. Jellyfin supports mount-prefix mappings.
4. **AI**: optionally test Ollama, discover installed models, select primary/fallback models, and run a prompt check.
5. **Runtime**: choose scan frequency and move/copy behavior. Native installations may set ownership and modes; containers show their effective UID/GID instead.
6. **Review**: validate and apply the entire draft, then start or reload the daemon and wait for IPC readiness.
7. **Initial scan**: optionally start a scan, then enter the dashboard.

Back navigation keeps the in-memory draft. An apply or activation error leaves the user on Review with actionable errors and does not unlock the dashboard.

## Setup State

`[setup]` metadata records the schema version and completion state.

- `version = 0` with a complete media pair is treated as a legacy configured installation and bypasses setup.
- `version = 1, completed = false` means Web setup started but activation has not succeeded.
- `version = 1, completed = true` means setup completed.

This distinction prevents an activation failure from being mistaken for a legacy configured installation.

## API

### `GET /api/v1/setup/status`

Returns setup state after the normal auth middleware, runtime type, effective UID/GID, daemon state, and a masked draft based on current configuration/defaults.

### `POST /api/v1/setup/apply`

Accepts one typed setup document. The server:

1. decodes with unknown-field rejection;
2. validates media pairs, durations, modes, URLs, model selection, and path mappings;
3. preflights every configured path;
4. loads the latest config from disk;
5. preserves auth and secrets omitted or masked by the request;
6. generates Jellyfin webhook/plugin secrets when required;
7. atomically writes `version = 1, completed = false`;
8. reloads a running daemon or launches a stopped daemon;
9. polls IPC status;
10. atomically marks setup complete after readiness succeeds.

Connection tests, path preflight, AI model discovery, prompt testing, and scan operations reuse existing APIs. Sonarr/Radarr compatibility check/fix is added beside the current connection-test handlers.

## Configuration Ownership

Current settings, watch-path, and library-path handlers retain separate config pointers. One handler can therefore overwrite a newer change made by another. The setup work will make each mutation load the latest disk config under one server-level mutex, persist it, and refresh the server snapshot. This keeps setup and later dashboard edits consistent without adding a general configuration framework.

## Runtime Activation

- If IPC status succeeds, issue reload and verify status again.
- If IPC is unavailable, use the existing launcher, then poll IPC with a bounded timeout.
- Container entrypoints may have started a daemon that exited against default config; direct launch is the expected recovery path.
- A missing launcher or readiness timeout is returned as a blocking setup error.

## Interface

The wizard is a dedicated `/setup` route without dashboard navigation.

- Dark-only terminal-console styling matching the current reskin.
- P2J transparent ASCII PNG as the primary mark.
- IBM Plex Mono for operational labels; Inter for form content.
- Compact fixed step rail on desktop and a progress header on small screens.
- Amber only for focus/current state, cyan/green for verified state, and red for blocking errors.
- Stable form dimensions, accessible labels, keyboard focus, error summaries, and no decorative gradients or nested cards.

The old `/onboarding` route redirects to `/setup`, removing the competing local-storage onboarding flow.

## Verification

- Go unit tests cover completion derivation, validation, secret preservation, atomic apply, activation success/failure, legacy bypass, and container detection.
- API handler tests cover status/apply and Arr compatibility actions.
- Frontend tests cover auth routing, optional TV/movie sections, blocking validation, service/AI draft flow, and apply completion.
- Run frontend tests, typecheck, production export, embedded-output equality, `make check-frontend`, and `go test ./...`.
- Run the built Web server against an isolated config and verify fresh setup status, configured bypass, static `/setup/`, and dashboard routing behavior.
