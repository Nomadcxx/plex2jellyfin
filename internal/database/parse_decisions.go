package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ParseDecision records one debounced processing attempt for a media file.
type ParseDecision struct {
	ID                   int64
	SourcePath           string
	SourceFilename       string
	EventAt              time.Time
	MediaTypeGuessed     string
	ParseMethod          string
	ParsedTitle          string
	ParsedYear           *int
	ParsedSeason         *int
	ParsedEpisode        *int
	ParserStrippedTokens string
	TargetPath           string
	TargetAt             *time.Time
	ExistingMatchMethod  string
	OrganizeOutcome      string
	OrganizeError        string
	JellyfinItemID       string
	JellyfinImdbID       string
	JellyfinTmdbID       string
	JellyfinTvdbID       string
	JellyfinResolvedAt   *time.Time
	JellyfinIdentified   *bool
	JellyfinFirstSeenAt  *time.Time
	AutoLabel            string
	AutoLabelAt          *time.Time
	HumanLabelOverride   string
}

// ParseUpdate carries updated parse metadata for a decision row.
type ParseUpdate struct {
	ParseMethod          string
	ParsedTitle          string
	ParsedYear           *int
	ParsedSeason         *int
	ParsedEpisode        *int
	ParserStrippedTokens string
	MediaTypeGuessed     string
}

// OrganizeUpdate carries updated organize metadata for a decision row.
type OrganizeUpdate struct {
	TargetPath          string
	TargetAt            *time.Time
	ExistingMatchMethod string
	OrganizeOutcome     string
	OrganizeError       string
}

// OutcomeUpdate carries updated Jellyfin resolution metadata for a decision row.
type OutcomeUpdate struct {
	JellyfinItemID      string
	JellyfinImdbID      string
	JellyfinTmdbID      string
	JellyfinTvdbID      string
	JellyfinResolvedAt  *time.Time
	JellyfinIdentified  *bool
	JellyfinFirstSeenAt *time.Time
}

// QueryFilter specifies filter criteria for QueryDecisions.
type QueryFilter struct {
	OrganizeOutcome    string
	AutoLabel          string
	AutoLabelIsNull    bool
	ParseMethod        string
	StrippedToken      string
	SourceContains     string
	JellyfinUnresolved bool
	TargetPathNotEmpty bool
	EventAfter         *time.Time
	EventBefore        *time.Time
	Limit              int
}

// InsertDecision inserts a new ParseDecision and returns its ID.
func (m *MediaDB) InsertDecision(d ParseDecision) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	res, err := m.db.Exec(`
		INSERT INTO parse_decisions
			(source_path, source_filename, event_at,
			 media_type_guessed, parse_method, parsed_title,
			 parsed_year, parsed_season, parsed_episode,
			 parser_stripped_tokens, target_path, target_at,
			 existing_match_method, organize_outcome, organize_error,
			 jellyfin_item_id, jellyfin_imdb_id, jellyfin_tmdb_id, jellyfin_tvdb_id,
			 jellyfin_resolved_at, auto_label, human_label_override)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.SourcePath, d.SourceFilename, d.EventAt,
		nullStr(d.MediaTypeGuessed), nullStr(d.ParseMethod), nullStr(d.ParsedTitle),
		nullIntPtr(d.ParsedYear), nullIntPtr(d.ParsedSeason), nullIntPtr(d.ParsedEpisode),
		nullStr(d.ParserStrippedTokens), nullStr(d.TargetPath), nullTimePtr(d.TargetAt),
		nullStr(d.ExistingMatchMethod), nullStr(d.OrganizeOutcome), nullStr(d.OrganizeError),
		nullStr(d.JellyfinItemID), nullStr(d.JellyfinImdbID), nullStr(d.JellyfinTmdbID), nullStr(d.JellyfinTvdbID),
		nullTimePtr(d.JellyfinResolvedAt), nullStr(d.AutoLabel), nullStr(d.HumanLabelOverride),
	)
	if err != nil {
		return 0, fmt.Errorf("InsertDecision: %w", err)
	}
	return res.LastInsertId()
}

// GetDecision returns the ParseDecision with the given ID, or nil if not found.
func (m *MediaDB) GetDecision(id int64) (*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(`SELECT `+decisionColumns+` FROM parse_decisions WHERE id = ?`, id)
	d, err := scanDecision(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// UpdateParse updates the parse metadata columns for the given decision row.
func (m *MediaDB) UpdateParse(id int64, u ParseUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		UPDATE parse_decisions SET
			parse_method = ?,
			parsed_title = ?,
			parsed_year = ?,
			parsed_season = ?,
			parsed_episode = ?,
			parser_stripped_tokens = ?,
			media_type_guessed = ?
		WHERE id = ?`,
		nullStr(u.ParseMethod), nullStr(u.ParsedTitle),
		nullIntPtr(u.ParsedYear), nullIntPtr(u.ParsedSeason), nullIntPtr(u.ParsedEpisode),
		nullStr(u.ParserStrippedTokens), nullStr(u.MediaTypeGuessed),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateParse: %w", err)
	}
	return nil
}

// UpdateOrganize updates the organize metadata columns for the given decision row.
func (m *MediaDB) UpdateOrganize(id int64, u OrganizeUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		UPDATE parse_decisions SET
			target_path = ?,
			target_at = ?,
			existing_match_method = ?,
			organize_outcome = ?,
			organize_error = ?
		WHERE id = ?`,
		nullStr(u.TargetPath), nullTimePtr(u.TargetAt),
		nullStr(u.ExistingMatchMethod), nullStr(u.OrganizeOutcome), nullStr(u.OrganizeError),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateOrganize: %w", err)
	}
	return nil
}

// UpdateOutcome updates the Jellyfin resolution columns for the given decision row.
func (m *MediaDB) UpdateOutcome(id int64, u OutcomeUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// First-write-wins: only resolve a row if it has not yet been
	// Jellyfin-resolved.  This prevents the sweeper and the webhook
	// handler (or two concurrent sweep windows) from overwriting each
	// other's provider IDs in a last-write-wins race.
	_, err := m.db.Exec(`
		UPDATE parse_decisions SET
			jellyfin_item_id = ?,
			jellyfin_imdb_id = ?,
			jellyfin_tmdb_id = ?,
			jellyfin_tvdb_id = ?,
			jellyfin_resolved_at = ?,
			jellyfin_identified = COALESCE(?, jellyfin_identified),
			jellyfin_first_seen_at = COALESCE(jellyfin_first_seen_at, ?)
		WHERE id = ? AND jellyfin_resolved_at IS NULL`,
		nullStr(u.JellyfinItemID), nullStr(u.JellyfinImdbID),
		nullStr(u.JellyfinTmdbID), nullStr(u.JellyfinTvdbID),
		nullTimePtr(u.JellyfinResolvedAt),
		nullBoolPtr(u.JellyfinIdentified),
		nullTimePtr(u.JellyfinFirstSeenAt),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateOutcome: %w", err)
	}
	return nil
}

// UpgradeOutcome updates Jellyfin resolution columns even when a previous
// resolution exists, but only when the incoming state is "more identified"
// than what's stored. Used by ItemUpdated webhook (metadata typically
// attaches via Update events, not Add) and by the unidentified sweeper pass
// (which can downgrade a row to identified=0).
func (m *MediaDB) UpgradeOutcome(id int64, u OutcomeUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		UPDATE parse_decisions SET
			jellyfin_item_id      = COALESCE(NULLIF(?, ''), jellyfin_item_id),
			jellyfin_imdb_id      = COALESCE(NULLIF(?, ''), jellyfin_imdb_id),
			jellyfin_tmdb_id      = COALESCE(NULLIF(?, ''), jellyfin_tmdb_id),
			jellyfin_tvdb_id      = COALESCE(NULLIF(?, ''), jellyfin_tvdb_id),
			jellyfin_resolved_at  = COALESCE(?, jellyfin_resolved_at),
			jellyfin_identified   = COALESCE(?, jellyfin_identified),
			jellyfin_first_seen_at = COALESCE(jellyfin_first_seen_at, ?)
		WHERE id = ?`,
		nullStr(u.JellyfinItemID), nullStr(u.JellyfinImdbID),
		nullStr(u.JellyfinTmdbID), nullStr(u.JellyfinTvdbID),
		nullTimePtr(u.JellyfinResolvedAt),
		nullBoolPtr(u.JellyfinIdentified),
		nullTimePtr(u.JellyfinFirstSeenAt),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpgradeOutcome: %w", err)
	}
	return nil
}

// ClearOutcome clears Jellyfin resolution fields when an item is removed
// from the Jellyfin library. Marks identified=0 so the row surfaces in
// "needs-review" lists. Does not touch auto_label.
func (m *MediaDB) ClearOutcome(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		UPDATE parse_decisions SET
			jellyfin_item_id = NULL,
			jellyfin_imdb_id = NULL,
			jellyfin_tmdb_id = NULL,
			jellyfin_tvdb_id = NULL,
			jellyfin_resolved_at = NULL,
			jellyfin_identified = 0
		WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ClearOutcome: %w", err)
	}
	return nil
}

// UpdateAutoLabel sets the auto_label column for the given decision row.
// It is idempotent under races: a write with computedAt only succeeds when no
// label exists yet OR the existing label is older than computedAt.  Callers
// without a specific timestamp can use UpdateAutoLabel which captures the
// current time.
func (m *MediaDB) UpdateAutoLabel(id int64, label string) error {
	return m.UpdateAutoLabelAt(id, label, time.Now().UTC())
}

// UpdateAutoLabelAt is the timestamp-aware variant of UpdateAutoLabel.  Pass
// the moment the caller derived the label; the write is suppressed when a
// fresher label is already present, preventing stale sweeper writes from
// overwriting a newer labeler decision.
func (m *MediaDB) UpdateAutoLabelAt(id int64, label string, computedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		UPDATE parse_decisions
		   SET auto_label = ?, auto_label_at = ?
		 WHERE id = ?
		   AND (auto_label IS NULL OR auto_label_at IS NULL OR auto_label_at < ?)`,
		nullStr(label), computedAt, id, computedAt)
	if err != nil {
		return fmt.Errorf("UpdateAutoLabel: %w", err)
	}
	return nil
}

// ClearAutoLabel removes any existing auto_label and timestamp for the given
// decision row, forcing the labeling runner to re-derive a label on its next
// pass.
func (m *MediaDB) ClearAutoLabel(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`UPDATE parse_decisions SET auto_label = NULL, auto_label_at = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ClearAutoLabel: %w", err)
	}
	return nil
}

// UpdateHumanOverride sets the human_label_override column for the given decision row.
func (m *MediaDB) UpdateHumanOverride(id int64, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE parse_decisions SET human_label_override = ? WHERE id = ?`, nullStr(label), id)
	if err != nil {
		return fmt.Errorf("UpdateHumanOverride: %w", err)
	}
	return nil
}

// QueryDecisions returns decision rows matching the given filter.
func (m *MediaDB) QueryDecisions(f QueryFilter) ([]*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var clauses []string
	var args []any

	if f.OrganizeOutcome != "" {
		clauses = append(clauses, "organize_outcome = ?")
		args = append(args, f.OrganizeOutcome)
	}
	if f.AutoLabel != "" {
		clauses = append(clauses, "auto_label = ?")
		args = append(args, f.AutoLabel)
	}
	if f.AutoLabelIsNull {
		clauses = append(clauses, "auto_label IS NULL")
	}
	if f.ParseMethod != "" {
		clauses = append(clauses, "parse_method = ?")
		args = append(args, f.ParseMethod)
	}
	if f.StrippedToken != "" {
		clauses = append(clauses, "parser_stripped_tokens LIKE ?")
		args = append(args, "%"+f.StrippedToken+"%")
	}
	if f.SourceContains != "" {
		clauses = append(clauses, "source_path LIKE ?")
		args = append(args, "%"+f.SourceContains+"%")
	}
	if f.JellyfinUnresolved {
		clauses = append(clauses, "jellyfin_resolved_at IS NULL")
	}
	if f.TargetPathNotEmpty {
		clauses = append(clauses, "target_path IS NOT NULL AND target_path != ''")
	}
	if f.EventAfter != nil {
		clauses = append(clauses, "event_at > ?")
		args = append(args, *f.EventAfter)
	}
	if f.EventBefore != nil {
		clauses = append(clauses, "event_at < ?")
		args = append(args, *f.EventBefore)
	}

	query := "SELECT " + decisionColumns + " FROM parse_decisions"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY id DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("QueryDecisions: %w", err)
	}
	defer rows.Close()

	var results []*ParseDecision
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, fmt.Errorf("QueryDecisions scan: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// GetUnresolvedDecisionByTargetPath returns the most recent successful organize
// decision for the given target path that has not yet been Jellyfin-resolved.
func (m *MediaDB) GetUnresolvedDecisionByTargetPath(targetPath string) (*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(`
		SELECT `+decisionColumns+`
		FROM parse_decisions
		WHERE target_path = ?
		  AND organize_outcome = 'success'
		  AND jellyfin_resolved_at IS NULL
		ORDER BY target_at DESC, event_at DESC, id DESC
		LIMIT 1`,
		targetPath,
	)
	d, err := scanDecision(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetDecisionByTargetPath returns the most-recent successful organize
// decision for the given target_path, regardless of Jellyfin resolution
// state. Used by ItemUpdated/ItemRemoved webhook handlers which need to
// find an already-resolved row to upgrade or clear.
func (m *MediaDB) GetDecisionByTargetPath(targetPath string) (*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(`
		SELECT `+decisionColumns+`
		FROM parse_decisions
		WHERE target_path = ?
		  AND organize_outcome = 'success'
		ORDER BY target_at DESC, event_at DESC, id DESC
		LIMIT 1`,
		targetPath,
	)
	d, err := scanDecision(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// HasRecentSuccessForSource returns true when there is a parse_decisions row
// for sourcePath with organize_outcome = 'success' and event_at within the
// given lookback window.  Used by the cleanup gate to ensure we only purge
// source directories that this daemon successfully organized — never bare
// files a user may have dropped in.
func (m *MediaDB) HasRecentSuccessForSource(sourcePath string, lookback time.Duration) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-lookback)
	var n int
	err := m.db.QueryRow(`
		SELECT COUNT(1)
		FROM parse_decisions
		WHERE source_path = ?
		  AND organize_outcome = 'success'
		  AND event_at >= ?`,
		sourcePath, cutoff,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// scanner abstracts *sql.Row and *sql.Rows for scanDecision.
type scanner interface {
	Scan(dest ...any) error
}

const decisionColumns = `id, source_path, source_filename, event_at,
	media_type_guessed, parse_method, parsed_title,
	parsed_year, parsed_season, parsed_episode,
	parser_stripped_tokens, target_path, target_at,
	existing_match_method, organize_outcome, organize_error,
	jellyfin_item_id, jellyfin_imdb_id, jellyfin_tmdb_id, jellyfin_tvdb_id,
	jellyfin_resolved_at, jellyfin_identified, jellyfin_first_seen_at,
	auto_label, auto_label_at, human_label_override`

func scanDecision(s scanner) (*ParseDecision, error) {
	var d ParseDecision
	var (
		mediaTypeGuessed     sql.NullString
		parseMethod          sql.NullString
		parsedTitle          sql.NullString
		parsedYear           sql.NullInt64
		parsedSeason         sql.NullInt64
		parsedEpisode        sql.NullInt64
		parserStrippedTokens sql.NullString
		targetPath           sql.NullString
		targetAt             sql.NullTime
		existingMatchMethod  sql.NullString
		organizeOutcome      sql.NullString
		organizeError        sql.NullString
		jellyfinItemID       sql.NullString
		jellyfinImdbID       sql.NullString
		jellyfinTmdbID       sql.NullString
		jellyfinTvdbID       sql.NullString
		jellyfinResolvedAt   sql.NullTime
		jellyfinIdentified   sql.NullInt64
		jellyfinFirstSeenAt  sql.NullTime
		autoLabel            sql.NullString
		autoLabelAt          sql.NullTime
		humanLabelOverride   sql.NullString
	)

	err := s.Scan(
		&d.ID, &d.SourcePath, &d.SourceFilename, &d.EventAt,
		&mediaTypeGuessed, &parseMethod, &parsedTitle,
		&parsedYear, &parsedSeason, &parsedEpisode,
		&parserStrippedTokens, &targetPath, &targetAt,
		&existingMatchMethod, &organizeOutcome, &organizeError,
		&jellyfinItemID, &jellyfinImdbID, &jellyfinTmdbID, &jellyfinTvdbID,
		&jellyfinResolvedAt, &jellyfinIdentified, &jellyfinFirstSeenAt,
		&autoLabel, &autoLabelAt, &humanLabelOverride,
	)
	if err != nil {
		return nil, err
	}

	d.MediaTypeGuessed = mediaTypeGuessed.String
	d.ParseMethod = parseMethod.String
	d.ParsedTitle = parsedTitle.String
	if parsedYear.Valid {
		v := int(parsedYear.Int64)
		d.ParsedYear = &v
	}
	if parsedSeason.Valid {
		v := int(parsedSeason.Int64)
		d.ParsedSeason = &v
	}
	if parsedEpisode.Valid {
		v := int(parsedEpisode.Int64)
		d.ParsedEpisode = &v
	}
	d.ParserStrippedTokens = parserStrippedTokens.String
	d.TargetPath = targetPath.String
	if targetAt.Valid {
		t := targetAt.Time
		d.TargetAt = &t
	}
	d.ExistingMatchMethod = existingMatchMethod.String
	d.OrganizeOutcome = organizeOutcome.String
	d.OrganizeError = organizeError.String
	d.JellyfinItemID = jellyfinItemID.String
	d.JellyfinImdbID = jellyfinImdbID.String
	d.JellyfinTmdbID = jellyfinTmdbID.String
	d.JellyfinTvdbID = jellyfinTvdbID.String
	if jellyfinResolvedAt.Valid {
		t := jellyfinResolvedAt.Time
		d.JellyfinResolvedAt = &t
	}
	if jellyfinIdentified.Valid {
		v := jellyfinIdentified.Int64 != 0
		d.JellyfinIdentified = &v
	}
	if jellyfinFirstSeenAt.Valid {
		t := jellyfinFirstSeenAt.Time
		d.JellyfinFirstSeenAt = &t
	}
	d.AutoLabel = autoLabel.String
	if autoLabelAt.Valid {
		t := autoLabelAt.Time
		d.AutoLabelAt = &t
	}
	d.HumanLabelOverride = humanLabelOverride.String

	return &d, nil
}

// QueryStaleLabeledDecisions returns rows that already have an auto_label but
// whose label was written more than olderThan ago AND that have been
// Jellyfin-resolved.  The labeling runner uses this to re-derive labels after
// a Jellyfin rename or a late provider-ID resolution would otherwise leave a
// stale PASS / DRIFT / FAIL on the row.
func (m *MediaDB) QueryStaleLabeledDecisions(olderThan time.Duration, limit int) ([]*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-olderThan)

	query := `SELECT ` + decisionColumns + `
		FROM parse_decisions
		WHERE auto_label IS NOT NULL
		  AND auto_label_at IS NOT NULL
		  AND auto_label_at < ?
		  AND jellyfin_resolved_at IS NOT NULL
		ORDER BY auto_label_at ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := m.db.Query(query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("QueryStaleLabeledDecisions: %w", err)
	}
	defer rows.Close()

	var results []*ParseDecision
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, fmt.Errorf("QueryStaleLabeledDecisions scan: %w", err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// GetMostRecentDecisionBySourcePath returns the most recent prior parse
// decision for the given source path, or nil when none exists.  The handler
// uses this to populate ExistingMatchMethod on a freshly inserted row so the
// audit can show how the same file was previously parsed.
func (m *MediaDB) GetMostRecentDecisionBySourcePath(sourcePath string) (*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(`
		SELECT `+decisionColumns+`
		FROM parse_decisions
		WHERE source_path = ?
		ORDER BY id DESC
		LIMIT 1`, sourcePath)
	d, err := scanDecision(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// DeterministicFailureRow is a minimal projection for hydrating the in-memory
// negative cache from persistent history at daemon startup. Each row
// represents the latest failed organize attempt for a given source path.
type DeterministicFailureRow struct {
	SourcePath string
	Failures   int
	LastError  string
	LastAt     time.Time
}

// GetRecentDeterministicFailures returns the most recent failed organize
// attempts whose source path appears more than once with a failure outcome
// and whose latest failure is within the given lookback window. The caller
// applies its own pattern filter (e.g. IsDeterministicUnparseable) on the
// returned LastError to decide whether to seed the cache. Aggregating in SQL
// keeps the result small even when parse_decisions has many rows.
func (m *MediaDB) GetRecentDeterministicFailures(lookback time.Duration) ([]DeterministicFailureRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-lookback)
	rows, err := m.db.Query(`
		SELECT source_path,
		       COUNT(1) AS failures,
		       MAX(event_at) AS last_at,
		       (SELECT organize_error
		          FROM parse_decisions p2
		         WHERE p2.source_path = p1.source_path
		           AND p2.organize_outcome = 'failed'
		         ORDER BY p2.event_at DESC LIMIT 1) AS last_error
		FROM parse_decisions p1
		WHERE organize_outcome = 'failed'
		  AND event_at >= ?
		GROUP BY source_path
		HAVING failures >= 1`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("GetRecentDeterministicFailures: %w", err)
	}
	defer rows.Close()

	var out []DeterministicFailureRow
	for rows.Next() {
		var (
			r        DeterministicFailureRow
			lastAtRaw string
			lastErr   sql.NullString
		)
		if err := rows.Scan(&r.SourcePath, &r.Failures, &lastAtRaw, &lastErr); err != nil {
			return nil, fmt.Errorf("GetRecentDeterministicFailures scan: %w", err)
		}
		r.LastError = lastErr.String
		// SQLite returns MAX(event_at) as a string; parse it back. We try
		// the canonical RFC3339 layout first (how InsertDecision writes it
		// via time.Now().UTC()) then fall back to SQLite's default
		// "2006-01-02 15:04:05" format for older rows.
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
			if t, perr := time.Parse(layout, lastAtRaw); perr == nil {
				r.LastAt = t
				break
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkDecisionQueued sets organize_outcome = 'queued' for a decision row that
// has been handed off to the AI enhancement queue, distinguishing it from
// rows that have not yet been organize-attempted (NULL) or that have failed.
func (m *MediaDB) MarkDecisionQueued(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`UPDATE parse_decisions SET organize_outcome = 'queued' WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("MarkDecisionQueued: %w", err)
	}
	return nil
}

// nullStr converts an empty string to sql.NullString{Valid:false}.
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullIntPtr converts a *int to sql.NullInt64.
func nullIntPtr(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

// nullTimePtr converts a *time.Time to sql.NullTime.
func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullBoolPtr converts a *bool to sql.NullInt64 (SQLite stores bools as 0/1).
func nullBoolPtr(b *bool) sql.NullInt64 {
	if b == nil {
		return sql.NullInt64{}
	}
	v := int64(0)
	if *b {
		v = 1
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

// IdentificationCounts is the result returned by IdentificationStats.
type IdentificationCounts struct {
	Total           int // organize_outcome='success' AND target_path NOT NULL
	Resolved        int // jellyfin_resolved_at NOT NULL
	Identified      int // jellyfin_identified=1
	Unidentified    int // jellyfin_identified=0
	PendingNoSeen   int // resolved still NULL
	FailedAutoLabel int // auto_label='FAIL'
}

// IdentificationStats returns counts feeding the dashboard identification
// card. Restricted to rows that successfully organized to a target path,
// since unsuccessful rows are not eligible for Jellyfin identification.
func (m *MediaDB) IdentificationStats() (IdentificationCounts, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var c IdentificationCounts
	row := m.db.QueryRow(`
		SELECT
			COUNT(*) AS total,
			SUM(CASE WHEN jellyfin_resolved_at IS NOT NULL THEN 1 ELSE 0 END) AS resolved,
			SUM(CASE WHEN jellyfin_identified = 1 THEN 1 ELSE 0 END) AS identified,
			SUM(CASE WHEN jellyfin_identified = 0 THEN 1 ELSE 0 END) AS unidentified,
			SUM(CASE WHEN jellyfin_resolved_at IS NULL THEN 1 ELSE 0 END) AS pending,
			SUM(CASE WHEN auto_label = 'FAIL' THEN 1 ELSE 0 END) AS failed
		FROM parse_decisions
		WHERE organize_outcome = 'success' AND target_path IS NOT NULL AND target_path != ''`)
	if err := row.Scan(&c.Total, &c.Resolved, &c.Identified, &c.Unidentified,
		&c.PendingNoSeen, &c.FailedAutoLabel); err != nil {
		return c, fmt.Errorf("IdentificationStats: %w", err)
	}
	return c, nil
}

// IdentificationStatusFilter selects which slice of the identification
// pipeline to enumerate via ListIdentificationItems.
type IdentificationStatusFilter string

const (
	IdentificationUnidentified IdentificationStatusFilter = "unidentified"
	IdentificationPending      IdentificationStatusFilter = "pending"
	IdentificationFailed       IdentificationStatusFilter = "failed"
	IdentificationIdentified   IdentificationStatusFilter = "identified"
)

// ListIdentificationItems returns parse_decisions rows matching the given
// identification status. Always restricted to organize_outcome='success'
// rows with a non-empty target_path; unsuccessful organizes don't surface
// in the identification dashboard.
func (m *MediaDB) ListIdentificationItems(status IdentificationStatusFilter, limit int) ([]*ParseDecision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	var where string
	switch status {
	case IdentificationUnidentified:
		where = "jellyfin_identified = 0"
	case IdentificationPending:
		where = "jellyfin_resolved_at IS NULL"
	case IdentificationFailed:
		where = "auto_label = 'FAIL'"
	case IdentificationIdentified:
		where = "jellyfin_identified = 1"
	default:
		return nil, fmt.Errorf("ListIdentificationItems: unknown status %q", status)
	}

	q := `SELECT ` + decisionColumns + ` FROM parse_decisions
	      WHERE organize_outcome = 'success'
	        AND target_path IS NOT NULL AND target_path != ''
	        AND ` + where + `
	      ORDER BY COALESCE(target_at, event_at) DESC
	      LIMIT ?`
	rows, err := m.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("ListIdentificationItems: %w", err)
	}
	defer rows.Close()

	var out []*ParseDecision
	for rows.Next() {
		d, err := scanDecision(rows)
		if err != nil {
			return nil, fmt.Errorf("ListIdentificationItems scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
// Used by the streaming parses-audit op to compute progress totals.
func (m *MediaDB) CountParseDecisions() (int, error) {
m.mu.RLock()
defer m.mu.RUnlock()
var n int
if err := m.db.QueryRow(`SELECT COUNT(*) FROM parse_decisions`).Scan(&n); err != nil {
return 0, fmt.Errorf("CountParseDecisions: %w", err)
}
return n, nil
}

// AuditParseDecisionsPage performs a no-op pass over a slice of rows. The
// streaming audit op exists to surface progress and let cancellation work; the
// underlying audit logic lives in cmd/jellywatch (CLI) and labeling/runner.go.
// This page-iterator simply scans rows so the operator sees granular progress
// while the labeler ticker runs separately. Returns the number of rows
// inspected (0 when offset is past the end).
func (m *MediaDB) AuditParseDecisionsPage(offset, limit int) (int, error) {
m.mu.RLock()
defer m.mu.RUnlock()
rows, err := m.db.Query(
`SELECT id FROM parse_decisions ORDER BY id LIMIT ? OFFSET ?`,
limit, offset)
if err != nil {
return 0, fmt.Errorf("AuditParseDecisionsPage: %w", err)
}
defer rows.Close()
n := 0
for rows.Next() {
var id int64
if err := rows.Scan(&id); err != nil {
return n, err
}
n++
}
return n, rows.Err()
}
