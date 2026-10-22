package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/library"
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

const maxRetryBackoffFailures = 6

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

// NegativeCache defers files after deterministic failures and until known
// destination capacity is available. The periodic scanner re-walks watch
// folders every few minutes; without this cache, failures can generate an
// unbounded retry loop.
//
// Parse failures use exponential backoff. Resource failures with a known
// destination remain deferred until that destination has enough space; if
// capacity cannot be checked, they fall back to a capped backoff.
//
// Cache entries are also evicted when the source file disappears (renamed by
// SAB post-processing, deleted by user, or moved by another tool).
type NegativeCache struct {
	mu             sync.Mutex
	entries        map[string]*negEntry
	availableSpace func(string) (int64, error)
}

type negEntry struct {
	failedAt        time.Time
	failures        int
	errMsg          string
	resourceFailure bool
	destinationPath string
	requiredBytes   int64
}

// backoffSchedule returns how long to defer a path after `failures` failures.
// 1st: 30m, 2nd: 2h, 3rd: 12h, 4th: 24h, 5th: 7d, 6th+: 30d.
func backoffSchedule(failures int) time.Duration {
	switch failures {
	case 0, 1:
		return 30 * time.Minute
	case 2:
		return 2 * time.Hour
	case 3:
		return 12 * time.Hour
	case 4:
		return 24 * time.Hour
	case 5:
		return 7 * 24 * time.Hour
	default:
		// ponytail: no parser-version fingerprint yet. The 30-day ceiling
		// bounds duplicate rows while parser upgrades still get another
		// attempt. Store the parser version if releases must clear entries.
		return 30 * 24 * time.Hour
	}
}

func resourceBackoffSchedule(failures int) time.Duration {
	switch failures {
	case 0, 1:
		return 30 * time.Minute
	case 2:
		return time.Hour
	case 3:
		return 2 * time.Hour
	case 4:
		return 4 * time.Hour
	default:
		return 6 * time.Hour
	}
}

func (e *negEntry) backoff() time.Duration {
	if e.resourceFailure {
		return resourceBackoffSchedule(e.failures)
	}
	return backoffSchedule(e.failures)
}

// NewNegativeCache returns an initialized NegativeCache.
func NewNegativeCache() *NegativeCache {
	return &NegativeCache{
		entries:        make(map[string]*negEntry),
		availableSpace: library.AvailableSpace,
	}
}

// IsDeferred reports whether `path` is currently blocked by backoff or capacity.
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
	info, err := os.Stat(path)
	if err != nil {
		delete(n.entries, path)
		return false, 0, ""
	}
	// ponytail: capacity retries keyed by a season-pack directory ignore its
	// mtime because rollback changes it; changed contents wait for capacity
	// recovery. Member snapshots can enable immediate invalidation if needed.
	if info.ModTime().After(e.failedAt) && !(info.IsDir() && e.resourceFailure) {
		delete(n.entries, path)
		return false, 0, ""
	}
	if e.resourceFailure && e.destinationPath != "" && e.requiredBytes > 0 && n.availableSpace != nil {
		available, err := n.availableSpace(e.destinationPath)
		if err == nil {
			if available < e.requiredBytes {
				return true, 0, e.errMsg
			}
			delete(n.entries, path)
			return false, 0, ""
		}
	}
	wait := e.backoff()
	elapsed := time.Since(e.failedAt)
	if elapsed >= wait {
		return false, 0, ""
	}
	return true, wait - elapsed, e.errMsg
}

// Record marks `path` as having failed with `errMsg`. Increments the failure
// count if an entry already exists, extending the backoff window.
func (n *NegativeCache) Record(path, errMsg string) {
	n.record(path, errMsg, "", 0)
}

// RecordResourceFailure defers a known byte-capacity failure until its
// destination has room for the source file. Other resource failures and
// unqueryable destinations use the normal bounded backoff.
func (n *NegativeCache) RecordResourceFailure(path, destinationPath string, requiredBytes int64, errMsg string) {
	n.record(path, errMsg, destinationPath, requiredBytes)
}

func (n *NegativeCache) record(path, errMsg, destinationPath string, requiredBytes int64) {
	if n == nil {
		return
	}
	if !IsKnownCapacityShortfall(errMsg) {
		destinationPath = ""
		requiredBytes = 0
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	e, ok := n.entries[path]
	resourceFailure := IsRetryableResourceFailure(errMsg)
	if !ok {
		e = &negEntry{resourceFailure: resourceFailure}
		n.entries[path] = e
	} else if e.resourceFailure != resourceFailure {
		// Backoff counts belong to one error class; a transient storage error
		// must not stretch a later parse failure to the 30-day ceiling.
		e.failures = 0
		e.resourceFailure = resourceFailure
	}
	e.failedAt = time.Now()
	e.destinationPath = destinationPath
	e.requiredBytes = requiredBytes
	// ponytail: counts saturate once parser backoff reaches 30 days; exact lifetime totals need separate storage if operators need them.
	if e.failures < maxRetryBackoffFailures {
		e.failures++
	}
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

// HydrateEntry pre-populates a cache slot from persistent history. Used at
// daemon startup so prior deterministic failures survive restarts; without
// this, a season-pack-only release like Supergirl.S03 (no episode marker)
// produces a fresh attempt every time the daemon comes up, which over a
// day's worth of build-deploys looks like an event-storm in the audit.
func (n *NegativeCache) HydrateEntry(path, errMsg string, failedAt time.Time, failures int) {
	n.HydrateResourceFailure(path, errMsg, failedAt, failures, "", 0)
}

// HydrateResourceFailure restores retry state, including destination capacity
// details, from persisted organize decisions.
func (n *NegativeCache) HydrateResourceFailure(path, errMsg string, failedAt time.Time, failures int, destinationPath string, requiredBytes int64) {
	if n == nil || failures <= 0 {
		return
	}
	if !IsKnownCapacityShortfall(errMsg) {
		destinationPath = ""
		requiredBytes = 0
	}
	if failures > maxRetryBackoffFailures {
		failures = maxRetryBackoffFailures
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.entries[path] = &negEntry{
		failedAt:        failedAt,
		failures:        failures,
		errMsg:          errMsg,
		resourceFailure: IsRetryableResourceFailure(errMsg),
		destinationPath: destinationPath,
		requiredBytes:   requiredBytes,
	}
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
		if _, err := os.Stat(path); os.IsNotExist(err) {
			delete(n.entries, path)
			continue
		}
		wait := e.backoff()
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
		"season_pack_unresolved",
		"extras_unresolved",
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

// IsRetryableResourceFailure identifies transient low-space errors. When the
// destination is known, capacity checks release the retry without waiting for
// the source file to change.
func IsRetryableResourceFailure(errMsg string) bool {
	msg := strings.ToLower(errMsg)
	for _, pattern := range []string{
		"insufficient space",
		"free space is unavailable",
		"not enough space",
		"no space left on device",
	} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// IsKnownCapacityShortfall distinguishes byte-capacity errors from resource
// failures such as exhausted inodes, which free-byte checks cannot resolve.
func IsKnownCapacityShortfall(errMsg string) bool {
	return strings.Contains(strings.ToLower(errMsg), "insufficient space")
}

func IsFailureWorthBackoff(errMsg string) bool {
	return IsDeterministicUnparseable(errMsg) ||
		strings.Contains(strings.ToLower(errMsg), "source is not a regular file") ||
		IsRetryableResourceFailure(errMsg)
}

func retryFailureStreak(recentErrors []string) int {
	if len(recentErrors) == 0 || !IsFailureWorthBackoff(recentErrors[0]) {
		return 0
	}
	resourceFailure := IsRetryableResourceFailure(recentErrors[0])
	failures := 0
	for _, errMsg := range recentErrors {
		if !IsFailureWorthBackoff(errMsg) || IsRetryableResourceFailure(errMsg) != resourceFailure {
			break
		}
		failures++
	}
	return failures
}
