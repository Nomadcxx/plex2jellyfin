package labeling

import (
	"fmt"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

const (
	pageSize          = 1000
	defaultTTL        = 7 * 24 * time.Hour
	defaultStaleAfter = 14 * 24 * time.Hour
)

// JellyfinNameFetcher resolves a Jellyfin item ID to the item's display name.
type JellyfinNameFetcher func(itemID string) (string, error)

// Runner processes unlabeled ParseDecision rows and writes auto-labels.
type Runner struct {
	db          *database.MediaDB
	getName     JellyfinNameFetcher
	ttl         time.Duration
	staleAfter  time.Duration
	stalePageSz int
}

// NewRunner returns a Runner with the default 7-day TTL and 14-day stale
// re-evaluation window.
func NewRunner(db *database.MediaDB, getName JellyfinNameFetcher) *Runner {
	return &Runner{
		db:          db,
		getName:     getName,
		ttl:         defaultTTL,
		staleAfter:  defaultStaleAfter,
		stalePageSz: pageSize,
	}
}

// SetStaleAfter overrides the stale re-evaluation window (primarily for tests).
func (r *Runner) SetStaleAfter(d time.Duration) { r.staleAfter = d }

// RunOnce queries all unlabeled rows (AutoLabelIsNull), derives a label for
// each, and persists non-empty labels.  It then re-evaluates any rows whose
// existing label was written more than staleAfter ago so that Jellyfin renames
// or late provider-ID resolutions are reflected in the auto-label.
func (r *Runner) RunOnce() error {
	var firstErr error

	for {
		rows, err := r.db.QueryDecisions(database.QueryFilter{
			AutoLabelIsNull: true,
			Limit:           pageSize,
		})
		if err != nil {
			return fmt.Errorf("labeling runner query: %w", err)
		}

		labeled := 0
		for _, dec := range rows {
			computedAt := time.Now().UTC()
			label, err := r.labelOne(dec)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if label == "" {
				continue
			}
			if err := r.db.UpdateAutoLabelAt(dec.ID, label, computedAt); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("UpdateAutoLabel id=%d: %w", dec.ID, err)
				}
				continue
			}
			labeled++
		}

		if len(rows) < pageSize {
			break
		}
		// Full page returned but nothing was labeled — the next query
		// would return the same rows.  Stop and let the next tick retry.
		if labeled == 0 {
			break
		}
	}

	if err := r.runStalePass(); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// runStalePass re-derives labels on rows whose label is older than staleAfter
// and overwrites them in place so the auditor sees the current ground truth.
func (r *Runner) runStalePass() error {
	if r.staleAfter <= 0 {
		return nil
	}
	rows, err := r.db.QueryStaleLabeledDecisions(r.staleAfter, r.stalePageSz)
	if err != nil {
		return fmt.Errorf("labeling stale query: %w", err)
	}
	var firstErr error
	for _, dec := range rows {
		computedAt := time.Now().UTC()
		label, err := r.labelOne(dec)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if label == "" {
			continue
		}
		if label == dec.AutoLabel {
			// Touch the timestamp so we don't re-process this row every
			// tick once it's stable.
			if err := r.db.UpdateAutoLabelAt(dec.ID, label, computedAt); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("UpdateAutoLabel(stale, unchanged) id=%d: %w", dec.ID, err)
			}
			continue
		}
		if err := r.db.UpdateAutoLabelAt(dec.ID, label, computedAt); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("UpdateAutoLabel(stale) id=%d: %w", dec.ID, err)
		}
	}
	return firstErr
}

func (r *Runner) labelOne(dec *database.ParseDecision) (string, error) {
	// Re-fetch the row immediately before labeling to close the TOCTOU
	// window between the bulk query in RunOnce and the per-row decision
	// here.  The Jellyfin sweeper writes provider IDs concurrently, and
	// stale data in `dec` would cause us to label a row FAIL when a
	// provider ID has just been resolved.
	if r.db != nil {
		fresh, err := r.db.GetDecision(dec.ID)
		if err != nil {
			return "", fmt.Errorf("refetch decision id=%d: %w", dec.ID, err)
		}
		if fresh != nil {
			dec = fresh
		}
	}

	if !hasProviderID(*dec) {
		return DeriveLabel(*dec, "", r.ttl), nil
	}

	name, err := r.getName(dec.JellyfinItemID)
	if err != nil {
		return "", fmt.Errorf("getName %q: %w", dec.JellyfinItemID, err)
	}

	return DeriveLabel(*dec, name, r.ttl), nil
}
