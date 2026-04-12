package database

import (
	"database/sql"
	"time"
)

// SyncLog represents a sync operation record
type SyncLog struct {
	ID             int64
	Source         string
	StartedAt      time.Time
	CompletedAt    *time.Time
	Status         string // "running", "success", "failed"
	ItemsProcessed int
	ItemsAdded     int
	ItemsUpdated   int
	ErrorMessage   string
}

// StartSyncLog creates a new sync log entry and returns its ID
func (m *MediaDB) StartSyncLog(source string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.db.Exec(`
		INSERT INTO sync_log (source, started_at, status)
		VALUES (?, ?, 'running')`,
		source, time.Now(),
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// CompleteSyncLog updates a sync log entry with completion details
func (m *MediaDB) CompleteSyncLog(id int64, status string, processed, added, updated int, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`
		UPDATE sync_log SET
			completed_at = ?,
			status = ?,
			items_processed = ?,
			items_added = ?,
			items_updated = ?,
			error_message = ?
		WHERE id = ?`,
		time.Now(), status, processed, added, updated, errorMsg, id,
	)

	return err
}

// RecoverStuckSyncLogs marks stale "running" entries as failed.
// Call on startup and periodically to prevent permanently stuck entries.
func (m *MediaDB) RecoverStuckSyncLogs(maxAge time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	result, err := m.db.Exec(`
		UPDATE sync_log SET
			status = 'failed',
			completed_at = ?,
			error_message = 'recovered: stale entry exceeded max age'
		WHERE status = 'running' AND started_at < ?`,
		time.Now(), cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetRecentSyncLogs returns the N most recent sync logs
func (m *MediaDB) GetRecentSyncLogs(limit int) ([]*SyncLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT id, source, started_at, completed_at, status,
		       items_processed, items_added, items_updated, error_message
		FROM sync_log
		ORDER BY started_at DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*SyncLog
	for rows.Next() {
		var log SyncLog
		err := rows.Scan(
			&log.ID, &log.Source, &log.StartedAt, &log.CompletedAt,
			&log.Status, &log.ItemsProcessed, &log.ItemsAdded,
			&log.ItemsUpdated, &log.ErrorMessage,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}

	return logs, rows.Err()
}

// GetLastSyncForSource returns the most recent sync for a specific source
func (m *MediaDB) GetLastSyncForSource(source string) (*SyncLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var log SyncLog
	err := m.db.QueryRow(`
		SELECT id, source, started_at, completed_at, status,
		       items_processed, items_added, items_updated, error_message
		FROM sync_log
		WHERE source = ?
		ORDER BY started_at DESC
		LIMIT 1`,
		source,
	).Scan(
		&log.ID, &log.Source, &log.StartedAt, &log.CompletedAt,
		&log.Status, &log.ItemsProcessed, &log.ItemsAdded,
		&log.ItemsUpdated, &log.ErrorMessage,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &log, nil
}
