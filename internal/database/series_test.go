package database

import (
	"testing"
)

func TestUpsertSeries_SetsDirtyFlag(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	err = db.SetSeriesDirty(series.ID)
	if err != nil {
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}
	if !retrieved.SonarrPathDirty {
		t.Error("series should be dirty after SetSeriesDirty")
	}

	err = db.MarkSeriesSynced(series.ID)
	if err != nil {
		t.Fatalf("MarkSeriesSynced failed: %v", err)
	}

	retrieved, err = db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID after sync failed: %v", err)
	}
	if retrieved.SonarrPathDirty {
		t.Error("series should not be dirty after MarkSeriesSynced")
	}
}

func TestUpsertSeries_ScansNewColumns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:          "Test Show",
		Year:           2020,
		CanonicalPath:  "/tv/Test Show (2020)",
		LibraryRoot:    "/tv",
		Source:         "jellywatch",
		SourcePriority: 75,
		EpisodeCount:   10,
	}

	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries with new columns failed: %v", err)
	}

	retrieved, err := db.GetSeriesByID(series.ID)
	if err != nil {
		t.Fatalf("GetSeriesByID failed: %v", err)
	}

	if retrieved.SourcePriority != 75 {
		t.Errorf("expected SourcePriority 75, got %d", retrieved.SourcePriority)
	}
	if retrieved.EpisodeCount != 10 {
		t.Errorf("expected EpisodeCount 10, got %d", retrieved.EpisodeCount)
	}

	if retrieved.SonarrPathDirty {
		t.Error("SonarrPathDirty should default to false")
	}
	if retrieved.RadarrPathDirty {
		t.Error("RadarrPathDirty should default to false")
	}
	if retrieved.SonarrSyncedAt != nil {
		t.Error("SonarrSyncedAt should default to nil")
	}
	if retrieved.RadarrSyncedAt != nil {
		t.Error("RadarrSyncedAt should default to nil")
	}
}

func TestGetAllSeries_IncludesDirtyFlags(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	series := &Series{
		Title:         "Test Show",
		Year:          2020,
		CanonicalPath: "/tv/Test Show (2020)",
		LibraryRoot:   "/tv",
		Source:        "jellywatch",
	}
	_, err := db.UpsertSeries(series)
	if err != nil {
		t.Fatalf("UpsertSeries failed: %v", err)
	}

	err = db.SetSeriesDirty(series.ID)
	if err != nil {
		t.Fatalf("SetSeriesDirty failed: %v", err)
	}

	allSeries, err := db.GetAllSeries()
	if err != nil {
		t.Fatalf("GetAllSeries failed: %v", err)
	}

	if len(allSeries) != 1 {
		t.Fatalf("expected 1 series, got %d", len(allSeries))
	}

	retrieved := allSeries[0]
	if !retrieved.SonarrPathDirty {
		t.Error("dirty flag not included in GetAllSeries result")
	}
	if retrieved.SonarrSyncedAt != nil {
		t.Error("SonarrSyncedAt should be nil before syncing")
	}
}

func TestGetAllSeries_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	allSeries, err := db.GetAllSeries()
	if err != nil {
		t.Fatalf("GetAllSeries failed on empty DB: %v", err)
	}

	if len(allSeries) != 0 {
		t.Errorf("expected 0 series, got %d", len(allSeries))
	}
}
