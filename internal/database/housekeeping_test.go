package database

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestBulkHousekeepingTasks(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	// Enqueue 3 tasks with distinct payloads (dedup key is derived from payload)
	payloads := []map[string]any{
		{"title": "alpha", "src": "/a", "dst": "/b"},
		{"title": "beta", "src": "/c", "dst": "/d"},
		{"title": "gamma", "src": "/e", "dst": "/f"},
	}
	var ids []int64
	for _, p := range payloads {
		id, err := db.EnqueueHousekeepingTask("test", TaskKindNoYearMerge, p, 10)
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		ids = append(ids, id)
	}

	// Claim and fail two of them
	task1, err := db.ClaimNextHousekeepingTask()
	if err != nil || task1 == nil {
		t.Fatalf("claim1: %v", err)
	}
	db.CompleteHousekeepingTask(task1.ID, errors.New("fail"), 1)
	task2, err := db.ClaimNextHousekeepingTask()
	if err != nil || task2 == nil {
		t.Fatalf("claim2: %v", err)
	}
	db.CompleteHousekeepingTask(task2.ID, errors.New("fail"), 1)

	// BulkRetry should reset the 2 failed tasks to pending
	n, err := db.BulkRetryHousekeepingTasks([]int64{task1.ID, task2.ID, ids[2]})
	if err != nil {
		t.Fatalf("BulkRetry: %v", err)
	}
	if n != 2 {
		t.Fatalf("BulkRetry affected %d, want 2", n)
	}

	// Verify both are now pending
	for _, id := range []int64{task1.ID, task2.ID} {
		var status string
		db.db.QueryRow(`SELECT status FROM housekeeping_tasks WHERE id = ?`, id).Scan(&status)
		if status != "pending" {
			t.Fatalf("task %d status = %s, want pending", id, status)
		}
	}

	// BulkCancel should cancel all 3 pending tasks
	n, err = db.BulkCancelHousekeepingTasks(ids)
	if err != nil {
		t.Fatalf("BulkCancel: %v", err)
	}
	if n != 3 {
		t.Fatalf("BulkCancel affected %d, want 3", n)
	}

	// Verify all are canceled
	for _, id := range ids {
		var status string
		db.db.QueryRow(`SELECT status FROM housekeeping_tasks WHERE id = ?`, id).Scan(&status)
		if status != "canceled" {
			t.Fatalf("task %d status = %s, want canceled", id, status)
		}
	}

	// Empty IDs should be a no-op
	n, err = db.BulkRetryHousekeepingTasks(nil)
	if err != nil || n != 0 {
		t.Fatalf("empty BulkRetry: n=%d err=%v, want 0 nil", n, err)
	}

	// Nonexistent ID should affect 0 rows
	n, err = db.BulkRetryHousekeepingTasks([]int64{99999})
	if err != nil || n != 0 {
		t.Fatalf("nonexistent BulkRetry: n=%d err=%v, want 0 nil", n, err)
	}
}

func TestEnqueueHousekeepingTaskSuppressesFailedManualReviewDedup(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	payload := map[string]any{
		"kind":     "tv",
		"title":    "upload",
		"src_path": "/mnt/STORAGE4/TVSHOWS/Upload",
		"dst_path": "/mnt/STORAGE6/TVSHOWS/Upload (2020)",
	}
	id, err := db.EnqueueHousekeepingTask("housekeeping.detect", TaskKindNoYearMerge, payload, 10)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if id == 0 {
		t.Fatal("first enqueue returned no task")
	}

	task, err := db.ClaimNextHousekeepingTask()
	if err != nil {
		t.Fatalf("ClaimNextHousekeepingTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected claimed task")
	}
	if err := db.CompleteHousekeepingTask(task.ID, errors.New("size mismatch - manual review required"), 1); err != nil {
		t.Fatalf("CompleteHousekeepingTask: %v", err)
	}

	againID, err := db.EnqueueHousekeepingTask("housekeeping.detect", TaskKindNoYearMerge, payload, 10)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if againID != 0 {
		t.Fatalf("manual-review failure was re-enqueued as task %d", againID)
	}

	rows, err := db.ListHousekeepingTasks("", 10)
	if err != nil {
		t.Fatalf("ListHousekeepingTasks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d task rows, want 1", len(rows))
	}
}

func TestCollapseDuplicateManualReviewFailures(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	payload := `{"dst_path":"/mnt/STORAGE6/TVSHOWS/Upload (2020)","src_path":"/mnt/STORAGE4/TVSHOWS/Upload","title":"upload"}`
	for i := 0; i < 3; i++ {
		if _, err := db.db.Exec(`
			INSERT INTO housekeeping_tasks
				(job_name, kind, payload, dedup_key, priority, status, attempts, last_error)
			VALUES
				('housekeeping.detect', ?, ?, 'same-dedup', 10, 'failed', 3, 'size mismatch - manual review required')`,
			TaskKindNoYearMerge, payload); err != nil {
			t.Fatalf("insert duplicate failed task %d: %v", i, err)
		}
	}
	if _, err := db.db.Exec(`
		INSERT INTO housekeeping_tasks
			(job_name, kind, payload, dedup_key, priority, status, attempts, last_error)
		VALUES
			('housekeeping.detect', ?, ?, 'different-dedup', 10, 'failed', 3, 'size mismatch - manual review required')`,
		TaskKindNoYearMerge, payload); err != nil {
		t.Fatalf("insert distinct failed task: %v", err)
	}

	duplicates, err := db.CountDuplicateManualReviewFailures()
	if err != nil {
		t.Fatalf("CountDuplicateManualReviewFailures before collapse: %v", err)
	}
	if duplicates != 2 {
		t.Fatalf("duplicates before collapse = %d, want 2", duplicates)
	}

	changed, err := db.CollapseDuplicateManualReviewFailures()
	if err != nil {
		t.Fatalf("CollapseDuplicateManualReviewFailures: %v", err)
	}
	if changed != 2 {
		t.Fatalf("changed = %d, want 2", changed)
	}

	var failed, canceled int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM housekeeping_tasks WHERE dedup_key = 'same-dedup' AND status = 'failed'`).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM housekeeping_tasks WHERE dedup_key = 'same-dedup' AND status = 'canceled'`).Scan(&canceled); err != nil {
		t.Fatal(err)
	}
	if failed != 1 || canceled != 2 {
		t.Fatalf("same-dedup failed/canceled = %d/%d, want 1/2", failed, canceled)
	}

	var distinctFailed int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM housekeeping_tasks WHERE dedup_key = 'different-dedup' AND status = 'failed'`).Scan(&distinctFailed); err != nil {
		t.Fatal(err)
	}
	if distinctFailed != 1 {
		t.Fatalf("distinct failed rows = %d, want 1", distinctFailed)
	}

	duplicates, err = db.CountDuplicateManualReviewFailures()
	if err != nil {
		t.Fatalf("CountDuplicateManualReviewFailures after collapse: %v", err)
	}
	if duplicates != 0 {
		t.Fatalf("duplicates after collapse = %d, want 0", duplicates)
	}
}
