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
	AutoLabel            string
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
	JellyfinItemID     string
	JellyfinImdbID     string
	JellyfinTmdbID     string
	JellyfinTvdbID     string
	JellyfinResolvedAt *time.Time
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

	_, err := m.db.Exec(`
		UPDATE parse_decisions SET
			jellyfin_item_id = ?,
			jellyfin_imdb_id = ?,
			jellyfin_tmdb_id = ?,
			jellyfin_tvdb_id = ?,
			jellyfin_resolved_at = ?
		WHERE id = ?`,
		nullStr(u.JellyfinItemID), nullStr(u.JellyfinImdbID),
		nullStr(u.JellyfinTmdbID), nullStr(u.JellyfinTvdbID),
		nullTimePtr(u.JellyfinResolvedAt),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateOutcome: %w", err)
	}
	return nil
}

// UpdateAutoLabel sets the auto_label column for the given decision row.
func (m *MediaDB) UpdateAutoLabel(id int64, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE parse_decisions SET auto_label = ? WHERE id = ?`, nullStr(label), id)
	if err != nil {
		return fmt.Errorf("UpdateAutoLabel: %w", err)
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
	jellyfin_resolved_at, auto_label, human_label_override`

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
		autoLabel            sql.NullString
		humanLabelOverride   sql.NullString
	)

	err := s.Scan(
		&d.ID, &d.SourcePath, &d.SourceFilename, &d.EventAt,
		&mediaTypeGuessed, &parseMethod, &parsedTitle,
		&parsedYear, &parsedSeason, &parsedEpisode,
		&parserStrippedTokens, &targetPath, &targetAt,
		&existingMatchMethod, &organizeOutcome, &organizeError,
		&jellyfinItemID, &jellyfinImdbID, &jellyfinTmdbID, &jellyfinTvdbID,
		&jellyfinResolvedAt, &autoLabel, &humanLabelOverride,
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
	d.AutoLabel = autoLabel.String
	d.HumanLabelOverride = humanLabelOverride.String

	return &d, nil
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
