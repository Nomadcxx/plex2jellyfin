# Jellyfin Cross-Drive Grouping: Plugin Research

**Goal:** Build a Jellyfin plugin (or complementary JellyWatch feature) that groups TV shows and movies by TMDB/IMDB ID regardless of physical drive location — matching Plex behavior.

**Problem statement:** Jellyfin splits shows across library roots by folder name. Season 2 of The Simpsons on Drive 1 and Season 3 on Drive 2 appear as two separate shows. Even same-drive splits occur when folder name casing varies (`the simpsons` vs. `The Simpsons`). JellyWatch already fixes naming compliance; this research asks whether a Jellyfin plugin can fix the grouping problem at the server level.

---

## Part 1: Existing Plugins & Prior Art

### 1. jellyfin-plugin-library-unifier
**URL:** https://github.com/tetrahydroc/jellyfin-plugin-library-unifier  
**Foundation score: HIGH — study or fork this first**

The most directly relevant project found. Creates a unified library structure using **symlinks or hardlinks** so that series spread across multiple root folders appear as a single series. Matching is done via metadata provider IDs — TVDB, IMDB, and TMDB — not folder name. If Season 2 is on Drive 1 and Season 3 is on Drive 2, the plugin creates a merged symlink tree pointing into both and Jellyfin sees one unified show.

Matching priority order:
1. TVDB ID (most reliable)
2. IMDB ID
3. TMDB ID
4. Name-based with quality-tag normalization (strips `1080p`, `BluRay`, etc.) as fallback

Architecture: `IScheduledTask` (runs every 24h) + `ILibraryPostScanTask`. Creates symlinks in a dedicated output directory, then triggers a re-scan of that unified root. Appeared in 2024–2025 search results.

**This is the state-of-the-art approach and the most natural codebase to study or extend.**

---

### 2. jellyfin-plugin-mergeversions
**URL:** https://github.com/danieladov/jellyfin-plugin-mergeversions  
**Foundation score: MEDIUM — useful pattern, wrong use case**

Scheduled-task plugin that groups duplicate movies/episodes into Jellyfin's native "alternate versions" UI (a single card with a version picker). The core grouping logic in `MergeVersionsManager.cs` uses `.GroupBy(x => x.ProviderIds["Tmdb"])` — grouping by TMDB ID, not folder name. Items sharing the same TMDB ID become one merged entry.

- Designed for "multiple quality versions of same movie" (1080p vs 4K), not cross-folder season splits
- If Jellyfin has already split `The Simpsons` and `the simpsons` into two separate Series objects, this plugin works at episode/movie level, not series level — it doesn't collapse the two Series entries
- Known bug: issue #24 — different shows incorrectly group together (heuristic fragility)
- Broke on 10.11.0 upgrade, largely working again; known issue #16280 (wrong artwork when merging across libraries with different GUIDs)
- The `.GroupBy(ProviderIds["Tmdb"])` pattern is directly reusable; codebase is ~200 lines of C#

---

### 3. jellyfin-plugin-tmdbboxsets (Official)
**URL:** https://github.com/jellyfin/jellyfin-plugin-tmdbboxsets  
**Foundation score: LOW (TV), REFERENCE (movies)**

Officially maintained plugin that creates Box Set Collections based on TMDB collection IDs (e.g., all Avengers films → "Avengers Collection"). Uses `ICollectionManager` — the correct API for movie franchise grouping. Does not help with TV season grouping across drives, but demonstrates how to call TMDB from a Jellyfin plugin and how to use `ICollectionManager`.

---

### 4. jellyfin-plugin-autoorganize (Archived)
**URL:** https://github.com/jellyfin-archive/jellyfin-plugin-autoorganize  
**Foundation score: LOW — archived, but `TvFolderOrganizer.cs` is useful reference**

Monitored drop folders and moved/renamed new media into the library following Jellyfin naming conventions. Archived February 2022 (too many bugs, no active maintainers). The `TvFolderOrganizer.cs` in the repo is useful as reference for how to rename/reorganize TV folders programmatically via the plugin API.

---

### 5. jellyfin-deduper
**URL:** https://github.com/kristoffersingleton/jellyfin-deduper  
**Foundation score: MEDIUM — algorithm is useful**

External Python script (not a plugin) that scans the library for duplicate media files and moves inferior copies to a trash directory. Two-pass algorithm:
- Pass 1: Group by shared TMDB or TVDB ID (exact match)
- Pass 2: Fuzzy filename matching via `difflib.SequenceMatcher` after stripping quality tokens, lowercasing, and bucketing by first two words. TV episodes must share the same `SxxExx` code.
- Quality ranking: width × height, then bitrate, then file size (via ffprobe; falls back to size)
- Safety: files moved to trash, not deleted; full original path preserved

The two-pass ID-first → fuzzy-name fallback algorithm is directly applicable to JellyWatch's duplicate detection logic.

---

### 6. jellyfin-episode-grouper
**URL:** https://github.com/christian-eriksson/jellyfin-episode-grouper  
**Foundation score: LOW — too narrow scope**

Python script that calls the Jellyfin API to find episodes duplicated within a series (by IMDB ID, TVDB ID, or normalized name) and groups them via Jellyfin's `MergeVersions` API endpoint. Works at the episode level only.

---

### 7. JellyMerger
**URL:** https://github.com/garnajee/JellyMerger  
**Foundation score: LOW**

Web frontend for finding and merging duplicate TV episodes via the official Jellyfin `MergeVersions` API endpoint. TV-only. Focused on episode-level dedup, not show-level folder normalization.

---

### 8. Shokofin (Anime — Advanced Prior Art)
**URL:** https://github.com/ShokoAnime/Shokofin  
**Foundation score: REFERENCE — most architecturally advanced prior art**

A sophisticated Jellyfin plugin that integrates Shoko Server (anime metadata manager) with Jellyfin. Key relevance: it **overrides `SeriesPresentationUniqueKey`** to return a Shoko group ID (backed by AniDB IDs) rather than the folder name. This is the plugin-level identity key override approach — presenting multiple separate AniDB series as one unified Jellyfin show. Also supports a VFS (Virtual File System) mode that creates a virtual directory tree independent of physical file layout.

This is the only known working implementation that overrides Jellyfin's internal grouping key from a plugin. Studying its approach to `IItemResolver` integration and the VFS pattern is valuable even though the codebase is complex and anime-specific.

---

### 9. Jellyfin Core: "Automatically Merge Series Across Multiple Folders"
**Not a plugin — built into Jellyfin server**

A library setting (`EnableAutomaticSeriesGrouping`) that attempts to merge series with the same name across different root paths within a single library.

- Matches on **folder name string only**, not TMDB/TVDB ID
- Fails when names differ in casing or punctuation across drives
- **Currently broken in 10.11.x** — issues #14121 and #16593 confirm it causes duplicates instead of merges (regression from EFCore/jellyfin.db migration in 10.11)
- Only works within a single library's configured root paths, not across different libraries
- Issue #8616 was explicitly closed as "not planned" for a proper fix

This is why a plugin is needed.

---

### Plugin Comparison Table

| Project | Language | Approach | Addresses Cross-Drive Split | Matching Method | Foundation Score |
|---|---|---|---|---|---|
| library-unifier | C# | Symlink tree | YES — directly | TVDB/IMDB/TMDB ID | **HIGH** |
| mergeversions | C# | UI version merge | PARTIAL (episode/movie only) | TMDB ID | MEDIUM |
| tmdbboxsets | C# | Movie BoxSets | NO (movies only) | TMDB collection ID | LOW (TV) |
| autoorganize | C# | Pre-ingest rename | NO (archived) | Name parsing | LOW |
| jellyfin-deduper | Python | File-level dedup | NO (removes files) | TMDB/TVDB ID → fuzzy | MEDIUM |
| Shokofin | C# | VFS + key override | YES (anime only) | AniDB group ID | REFERENCE |
| Core "merge series" | C# | Name-match | PARTIAL (broken) | Folder name string | N/A |

---

## Part 2: Open GitHub Issues Confirming the Gap

These issues confirm the problem is real, long-standing, and unresolved at the Jellyfin core level.

| Issue | Title | Status |
|---|---|---|
| [#904](https://github.com/jellyfin/jellyfin/issues/904) | Single TV show in multiple locations shows duplicate | **Open since Feb 2019** |
| [#2978](https://github.com/jellyfin/jellyfin/issues/2978) | Having seasons at two different places causes duplicate listing | Open |
| [#7711](https://github.com/jellyfin/jellyfin/issues/7711) | Merge folders of the same name in different locations | Closed (workaround documented, root cause unfixed) |
| [#8616](https://github.com/jellyfin/jellyfin/issues/8616) | Can't automatically merge series across multiple folders | **Closed as "not planned"** |
| [#14121](https://github.com/jellyfin/jellyfin/issues/14121) | Auto-merge regression in 10.11 (EFCore migration) | Open |
| [#16593](https://github.com/jellyfin/jellyfin/issues/16593) | Auto-merge not working correctly in 10.11.x | **Open** |
| [#13971](https://github.com/jellyfin/jellyfin/issues/13971) | Jellyfin doesn't merge movie versions from different folders | Open |
| [#787](https://github.com/jellyfin/jellyfin/issues/787) | Album not showing up with different-casing empty folder | Closed (same root cause) |
| [#2187](https://github.com/jellyfin/jellyfin/issues/2187) | Custom resolver plugin support | **Open since 2019, no timeline** |
| [PR #13615](https://github.com/jellyfin/jellyfin/pull/13615) | Support custom resolvers (closed, not merged) | **Closed** |

Feature requests on features.jellyfin.org:
- [Post #1449](https://features.jellyfin.org/posts/1449) — Merge versions across library folders in the same library
- [Post #174](https://features.jellyfin.org/posts/174) — Better handling of movie versions in multiple directories
- [Post #350](https://features.jellyfin.org/posts/350) — Auto group movies with different versions

**Issue #904 is the canonical long-open tracker. It has been open for 7+ years with no roadmap item.**

---

## Part 3: Jellyfin Plugin System Architecture

### Plugin Structure

Jellyfin plugins are **C# class libraries targeting .NET 8**, compiled as DLLs dropped into `{data-dir}/plugins/{PluginName}_{Version}/`. Discovered at startup via reflection — no registration config needed.

Entry point:
```csharp
public class Plugin : BasePlugin<PluginConfiguration>, IHasWebPages
{
    public override string Name => "My Plugin";
    public override Guid Id => new Guid("your-unique-guid-here");
}
```

Full constructor injection is supported. `IPluginServiceRegistrator` lets you register your own services before the DI container is built.

### Available Plugin Hooks

| Interface | When it fires | What it can do |
|---|---|---|
| `ILibraryPostScanTask` | After every library scan completes | Read/modify item metadata, call `UpdateToRepository()` |
| `IScheduledTask` | On demand or cron (admin dashboard) | Full read/write access via `ILibraryManager`, progress reporting |
| `IMetadataProvider<T>` / `IRemoteMetadataProvider<T,TInfo>` | During metadata fetch per item | Return provider IDs (TMDB, TVDB), titles, artwork |
| `IItemResolver` / `IMultiItemResolver` | Path-to-entity resolution pipeline | **NOT officially supported for third-party plugins** |
| `IResolverIgnoreRule` | During scan, per path | Tell scanner to skip specific paths |
| `ICollectionManager` | Any time (injectable) | Create/manage BoxSet collections (movies) |
| `IHostedService` | Background service lifetime | Subscribe to `ILibraryManager.ItemAdded` events |
| `ControllerBase` | REST API | Expose custom HTTP API endpoints |

### ILibraryManager — Key API Surface

The central orchestrator for all library operations.

- `GetItemList(InternalItemsQuery)` — returns items matching a query; supports filtering by `IncludeItemTypes`, `HasTmdbId`, `SeriesPresentationUniqueKey`, etc.
- `UpdateItemAsync(BaseItem, ...)` — persists metadata changes to the database
- `GetCollectionFolders(BaseItem)` — returns the virtual library folders containing an item
- `ValidateMediaLibrary(...)` — triggers a library re-scan

**Stability note:** `GetItemList` was briefly removed/refactored in the 10.11 cycle, breaking the official TMDb Box Sets plugin. Plugin APIs are not stable across minor versions — always pin your `targetAbi` in the manifest.

**Critical limitation:** `InternalItemsQuery` cannot filter by a specific TMDB ID value (e.g., "all Series where TMDB ID = 12345"). It can only assert "has any TMDB ID". Workaround: fetch all series with any TMDB ID, then filter in-process on `item.ProviderIds["Tmdb"]`.

### SeriesPresentationUniqueKey — The Core Grouping Mechanism

Every `Episode` item stores a denormalized `SeriesPresentationUniqueKey` string. All UI views (seasons, Next Up, episode lists) GROUP BY this key. This determines what constitutes "one show" in the Jellyfin UI.

**How it's generated (from `Series.cs`):**
- `EnableAutomaticSeriesGrouping` **off**: key is path-based → each physical folder = separate Series
- `EnableAutomaticSeriesGrouping` **on**: key combines user data keys + language preference + `CollectionFolder` GUIDs of containing libraries

The key is **calculated by Jellyfin's own logic on each scan and is not settable** via a public property. Plugins cannot rewrite it and have it persist without triggering a re-scan — and even then Jellyfin recalculates it using its name-based logic, not provider IDs.

### Data Model: One Directory = One Series

`Series` entity has its `Path` set to a single on-disk directory. There is **no native multi-path concept on the Series entity**. `Season` items are children of that single Series, resolved from subdirectories within the same root. Two physical folders = two Series rows in the DB, period — unless the symlink or re-parenting approach is used to change that.

**Database schema (`jellyfin.db`, `TypedBaseItems` table):**

| Column | Purpose |
|---|---|
| `guid` | Primary key |
| `type` | e.g., `MediaBrowser.Controller.Entities.TV.Series` |
| `Path` | Physical filesystem path (single path per item) |
| `ParentId` | GUID FK to parent item |
| `SeriesPresentationUniqueKey` | Denormalized grouping key |
| `Name` | Display name |
| `ProviderIds` | JSON blob with TMDB/TVDB IDs |

### Plugin Manifest Format

```json
[
  {
    "guid": "your-unique-guid",
    "name": "Plugin Name",
    "overview": "Short description",
    "category": "General",
    "versions": [
      {
        "version": "1.0.0.0",
        "targetAbi": "10.9.0.0",
        "sourceUrl": "https://github.com/.../releases/download/v1.0.0/plugin.zip",
        "checksum": "md5-hash",
        "timestamp": "2024-01-01T00:00:00Z"
      }
    ]
  }
]
```

Distribution is self-hosted — any HTTPS URL hosting this JSON can be added as a plugin repo in the admin panel. No central registry required. The official repo at `https://repo.jellyfin.org/files/plugin/manifest-stable.json` requires review/approval.

---

## Part 4: Technical Feasibility Assessment

### What the Plugin API CAN Do

- Read all `Series` items and their TMDB/TVDB/IMDB provider IDs via `ILibraryManager.GetItemList`
- Identify series sharing a TMDB ID but split across physical folders
- Update metadata fields on items via `UpdateItemAsync`
- Create `BoxSet` collections grouping movies by TMDB collection ID (`ICollectionManager`)
- Create filesystem **symlinks** (cross-drive) or hardlinks (same filesystem) in a unified staging directory
- Trigger a library re-scan via `ValidateMediaLibrary`
- Run all of the above automatically after every library scan via `ILibraryPostScanTask`
- Subscribe to `ItemAdded` events for near-real-time reaction
- Expose custom REST endpoints and admin configuration UI

### What the Plugin API CANNOT Do

- **Cannot override the series resolver** — `IItemResolver` is not hookable by third-party plugins; Jellyfin's built-in resolvers run regardless (PR #13615 closed, issue #2187 open since 2019 with no timeline)
- **Cannot intercept scan results before items are written to the DB** — no pre-commit hook exists; `ILibraryPostScanTask` fires after the write
- **Cannot directly rewrite `SeriesPresentationUniqueKey`** in a way that persists through the next scan; Jellyfin recalculates it by its name-based logic anyway
- **Cannot query items by a specific TMDB ID value** — must fetch-all and filter in-process
- **Cannot make Jellyfin treat two physical Series directories as one Series item** without a backing filesystem change

### Viable Implementation Approaches

#### Approach A: Symlink Unification Plugin (RECOMMENDED)

The architecture used by `jellyfin-plugin-library-unifier`. A `IScheduledTask` + `ILibraryPostScanTask`:

1. Query all `Series` items via `ILibraryManager.GetItemList`, collect `ProviderIds`
2. Group by TMDB ID (primary), TVDB ID (secondary), normalized name (fallback)
3. For each group with 2+ physical paths, create symlinks in a dedicated staging directory (e.g., `/unified-tv/The Simpsons/Season 02/`, `/unified-tv/The Simpsons/Season 03/`)
4. Add the staging directory as a Jellyfin library root (once, at setup time)
5. Trigger a re-scan of that library root via `ValidateMediaLibrary`
6. Jellyfin's native scanner sees all seasons in one folder → one Series, natively

**Pros:** Works within plugin API limits; no internal API abuse; symlinks work cross-drive; survives rescans by design.  
**Cons:** Requires a dedicated "unified" library root; adds a symlink indirection layer; re-scan adds latency after each library scan.

#### Approach B: Post-Scan Re-Parenting via ILibraryPostScanTask (FRAGILE — not recommended standalone)

After each scan: find duplicate Series by TMDB ID → delete N-1 via `DeleteItem()` → re-parent seasons/episodes to the surviving Series via `UpdateToRepository()`.

**Critical problem:** The next scan re-creates deleted Series items from disk (files still exist), undoing the merge. This creates an infinite merge→undo loop. Only viable if combined with Approach A (filesystem must match the desired logical structure).

#### Approach C: Post-Scan API Merge via MergeVersions Endpoint

Query for Series/episodes sharing TMDB/TVDB IDs with different `SeriesPresentationUniqueKey` values → call Jellyfin's `/Items/MergeVersions` API endpoint. Works for movies and episodes as alternate versions. Does not collapse two Series entries into one at the show level.

**Best quick-win:** Doesn't fix the root cause but makes the UI display correctly for most users. Can be implemented from JellyWatch's daemon in Go against the Jellyfin REST API — no C# plugin needed.

#### Approach D: JellyWatch-Side Normalization (No Plugin Required)

JellyWatch already knows TMDB IDs from its verifier. After consolidation, JellyWatch could emit a canonical directory layout (move/symlink files into a single unified tree per show, keyed on TMDB ID) before Jellyfin scans. Jellyfin's native resolver finds all seasons in one place and groups them natively.

**Pros:** Entirely in Go; no C# required; leverages existing JellyWatch TMDB knowledge; no Jellyfin plugin API risk.  
**Cons:** JellyWatch must manage file locations; adds complexity to the consolidation step.

#### Approach E: Plugin-Level Identity Key Override (COMPLEX — Shokofin pattern)

Override `SeriesPresentationUniqueKey` computation via deep `IItemResolver` integration (Shokofin demonstrates this). Requires implementing a VFS that presents a virtual directory tree independent of physical file layout, with series keyed by provider ID rather than folder name.

**Verdict:** Architecturally correct and the "right" long-term fix. However, it requires deep Jellyfin internals knowledge, close version tracking, and risks breaking on minor Jellyfin updates. Not appropriate as a starting point — study Shokofin's approach and revisit once Approach A is proven.

#### Approach F: Requires Jellyfin Server Fork

A proper native fix would modify `SeriesResolver.cs` and `LibraryManager.cs` to group series by TMDB ID rather than directory structure. This is beyond plugin scope. Given that issue #8616 was closed as "not planned," this is unlikely to be merged upstream.

---

## Part 5: Technology Stack

| Component | Requirement |
|---|---|
| Runtime | .NET 8.0 (`net8.0` target in `.csproj`) |
| Core NuGet | `Jellyfin.Model` 10.11.x + `Jellyfin.Controller` 10.11.x |
| Additional NuGet | `Jellyfin.Extensions` 10.11.x |
| Build | `dotnet` SDK 8.0, standard MSBuild |
| License | GPLv3 (plugin binary links GPLv3 Jellyfin assemblies) |
| Key DI interfaces | `ILibraryManager`, `IScheduledTask`, `ILibraryPostScanTask`, `BasePlugin<PluginConfiguration>`, `IPluginServiceRegistrator` |
| Starting point | [jellyfin/jellyfin-plugin-template](https://github.com/jellyfin/jellyfin-plugin-template) |

**Version pinning is critical:** NuGet package versions must exactly match the target Jellyfin server version. A 10.11.8 server requires 10.11.8 NuGet packages.

---

## Part 6: Recommendations

### Immediate (JellyWatch daemon, Go, no C# required)

Add a `MergeVersions` housekeeping task to JellyWatch's daemon that:
1. Queries the Jellyfin REST API for all Series items in the library
2. Groups by TMDB/TVDB ID using JellyWatch's existing verifier data
3. Calls `/Items/MergeVersions` for duplicates

This is a quick win that improves the UI without requiring a Jellyfin plugin or filesystem changes.

### Medium-term (C# plugin)

Fork or extend `jellyfin-plugin-library-unifier` as a production-quality `ILibraryPostScanTask` plugin with:
- TMDB ID as the primary match key (not just TVDB)
- Robust edge case handling: partial seasons, missing provider IDs, symlink staleness cleanup
- Integration with JellyWatch's compliance database via REST
- Configuration UI for staging directory path and match strategy

### Long-term (if plugin approach proves insufficient)

Study Shokofin's VFS + `SeriesPresentationUniqueKey` override technique and build an equivalent for general TMDB/TVDB-based grouping. This is the architecturally correct fix but requires significantly more investment and close Jellyfin version tracking.

### Avoid

- Trying to override `SeriesPresentationUniqueKey` or the resolver pipeline without understanding the Shokofin approach fully — the plugin API officially does not support this
- Post-scan re-parenting without backing filesystem changes — it will be undone by the next scan
- Forking the Jellyfin server — maintenance burden is too high and PRs fixing this have been rejected

---

## Key References

| Resource | URL |
|---|---|
| library-unifier (best fork candidate) | https://github.com/tetrahydroc/jellyfin-plugin-library-unifier |
| mergeversions (TMDB GroupBy pattern) | https://github.com/danieladov/jellyfin-plugin-mergeversions |
| tmdbboxsets (ICollectionManager reference) | https://github.com/jellyfin/jellyfin-plugin-tmdbboxsets |
| Shokofin (VFS + key override) | https://github.com/ShokoAnime/Shokofin |
| jellyfin-deduper (two-pass ID→fuzzy algo) | https://github.com/kristoffersingleton/jellyfin-deduper |
| Plugin template | https://github.com/jellyfin/jellyfin-plugin-template |
| Plugin system docs | https://jellyfin.org/docs/general/server/plugins/ |
| Library scanning internals | https://deepwiki.com/jellyfin/jellyfin/2.4-file-system-and-library-scanning |
| Plugin system internals | https://deepwiki.com/jellyfin/jellyfin/8-plugin-system |
| Canonical open issue | https://github.com/jellyfin/jellyfin/issues/904 |
| Current 10.11 auto-merge regression | https://github.com/jellyfin/jellyfin/issues/16593 |
| Feature request: cross-folder merge | https://features.jellyfin.org/posts/1449 |
| NuGet: Jellyfin.Model | https://www.nuget.org/packages/Jellyfin.Model |
| NuGet: Jellyfin.Controller | https://www.nuget.org/packages/Jellyfin.Controller |

---

*Research completed 2026-05-01. Four parallel agents: plugin survey ×2, plugin system architecture, feasibility deep-dive.*
