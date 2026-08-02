package jellyfin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

const (
	sweepPageSize       = 200
	sweepAutoLabelFail  = "FAIL"
	sweepDefaultDelay   = 50 * time.Millisecond
	sweepRequestTimeout = 30 * time.Second

	// sweepMinInventoryRatio is the fraction of the cached snapshot a fresh
	// Jellyfin inventory must still cover before it is trusted as complete.
	sweepMinInventoryRatio = 0.5
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

	if err := s.db.MarkJellyfinInventoryIncomplete(); err != nil {
		return fmt.Errorf("mark Jellyfin inventory incomplete: %w", err)
	}
	inventory, err := s.fetchInventory(ctx)
	if err != nil {
		return err
	}
	if err := s.guardInventoryPlausible(len(inventory)); err != nil {
		return err
	}
	if err := s.reconcileInventory(pathMap, inventory); err != nil {
		return err
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

	for _, folder := range folders {
		if err := ctx.Err(); err != nil {
			return err
		}
		mismatches, err := verifier.GetUnidentifiedItems(folder.ItemID)
		if err != nil {
			slog.Warn("skipping library after error", "folder", folder.ItemID, "error", err)
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
			if err := s.db.MarkOutcomeUnidentified(dec.ID, m.ItemID, &now); err != nil {
				return fmt.Errorf("downgrade id=%d: %w", dec.ID, err)
			}
		}
	}
	return nil
}

func (s *Sweeper) fetchInventory(ctx context.Context) ([]Item, error) {
	startIndex := 0
	pageSize := sweepPageSize
	total := -1
	var inventory []Item
	seenIDs := make(map[string]struct{})
	seenPaths := make(map[string]struct{})

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		page, err := s.fetchPage(ctx, startIndex, pageSize)
		if err != nil {
			return nil, fmt.Errorf("ListItemsPage(start=%d): %w", startIndex, err)
		}
		if total == -1 {
			total = page.TotalRecordCount
		} else if page.TotalRecordCount != total {
			return nil, fmt.Errorf("incomplete Jellyfin inventory: total changed from %d to %d", total, page.TotalRecordCount)
		}
		if total < 0 {
			return nil, fmt.Errorf("incomplete Jellyfin inventory: negative total %d", total)
		}
		for i, item := range page.Items {
			path := strings.TrimSpace(item.Path)
			id := strings.TrimSpace(item.ID)
			if path == "" || id == "" {
				return nil, fmt.Errorf("incomplete Jellyfin inventory: unusable item at index %d", startIndex+i)
			}
			if _, exists := seenIDs[id]; exists {
				return nil, fmt.Errorf("incomplete Jellyfin inventory: duplicate item id %q", id)
			}
			if _, exists := seenPaths[path]; exists {
				return nil, fmt.Errorf("incomplete Jellyfin inventory: duplicate item path %q", path)
			}
			seenIDs[id] = struct{}{}
			seenPaths[path] = struct{}{}
		}

		inventory = append(inventory, page.Items...)
		startIndex += len(page.Items)
		if startIndex > total {
			return nil, fmt.Errorf("incomplete Jellyfin inventory: received %d items for total %d", startIndex, total)
		}
		if startIndex == total {
			return inventory, nil
		}
		if len(page.Items) == 0 {
			return nil, fmt.Errorf("incomplete Jellyfin inventory: received 0 of %d remaining items", total-startIndex)
		}

		// Rate-limit pagination to avoid hammering the Jellyfin server.
		// Cancellable so daemon shutdown does not stall here.
		if s.pageDelay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(s.pageDelay):
			}
		}
	}
}

// guardInventoryPlausible refuses to treat a suspiciously small Jellyfin
// inventory as an authoritative snapshot. A server that is reachable but
// still warming up (library scan in progress, storage not yet mounted, API
// key scoped to a user with no library access) answers 200 OK with zero or
// very few items; without this guard that answer reconciles as "everything
// was deleted" and clears every cached confirmation and successful outcome.
//
// Refusing leaves the previous confirmations in place and marks the snapshot
// incomplete, so verification reports inconclusive rather than fabricating a
// library-wide mismatch. A genuine bulk deletion below the floor therefore
// needs operator attention: clear jellyfin_items to re-baseline.
func (s *Sweeper) guardInventoryPlausible(count int) error {
	cached, err := s.db.CountJellyfinItems()
	if err != nil {
		return fmt.Errorf("count cached Jellyfin items: %w", err)
	}
	if cached == 0 {
		// Nothing to protect; an empty library is a legitimate first snapshot.
		return nil
	}
	if count == 0 {
		return fmt.Errorf("implausible Jellyfin inventory: reported 0 items while %d paths are cached; refusing to treat as authoritative", cached)
	}
	if float64(count) < float64(cached)*sweepMinInventoryRatio {
		return fmt.Errorf("implausible Jellyfin inventory: reported %d items, down from %d cached (below %.0f%% floor); refusing to treat as authoritative", count, cached, sweepMinInventoryRatio*100)
	}
	return nil
}

func (s *Sweeper) reconcileInventory(pathMap map[string][]*database.ParseDecision, inventory []Item) error {
	cacheItems := make([]database.JellyfinItem, 0, len(inventory))
	for _, item := range inventory {
		if item.Path == "" || item.ID == "" {
			continue
		}
		path := s.translator.JellyfinToDaemon(item.Path)
		cacheItems = append(cacheItems, database.JellyfinItem{
			Path:           path,
			JellyfinItemID: item.ID,
			ItemName:       item.Name,
			ItemType:       item.Type,
		})

		rows, ok := pathMap[path]
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
		delete(pathMap, path)
	}

	removed, err := s.db.ReconcileJellyfinItems(cacheItems)
	if err != nil {
		return fmt.Errorf("reconcile Jellyfin items: %w", err)
	}
	for _, path := range removed {
		if err := s.db.ClearOutcomesByTargetPath(path); err != nil {
			return fmt.Errorf("clear stale outcomes for path %q: %w", path, err)
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
