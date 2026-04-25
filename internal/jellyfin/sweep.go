package jellyfin

import (
	"fmt"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

const (
	sweepPageSize      = 200
	sweepAutoLabelFail = "FAIL"
)

// Sweeper reconciles unresolved parse_decisions rows against the Jellyfin
// library by enumerating items and matching by Path. Rows that remain
// unresolved past the TTL are auto-labeled as FAIL.
type Sweeper struct {
	client *Client
	db     *database.MediaDB
}

// NewSweeper constructs a Sweeper over the given Jellyfin client and database.
func NewSweeper(client *Client, db *database.MediaDB) *Sweeper {
	return &Sweeper{client: client, db: db}
}

// RunOnce performs a single sweep pass: it walks the Jellyfin library and
// resolves any unresolved decisions whose target_path matches a Jellyfin
// item's Path within the lookback window, then labels long-unresolved rows
// as FAIL when older than the TTL.
func (s *Sweeper) RunOnce(lookback, ttl time.Duration) error {
	if s == nil || s.client == nil || s.db == nil {
		return nil
	}
	now := time.Now().UTC()
	since := now.Add(-lookback)
	ttlCutoff := now.Add(-ttl)

	rows, err := s.db.QueryDecisions(database.QueryFilter{
		JellyfinUnresolved: true,
		TargetPathNotEmpty: true,
		EventAfter:         &since,
		Limit:              500,
	})
	if err != nil {
		return fmt.Errorf("sweep query: %w", err)
	}

	pathMap := make(map[string]*database.ParseDecision, len(rows))
	for _, row := range rows {
		pathMap[row.TargetPath] = row
	}

	if len(pathMap) > 0 {
		if err := s.sweepByPath(pathMap); err != nil {
			return err
		}
	}

	ttlRows, err := s.db.QueryDecisions(database.QueryFilter{
		JellyfinUnresolved: true,
		TargetPathNotEmpty: true,
		EventBefore:        &ttlCutoff,
		AutoLabelIsNull:    true,
		Limit:              1000,
	})
	if err != nil {
		return fmt.Errorf("ttl sweep query: %w", err)
	}

	for _, row := range ttlRows {
		if err := s.db.UpdateAutoLabel(row.ID, sweepAutoLabelFail); err != nil {
			return fmt.Errorf("marking FAIL for id=%d: %w", row.ID, err)
		}
	}

	return nil
}

func (s *Sweeper) sweepByPath(pathMap map[string]*database.ParseDecision) error {
	startIndex := 0
	pageSize := sweepPageSize

	for {
		page, err := s.client.ListItemsPage(startIndex, pageSize)
		if err != nil {
			return fmt.Errorf("ListItemsPage(start=%d): %w", startIndex, err)
		}

		for _, item := range page.Items {
			if item.Path == "" {
				continue
			}
			row, ok := pathMap[item.Path]
			if !ok {
				continue
			}
			now := time.Now().UTC()
			_ = s.db.UpdateOutcome(row.ID, database.OutcomeUpdate{
				JellyfinItemID:     item.ID,
				JellyfinImdbID:     item.ProviderIDs["Imdb"],
				JellyfinTmdbID:     item.ProviderIDs["Tmdb"],
				JellyfinTvdbID:     item.ProviderIDs["Tvdb"],
				JellyfinResolvedAt: &now,
			})
			delete(pathMap, item.Path)
		}

		startIndex += len(page.Items)
		if len(page.Items) == 0 || startIndex >= page.TotalRecordCount {
			break
		}
	}

	return nil
}
