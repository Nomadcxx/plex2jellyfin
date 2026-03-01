package database

import "testing"

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
