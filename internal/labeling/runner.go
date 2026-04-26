package labeling

import (
	"fmt"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

const (
	pageSize   = 1000
	defaultTTL = 7 * 24 * time.Hour
)

// JellyfinNameFetcher resolves a Jellyfin item ID to the item's display name.
type JellyfinNameFetcher func(itemID string) (string, error)

// Runner processes unlabeled ParseDecision rows and writes auto-labels.
type Runner struct {
	db      *database.MediaDB
	getName JellyfinNameFetcher
	ttl     time.Duration
}

// NewRunner returns a Runner with the default 7-day TTL.
func NewRunner(db *database.MediaDB, getName JellyfinNameFetcher) *Runner {
	return &Runner{db: db, getName: getName, ttl: defaultTTL}
}

// RunOnce queries all unlabeled rows (AutoLabelIsNull), derives a label for
// each, and persists non-empty labels.  It loops in pages of 1 000 until a
// page returns fewer rows than the limit or until a full page yields zero
// label writes (meaning every remaining row is currently un-derivable —
// e.g. inside the TTL window with no resolved provider ID — so re-querying
// would just return the same page forever).
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
			if err := r.db.UpdateAutoLabel(dec.ID, label); err != nil {
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

	return firstErr
}

func (r *Runner) labelOne(dec *database.ParseDecision) (string, error) {
	if !hasProviderID(*dec) {
		return DeriveLabel(*dec, "", r.ttl), nil
	}

	name, err := r.getName(dec.JellyfinItemID)
	if err != nil {
		return "", fmt.Errorf("getName %q: %w", dec.JellyfinItemID, err)
	}

	return DeriveLabel(*dec, name, r.ttl), nil
}
