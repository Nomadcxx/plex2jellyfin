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
		Source:        "plex2jellyfin",
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
		Source:         "plex2jellyfin",
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

func TestUpsertSeries_ReusesExistingCanonicalPathForFragmentTitle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	canonical := "/media/TV/Project Runway (2004)"
	_, err := db.UpsertSeries(&Series{
		Title:          "Project Runway",
		Year:           2004,
		CanonicalPath:  canonical,
		LibraryRoot:    "/media/TV",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   1,
	})
	if err != nil {
		t.Fatalf("canonical UpsertSeries: %v", err)
	}

	_, err = db.UpsertSeries(&Series{
		Title:          "Welcome to the Urban Jungle",
		Year:           2023,
		CanonicalPath:  canonical,
		LibraryRoot:    "/media/TV",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   1,
	})
	if err != nil {
		t.Fatalf("fragment UpsertSeries: %v", err)
	}

	rows, err := db.db.Query(`SELECT title, year, canonical_path FROM series WHERE canonical_path = ?`, canonical)
	if err != nil {
		t.Fatalf("query series: %v", err)
	}
	defer rows.Close()

	var count int
	var title string
	var year int
	for rows.Next() {
		count++
		if err := rows.Scan(&title, &year, new(string)); err != nil {
			t.Fatalf("scan series: %v", err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("series rows for canonical path = %d, want 1", count)
	}
	if title != "Project Runway" || year != 2004 {
		t.Fatalf("series row = %q (%d), want canonical Project Runway (2004)", title, year)
	}
}

func TestUpsertSeries_MergesCanonicalPathRowWhenIdentityCorrectionCollides(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	keeper := &Series{
		Title:          "Last Week Tonight with John Oliver",
		Year:           2014,
		CanonicalPath:  "/mnt/STORAGE5/TVSHOWS/Last Week Tonight with John Oliver (2014)",
		LibraryRoot:    "/mnt/STORAGE5/TVSHOWS",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   52,
	}
	if _, err := db.UpsertSeries(keeper); err != nil {
		t.Fatalf("keeper UpsertSeries: %v", err)
	}

	legacy := &Series{
		Title:          "Last Week Tonight",
		Year:           0,
		CanonicalPath:  "/mnt/STORAGE2/TVSHOWS/Last Week Tonight with John Oliver (2014)",
		LibraryRoot:    "/mnt/STORAGE2/TVSHOWS",
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   1,
	}
	if _, err := db.UpsertSeries(legacy); err != nil {
		t.Fatalf("legacy UpsertSeries: %v", err)
	}

	corrected := &Series{
		Title:          "Last Week Tonight with John Oliver",
		Year:           2014,
		CanonicalPath:  legacy.CanonicalPath,
		LibraryRoot:    legacy.LibraryRoot,
		Source:         "filesystem",
		SourcePriority: 50,
		EpisodeCount:   1,
	}
	if _, err := db.UpsertSeries(corrected); err != nil {
		t.Fatalf("corrected UpsertSeries should merge legacy row instead of violating unique identity: %v", err)
	}

	var rows int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM series WHERE title_normalized = ? AND year = ?`,
		"lastweektonightwithjohnoliver", 2014,
	).Scan(&rows); err != nil {
		t.Fatalf("count corrected identity: %v", err)
	}
	if rows != 1 {
		t.Fatalf("corrected identity rows = %d, want 1", rows)
	}

	var legacyRows int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM series WHERE id = ?`, legacy.ID).Scan(&legacyRows); err != nil {
		t.Fatalf("count legacy row: %v", err)
	}
	if legacyRows != 0 {
		t.Fatalf("legacy row still exists after collision merge")
	}

	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		t.Fatalf("GetUnresolvedConflicts: %v", err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %d, want 1", len(conflicts))
	}
	if len(conflicts[0].Locations) != 2 {
		t.Fatalf("conflict locations = %v, want both paths", conflicts[0].Locations)
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
		Source:        "plex2jellyfin",
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
