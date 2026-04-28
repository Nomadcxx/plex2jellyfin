package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// obfuscatedHashName matches SAB temp-hash filenames such as
// "SXvWQZqPGRTeZvy6oGudBsA2FUBH1HUd.mkv": 20+ alphanumeric chars and a
// recognised media extension. SAB renames these to a real release name once
// post-processing finishes; deferring spares us a parse attempt and keeps the
// log clean.
var obfuscatedHashName = regexp.MustCompile(`^[A-Za-z0-9]{20,}\.(mkv|mp4|avi|m4v|mov|wmv|flv|ts|webm)$`)

// parentFolderHasMarkers checks whether the immediate parent directory looks
// like a real release folder (SxxExx, season pack, year, etc.). When true,
// the parser can extract show/movie info from the folder name even though the
// file itself is obfuscated, so we should NOT defer.
var parentFolderHasMarkers = regexp.MustCompile(`(?i)(s\d{1,2}e\d{1,3}|season[._\s-]?\d+|\b(19|20)\d{2}\b|\bcomplete\b|\bs\d{2}\b)`)

// IsObfuscatedSABFilename reports whether the basename looks like a SAB
// temp-hash file with no parseable context in its parent folder. Files whose
// parent folders carry valid release markers are not considered SAB
// temp-hashes and should fall through to the normal parser.
func IsObfuscatedSABFilename(path string) bool {
	base := filepath.Base(path)
	if !obfuscatedHashName.MatchString(base) {
		return false
	}
	parent := filepath.Base(filepath.Dir(path))
	if parentFolderHasMarkers.MatchString(parent) {
		return false
	}
	return true
}

// NegativeCache defers re-processing of files that fail with deterministic,
// non-recoverable parse/organize errors. The periodic scanner re-walks watch
// folders every few minutes; without this cache, a single unparseable file
// (obfuscated SAB temp hash, season-pack-only release with no episode marker,
// release like "Crime 101" / "Catch.22" with a numeric token confused for an
// episode code) generates an unbounded retry loop.
//
// Entries expire on an exponential backoff. After the backoff window passes,
// the file gets exactly one more chance to parse; if it fails again, the
// failure count increments and the next defer is longer.
//
// Cache entries are also evicted when the source file disappears (renamed by
// SAB post-processing, deleted by user, or moved by another tool).
type NegativeCache struct {
	mu      sync.Mutex
	entries map[string]*negEntry
}

type negEntry struct {
	failedAt time.Time
	failures int
	errMsg   string
}

// backoffSchedule returns how long to defer a path after `failures` failures.
// 1st: 30 min, 2nd: 2h, 3rd: 12h, 4th+: 24h.
func backoffSchedule(failures int) time.Duration {
	switch failures {
	case 0, 1:
		return 30 * time.Minute
	case 2:
		return 2 * time.Hour
	case 3:
		return 12 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// NewNegativeCache returns an initialized NegativeCache.
func NewNegativeCache() *NegativeCache {
	return &NegativeCache{entries: make(map[string]*negEntry)}
}

// IsDeferred reports whether `path` is currently within its defer window.
// As a side effect it evicts entries whose source file no longer exists
// (because rename/delete makes the original key meaningless).
func (n *NegativeCache) IsDeferred(path string) (bool, time.Duration, string) {
	if n == nil {
		return false, 0, ""
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	e, ok := n.entries[path]
	if !ok {
		return false, 0, ""
	}
	if _, err := os.Stat(path); err != nil {
		delete(n.entries, path)
		return false, 0, ""
	}
	wait := backoffSchedule(e.failures)
	elapsed := time.Since(e.failedAt)
	if elapsed >= wait {
		return false, 0, ""
	}
	return true, wait - elapsed, e.errMsg
}

// Record marks `path` as having failed with `errMsg`. Increments the failure
// count if an entry already exists, extending the backoff window.
func (n *NegativeCache) Record(path, errMsg string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	e, ok := n.entries[path]
	if !ok {
		e = &negEntry{}
		n.entries[path] = e
	}
	e.failedAt = time.Now()
	e.failures++
	e.errMsg = errMsg
}

// Forget removes `path` from the cache (e.g. after a successful organize on a
// previously-deferred path, or from an admin clear-cache action).
func (n *NegativeCache) Forget(path string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.entries, path)
}

// Snapshot returns a stable copy of current entries, suitable for surfacing
// via the API. Each entry includes the path, current failure count, last
// error, time of last attempt, and remaining backoff.
func (n *NegativeCache) Snapshot() []NegativeCacheEntry {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]NegativeCacheEntry, 0, len(n.entries))
	for path, e := range n.entries {
		wait := backoffSchedule(e.failures)
		remain := wait - time.Since(e.failedAt)
		if remain < 0 {
			remain = 0
		}
		out = append(out, NegativeCacheEntry{
			Path:           path,
			Failures:       e.failures,
			LastError:      e.errMsg,
			LastAttemptAt:  e.failedAt,
			DeferUntil:     e.failedAt.Add(wait),
			RemainingDefer: remain,
		})
	}
	return out
}

// NegativeCacheEntry is a snapshot of a single cache row.
type NegativeCacheEntry struct {
	Path           string        `json:"path"`
	Failures       int           `json:"failures"`
	LastError      string        `json:"last_error"`
	LastAttemptAt  time.Time     `json:"last_attempt_at"`
	DeferUntil     time.Time     `json:"defer_until"`
	RemainingDefer time.Duration `json:"remaining_defer_ns"`
}

// IsDeterministicUnparseable returns true when an organize/parse error is
// deterministic and non-recoverable — i.e. retrying without a code change
// will produce the same error. Matching is substring-based to be resilient
// to error-wrapping changes.
func IsDeterministicUnparseable(errMsg string) bool {
	if errMsg == "" {
		return false
	}
	msg := strings.ToLower(errMsg)
	patterns := []string{
		"could not extract tv show info from path",
		"obfuscated filename, no episode markers",
		"no episode markers in parent folders",
		"unable to parse tv show name",
		"could not extract movie info from path",
		"unable to parse movie name",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
