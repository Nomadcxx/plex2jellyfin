package database

import (
	"database/sql"
	"fmt"
	"time"
)

// UnknownSeasonRefreshState tracks automated unknown-season series refresh attempts.
type UnknownSeasonRefreshState struct {
	SeriesID      string
	SeriesName    string
	AttemptCount  int
	LastAttemptAt *time.Time
	NextAttemptAt *time.Time
	LastOutcome   string
	LastError     string
}

func (m *MediaDB) GetUnknownSeasonRefresh(seriesID string) (*UnknownSeasonRefreshState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var (
		state     UnknownSeasonRefreshState
		lastAt    sql.NullTime
		nextAt    sql.NullTime
		name      sql.NullString
		outcome   sql.NullString
		lastError sql.NullString
	)
	err := m.db.QueryRow(`
		SELECT series_id, series_name, attempt_count, last_attempt_at, next_attempt_at,
		       last_outcome, last_error
		  FROM unknown_season_refresh_state
		 WHERE series_id = ?`, seriesID).Scan(
		&state.SeriesID, &name, &state.AttemptCount, &lastAt, &nextAt, &outcome, &lastError)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetUnknownSeasonRefresh: %w", err)
	}
	state.SeriesName = name.String
	state.LastOutcome = outcome.String
	state.LastError = lastError.String
	if lastAt.Valid {
		t := lastAt.Time.UTC()
		state.LastAttemptAt = &t
	}
	if nextAt.Valid {
		t := nextAt.Time.UTC()
		state.NextAttemptAt = &t
	}
	return &state, nil
}

func (m *MediaDB) RecordUnknownSeasonRefresh(state UnknownSeasonRefreshState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO unknown_season_refresh_state (
			series_id, series_name, attempt_count, last_attempt_at, next_attempt_at,
			last_outcome, last_error, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(series_id) DO UPDATE SET
			series_name = excluded.series_name,
			attempt_count = excluded.attempt_count,
			last_attempt_at = excluded.last_attempt_at,
			next_attempt_at = excluded.next_attempt_at,
			last_outcome = excluded.last_outcome,
			last_error = excluded.last_error,
			updated_at = CURRENT_TIMESTAMP`,
		state.SeriesID,
		nullStr(state.SeriesName),
		state.AttemptCount,
		nullTimePtr(state.LastAttemptAt),
		nullTimePtr(state.NextAttemptAt),
		nullStr(state.LastOutcome),
		nullStr(state.LastError),
	)
	if err != nil {
		return fmt.Errorf("RecordUnknownSeasonRefresh: %w", err)
	}
	return nil
}

// UnknownSeasonRefreshDue reports whether seriesID may be refreshed at now.
// Missing rows are always due.
func (m *MediaDB) UnknownSeasonRefreshDue(seriesID string, now time.Time) (bool, error) {
	state, err := m.GetUnknownSeasonRefresh(seriesID)
	if err != nil {
		return false, err
	}
	if state == nil || state.NextAttemptAt == nil {
		return true, nil
	}
	return !now.Before(state.NextAttemptAt.UTC()), nil
}
