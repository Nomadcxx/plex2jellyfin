package labeling

import (
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

// hasProviderID returns true when at least one external ID has been resolved
// for the decision.
func hasProviderID(dec database.ParseDecision) bool {
	return dec.JellyfinItemID != "" ||
		dec.JellyfinImdbID != "" ||
		dec.JellyfinTmdbID != "" ||
		dec.JellyfinTvdbID != ""
}

// DeriveLabel computes an auto-label for a ParseDecision given the current
// Jellyfin item name and an age TTL:
//
//   - PASS  – provider ID resolved and parsed title fuzzy-matches jellyfinName.
//   - DRIFT – provider ID resolved, jellyfinName non-empty, but titles differ.
//   - FAIL  – no provider ID and the decision is older than ttl.
//   - ""    – not enough information yet (inside TTL, or name unavailable).
func DeriveLabel(dec database.ParseDecision, jellyfinName string, ttl time.Duration) string {
	if hasProviderID(dec) {
		if jellyfinName == "" {
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
