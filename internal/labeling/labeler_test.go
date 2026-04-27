package labeling_test

import (
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/labeling"
)

const testTTL = 24 * time.Hour

func decWithProviderID(parsedTitle string) database.ParseDecision {
	return database.ParseDecision{
		EventAt:        time.Now().Add(-1 * time.Hour),
		ParsedTitle:    parsedTitle,
		JellyfinItemID: "item-abc",
	}
}

func decNoProviderID(age time.Duration) database.ParseDecision {
	return database.ParseDecision{
		EventAt:     time.Now().Add(-age),
		ParsedTitle: "Some Show",
	}
}

func TestDeriveLabel(t *testing.T) {
	t.Run("provider ID and fuzzy title match returns PASS", func(t *testing.T) {
		dec := decWithProviderID("Tracker")
		got := labeling.DeriveLabel(dec, "Tracker", testTTL)
		if got != "PASS" {
			t.Errorf("got %q, want PASS", got)
		}
	})

	t.Run("provider ID and fuzzy title mismatch returns DRIFT", func(t *testing.T) {
		dec := decWithProviderID("Tracker")
		got := labeling.DeriveLabel(dec, "Breaking Bad", testTTL)
		if got != "DRIFT" {
			t.Errorf("got %q, want DRIFT", got)
		}
	})

	t.Run("no provider ID beyond TTL returns FAIL", func(t *testing.T) {
		dec := decNoProviderID(48 * time.Hour)
		got := labeling.DeriveLabel(dec, "", testTTL)
		if got != "FAIL" {
			t.Errorf("got %q, want FAIL", got)
		}
	})

	t.Run("no provider ID inside TTL returns empty string", func(t *testing.T) {
		dec := decNoProviderID(1 * time.Hour)
		got := labeling.DeriveLabel(dec, "", testTTL)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("provider ID with empty jellyfinName returns empty string not DRIFT", func(t *testing.T) {
		dec := decWithProviderID("Tracker")
		got := labeling.DeriveLabel(dec, "", testTTL)
		if got != "" {
			t.Errorf("got %q, want empty string (not DRIFT)", got)
		}
	})

	t.Run("no provider ID just inside TTL returns empty string", func(t *testing.T) {
		// Boundary check: time.Since(EventAt) just below ttl must not yet
		// emit FAIL.  We pick a 100 ms buffer to absorb test-runner jitter
		// while still pinning the inside-the-window semantics.
		dec := decNoProviderID(testTTL - 100*time.Millisecond)
		got := labeling.DeriveLabel(dec, "", testTTL)
		if got != "" {
			t.Errorf("got %q just inside ttl, want empty string", got)
		}
	})

	t.Run("no provider ID just past TTL returns FAIL", func(t *testing.T) {
		// Boundary check: time.Since(EventAt) just above ttl must emit FAIL.
		dec := decNoProviderID(testTTL + 100*time.Millisecond)
		got := labeling.DeriveLabel(dec, "", testTTL)
		if got != "FAIL" {
			t.Errorf("got %q just past ttl, want FAIL", got)
		}
	})
}
