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
// page returns fewer rows than the limit, guaranteeing that more than 1 000
// labeled rows in the database never hide unlabeled ones.
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
			}
		}

		if len(rows) < pageSize {
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
