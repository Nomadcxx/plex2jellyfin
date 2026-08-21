package labeling

import (
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

// DefaultTTL is how long a decision may stay unlabeled before the labeler
// settles the absence of metadata as a failure.
const DefaultTTL = 7 * 24 * time.Hour

// hasProviderID returns true when at least one external ID has been resolved
// for the decision.
func hasProviderID(dec database.ParseDecision) bool {
	return dec.JellyfinImdbID != "" ||
		dec.JellyfinTmdbID != "" ||
		dec.JellyfinTvdbID != ""
}

// DeriveLabel computes an auto-label for a ParseDecision given the current
// Jellyfin item name and an age TTL:
//
//   - PASS  – provider ID resolved and parsed title fuzzy-matches jellyfinName.
//   - DRIFT – provider ID resolved, jellyfinName non-empty, but titles differ.
//   - VANISHED – provider ID resolved, Jellyfin no longer has the item, and the
//     decision is older than ttl.
//   - FAIL  – no provider ID and the decision is older than ttl.
//   - ""    – not enough information yet (inside TTL, or name unavailable).
func DeriveLabel(dec database.ParseDecision, jellyfinName string, ttl time.Duration) string {
	if hasProviderID(dec) {
		if jellyfinName == "" {
			if time.Since(dec.EventAt) > ttl {
				return "VANISHED"
			}
			return ""
		}
		if FuzzyTitleEqual(dec.ParsedTitle, jellyfinName) {
			return "PASS"
		}
		return "DRIFT"
	}

	if time.Since(dec.EventAt) > ttl {
		return "FAIL"
	}
	return ""
}
