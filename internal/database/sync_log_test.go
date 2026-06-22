package database

import "testing"

func TestGetRecentSyncLogsAllowsNullErrorMessage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if _, err := db.DB().Exec(`
		INSERT INTO sync_log (source, started_at, completed_at, status, items_processed, items_added, items_updated, error_message)
		VALUES ('filesystem', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'success', 1, 1, 0, NULL)
	`); err != nil {
		t.Fatal(err)
	}

	logs, err := db.GetRecentSyncLogs(10)
	if err != nil {
		t.Fatalf("GetRecentSyncLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].ErrorMessage != "" {
		t.Fatalf("expected empty error message for NULL, got %q", logs[0].ErrorMessage)
	}
}
