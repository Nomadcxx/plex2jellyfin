# Agent Brief: Jellyfin Cross-Drive Show Grouping — Explore & Recommend

## Your Task

Explore how JellyWatch can solve Jellyfin's cross-drive show grouping problem. Read the existing research, understand the codebase, evaluate the known approaches, and look for alternatives we may have missed. Return a structured recommendation covering all viable options ranked by effort and durability.

---

## The Problem

Jellyfin identifies TV shows by folder name, not by TMDB/TVDB ID. This causes two failure modes:

1. **Cross-drive split:** Season 2 of *The Simpsons* on `/media/drive1/` and Season 3 on `/media/drive2/` appear as two separate shows in the Jellyfin library.
2. **Casing split:** A folder named `the simpsons` and another named `The Simpsons` on the same drive are treated as different shows.

Plex avoids both by grouping on IMDB/TMDB ID regardless of folder structure. Jellyfin does not — issue [#904](https://github.com/jellyfin/jellyfin/issues/904) has been open since 2019 and was explicitly closed as "not planned" at the core level.

JellyWatch already fixes naming compliance (making folders Jellyfin-compliant). This feature would fix the grouping problem that naming compliance alone cannot solve.

---

## Read First

**Research report** (completed — read this before exploring anything else):
```
/home/nomadx/Documents/jellywatch/docs/jellyfin-plugin-research.md
```

This covers every known plugin that touches the problem, how Jellyfin's internal grouping key (`SeriesPresentationUniqueKey`) works, what the plugin API can and cannot do, and all relevant open GitHub issues.

---

## The Codebase

**Repo root:** `/home/nomadx/Documents/jellywatch/`

Key areas to understand before forming recommendations:

| Path | What it does |
|---|---|
| `plugin/` | Existing C# Jellyfin plugin (v1.1.0, targets 10.11) — already installed alongside JellyWatch |
| `plugin/JellyWatchPlugin.cs` | Plugin entry point |
| `plugin/EventHandlers/EventForwarder.cs` | Forwards Jellyfin events to JellyWatch |
| `plugin/Api/JellyWatchController.cs` | Custom `/jellywatch/*` endpoints in Jellyfin |
| `internal/jellyfin/` | Go Jellyfin API client (library refresh, item queries, remediation) |
| `internal/housekeeping/engine.go` | Detect → drain housekeeping engine; existing task kinds here |
| `internal/tmdb/verifier.go` | TMDB verifier — JellyWatch already resolves TMDB IDs for media items |
| `internal/consolidate/` | Consolidation logic — moves/merges files across paths |

---

## Known Approach to Explore

One approach has already been identified as a candidate — **explore it, but do not treat it as the only option**:

**Go daemon housekeeping task:** Add a new task kind to the existing housekeeping engine (`internal/housekeeping/engine.go`) that detects series split across multiple Jellyfin library entries (same TMDB/TVDB ID, different Jellyfin items), then calls Jellyfin's `/Items/MergeVersions` REST endpoint to link them as alternate versions. JellyWatch already has the TMDB knowledge and the Jellyfin API client to do this with minimal new code.

Assess this honestly — including its known weakness: duplicate Series rows survive in Jellyfin's DB and may re-appear on re-scan.

---

## What We Want From You

**Do not just evaluate the known approach.** Actively look for alternatives we have not considered, including but not limited to:

- Whether the existing C# plugin (`plugin/`) could be extended with an `ILibraryPostScanTask` or `IScheduledTask` to do something more durable
- Whether JellyWatch's consolidation engine (`internal/consolidate/`) could emit a canonical on-disk layout (symlinks or moves) that makes the problem disappear before Jellyfin even scans
- Whether NFO sidecar files, a custom metadata provider, or any other Jellyfin-side mechanism could enforce consistent provider IDs and prevent the split from occurring
- Any approach that touches the problem from a completely different angle

For each option you identify (known or new), assess:
1. **How durable is it?** Does it survive Jellyfin re-scans, or is it undone on the next scan?
2. **How much new code?** Estimate relative effort (small/medium/large)
3. **What are the failure modes?** Edge cases, known API instabilities, platform limits
4. **Where does it live?** Go daemon, C# plugin, filesystem, Jellyfin API call, or some combination
5. **Is it user-visible?** Does it require the user to set up a staging directory, add a library, or is it transparent?

Return a ranked recommendation table followed by a short write-up on your top pick and why. Be honest about trade-offs.

---

## Constraints

- Do not modify Jellyfin server source — plugin API and REST API only
- Any new housekeeping task must follow the existing `detect → enqueue → drain` pattern in `engine.go`
- Cross-drive solutions must work across different filesystems (symlinks, not hardlinks)
- The existing C# plugin targets `10.11.0.0` — note any approach that would require bumping this

---

## Success Criteria (for any solution)

- A show split across two drives appears as **one show** in Jellyfin with all seasons visible
- A show split by folder-name casing is handled the same way
- The solution survives Jellyfin library re-scans without re-introducing duplicates
- Grouping uses TMDB/TVDB ID as the primary match key, not folder name

---

*Brief written 2026-05-01. Research: `/home/nomadx/Documents/jellywatch/docs/jellyfin-plugin-research.md`*
