package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRepairEventRoundTrip(t *testing.T) {
	db, err := OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	event := RepairEvent{
		EventAt:      time.Date(2026, 6, 19, 2, 0, 0, 0, time.UTC),
		Action:       "parser_drift_rename",
		SafetyClass:  "auto_safe",
		Confidence:   0.98,
		SourcePath:   "/mnt/STORAGE1/MOVIES/Ratatouille RoDubbed (2007)/Ratatouille RoDubbed (2007).mkv",
		TargetPath:   "/mnt/STORAGE1/MOVIES/Ratatouille (2007)/Ratatouille (2007).mkv",
		Outcome:      "success",
		LLMConsulted: false,
		EvidenceJSON: `{"parser":"Ratatouille"}`,
	}

	id, err := db.InsertRepairEvent(event)
	if err != nil {
		t.Fatalf("InsertRepairEvent: %v", err)
	}
	if id == 0 {
		t.Fatal("expected inserted id")
	}

	rows, err := db.ListRepairEventsSince(event.EventAt.Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("ListRepairEventsSince: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Action != event.Action || rows[0].TargetPath != event.TargetPath {
		t.Fatalf("round trip mismatch: %+v", rows[0])
	}
}
