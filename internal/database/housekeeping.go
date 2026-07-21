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
	TaskKindMoveMerge      = "move_merge"      // legacy folder-walk merge; no longer enqueued by detect, retained for executor compat
	TaskKindNoYearMerge    = "no_year_merge"   // merge no-year folder into year-bearing twin (same library, naming workflow)
	TaskKindYearMismatch   = "year_mismatch"   // flag-only, naming workflow
	TaskKindPollutedName   = "polluted_name"   // flag-only, naming workflow
	TaskKindOrphanSource   = "orphan_source"   // remove empty watch-dir source dir
	TaskKindStuckSync      = "stuck_sync"      // mark stuck sync_log row as error
	TaskKindSubdirMismatch = "subdir_mismatch" // flag-only, naming workflow

	// Convergence (housekeeper ↔ consolidator/cleanup): three workflows.
	// Duplicate-removal workflow (movies + TV):
	TaskKindConsolidateDuplicate = "consolidate_duplicate"  // auto, delete inferior copies via service.CleanupService
	TaskKindCrossVolumeDuplicate = "cross_volume_duplicate" // flag, low-confidence duplicate awaiting human approval
	// Consolidation workflow (TV scatter only):
	TaskKindSeriesConsolidate = "series_consolidate" // auto, move one TV series' scattered episodes onto a single volume
	// Naming workflow:
	TaskKindFolderRename        = "folder_rename"          // auto, rename folder in-place to canonical case (no cross-volume work)
	TaskKindParserDriftRename   = "parser_drift_rename"    // auto, repair Plex2Jellyfin-created movie path after parser fixes
	TaskKindParserDriftTVRename = "parser_drift_tv_rename" // auto, repair Plex2Jellyfin-created TV episode path after parser fixes
	TaskKindVerifierRename      = "verifier_rename"        // auto, rename movie folder to verifier-confirmed title (Phase 2 corrector)
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

// RequeueStaleRunningTasks resets housekeeping_tasks rows left in 'running'
// state by a previous daemon process. The running flag is in-memory only;
// on a clean shutdown each goroutine writes a final status, but on a crash,
// SIGKILL, or systemd restart the row stays "running" forever, blocking the
// queue and confusing the UI. Returns the number of rows updated.
//
// We requeue (not fail) because the previous attempt did not actually
// complete — attempts is left untouched so the retry budget is preserved.
func (m *MediaDB) RequeueStaleRunningTasks() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`
		UPDATE housekeeping_tasks
		   SET status = 'pending',
		       started_at = NULL,
		       last_error = COALESCE(last_error, 'requeued: daemon restarted while running')
		 WHERE status = 'running'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CountDuplicateManualReviewFailures returns how many older failed rows would
// be canceled by CollapseDuplicateManualReviewFailures.
func (m *MediaDB) CountDuplicateManualReviewFailures() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	err := m.db.QueryRow(`
		SELECT COUNT(*)
		  FROM housekeeping_tasks
		 WHERE status = 'failed'
		   AND (last_error LIKE '%manual review required%'
		        OR last_error LIKE '%manual-review required%')
		   AND dedup_key IN (
		       SELECT dedup_key
		         FROM housekeeping_tasks
		        WHERE status = 'failed'
		          AND (last_error LIKE '%manual review required%'
		               OR last_error LIKE '%manual-review required%')
		        GROUP BY dedup_key
		       HAVING COUNT(*) > 1
		   )
		   AND id NOT IN (
		       SELECT MAX(id)
		         FROM housekeeping_tasks
		        WHERE status = 'failed'
		          AND (last_error LIKE '%manual review required%'
		               OR last_error LIKE '%manual-review required%')
		        GROUP BY dedup_key
		   )`).Scan(&count)
	return count, err
}

// CollapseDuplicateManualReviewFailures cancels older failed rows for the same
// dedup key when the failure already says manual review is required. This keeps
// one visible review item while removing historical retry noise from the queue.
func (m *MediaDB) CollapseDuplicateManualReviewFailures() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	res, err := m.db.Exec(`
		UPDATE housekeeping_tasks
		   SET status = 'canceled',
		       finished_at = COALESCE(finished_at, CURRENT_TIMESTAMP),
		       last_error = COALESCE(last_error, 'superseded duplicate manual-review failure')
		 WHERE status = 'failed'
		   AND (last_error LIKE '%manual review required%'
		        OR last_error LIKE '%manual-review required%')
		   AND dedup_key IN (
		       SELECT dedup_key
		         FROM housekeeping_tasks
		        WHERE status = 'failed'
		          AND (last_error LIKE '%manual review required%'
		               OR last_error LIKE '%manual-review required%')
		        GROUP BY dedup_key
		       HAVING COUNT(*) > 1
		   )
		   AND id NOT IN (
		       SELECT MAX(id)
		         FROM housekeeping_tasks
		        WHERE status = 'failed'
		          AND (last_error LIKE '%manual review required%'
		               OR last_error LIKE '%manual-review required%')
		        GROUP BY dedup_key
		   )`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
	if m.housekeepingManualReviewFailedLocked(dedup) {
		return 0, nil
	}

	// Decide initial status: flag-only kinds skip execution and stay
	// in 'flagged' for human review via the WebUI.
	status := TaskStatusPending
	switch kind {
	case TaskKindYearMismatch,
		TaskKindPollutedName,
		TaskKindSubdirMismatch,
		TaskKindCrossVolumeDuplicate:
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

func (m *MediaDB) housekeepingManualReviewFailedLocked(dedup string) bool {
	var status string
	var lastError sql.NullString
	err := m.db.QueryRow(`
		SELECT status, last_error
		  FROM housekeeping_tasks
		 WHERE dedup_key = ?
		 ORDER BY id DESC
		 LIMIT 1`, dedup).Scan(&status, &lastError)
	if err != nil {
		return false
	}
	return status == TaskStatusFailed &&
		lastError.Valid &&
		containsAny(lastError.String, "manual review required", "manual-review required")
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

// ListHousekeepingTasksByKind returns up to `limit` tasks of the given
// kind+status, oldest first. Used by the verifier sweep.
func (m *MediaDB) ListHousekeepingTasksByKind(kind, status string, limit int) ([]*HousekeepingTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 500
	}
	rows, err := m.db.Query(`
		SELECT id, job_name, kind, payload, dedup_key, priority,
		       status, attempts, last_error, created_at, started_at,
		       finished_at, next_attempt_at
		  FROM housekeeping_tasks
		 WHERE kind = ? AND status = ?
		 ORDER BY id ASC
		 LIMIT ?`, kind, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*HousekeepingTask{}
	for rows.Next() {
		t, err := scanHousekeepingTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateHousekeepingTask rewrites a task's kind/status/payload (used when
// the verifier reclassifies a flagged year_mismatch as distinct or as a
// genuine duplicate).
func (m *MediaDB) UpdateHousekeepingTask(id int64, kind, status string, payload map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = m.db.Exec(`
		UPDATE housekeeping_tasks
		   SET kind = ?, status = ?, payload = ?,
		       last_error = NULL,
		       finished_at = CASE WHEN ? IN ('skipped','done','failed','canceled')
		                         THEN CURRENT_TIMESTAMP ELSE finished_at END
		 WHERE id = ?`, kind, status, string(body), status, id)
	return err
}

// GetHousekeepingTask fetches a single task by id.
func (m *MediaDB) GetHousekeepingTask(id int64) (*HousekeepingTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	row := m.db.QueryRow(`
		SELECT id, job_name, kind, payload, dedup_key, priority,
		       status, attempts, last_error, created_at, started_at,
		       finished_at, next_attempt_at
		  FROM housekeeping_tasks
		 WHERE id = ?`, id)
	t, err := scanHousekeepingTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// PurgeHousekeepingTasks deletes terminal-state tasks (done/skipped/
// canceled/failed) so the table doesn't grow unbounded. Pass nil to use
// the default safe set: {done, skipped, canceled}.
func (m *MediaDB) PurgeHousekeepingTasks(statuses []string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(statuses) == 0 {
		statuses = []string{TaskStatusDone, TaskStatusSkipped, TaskStatusCanceled}
	}
	// Build IN-clause safely.
	placeholders := ""
	args := make([]any, 0, len(statuses))
	for i, s := range statuses {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, s)
	}
	res, err := m.db.Exec(`DELETE FROM housekeeping_tasks WHERE status IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// bulkExecHousekeepingTasks runs a parameterized UPDATE across the given IDs
// inside a single transaction, returning the total affected row count.
// Returns partial count on error (does not roll back already-executed rows).
func (m *MediaDB) bulkExecHousekeepingTasks(ids []int64, query string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var n int64
	for _, id := range ids {
		r, err := stmt.Exec(id)
		if err != nil {
			return n, err
		}
		c, _ := r.RowsAffected()
		n += c
	}
	return n, tx.Commit()
}

// BulkRetryHousekeepingTasks resets multiple tasks back to pending.
func (m *MediaDB) BulkRetryHousekeepingTasks(ids []int64) (int64, error) {
	return m.bulkExecHousekeepingTasks(ids, `
		UPDATE housekeeping_tasks
		   SET status = 'pending', attempts = 0, last_error = NULL,
		       started_at = NULL, finished_at = NULL, next_attempt_at = NULL
		 WHERE id = ? AND status IN ('failed','canceled','flagged','skipped')`)
}

// BulkCancelHousekeepingTasks cancels multiple pending/flagged tasks.
func (m *MediaDB) BulkCancelHousekeepingTasks(ids []int64) (int64, error) {
	return m.bulkExecHousekeepingTasks(ids, `
		UPDATE housekeeping_tasks
		   SET status = 'canceled', finished_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status IN ('pending','flagged','failed')`)
}

// BulkApproveHousekeepingTasks promotes flagged review-required tasks
// (cross_volume_duplicate, year_mismatch) into the auto-execute path by
// switching their kind to consolidate_duplicate and resetting status to
// pending. Mirrors the single-task approve flow in taskApproveHandler.
func (m *MediaDB) BulkApproveHousekeepingTasks(ids []int64) (int64, error) {
	return m.bulkExecHousekeepingTasks(ids, `
		UPDATE housekeeping_tasks
		   SET kind = 'consolidate_duplicate', status = 'pending',
		       attempts = 0, last_error = NULL,
		       started_at = NULL, finished_at = NULL, next_attempt_at = NULL
		 WHERE id = ?
		   AND status = 'flagged'
		   AND kind IN ('cross_volume_duplicate','year_mismatch')`)
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
