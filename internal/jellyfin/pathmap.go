package jellyfin

import (
	"sort"
	"strings"
)

// PathMapping describes a one-to-one prefix translation between the path
// Jellyfin reports for a media item and the path the daemon writes to disk.
//
// Operators typically run Jellyfin in a container with bind mounts that
// expose the same files under a different root (e.g. host path
// "/mnt/STORAGE5/TVSHOWS" mounted as "/tv5" inside the container). The
// daemon and the sweeper need to translate between these two views in
// order to correlate parse_decisions rows (daemon view) with Jellyfin
// items (Jellyfin view).
type PathMapping struct {
	// Jellyfin is the prefix as reported by the Jellyfin server (the
	// container-internal path).
	Jellyfin string `mapstructure:"jellyfin"`
	// Daemon is the prefix as observed by the jellywatch daemon on the
	// host filesystem.
	Daemon string `mapstructure:"daemon"`
}

// PathTranslator applies a list of PathMappings to translate between the
// Jellyfin view and the daemon view. Mappings are matched longest-prefix
// first to disambiguate nested mounts. A nil translator is a no-op.
type PathTranslator struct {
	jellyfinFirst []PathMapping
	daemonFirst   []PathMapping
}

// NewPathTranslator returns a translator that applies the given mappings.
// Empty or whitespace-only entries are silently skipped. The returned
// translator is safe for concurrent use; mappings are not mutated after
// construction.
func NewPathTranslator(mappings []PathMapping) *PathTranslator {
	cleaned := make([]PathMapping, 0, len(mappings))
	for _, m := range mappings {
		j := strings.TrimRight(strings.TrimSpace(m.Jellyfin), "/")
		d := strings.TrimRight(strings.TrimSpace(m.Daemon), "/")
		if j == "" || d == "" {
			continue
		}
		cleaned = append(cleaned, PathMapping{Jellyfin: j, Daemon: d})
	}
	jf := make([]PathMapping, len(cleaned))
	copy(jf, cleaned)
	sort.SliceStable(jf, func(i, k int) bool {
		return len(jf[i].Jellyfin) > len(jf[k].Jellyfin)
	})
	df := make([]PathMapping, len(cleaned))
	copy(df, cleaned)
	sort.SliceStable(df, func(i, k int) bool {
		return len(df[i].Daemon) > len(df[k].Daemon)
	})
	return &PathTranslator{jellyfinFirst: jf, daemonFirst: df}
}

// JellyfinToDaemon translates a path from the Jellyfin view to the daemon
// view by replacing the longest matching Jellyfin prefix with its Daemon
// counterpart. If no mapping matches, the path is returned unchanged.
func (t *PathTranslator) JellyfinToDaemon(p string) string {
	if t == nil || p == "" {
		return p
	}
	for _, m := range t.jellyfinFirst {
		if hasPathPrefix(p, m.Jellyfin) {
			return m.Daemon + p[len(m.Jellyfin):]
		}
	}
	return p
}

// DaemonToJellyfin translates a path from the daemon view to the Jellyfin
// view by replacing the longest matching Daemon prefix with its Jellyfin
// counterpart. If no mapping matches, the path is returned unchanged.
func (t *PathTranslator) DaemonToJellyfin(p string) string {
	if t == nil || p == "" {
		return p
	}
	for _, m := range t.daemonFirst {
		if hasPathPrefix(p, m.Daemon) {
			return m.Jellyfin + p[len(m.Daemon):]
		}
	}
	return p
}

// hasPathPrefix returns true when p starts with prefix and the next
// character (if any) is a path separator. This avoids matching
// "/mnt/STORAGE5" against "/mnt/STORAGE50/...".
func hasPathPrefix(p, prefix string) bool {
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	if len(p) == len(prefix) {
		return true
	}
	return p[len(prefix)] == '/'
}
