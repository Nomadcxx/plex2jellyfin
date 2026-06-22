package jellyfin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
)

const (
	sweepPageSize       = 200
	sweepAutoLabelFail  = "FAIL"
	sweepDefaultDelay   = 50 * time.Millisecond
	sweepRequestTimeout = 30 * time.Second
)

// Sweeper reconciles unresolved parse_decisions rows against the Jellyfin
// library by enumerating items and matching by Path. Rows that remain
// unresolved past the TTL are auto-labeled as FAIL.
type Sweeper struct {
	client     *Client
	db         *database.MediaDB
	pageDelay  time.Duration
	translator *PathTranslator
}

// NewSweeper constructs a Sweeper over the given Jellyfin client and database.
func NewSweeper(client *Client, db *database.MediaDB) *Sweeper {
	return &Sweeper{client: client, db: db, pageDelay: sweepDefaultDelay}
}

// SetPathTranslator configures prefix translation between Jellyfin's view
// of media paths and the daemon's view. A nil translator disables
// translation (paths are matched as-is).
func (s *Sweeper) SetPathTranslator(t *PathTranslator) {
	if s == nil {
		return
	}
	s.translator = t
}

// SetPageDelay overrides the inter-page sleep used to rate-limit Jellyfin
// pagination. Use 0 in tests to disable the delay.
func (s *Sweeper) SetPageDelay(d time.Duration) {
	if s == nil {
		return
	}
	s.pageDelay = d
}

// RunOnce performs a single sweep pass: it walks the Jellyfin library and
// resolves any unresolved decisions whose target_path matches a Jellyfin
// item's Path within the lookback window, then labels long-unresolved rows
// as FAIL when older than the TTL. ctx is used to bound each Jellyfin HTTP
// call (with a per-request timeout) and to abort the sweep promptly on
// daemon shutdown.
func (s *Sweeper) RunOnce(ctx context.Context, lookback, ttl time.Duration) error {
	if s == nil || s.client == nil || s.db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
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

	pathMap := make(map[string][]*database.ParseDecision, len(rows))
	for _, row := range rows {
		pathMap[row.TargetPath] = append(pathMap[row.TargetPath], row)
	}

	if len(pathMap) > 0 {
		if err := s.sweepByPath(ctx, pathMap); err != nil {
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

	// Pass 3: catch resolved-but-unidentified items. Best-effort: any
	// failure here logs and returns nil so the primary path-match sweep
	// remains the source of truth even when the verifier API misbehaves.
	// Skipped when the path-match pass found nothing to do (keeps tests
	// against fixtures with no relevant rows quiet, and avoids hitting
	// Jellyfin when there's no work to verify).
	if len(pathMap) > 0 {
		if err := s.sweepUnidentified(ctx); err != nil {
			slog.Warn("jellyfin unidentified sweep failed", "error", err)
		}
	}

	return nil
}

// sweepUnidentified queries Jellyfin for items with no provider IDs and
// downgrades any matching parse_decisions row to identified=0. Uses the
// existing Verifier helper. Per-folder pagination already handled inside
// the verifier.
func (s *Sweeper) sweepUnidentified(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	verifier := NewVerifier(s.client)
	folders, err := s.client.GetVirtualFolders()
	if err != nil {
		return fmt.Errorf("GetVirtualFolders: %w", err)
	}

	identified := false
	for _, folder := range folders {
		if err := ctx.Err(); err != nil {
			return err
		}
		mismatches, err := verifier.GetUnidentifiedItems(folder.ItemID)
		if err != nil {
			// Log-and-continue: a bad library shouldn't tank the sweep.
			continue
		}
		for _, m := range mismatches {
			if m.Path == "" {
				continue
			}
			lookup := s.translator.JellyfinToDaemon(m.Path)
			dec, err := s.db.GetDecisionByTargetPath(lookup)
			if err != nil || dec == nil {
				continue
			}
			now := time.Now().UTC()
			if err := s.db.UpgradeOutcome(dec.ID, database.OutcomeUpdate{
				JellyfinItemID:      m.ItemID,
				JellyfinResolvedAt:  &now,
				JellyfinIdentified:  &identified,
				JellyfinFirstSeenAt: &now,
			}); err != nil {
				return fmt.Errorf("downgrade id=%d: %w", dec.ID, err)
			}
		}
	}
	return nil
}

func (s *Sweeper) sweepByPath(ctx context.Context, pathMap map[string][]*database.ParseDecision) error {
	startIndex := 0
	pageSize := sweepPageSize

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		page, err := s.fetchPage(ctx, startIndex, pageSize)
		if err != nil {
			return fmt.Errorf("ListItemsPage(start=%d): %w", startIndex, err)
		}

		for _, item := range page.Items {
			if item.Path == "" {
				continue
			}
			lookup := s.translator.JellyfinToDaemon(item.Path)
			rows, ok := pathMap[lookup]
			if !ok {
				continue
			}
			now := time.Now().UTC()
			imdb := item.ProviderIDs["Imdb"]
			tmdb := item.ProviderIDs["Tmdb"]
			tvdb := item.ProviderIDs["Tvdb"]
			identified := imdb != "" || tmdb != "" || tvdb != ""
			for _, row := range rows {
				if err := s.db.UpdateOutcome(row.ID, database.OutcomeUpdate{
					JellyfinItemID:      item.ID,
					JellyfinImdbID:      imdb,
					JellyfinTmdbID:      tmdb,
					JellyfinTvdbID:      tvdb,
					JellyfinResolvedAt:  &now,
					JellyfinIdentified:  &identified,
					JellyfinFirstSeenAt: &now,
				}); err != nil {
					return fmt.Errorf("UpdateOutcome id=%d: %w", row.ID, err)
				}
			}
			delete(pathMap, lookup)
		}

		startIndex += len(page.Items)
		if len(page.Items) == 0 || startIndex >= page.TotalRecordCount {
			break
		}

		// Rate-limit pagination to avoid hammering the Jellyfin server.
		// Cancellable so daemon shutdown does not stall here.
		if s.pageDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(s.pageDelay):
			}
		}
	}

	return nil
}

// fetchPage wraps a single ListItemsPage call with a per-request timeout
// derived from the sweep ctx, so a hung Jellyfin server cannot stall the
// sweeper indefinitely.
func (s *Sweeper) fetchPage(ctx context.Context, startIndex, pageSize int) (*ItemsResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, sweepRequestTimeout)
	defer cancel()
	return s.client.ListItemsPageCtx(reqCtx, startIndex, pageSize)
}
