# PROJECT KNOWLEDGE BASE

**Generated:** 2026-02-04 01:37:19 +11:00
**Commit:** 7d8f2e3
**Branch:** main

## OVERVIEW
Media file watcher and organizer for Jellyfin libraries. Go application with embedded Next.js web UI that monitors download directories, parses media filenames using extensive regex patterns, and organizes files according to Jellyfin naming standards. Integrates with Sonarr/Radarr APIs and provides local AI (Ollama) support for ambiguous files.

## STRUCTURE
```
jellywatch/
├── cmd/                    # Entry points for executables
│   ├── jellywatch/         # Main CLI (Cobra-based, 20+ subcommands)
│   ├── jellywatchd/        # Daemon service (systemd integration)
│   ├── installer/           # Interactive TUI installer (Bubble Tea)
│   ├── test-hint/          # Debug utility for classification
│   └── classification-audit/ # Database audit tool
├── internal/                # Private application packages (28 modules)
│   ├── ai/                 # Ollama integration with circuit breaker, keepalive
│   ├── naming/             # Core regex patterns for name parsing (3976 lines)
│   ├── database/            # SQLite operations with WAL mode (6503 lines)
│   ├── consolidate/         # Series consolidation logic
│   ├── scanner/             # File system scanning with AI hints
│   ├── quality/             # Quality detection (REMUX, BluRay, WEB-DL)
│   ├── sync/               # Sonarr/Radarr sync with backoff
│   ├── organizer/           # File organization (transfers via rsync)
│   ├── transfer/            # Robust file ops with timeout/retry
│   ├── sonarr/             # Sonarr API client
│   ├── radarr/             # Radarr API client
│   ├── library/             # Drive/library selection by space
│   ├── migration/           # Path mismatch detection & fixing
│   ├── config/              # TOML config loading
│   ├── daemon/              # Background service orchestration
│   ├── plans/               # Consolidate/duplicate plan generation
│   ├── analyzer/            # Media analysis logic
│   ├── service/             # Background service management
│   ├── api/                 # REST API for web UI
│   ├── wizard/              # Interactive setup wizard
│   └── [other packages]   # 12 additional supporting modules
├── web/                    # Next.js 14 web UI (TypeScript, Tailwind)
├── embedded/                # Go-embedded static assets (web/dist → embedded/web)
├── systemd/                 # Systemd unit files for daemon
├── Makefile                # Hybrid Go + Next.js build
└── docs/                   # Architecture, plans, guides
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Name parsing & regex patterns | `internal/naming/` | Core logic: 925-line advanced.go, release marker stripping |
| File organization | `internal/organizer/` | 677-line organizer.go - refactoring needed |
| Configuration | `internal/config/` | TOML parsing at `~/.config/jellywatch/config.toml` |
| Robust file transfers | `internal/transfer/` | rsync backend, timeout, retry, checksum |
| AI integration | `internal/ai/` | Ollama HTTP client, circuit breaker, keepalive, cache |
| Database operations | `internal/database/` | SQLite WAL mode, conflict detection, migrations |
| Sync service | `internal/sync/` | Hybrid sync: immediate + daily + retry with backoff |
| Sonarr integration | `internal/sonarr/` | API client, commands, series/episode sync |
| Radarr integration | `internal/radarr/` | API client, movie sync |
| Quality detection | `internal/quality/` | Extracts REMUX, BluRay, WEB-DL, resolution, codec |
| Consolidation | `internal/consolidate/` | Finds scattered series, plans consolidation |
| File watching | `internal/watcher/` | fsnotify-based download monitoring |
| Daemon service | `internal/daemon/` | Systemd integration, signal handling, background ops |
| Web UI | `web/` | Next.js 14, TypeScript, shadcn/ui components |
| Library selection | `internal/library/` | Picks target drive based on space/content |
| Path migration | `internal/migration/` | Detects Sonarr/Radarr path mismatches |
| Path resolution | `internal/paths/` | Sudo-aware home/config paths |
| Privilege escalation | `internal/privilege/` | Auto sudo via syscall.Exec |

## AGENT-SPECIFIC GUIDANCE

### REQUIRED WORKFLOW

**MANDATORY** - Always follow this three-step workflow for any code changes:

```
1. brainstorming
   └─> Understand user intent and requirements
   └─> Ask clarifying questions one at a time
   └─> Explore alternatives before settling on approach
   └─> DO NOT write code or make changes yet

2. writing-plans
   └─> Create detailed implementation plan
   └─> Break down into atomic tasks
   └─> Specify verification steps
   └─> DO NOT execute any code yet

3. executing-plans
   └─> Execute plan task-by-task with checkpoints
   └─> Verify each step before proceeding
   └─> Report progress at checkpoints
   └─> Commit after verification passes
```

**CRITICAL RULES:**
- **NEVER** skip brainstorming - always explore intent first
- **NEVER** skip writing-plans - always create detailed plan first
- **NEVER** write code before plan is written and reviewed
- **ALWAYS** use executing-plans with the plan, not ad-hoc implementation

### Core Principles
1. **Jellyfin Compliance First** - All operations must result in files that Jellyfin can correctly identify and scrape
2. **Data Preservation** - Never delete source files without explicit confirmation (`delete_source` config)
3. **Pattern Recognition** - The naming package has extensive regex patterns for release markers, quality indicators, metadata
4. **Quality Awareness** - Higher quality versions preserved when duplicates found (REMUX > BluRay > WEB-DL)
5. **Robust File Ops** - Use rsync with timeout/retry, never `os.Rename()` or `io.Copy()`

### Field Experience Lessons
1. **RAR Archives**: Folders with many .rar files → check for extraction before discarding
2. **Sample Files**: Folders with only sample files (<100MB) → remove if complete version exists
3. **Incomplete Archives**: Folders with only .rar parts but no complete movie → remove
4. **Obfuscated Names**: Random character names common; if folder >600MB, keep and parse the movie
5. **Special Characters**: Handle apostrophes, colons, etc. by removing/replacing: `< > : " / \ | ? *`
6. **Release Groups**: Many legitimate movies have release group markers (`-RARBG`, `-YTS`) that must be stripped
7. **Quality Indicators**: Look for `REMUX`, `BluRay`, `WEB-DL`, `1080p`, `2160p` when determining duplicates

### Release Group Patterns to Remove
- Resolution: `1080p`, `720p`, `2160p`, `4K`, `UHD`
- Source: `BluRay`, `Blu-ray`, `BDRip`, `REMUX`, `WEB-DL`, `WEBDL`, `WEBRip`, `HDTV`
- Codec: `x264`, `x265`, `HEVC`, `H264`, `H265`, `AVC`, `AV1`
- Audio: `DDP5.1`, `AAC2.0`, `AC3`, `TrueHD`, `Atmos`, `DTS-HD.MA`
- HDR: `Dolby.Vision`, `DoVi`, `HDR10`, `HDR10+`, `DV`
- Streaming sources: `AMZN`, `NF` (Netflix), `ATVP` (Apple TV+), `HULU`

### Quality Hierarchy
When encountering duplicates, preserve in priority order:
1. **REMUX** (highest quality, direct bluray rip)
2. **BluRay / BDRip** (bluray source)
3. **WEB-DL** (streaming service download)
4. **WEBRip** (stream capture)
5. **HDTV** (broadcast capture)
6. **DVDRip** (dvd source)

Resolution priority: `2160p/4K` > `1080p` > `720p`

## CONVENTIONS
- **Hybrid build**: Makefile builds Next.js first, copies to `embedded/web/`, then compiles Go binaries
- **File transfers**: Use `internal/transfer` package with rsync backend (`--timeout` flag). Never `os.Rename()` or `io.Copy()`.
- **Release marker stripping**: See `internal/naming/init()` for extensive regex list covering resolution, codec, audio, HDR, streaming sources.
- **Daemon via systemd**: Background service with `systemd/jellywatchd.service`, runs as root for CAP_CHOWN capability.
- **CLI built with Cobra**: Command structure in `cmd/jellywatch/`.
- **Configuration in TOML**: User config at `~/.config/jellywatch/config.toml`.
- **Database**: SQLite with WAL mode for concurrency.
- **Testing**: Standard Go `*_test.go` pattern. 61 test files. Custom `test-all.sh` for gap-focused testing.

## ANTI-PATTERNS (THIS PROJECT)

**NEVER** delete source files without explicit `delete_source = true` config. Data preservation is paramount.

**NEVER** use: `os.Rename()`, `io.Copy()` for file transfers - they hang indefinitely on failing disks. Use `internal/transfer` package.

**NEVER** skip the 3-phase workflow: brainstorming → writing-plans → executing-plans.

**NEVER** update git config during commits, never run destructive git commands or force push to main/master without explicit request.

**AVOID** hardcoded quality hierarchies - use the regex-based `internal/naming` detection system.

**ALWAYS** use rsync for file transfers with timeout handling.

## UNIQUE STYLES

- **Timeout-aware file ops**: All transfers include timeout and retry logic via rsync backend.
- **Jellyfin compliance first**: All naming must match Jellyfin's strict standards.
- **Quality-aware deduplication**: When duplicates found, preserve higher quality per hierarchy.
- **Circuit breaker pattern**: AI integration uses circuit breaker to prevent cascading failures.
- **Keepalive pings**: AI client sends periodic keepalive to maintain Ollama connection.
- **Hybrid sync**: Database → Sonarr/Radarr sync with immediate, daily, and retry-with-backoff strategies.
- **Embedded web UI**: Next.js built separately, then Go-embedded for single-binary deployment.

## COMPLEXITY HOTSPOTS

### Critical Refactoring Needed

**internal/organizer/organizer.go** (677 lines)
- `OrganizeMovie()`: ~120 lines, high cyclomatic complexity, 15+ conditionals
- `OrganizeTVEpisode()`: ~140 lines, similar complexity
- Duplicated logic (~60%) between both methods
- Mixed concerns (parsing, transfer, DB updates)

### Moderate Issues

**internal/naming/advanced.go** (925 lines)
- Complex regex parsing and deobfuscation logic
- Likely contains oversized functions

**internal/scanner/scanner.go** (513 lines)
- Multiple scanning methods with file system walk logic

**internal/plans/plans.go** (556 lines)
- Duplicate structure definitions (DuplicatePlan vs ConsolidatePlan)
- Similar generation logic repeated per plan type

### Architectural Issues

| Issue | Location | Impact |
|-------|----------|--------|
| Massive duplicated functions | organizer.go | High - maintenance burden |
| Mixed concerns | organizer.go | Medium - testing difficulty |
| Giant TOML generator | config.go | Low - pure string building |
| No common plan interface | plans.go | Medium - extensibility |

## COMMANDS

```bash
# Build
make all          # Full build: deps + frontend + binaries
make build         # Go binaries only (requires make frontend first)
make frontend       # Build Next.js, copy to embedded/web/
make dev           # Run daemon + Next.js dev server
make clean         # Remove build artifacts

# Run CLI
./jellywatch scan                    # Index your library
./jellywatch watch /downloads          # Watch for new files
./jellywatch organize /library         # Fix existing files
./jellywatch audit generate           # Find low-confidence parses
./jellywatch duplicates generate       # Find duplicates
./jellywatch consolidate generate      # Find scattered series
./jellywatch migrate                 # Sync Sonarr/Radarr paths

# Run tests
go test ./...                         # All tests
./test-all.sh                          # Gap-focused test runner

# Daemon management
sudo systemctl start jellywatchd
systemctl status jellywatchd
journalctl -u jellywatchd -f
```

## NOTES

- **Movie format**: `Movies/Movie Name (YYYY)/Movie Name (YYYY).ext` (year in parentheses)
- **TV format**: `TV Shows/Show Name (Year)/Season 01/Show Name (Year) S01E01.ext`
- **Folder size heuristic**: >600MB = likely movie, >50MB = likely TV episode
- **Special characters**: Removed: `< > : " / \ | ? *`
- **Daemon permissions**: Must run as root for CAP_CHOWN (file ownership). See AGENTS.md files in internal packages for details on specific patterns.
- **Web UI**: Embedded in Go binary via `embed.FS`. No separate web server process.
- **No CI/CD**: Project lacks GitHub Actions workflows. Consider adding `ci.yml` and `release.yml`.
- **Auto sudo**: Commands requiring root automatically re-exec with sudo. Uses `internal/privilege` package.

## Agent Types for This Project

- **explore**: For finding usage patterns of naming functions, config structures
- **librarian**: For looking up Jellyfin documentation and Go best practices
- **oracle**: For architecture decisions about file handling, naming logic, or AI integration
- **review**: For security review of file operation code

See subdirectory AGENTS.md files for package-specific guidance.
