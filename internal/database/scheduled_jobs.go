package database

import (
	"database/sql"
	"errors"
	"time"
)

// ScheduledJob is a recurring job definition persisted in the database.
type ScheduledJob struct {
	Name           string
	Schedule       string // "HH:MM", "@hourly", "@continuous"
	Enabled        bool
	LastRunAt      sql.NullTime
	LastDurationMS sql.NullInt64
	LastResult     sql.NullString
	LastError      sql.NullString
	NextRunAt      sql.NullTime
	Running        bool
	Config         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UpsertScheduledJob inserts or updates a job definition. Existing
// schedule/enabled values are preserved when the row already exists so
// user edits via the WebUI aren't clobbered on daemon restart.
func (m *MediaDB) UpsertScheduledJob(name, schedule string, enabled bool, config string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		INSERT INTO scheduled_jobs (name, schedule, enabled, config)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO NOTHING`,
		name, schedule, boolToInt(enabled), config)
	return err
}

// GetScheduledJob returns a single job by name.
func (m *MediaDB) GetScheduledJob(name string) (*ScheduledJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row := m.db.QueryRow(`
		SELECT name, schedule, enabled, last_run_at, last_duration_ms, last_result, last_error,
		       next_run_at, running, config, created_at, updated_at
		  FROM scheduled_jobs WHERE name = ?`, name)
	j, err := scanScheduledJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return j, nil
}

// ListScheduledJobs returns all jobs ordered by name.
func (m *MediaDB) ListScheduledJobs() ([]ScheduledJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rows, err := m.db.Query(`
		SELECT name, schedule, enabled, last_run_at, last_duration_ms, last_result, last_error,
		       next_run_at, running, config, created_at, updated_at
		  FROM scheduled_jobs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduledJob
	for rows.Next() {
		j, err := scanScheduledJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// UpdateScheduledJob mutates schedule/enabled (used by API/IPC).
func (m *MediaDB) UpdateScheduledJob(name, schedule string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		UPDATE scheduled_jobs
		   SET schedule = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE name = ?`, schedule, boolToInt(enabled), name)
	return err
}

// MarkScheduledJobRunning flips the running flag.
func (m *MediaDB) MarkScheduledJobRunning(name string, running bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		UPDATE scheduled_jobs SET running = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?`,
		boolToInt(running), name)
	return err
}

// ClearAllRunningJobs resets every scheduled_jobs.running flag to 0. Called
// at daemon startup because the flag is in-memory state owned by the
// previous process — if it crashed or was SIGKILLed, jobs would otherwise
// remain "running" forever and the scheduler would never re-fire them.
func (m *MediaDB) ClearAllRunningJobs() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`UPDATE scheduled_jobs SET running = 0 WHERE running != 0`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RecordScheduledJobRun stores the outcome of a completed run.
func (m *MediaDB) RecordScheduledJobRun(name, result, errStr string, duration time.Duration, nextRun time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var nextRunArg any
	if !nextRun.IsZero() {
		nextRunArg = nextRun.UTC().Format("2006-01-02 15:04:05")
	}
	_, err := m.db.Exec(`
		UPDATE scheduled_jobs
		   SET last_run_at = CURRENT_TIMESTAMP,
		       last_duration_ms = ?,
		       last_result = ?,
		       last_error = NULLIF(?, ''),
		       next_run_at = ?,
		       running = 0,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE name = ?`,
		duration.Milliseconds(), result, errStr, nextRunArg, name)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanScheduledJob(s hkScanner) (*ScheduledJob, error) {
	var j ScheduledJob
	var enabled, running int
	if err := s.Scan(&j.Name, &j.Schedule, &enabled, &j.LastRunAt, &j.LastDurationMS,
		&j.LastResult, &j.LastError, &j.NextRunAt, &running, &j.Config,
		&j.CreatedAt, &j.UpdatedAt); err != nil {
		return nil, err
	}
	j.Enabled = enabled != 0
	j.Running = running != 0
	return &j, nil
}
