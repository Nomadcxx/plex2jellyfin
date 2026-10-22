package database

import (
	"testing"
	"time"
)

func TestUpsertAndGetJellyfinItem(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	path := "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	if err := db.UpsertJellyfinItem(path, "jf-123", "The Matrix", "Movie"); err != nil {
		t.Fatalf("UpsertJellyfinItem failed: %v", err)
	}

	item, err := db.GetJellyfinItemByPath(path)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath failed: %v", err)
	}
	if item == nil {
		t.Fatalf("expected jellyfin item to exist")
	}
	if item.JellyfinItemID != "jf-123" {
		t.Fatalf("expected item id jf-123, got %s", item.JellyfinItemID)
	}
	if item.ItemName != "The Matrix" {
		t.Fatalf("expected item name The Matrix, got %s", item.ItemName)
	}
}

func TestUpsertJellyfinItemUpdatesExistingPath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	path := "/library/TV/Show/Season 01/Show S01E01.mkv"
	if err := db.UpsertJellyfinItem(path, "jf-1", "Show", "Episode"); err != nil {
		t.Fatalf("UpsertJellyfinItem first insert failed: %v", err)
	}
	if err := db.UpsertJellyfinItem(path, "jf-2", "Show Updated", "Episode"); err != nil {
		t.Fatalf("UpsertJellyfinItem update failed: %v", err)
	}

	item, err := db.GetJellyfinItemByPath(path)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath failed: %v", err)
	}
	if item == nil {
		t.Fatalf("expected jellyfin item to exist")
	}
	if item.JellyfinItemID != "jf-2" {
		t.Fatalf("expected updated item id jf-2, got %s", item.JellyfinItemID)
	}
	if item.ItemName != "Show Updated" {
		t.Fatalf("expected updated item name, got %s", item.ItemName)
	}
}

func TestDeleteJellyfinItemByPathIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	path := "/library/Movies/Old Movie (2020)/Old Movie (2020).mkv"
	if err := db.UpsertJellyfinItem(path, "jf-old", "Old Movie", "Movie"); err != nil {
		t.Fatalf("UpsertJellyfinItem failed: %v", err)
	}

	if err := db.DeleteJellyfinItemByPath(path); err != nil {
		t.Fatalf("DeleteJellyfinItemByPath failed: %v", err)
	}
	if err := db.DeleteJellyfinItemByPath(path); err != nil {
		t.Fatalf("second DeleteJellyfinItemByPath failed: %v", err)
	}

	item, err := db.GetJellyfinItemByPath(path)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath failed: %v", err)
	}
	if item != nil {
		t.Fatalf("expected jellyfin item to be deleted, got %+v", item)
	}
}

func TestReconcileJellyfinItemsReplacesCompleteInventory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	stalePath := "/library/Movies/Stale (2020)/Stale (2020).mkv"
	livePath := "/library/Movies/Live (2026)/Live (2026).mkv"
	if err := db.UpsertJellyfinItem(stalePath, "jf-stale", "Stale", "Movie"); err != nil {
		t.Fatalf("seed stale item: %v", err)
	}

	removed, err := db.ReconcileJellyfinItems([]JellyfinItem{{
		Path:           livePath,
		JellyfinItemID: "jf-live",
		ItemName:       "Live",
		ItemType:       "Movie",
	}})
	if err != nil {
		t.Fatalf("ReconcileJellyfinItems failed: %v", err)
	}
	if len(removed) != 1 || removed[0] != stalePath {
		t.Fatalf("expected removed paths [%q], got %v", stalePath, removed)
	}

	items, err := db.ListJellyfinItems()
	if err != nil {
		t.Fatalf("ListJellyfinItems failed: %v", err)
	}
	if len(items) != 1 || items[0].Path != livePath || items[0].JellyfinItemID != "jf-live" {
		t.Fatalf("unexpected reconciled inventory: %+v", items)
	}
}

func TestJellyfinInventoryCompletenessRequiresUnmodifiedReconciliation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	first := JellyfinItem{
		Path:           "/library/Movies/First (2026)/First (2026).mkv",
		JellyfinItemID: "jf-first",
		ItemName:       "First",
		ItemType:       "Movie",
	}
	if err := db.UpsertJellyfinItem(first.Path, first.JellyfinItemID, first.ItemName, first.ItemType); err != nil {
		t.Fatalf("UpsertJellyfinItem: %v", err)
	}
	complete, err := db.IsJellyfinInventoryComplete()
	if err != nil {
		t.Fatalf("IsJellyfinInventoryComplete: %v", err)
	}
	if complete {
		t.Fatal("webhook-populated cache must not be complete")
	}

	if _, err := db.ReconcileJellyfinItems([]JellyfinItem{first}); err != nil {
		t.Fatalf("ReconcileJellyfinItems: %v", err)
	}
	complete, err = db.IsJellyfinInventoryComplete()
	if err != nil {
		t.Fatalf("IsJellyfinInventoryComplete after reconcile: %v", err)
	}
	if !complete {
		t.Fatal("completed reconciliation should mark inventory complete")
	}

	if err := db.UpsertJellyfinItem(
		"/library/Movies/Second (2026)/Second (2026).mkv",
		"jf-second",
		"Second",
		"Movie",
	); err != nil {
		t.Fatalf("second UpsertJellyfinItem: %v", err)
	}
	complete, err = db.IsJellyfinInventoryComplete()
	if err != nil {
		t.Fatalf("IsJellyfinInventoryComplete after webhook mutation: %v", err)
	}
	if complete {
		t.Fatal("webhook mutation must invalidate inventory completeness")
	}
}

func TestUpsertJellyfinItemSkipsUnchangedValues(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	item := JellyfinItem{
		Path:           "/library/Movies/No-op (2026)/No-op (2026).mkv",
		JellyfinItemID: "jf-no-op",
		ItemName:       "No-op",
		ItemType:       "Movie",
	}
	if err := db.UpsertJellyfinItem(item.Path, item.JellyfinItemID, item.ItemName, item.ItemType); err != nil {
		t.Fatalf("seed Jellyfin item: %v", err)
	}
	if _, err := db.ReconcileJellyfinItems([]JellyfinItem{item}); err != nil {
		t.Fatalf("complete Jellyfin inventory: %v", err)
	}

	sentinel := time.Date(2000, time.January, 2, 3, 4, 5, 0, time.UTC)
	if _, err := db.DB().Exec(`UPDATE jellyfin_items SET updated_at = ? WHERE path = ?`, sentinel, item.Path); err != nil {
		t.Fatalf("set timestamp sentinel: %v", err)
	}
	before, err := db.GetJellyfinItemByPath(item.Path)
	if err != nil {
		t.Fatalf("read item before no-op: %v", err)
	}
	beforeGeneration, complete, err := db.JellyfinInventoryGeneration()
	if err != nil {
		t.Fatalf("read inventory before no-op: %v", err)
	}
	if !complete {
		t.Fatal("inventory should start complete")
	}

	if err := db.UpsertJellyfinItem(item.Path, item.JellyfinItemID, item.ItemName, item.ItemType); err != nil {
		t.Fatalf("repeat unchanged upsert: %v", err)
	}
	after, err := db.GetJellyfinItemByPath(item.Path)
	if err != nil {
		t.Fatalf("read item after no-op: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("unchanged upsert updated timestamp: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
	afterGeneration, complete, err := db.JellyfinInventoryGeneration()
	if err != nil {
		t.Fatalf("read inventory after no-op: %v", err)
	}
	if !complete || afterGeneration != beforeGeneration {
		t.Fatalf("unchanged upsert changed inventory state: generation %d -> %d, complete=%v", beforeGeneration, afterGeneration, complete)
	}

	if err := db.UpsertJellyfinItem(item.Path, item.JellyfinItemID, "Changed", item.ItemType); err != nil {
		t.Fatalf("update changed item: %v", err)
	}
	changedGeneration, complete, err := db.JellyfinInventoryGeneration()
	if err != nil {
		t.Fatalf("read inventory after changed item: %v", err)
	}
	if complete || changedGeneration <= afterGeneration {
		t.Fatalf("changed upsert did not invalidate inventory: generation %d -> %d, complete=%v", afterGeneration, changedGeneration, complete)
	}
}

func TestJellyfinInventoryGenerationChangesAcrossStateTransitions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	item := JellyfinItem{
		Path:           "/library/Movies/Generation (2026)/Generation (2026).mkv",
		JellyfinItemID: "jf-generation",
		ItemName:       "Generation",
		ItemType:       "Movie",
	}
	if _, err := db.ReconcileJellyfinItems([]JellyfinItem{item}); err != nil {
		t.Fatalf("ReconcileJellyfinItems: %v", err)
	}
	first, complete, err := db.JellyfinInventoryGeneration()
	if err != nil {
		t.Fatalf("JellyfinInventoryGeneration: %v", err)
	}
	if !complete {
		t.Fatal("reconciled inventory should be complete")
	}

	if err := db.MarkJellyfinInventoryIncomplete(); err != nil {
		t.Fatalf("MarkJellyfinInventoryIncomplete: %v", err)
	}
	second, complete, err := db.JellyfinInventoryGeneration()
	if err != nil {
		t.Fatalf("JellyfinInventoryGeneration after invalidation: %v", err)
	}
	if complete || second <= first {
		t.Fatalf("invalidation should advance incomplete generation: first=%d second=%d complete=%v", first, second, complete)
	}

	if _, err := db.ReconcileJellyfinItems([]JellyfinItem{item}); err != nil {
		t.Fatalf("second ReconcileJellyfinItems: %v", err)
	}
	third, complete, err := db.JellyfinInventoryGeneration()
	if err != nil {
		t.Fatalf("JellyfinInventoryGeneration after second reconcile: %v", err)
	}
	if !complete || third <= second {
		t.Fatalf("reconciliation should advance complete generation: second=%d third=%d complete=%v", second, third, complete)
	}
}
