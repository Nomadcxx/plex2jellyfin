package database

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Task statuses for housekeeping_tasks.
const (
	TaskStatusPending  = "pending"
	TaskStatusRunning  = "running"
	TaskStatusDone     = "done"
	TaskStatusFailed   = "failed"
	TaskStatusSkipped  = "skipped"
	TaskStatusFlagged  = "flagged" // surfaced in WebUI, never auto-executed
	TaskStatusCanceled = "canceled"
)

// Task kinds (extend as more detectors are added).
const (
	TaskKindMoveMerge       = "move_merge"        // consolidate cross-volume duplicate
	TaskKindNoYearMerge     = "no_year_merge"     // merge no-year folder into year-bearing twin
	TaskKindYearMismatch    = "year_mismatch"     // flag-only
	TaskKindPollutedName    = "polluted_name"     // flag-only
	TaskKindOrphanSource    = "orphan_source"     // remove empty watch-dir source dir
	TaskKindStuckSync       = "stuck_sync"        // mark stuck sync_log row as error
	TaskKindSubdirMismatch  = "subdir_mismatch"   // flag-only
)

// HousekeepingTask is a queued (or completed) housekeeping action.
type HousekeepingTask struct {
	ID            int64
	JobName       string
	Kind          string
	Payload       map[string]any
	DedupKey      string
	Priority      int
	Status        string
	Attempts      int
	LastError     sql.NullString
	CreatedAt     time.Time
	StartedAt     sql.NullTime
	FinishedAt    sql.NullTime
	NextAttemptAt sql.NullTime
}

// EnqueueHousekeepingTask inserts a task. If a pending/running row with the
// same dedup_key exists, the call is a no-op (returns 0, nil).
func (m *MediaDB) EnqueueHousekeepingTask(jobName, kind string, payload map[string]any, priority int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}
	dedup := housekeepingDedupKey(jobName, kind, body)

	// Decide initial status: flag-only kinds skip execution and stay
	// in 'flagged' for human review via the WebUI.
	status := TaskStatusPending
	switch kind {
	case TaskKindYearMismatch, TaskKindPollutedName, TaskKindSubdirMismatch:
		status = TaskStatusFlagged
	}

	res, err := m.db.Exec(`
		INSERT INTO housekeeping_tasks (job_name, kind, payload, dedup_key, priority, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		jobName, kind, string(body), dedup, priority, status)
	if err != nil {
		// Unique-index violation = already enqueued; treat as success.
		var sqliteErr interface{ Error() string }
		if errors.As(err, &sqliteErr) {
			msg := err.Error()
			if containsAny(msg, "UNIQUE constraint", "constraint failed") {
				return 0, nil
			}
		}
		return 0, err
	}
	return res.LastInsertId()
}

func housekeepingDedupKey(jobName, kind string, payload []byte) string {
	h := sha1.New()
	h.Write([]byte(jobName))
	h.Write([]byte{0})
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// ClaimNextHousekeepingTask atomically marks the highest-priority pending
// task as running and returns it. Returns (nil, nil) if nothing ready.
func (m *MediaDB) ClaimNextHousekeepingTask() (*HousekeepingTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
		SELECT id, job_name, kind, payload, dedup_key, priority, status, attempts, last_error,
		       created_at, started_at, finished_at, next_attempt_at
		  FROM housekeeping_tasks
		 WHERE status = 'pending'
		   AND (next_attempt_at IS NULL OR next_attempt_at <= CURRENT_TIMESTAMP)
		 ORDER BY priority ASC, id ASC
		 LIMIT 1`)
	t, err := scanHousekeepingTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if _, err := tx.Exec(`
		UPDATE housekeeping_tasks
		   SET status = 'running', started_at = CURRENT_TIMESTAMP, attempts = attempts + 1
		 WHERE id = ?`, t.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t.Status = TaskStatusRunning
	t.Attempts++
	return t, nil
}

// CompleteHousekeepingTask records the outcome. If err is non-nil and
// retries remain, the task is requeued with backoff; otherwise marked failed.
func (m *MediaDB) CompleteHousekeepingTask(id int64, taskErr error, maxAttempts int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if taskErr == nil {
		_, err := m.db.Exec(`
			UPDATE housekeeping_tasks
			   SET status = 'done', finished_at = CURRENT_TIMESTAMP, last_error = NULL
			 WHERE id = ?`, id)
		return err
	}

	var attempts int
	if err := m.db.QueryRow(`SELECT attempts FROM housekeeping_tasks WHERE id = ?`, id).Scan(&attempts); err != nil {
		return err
	}

	if attempts >= maxAttempts {
		_, err := m.db.Exec(`
			UPDATE housekeeping_tasks
			   SET status = 'failed', finished_at = CURRENT_TIMESTAMP, last_error = ?
			 WHERE id = ?`, taskErr.Error(), id)
		return err
	}

	// Exponential backoff: 2^attempts minutes, capped at 60.
	delayMin := 1 << attempts
	if delayMin > 60 {
		delayMin = 60
	}
	_, err := m.db.Exec(`
		UPDATE housekeeping_tasks
		   SET status = 'pending', last_error = ?, next_attempt_at = datetime('now', ?)
		 WHERE id = ?`, taskErr.Error(), fmt.Sprintf("+%d minutes", delayMin), id)
	return err
}

// ListHousekeepingTasks returns recent tasks, optionally filtered by status.
func (m *MediaDB) ListHousekeepingTasks(status string, limit int) ([]HousekeepingTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 200
	}
	q := `SELECT id, job_name, kind, payload, dedup_key, priority, status, attempts, last_error,
	             created_at, started_at, finished_at, next_attempt_at
	        FROM housekeeping_tasks`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HousekeepingTask
	for rows.Next() {
		t, err := scanHousekeepingTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// CountHousekeepingTasks returns counts grouped by status for the WebUI.
func (m *MediaDB) CountHousekeepingTasks() (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`SELECT status, COUNT(*) FROM housekeeping_tasks GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

// RetryHousekeepingTask resets a failed/canceled/flagged task to pending.
func (m *MediaDB) RetryHousekeepingTask(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		UPDATE housekeeping_tasks
		   SET status = 'pending', attempts = 0, last_error = NULL,
		       started_at = NULL, finished_at = NULL, next_attempt_at = NULL
		 WHERE id = ? AND status IN ('failed','canceled','flagged','skipped')`, id)
	return err
}

// CancelHousekeepingTask marks a pending or flagged task as canceled.
func (m *MediaDB) CancelHousekeepingTask(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`
		UPDATE housekeeping_tasks
		   SET status = 'canceled', finished_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status IN ('pending','flagged')`, id)
	return err
}

type hkScanner interface {
	Scan(dest ...any) error
}

func scanHousekeepingTask(s hkScanner) (*HousekeepingTask, error) {
	var t HousekeepingTask
	var payload string
	if err := s.Scan(&t.ID, &t.JobName, &t.Kind, &payload, &t.DedupKey, &t.Priority,
		&t.Status, &t.Attempts, &t.LastError, &t.CreatedAt, &t.StartedAt, &t.FinishedAt,
		&t.NextAttemptAt); err != nil {
		return nil, err
	}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &t.Payload)
	}
	return &t, nil
}
