package jellyfin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
)

const (
	MetadataStateUnknown                      = ""
	MetadataStateIdentified                   = "identified"
	MetadataStateMissingProviderIDs           = "missing_provider_ids"
	MetadataStateMissingEpisodeNumbers        = "missing_episode_numbers"
	MetadataStateSeriesUnidentified           = "series_unidentified"
	MetadataStateSeriesIdentifiedEpisodeStale = "series_identified_episode_stale"
	MetadataStateJellyfinItemMissing          = "jellyfin_item_missing"
	MetadataStateTargetFileMissing            = "target_file_missing"
	MetadataStatePathMismatch                 = "path_mismatch"
	MetadataStateRecentImportWaiting          = "recent_import_waiting"
	MetadataStateNeedsReview                  = "needs_review"
)

const metadataInitialWait = 15 * time.Minute

type MetadataClassification struct {
	State      string
	Identified bool
	Error      string
	NextCheck  time.Duration
}

type MetadataRecoveryConfig struct {
	RepairCooldown   time.Duration
	NeedsReviewAfter int
	CorrectionEnabled bool
}

type MetadataRunSummary struct {
	Checked    int
	Identified int
	Repaired   int
	Skipped    int
	Errors     int
}

type MetadataClient interface {
	GetItemsByIDs(ctx context.Context, ids []string) (*ItemsResponse, error)
	ListItemsPageCtx(ctx context.Context, startIndex, limit int) (*ItemsResponse, error)
}

type MetadataRepairClient interface {
	MetadataClient
	RefreshItemFullMetadata(itemID string) error
	RefreshItemFullMetadataRecursive(itemID string) error
}

type MetadataStore interface {
	GetDecision(id int64) (*database.ParseDecision, error)
	ListDueMetadataChecks(now time.Time, limit int) ([]*database.ParseDecision, error)
	UpgradeOutcome(id int64, u database.OutcomeUpdate) error
	UpdateMetadataCheckState(id int64, state, errMsg string, nextCheck *time.Time) error
	UpdateMetadataRepairState(id int64, state, errMsg string, nextCheck *time.Time, repairedAt *time.Time) error
}

// CorrectionDecision is the result of a movie name correction attempt.
type CorrectionDecision struct {
	Action   string
	NewTitle string
	NewYear  string
	TmdbID   string
	Reason   string
}

// MovieCorrector decides whether a movie title should be corrected.
type MovieCorrector interface {
	Decide(ctx context.Context, currentTitle, year string) CorrectionDecision
}

// CorrectionEnqueuer enqueues a verifier-driven rename task.
type CorrectionEnqueuer interface {
	EnqueueVerifierRename(parseDecisionID int64, srcPath, dstPath, newTitle, newYear, tmdbID string) error
}

type MetadataReconciler struct {
	client            MetadataRepairClient
	store             MetadataStore
	translator        *PathTranslator
	cfg               MetadataRecoveryConfig
	now               func() time.Time
	corrector         MovieCorrector
	enqueuer          CorrectionEnqueuer
	correctionEnabled bool
	correctionGrace   time.Duration
}

func NewMetadataReconciler(client MetadataRepairClient, store MetadataStore, cfg MetadataRecoveryConfig) *MetadataReconciler {
	if cfg.RepairCooldown <= 0 {
		cfg.RepairCooldown = 6 * time.Hour
	}
	if cfg.NeedsReviewAfter <= 0 {
		cfg.NeedsReviewAfter = 4
	}
	return &MetadataReconciler{
		client:            client,
		store:             store,
		cfg:               cfg,
		correctionEnabled: cfg.CorrectionEnabled,
		correctionGrace:   48 * time.Hour,
		now:               func() time.Time { return time.Now().UTC() },
	}
}

func (r *MetadataReconciler) SetPathTranslator(t *PathTranslator) {
	if r == nil {
		return
	}
	r.translator = t
}

func (r *MetadataReconciler) SetCorrector(c MovieCorrector) {
	if r == nil {
		return
	}
	r.corrector = c
}

func (r *MetadataReconciler) SetEnqueuer(e CorrectionEnqueuer) {
	if r == nil {
		return
	}
	r.enqueuer = e
}

func (r *MetadataReconciler) RunPassive(ctx context.Context, limit int, progress chan<- database.ProgressEvent) (MetadataRunSummary, error) {
	var summary MetadataRunSummary
	if r == nil || r.client == nil || r.store == nil {
		return summary, nil
	}
	now := r.now()
	rows, err := r.store.ListDueMetadataChecks(now, limit)
	if err != nil {
		return summary, fmt.Errorf("list metadata checks: %w", err)
	}
	if len(rows) == 0 {
		sendMetadataProgress(progress, "complete", "no metadata rows due", 0, 0)
		return summary, nil
	}
	sendMetadataProgress(progress, "checking", "fetching Jellyfin metadata", 0, len(rows))
	result, err := r.classifyRows(ctx, rows, now)
	if err != nil {
		return summary, err
	}
	for i, row := range rows {
		classification := result.classifications[row.ID]
		summary.Checked++
		if err := r.applyPassiveClassification(ctx, row, result.itemFor(row), classification, now); err != nil {
			summary.Errors++
			sendMetadataProgress(progress, "checking", fmt.Sprintf("row %d: %v", row.ID, err), i+1, len(rows))
			continue
		}
		if classification.Identified {
			summary.Identified++
		}
		sendMetadataProgress(progress, "checking", metadataProgressMessage(row, classification), i+1, len(rows))
	}
	sendMetadataProgress(progress, "complete", metadataSummaryMessage(summary), summary.Checked, len(rows))
	return summary, nil
}

func (r *MetadataReconciler) RunRepair(ctx context.Context, limit int, progress chan<- database.ProgressEvent) (MetadataRunSummary, error) {
	if r == nil || r.store == nil {
		return MetadataRunSummary{}, nil
	}
	now := r.now()
	rows, err := r.store.ListDueMetadataChecks(now, limit)
	if err != nil {
		return MetadataRunSummary{}, fmt.Errorf("list repair checks: %w", err)
	}
	return r.repairRows(ctx, rows, progress)
}

func (r *MetadataReconciler) RepairDecision(ctx context.Context, id int64, progress chan<- database.ProgressEvent) (MetadataRunSummary, error) {
	if r == nil || r.store == nil {
		return MetadataRunSummary{}, nil
	}
	row, err := r.store.GetDecision(id)
	if err != nil {
		return MetadataRunSummary{}, fmt.Errorf("get decision %d: %w", id, err)
	}
	if row == nil {
		return MetadataRunSummary{Errors: 1}, fmt.Errorf("decision %d not found", id)
	}
	return r.repairRows(ctx, []*database.ParseDecision{row}, progress)
}

func (r *MetadataReconciler) RepairDecisions(ctx context.Context, ids []int64, progress chan<- database.ProgressEvent) (MetadataRunSummary, error) {
	if r == nil || r.store == nil {
		return MetadataRunSummary{}, nil
	}
	rows := make([]*database.ParseDecision, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		row, err := r.store.GetDecision(id)
		if err != nil {
			return MetadataRunSummary{}, fmt.Errorf("get decision %d: %w", id, err)
		}
		if row != nil {
			rows = append(rows, row)
		}
	}
	return r.repairRows(ctx, rows, progress)
}

func (r *MetadataReconciler) repairRows(ctx context.Context, rows []*database.ParseDecision, progress chan<- database.ProgressEvent) (MetadataRunSummary, error) {
	var summary MetadataRunSummary
	if r == nil || r.client == nil || r.store == nil {
		return summary, nil
	}
	now := r.now()
	if len(rows) == 0 {
		sendMetadataProgress(progress, "complete", "no metadata repair rows", 0, 0)
		return summary, nil
	}
	sendMetadataProgress(progress, "checking", "preflighting Jellyfin metadata", 0, len(rows))
	result, err := r.classifyRows(ctx, rows, now)
	if err != nil {
		return summary, err
	}
	refreshes := make(map[metadataRefreshKey]error)
	for i, row := range rows {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		default:
		}

		item := result.itemFor(row)
		classification := result.classifications[row.ID]
		summary.Checked++

		if err := r.applyPassiveClassification(ctx, row, item, classification, now); err != nil {
			summary.Errors++
			sendMetadataProgress(progress, "repairing", fmt.Sprintf("row %d: %v", row.ID, err), i+1, len(rows))
			continue
		}
		if classification.Identified {
			summary.Identified++
			summary.Skipped++
			sendMetadataProgress(progress, "repairing", metadataProgressMessage(row, classification), i+1, len(rows))
			continue
		}
		repairID, recursive := result.repairTarget(item, classification)
		if !metadataRepairable(classification.State) || repairID == "" {
			summary.Skipped++
			sendMetadataProgress(progress, "repairing", metadataProgressMessage(row, classification), i+1, len(rows))
			continue
		}
		if r.needsReview(row) {
			summary.Skipped++
			next := now.Add(7 * 24 * time.Hour)
			_ = r.store.UpdateMetadataRepairState(row.ID, MetadataStateNeedsReview, "repair attempt limit reached", &next, nil)
			sendMetadataProgress(progress, "repairing", fmt.Sprintf("row %d needs review", row.ID), i+1, len(rows))
			continue
		}
		if next, ok := r.repairCooldown(row, now); ok {
			summary.Skipped++
			_ = r.store.UpdateMetadataRepairState(row.ID, classification.State, "repair cooldown active", &next, nil)
			sendMetadataProgress(progress, "repairing", fmt.Sprintf("row %d repair cooldown active", row.ID), i+1, len(rows))
			continue
		}

		key := metadataRefreshKey{id: repairID, recursive: recursive}
		err, refreshed := refreshes[key]
		if !refreshed {
			if recursive {
				err = r.client.RefreshItemFullMetadataRecursive(repairID)
			} else {
				err = r.client.RefreshItemFullMetadata(repairID)
			}
			refreshes[key] = err
		}
		if err != nil {
			summary.Errors++
			next := now.Add(r.cfg.RepairCooldown)
			_ = r.store.UpdateMetadataRepairState(row.ID, classification.State, err.Error(), &next, nil)
			sendMetadataProgress(progress, "repairing", fmt.Sprintf("row %d repair failed: %v", row.ID, err), i+1, len(rows))
			continue
		}

		summary.Repaired++
		repairedAt := now
		next := now.Add(metadataInitialWait)
		msg := "full metadata refresh queued"
		if err := r.store.UpdateMetadataRepairState(row.ID, classification.State, msg, &next, &repairedAt); err != nil {
			summary.Errors++
			sendMetadataProgress(progress, "repairing", fmt.Sprintf("row %d: %v", row.ID, err), i+1, len(rows))
			continue
		}
		sendMetadataProgress(progress, "repairing", fmt.Sprintf("row %d: %s", row.ID, msg), i+1, len(rows))
	}
	sendMetadataProgress(progress, "complete", metadataSummaryMessage(summary), summary.Checked, len(rows))
	return summary, nil
}

type metadataRefreshKey struct {
	id        string
	recursive bool
}

type metadataClassificationResult struct {
	items           map[string]*Item
	rowItems        map[int64]*Item
	series          map[string]*Item
	classifications map[int64]MetadataClassification
}

func (r *MetadataReconciler) classifyRows(ctx context.Context, rows []*database.ParseDecision, now time.Time) (metadataClassificationResult, error) {
	result := metadataClassificationResult{
		items:           map[string]*Item{},
		rowItems:        map[int64]*Item{},
		series:          map[string]*Item{},
		classifications: map[int64]MetadataClassification{},
	}
	ids := make([]string, 0, len(rows))
	seenIDs := map[string]struct{}{}
	for _, row := range rows {
		id := strings.TrimSpace(row.JellyfinItemID)
		if id == "" {
			continue
		}
		if _, ok := seenIDs[id]; ok {
			continue
		}
		seenIDs[id] = struct{}{}
		ids = append(ids, id)
	}

	resp, err := r.client.GetItemsByIDs(ctx, ids)
	if err != nil {
		return result, fmt.Errorf("fetch jellyfin items: %w", err)
	}
	for i := range resp.Items {
		item := translateMetadataItem(&resp.Items[i], r.translator)
		result.items[item.ID] = item
	}

	if err := r.recoverMissingItemsByPath(ctx, rows, &result); err != nil {
		return result, err
	}

	seriesIDs := make([]string, 0)
	seenSeries := map[string]struct{}{}
	for _, item := range result.items {
		id := strings.TrimSpace(item.SeriesID)
		if id == "" {
			continue
		}
		if _, ok := seenSeries[id]; ok {
			continue
		}
		seenSeries[id] = struct{}{}
		seriesIDs = append(seriesIDs, id)
	}
	if len(seriesIDs) > 0 {
		resp, err := r.client.GetItemsByIDs(ctx, seriesIDs)
		if err != nil {
			return result, fmt.Errorf("fetch jellyfin series: %w", err)
		}
		for i := range resp.Items {
			item := translateMetadataItem(&resp.Items[i], r.translator)
			result.series[item.ID] = item
		}
	}

	for _, row := range rows {
		item := result.itemFor(row)
		var parent *Item
		if item != nil && item.SeriesID != "" {
			parent = result.series[item.SeriesID]
		}
		if item == nil && targetFileMissing(row) {
			result.classifications[row.ID] = MetadataClassification{
				State: MetadataStateTargetFileMissing,
				Error: "target file is missing",
			}
			continue
		}
		result.classifications[row.ID] = ClassifyMetadata(row, item, parent, now)
	}

	return result, nil
}

func (r *MetadataReconciler) recoverMissingItemsByPath(ctx context.Context, rows []*database.ParseDecision, result *metadataClassificationResult) error {
	if result == nil {
		return nil
	}
	missingPaths := make(map[string]struct{})
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.TargetPath) == "" {
			continue
		}
		if strings.TrimSpace(row.JellyfinItemID) != "" {
			if _, ok := result.items[row.JellyfinItemID]; ok {
				continue
			}
		}
		missingPaths[row.TargetPath] = struct{}{}
	}
	if len(missingPaths) == 0 {
		return nil
	}

	const pageSize = 200
	for start := 0; ; {
		resp, err := r.client.ListItemsPageCtx(ctx, start, pageSize)
		if err != nil {
			return fmt.Errorf("recover jellyfin items by path: %w", err)
		}
		for i := range resp.Items {
			item := translateMetadataItem(&resp.Items[i], r.translator)
			if _, ok := missingPaths[item.Path]; !ok {
				continue
			}
			result.items[item.ID] = item
			for _, row := range rows {
				if row != nil && row.TargetPath == item.Path {
					result.rowItems[row.ID] = item
				}
			}
			delete(missingPaths, item.Path)
		}
		start += len(resp.Items)
		if len(missingPaths) == 0 || len(resp.Items) == 0 || start >= resp.TotalRecordCount {
			break
		}
	}
	return nil
}

func targetFileMissing(row *database.ParseDecision) bool {
	if row == nil || strings.TrimSpace(row.TargetPath) == "" {
		return false
	}
	_, err := os.Stat(row.TargetPath)
	return os.IsNotExist(err)
}

func (r metadataClassificationResult) itemFor(row *database.ParseDecision) *Item {
	if row == nil {
		return nil
	}
	if item := r.rowItems[row.ID]; item != nil {
		return item
	}
	return r.items[row.JellyfinItemID]
}

func (r metadataClassificationResult) repairTarget(item *Item, c MetadataClassification) (string, bool) {
	if item == nil {
		return "", false
	}
	switch c.State {
	case MetadataStateSeriesUnidentified, MetadataStateSeriesIdentifiedEpisodeStale:
		if item.SeriesID == "" {
			return item.ID, false
		}
		if _, ok := r.series[item.SeriesID]; ok {
			return item.SeriesID, true
		}
		return item.ID, false
	default:
		return item.ID, false
	}
}

func (r *MetadataReconciler) applyPassiveClassification(ctx context.Context, row *database.ParseDecision, item *Item, c MetadataClassification, now time.Time) error {
	if c.Identified {
		identified := true
		imdb, tmdb, tvdb := metadataProviderIDs(item)
		if err := r.store.UpgradeOutcome(row.ID, database.OutcomeUpdate{
			JellyfinItemID:      item.ID,
			JellyfinImdbID:      imdb,
			JellyfinTmdbID:      tmdb,
			JellyfinTvdbID:      tvdb,
			JellyfinResolvedAt:  &now,
			JellyfinIdentified:  &identified,
			JellyfinFirstSeenAt: &now,
		}); err != nil {
			return fmt.Errorf("upgrade outcome: %w", err)
		}
		return r.store.UpdateMetadataCheckState(row.ID, c.State, "", nil)
	}

	next := metadataNextCheck(row, c, now)

	if r.maybeCorrect(ctx, row, c, next, now) {
		return nil
	}

	return r.store.UpdateMetadataCheckState(row.ID, c.State, c.Error, next)
}

// maybeCorrect attempts a verifier-driven movie name correction when a movie
// row has been stuck in missing_provider_ids past the correction grace period
// and the attempt cap has not been reached. On a successful correction it
// enqueues a verifier_rename task and consumes one attempt via
// UpdateMetadataRepairState (non-nil repairedAt triggers the increment).
// Returns true when a correction was enqueued (caller should skip the normal
// UpdateMetadataCheckState return).
func (r *MetadataReconciler) maybeCorrect(ctx context.Context, row *database.ParseDecision, c MetadataClassification, next *time.Time, now time.Time) bool {
	if !r.correctionEnabled || r.enqueuer == nil || r.corrector == nil {
		return false
	}
	if c.Identified || c.State != MetadataStateMissingProviderIDs {
		return false
	}
	if !isMovieRow(row) || !correctionDue(row, r.correctionGrace, now) {
		return false
	}
	if row.MetadataRepairCount >= r.cfg.NeedsReviewAfter {
		return false
	}

	year := ""
	if row.ParsedYear != nil {
		year = fmt.Sprintf("%d", *row.ParsedYear)
	}
	d := r.corrector.Decide(ctx, row.ParsedTitle, year)
	if d.Action != "correct" || d.NewTitle == "" {
		return false
	}

	dst, err := r.computeCorrectedTargetPath(row, d.NewTitle, d.NewYear)
	if err != nil {
		return false
	}

	if err := r.enqueuer.EnqueueVerifierRename(row.ID, row.TargetPath, dst, d.NewTitle, d.NewYear, d.TmdbID); err != nil {
		return false
	}

	repairedAt := now
	_ = r.store.UpdateMetadataRepairState(row.ID, c.State, "correction enqueued: "+d.NewTitle, next, &repairedAt)
	return true
}

func isMovieRow(row *database.ParseDecision) bool {
	return row != nil && row.MediaTypeGuessed == "movie"
}

func correctionDue(row *database.ParseDecision, grace time.Duration, now time.Time) bool {
	return row.TargetAt != nil && now.Sub(*row.TargetAt) >= grace
}

// computeCorrectedTargetPath builds the destination path for a corrected movie
// by reusing the row's existing library root and file extension.
func (r *MetadataReconciler) computeCorrectedTargetPath(row *database.ParseDecision, newTitle, newYear string) (string, error) {
	src := row.TargetPath
	if src == "" {
		return "", fmt.Errorf("empty target path")
	}
	srcDir := filepath.Dir(src)
	parentDir := filepath.Dir(srcDir) // library root or intermediate folder
	ext := filepath.Ext(src)
	folderName := naming.NormalizeMediaName(newTitle, newYear)
	fileName := naming.FormatMovieFilename(newTitle, newYear, ext)
	return filepath.Join(parentDir, folderName, fileName), nil
}

func ClassifyMetadata(row *database.ParseDecision, item *Item, series *Item, now time.Time) MetadataClassification {
	if recentlyImported(row, now) {
		next := metadataInitialWait - now.Sub(*row.TargetAt)
		if next < time.Minute {
			next = time.Minute
		}
		return MetadataClassification{
			State:     MetadataStateRecentImportWaiting,
			NextCheck: next,
		}
	}

	if item == nil {
		return MetadataClassification{
			State: MetadataStateJellyfinItemMissing,
			Error: "jellyfin item is missing",
		}
	}

	if row != nil && strings.TrimSpace(row.TargetPath) != "" && strings.TrimSpace(item.Path) != "" && row.TargetPath != item.Path {
		return MetadataClassification{
			State: MetadataStatePathMismatch,
			Error: fmt.Sprintf("target path %q does not match jellyfin path %q", row.TargetPath, item.Path),
		}
	}

	if item.Type == "Episode" {
		if !HasProviderIDs(item) {
			if series != nil && HasProviderIDs(series) {
				if HasEpisodeNumbers(item) && HasEpisodeMetadata(item) {
					return MetadataClassification{
						State:      MetadataStateIdentified,
						Identified: true,
					}
				}
				return MetadataClassification{
					State: MetadataStateSeriesIdentifiedEpisodeStale,
					Error: "episode is missing provider ids but parent series is identified",
				}
			}
			if series != nil && !HasProviderIDs(series) {
				return MetadataClassification{
					State: MetadataStateSeriesUnidentified,
					Error: "parent series is missing provider ids",
				}
			}
			return MetadataClassification{
				State: MetadataStateMissingProviderIDs,
				Error: "item is missing provider ids",
			}
		}

		if item.SeriesID != "" && !HasEpisodeNumbers(item) {
			return MetadataClassification{
				State: MetadataStateMissingEpisodeNumbers,
				Error: "episode is missing season or episode number",
			}
		}
	}

	if !HasProviderIDs(item) {
		return MetadataClassification{
			State: MetadataStateMissingProviderIDs,
			Error: "item is missing provider ids",
		}
	}

	return MetadataClassification{
		State:      MetadataStateIdentified,
		Identified: true,
	}
}

func HasProviderIDs(item *Item) bool {
	if item == nil {
		return false
	}
	for _, id := range item.ProviderIDs {
		if strings.TrimSpace(id) != "" {
			return true
		}
	}
	return false
}

func HasEpisodeNumbers(item *Item) bool {
	return item != nil && item.IndexNumber != nil && item.ParentIndexNumber != nil
}

func HasEpisodeMetadata(item *Item) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(item.Overview) != "" || strings.TrimSpace(item.PremiereDate) != "" {
		return true
	}
	for _, tag := range item.ImageTags {
		if strings.TrimSpace(tag) != "" {
			return true
		}
	}
	return false
}

func recentlyImported(row *database.ParseDecision, now time.Time) bool {
	return row != nil && row.TargetAt != nil && now.Sub(*row.TargetAt) < metadataInitialWait
}

func metadataNextCheck(row *database.ParseDecision, c MetadataClassification, now time.Time) *time.Time {
	if c.NextCheck > 0 {
		next := now.Add(c.NextCheck)
		return &next
	}
	if c.State == MetadataStateNeedsReview {
		next := now.Add(7 * 24 * time.Hour)
		return &next
	}
	delay := time.Hour
	if row != nil {
		switch {
		case row.MetadataCheckCount >= 2:
			delay = 24 * time.Hour
		case row.MetadataCheckCount >= 1:
			delay = 6 * time.Hour
		}
	}
	next := now.Add(delay)
	return &next
}

func metadataRepairable(state string) bool {
	switch state {
	case MetadataStateMissingProviderIDs, MetadataStateMissingEpisodeNumbers, MetadataStateSeriesIdentifiedEpisodeStale, MetadataStateSeriesUnidentified:
		return true
	default:
		return false
	}
}

func (r *MetadataReconciler) needsReview(row *database.ParseDecision) bool {
	return row != nil && row.MetadataRepairCount >= r.cfg.NeedsReviewAfter
}

func (r *MetadataReconciler) repairCooldown(row *database.ParseDecision, now time.Time) (time.Time, bool) {
	if row == nil || row.LastMetadataRepairAt == nil {
		return time.Time{}, false
	}
	next := row.LastMetadataRepairAt.Add(r.cfg.RepairCooldown)
	if now.Before(next) {
		return next, true
	}
	return time.Time{}, false
}

func translateMetadataItem(item *Item, translator *PathTranslator) *Item {
	if item == nil {
		return nil
	}
	copy := *item
	copy.Path = translator.JellyfinToDaemon(copy.Path)
	return &copy
}

func metadataProviderIDs(item *Item) (string, string, string) {
	if item == nil {
		return "", "", ""
	}
	return item.ProviderIDs["Imdb"], item.ProviderIDs["Tmdb"], item.ProviderIDs["Tvdb"]
}

func sendMetadataProgress(progress chan<- database.ProgressEvent, phase, msg string, current, total int) {
	if progress == nil {
		return
	}
	progress <- database.ProgressEvent{Phase: phase, Msg: msg, Current: current, Total: total}
}

func metadataProgressMessage(row *database.ParseDecision, c MetadataClassification) string {
	title := ""
	if row != nil {
		title = row.ParsedTitle
		if title == "" {
			title = row.TargetPath
		}
	}
	if title == "" {
		title = "row"
	}
	if c.Identified {
		return fmt.Sprintf("%s identified", title)
	}
	if c.Error != "" {
		return fmt.Sprintf("%s: %s", title, c.Error)
	}
	return fmt.Sprintf("%s: %s", title, c.State)
}

func metadataSummaryMessage(s MetadataRunSummary) string {
	return fmt.Sprintf("checked=%d identified=%d repaired=%d skipped=%d errors=%d", s.Checked, s.Identified, s.Repaired, s.Skipped, s.Errors)
}
