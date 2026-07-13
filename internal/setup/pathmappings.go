package setup

import (
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
)

// DocsPathMappingsURL is the published guide for [[jellyfin.path_mappings]].
const DocsPathMappingsURL = "https://nomadcxx.github.io/plex2jellyfin/docs/getting-started/path-mappings/"

// PathMappingsWhat explains what path mappings are (CLI/TUI/web share this story).
const PathMappingsWhat = "Jellyfin in Docker (or any bind mount) often sees library roots as /movies1, /tv1, etc. Plex2Jellyfin organizes on the host as /mnt/.../MOVIES. Path mappings translate between those views."

// PathMappingsWhy explains why they matter for the feedback loop.
const PathMappingsWhy = "Without mappings, organizes still succeed, but plugin webhooks arrive with Jellyfin paths that never match parse_decisions targets. jellyfin_item_id stays empty, so PASS/DRIFT/FAIL labeling never runs."

// PathMappingsHow explains how to fill each row.
const PathMappingsHow = "Add one row per library root: Jellyfin path (container) → P2J path (host). Example: /movies1 → /mnt/STORAGE1/MOVIES. Longest prefix wins when roots nest."

// PathMappingsWhenSkip explains when mappings are unnecessary.
const PathMappingsWhenSkip = "Skip only when Jellyfin Locations already match your [libraries] paths (same layout on both sides)."

// PathMappingsVerify explains how to confirm mappings work.
const PathMappingsVerify = "Re-test Jellyfin until no unmapped roots remain. Daemon log should show \"Jellyfin path mappings configured\". Webhook activity should resolve to host paths under [libraries]."

// PathMappingsLines returns the shared explanation as separate lines for CLI/TUI printing.
func PathMappingsLines() []string {
	return []string{
		PathMappingsWhat,
		PathMappingsWhy,
		PathMappingsHow,
		PathMappingsWhenSkip,
		PathMappingsVerify,
		"Guide: " + DocsPathMappingsURL,
	}
}

// ToJellyfinPathMappings converts config mappings for jellyfin.UnmappedJellyfinLocations.
func ToJellyfinPathMappings(in []config.JellyfinPathMapping) []jellyfin.PathMapping {
	out := make([]jellyfin.PathMapping, 0, len(in))
	for _, m := range in {
		out = append(out, jellyfin.PathMapping{
			Jellyfin: strings.TrimSpace(m.Jellyfin),
			Daemon:   strings.TrimSpace(m.Daemon),
		})
	}
	return out
}

// LibraryRoots flattens movie + TV library paths for coverage checks.
func LibraryRoots(movies, tv []string) []string {
	out := make([]string, 0, len(movies)+len(tv))
	out = append(out, movies...)
	out = append(out, tv...)
	return out
}

// FindUnmappedJellyfinRoots lists Jellyfin virtual-folder roots that still need
// [[jellyfin.path_mappings]] given the configured library roots.
func FindUnmappedJellyfinRoots(folders []jellyfin.VirtualFolder, movieLibs, tvLibs []string, mappings []config.JellyfinPathMapping) []string {
	return jellyfin.UnmappedJellyfinLocations(folders, LibraryRoots(movieLibs, tvLibs), ToJellyfinPathMappings(mappings))
}
