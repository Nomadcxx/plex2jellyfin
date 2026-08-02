package database

import (
	"testing"
	"time"
)

func TestUnknownSeasonRefreshStatePersistsAttemptsAndCooldown(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	next := now.Add(6 * time.Hour)

	if err := db.RecordUnknownSeasonRefresh(UnknownSeasonRefreshState{
		SeriesID:      "series-1",
		SeriesName:    "Clear Show",
		AttemptCount:  1,
		LastAttemptAt: &now,
		NextAttemptAt: &next,
		LastOutcome:   "refreshed",
	}); err != nil {
		t.Fatalf("RecordUnknownSeasonRefresh: %v", err)
	}

	got, err := db.GetUnknownSeasonRefresh("series-1")
	if err != nil {
		t.Fatalf("GetUnknownSeasonRefresh: %v", err)
	}
	if got == nil {
		t.Fatal("expected refresh state row")
	}
	if got.SeriesID != "series-1" || got.SeriesName != "Clear Show" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.AttemptCount != 1 || got.LastOutcome != "refreshed" {
		t.Fatalf("unexpected attempt fields: %+v", got)
	}
	if got.LastAttemptAt == nil || !got.LastAttemptAt.Equal(now) {
		t.Fatalf("unexpected last_attempt_at: %v", got.LastAttemptAt)
	}
	if got.NextAttemptAt == nil || !got.NextAttemptAt.Equal(next) {
		t.Fatalf("unexpected next_attempt_at: %v", got.NextAttemptAt)
	}

	due, err := db.UnknownSeasonRefreshDue("series-1", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("UnknownSeasonRefreshDue during cooldown: %v", err)
	}
	if due {
		t.Fatal("expected series to be cooling down")
	}

	due, err = db.UnknownSeasonRefreshDue("series-1", next)
	if err != nil {
		t.Fatalf("UnknownSeasonRefreshDue at boundary: %v", err)
	}
	if !due {
		t.Fatal("expected series to be due at next_attempt_at")
	}

	due, err = db.UnknownSeasonRefreshDue("missing", now)
	if err != nil {
		t.Fatalf("UnknownSeasonRefreshDue missing: %v", err)
	}
	if !due {
		t.Fatal("missing series should be due")
	}
}
