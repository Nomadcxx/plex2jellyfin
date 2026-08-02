package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func newSweepDB(t *testing.T) *database.MediaDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sweep.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newFakeJellyfinServer(t *testing.T, items []Item) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		startIndex := 0
		limit := 200
		if v := r.URL.Query().Get("StartIndex"); v != "" {
			startIndex, _ = strconv.Atoi(v)
		}
		if v := r.URL.Query().Get("Limit"); v != "" {
			limit, _ = strconv.Atoi(v)
		}
		end := startIndex + limit
		if end > len(items) {
			end = len(items)
		}
		page := []Item{}
		if startIndex < len(items) {
			page = items[startIndex:end]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ItemsResponse{Items: page, TotalRecordCount: len(items)})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestSweep_RecentRowIsSweepedWithinLookback(t *testing.T) {
	db := newSweepDB(t)
	targetPath := "/library/Movies/The Matrix.mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/matrix.mkv",
		SourceFilename:  "matrix.mkv",
		EventAt:         time.Now().UTC().Add(-1 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	srv, _ := newFakeJellyfinServer(t, []Item{
		{ID: "jf-1", Path: targetPath, ProviderIDs: map[string]string{"Imdb": "tt0133093", "Tmdb": "603"}},
	})
	client := NewClient(Config{URL: srv.URL, APIKey: "k"})

	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.JellyfinItemID != "jf-1" {
		t.Errorf("expected JellyfinItemID=jf-1, got %q", dec.JellyfinItemID)
	}
	if dec.JellyfinImdbID != "tt0133093" {
		t.Errorf("expected JellyfinImdbID=tt0133093, got %q", dec.JellyfinImdbID)
	}
	if dec.JellyfinResolvedAt == nil {
		t.Errorf("expected JellyfinResolvedAt to be set")
	}
	item, err := db.GetJellyfinItemByPath(targetPath)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath: %v", err)
	}
	if item == nil || item.JellyfinItemID != "jf-1" {
		t.Fatalf("expected live path cached from inventory, got %+v", item)
	}
}

func TestSweep_ResolvesAllRowsForSameTargetPath(t *testing.T) {
	db := newSweepDB(t)
	targetPath := "/library/Movies/F Valentines Day (2026)/F Valentines Day (2026).mkv"
	oldID, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/old.mkv",
		SourceFilename:  "old.mkv",
		EventAt:         time.Now().UTC().Add(-2 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
		AutoLabel:       "FAIL",
	})
	if err != nil {
		t.Fatalf("InsertDecision old: %v", err)
	}
	newID, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/new.mkv",
		SourceFilename:  "new.mkv",
		EventAt:         time.Now().UTC().Add(-1 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision new: %v", err)
	}

	srv, _ := newFakeJellyfinServer(t, []Item{
		{ID: "jf-fvd", Path: targetPath, ProviderIDs: map[string]string{"Imdb": "tt34622232", "Tmdb": "1429605"}},
	})
	client := NewClient(Config{URL: srv.URL, APIKey: "k"})

	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	for _, id := range []int64{oldID, newID} {
		dec, err := db.GetDecision(id)
		if err != nil {
			t.Fatalf("GetDecision %d: %v", id, err)
		}
		if dec.JellyfinItemID != "jf-fvd" {
			t.Errorf("id=%d expected JellyfinItemID=jf-fvd, got %q", id, dec.JellyfinItemID)
		}
		if dec.JellyfinImdbID != "tt34622232" {
			t.Errorf("id=%d expected JellyfinImdbID=tt34622232, got %q", id, dec.JellyfinImdbID)
		}
		if dec.AutoLabel != "" {
			t.Errorf("id=%d expected auto_label cleared after resolution, got %q", id, dec.AutoLabel)
		}
	}
}

func TestSweep_OldRowSkippedByNormalSweep(t *testing.T) {
	db := newSweepDB(t)
	targetPath := "/library/Movies/Old.mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/old.mkv",
		SourceFilename:  "old.mkv",
		EventAt:         time.Now().UTC().Add(-48 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	srv, calls := newFakeJellyfinServer(t, []Item{
		{ID: "jf-old", Path: targetPath},
	})
	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	// 24h lookback, 7d ttl: row is 48h old, outside lookback but inside TTL.
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.JellyfinItemID != "" {
		t.Errorf("expected old row to be skipped, got JellyfinItemID=%q", dec.JellyfinItemID)
	}
	if dec.AutoLabel != "" {
		t.Errorf("expected old row not yet TTL-labeled, got auto_label=%q", dec.AutoLabel)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Errorf("expected one complete inventory request, got %d", *calls)
	}
}

func TestSweep_UnresolvedRowOlderThanTTLIsLabeledFAIL(t *testing.T) {
	db := newSweepDB(t)
	targetPath := "/library/Movies/Stale.mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/stale.mkv",
		SourceFilename:  "stale.mkv",
		EventAt:         time.Now().UTC().Add(-10 * 24 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	srv, _ := newFakeJellyfinServer(t, []Item{})
	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)

	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.AutoLabel != "FAIL" {
		t.Errorf("expected auto_label=FAIL, got %q", dec.AutoLabel)
	}
}

func TestSweep_PaginationFollowsTotalRecordCount(t *testing.T) {
	db := newSweepDB(t)
	paths := []string{
		"/library/A.mkv",
		"/library/B.mkv",
		"/library/C.mkv",
	}
	for _, p := range paths {
		_, err := db.InsertDecision(database.ParseDecision{
			SourcePath:      "/dl/" + p,
			SourceFilename:  filepath.Base(p),
			EventAt:         time.Now().UTC().Add(-1 * time.Hour),
			TargetPath:      p,
			OrganizeOutcome: "success",
		})
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
	}

	items := []Item{
		{ID: "a", Path: paths[0]},
		{ID: "b", Path: paths[1]},
		{ID: "c", Path: paths[2]},
	}

	var calls int32
	var startIndices []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		startIndex, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
		startIndices = append(startIndices, startIndex)
		// Force page size of 2 regardless of client request.
		const limit = 2
		end := startIndex + limit
		if end > len(items) {
			end = len(items)
		}
		var page []Item
		if startIndex < len(items) {
			page = items[startIndex:end]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ItemsResponse{Items: page, TotalRecordCount: len(items)})
	}))
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected 2 paginated requests, got %d", calls)
	}
	if len(startIndices) != 2 || startIndices[0] != 0 || startIndices[1] != 2 {
		t.Errorf("expected startIndices=[0,2], got %v", startIndices)
	}
}

func TestSweep_APIErrorDoesNotMarkRows(t *testing.T) {
	db := newSweepDB(t)
	targetPath := "/library/x.mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/x.mkv",
		SourceFilename:  "x.mkv",
		EventAt:         time.Now().UTC().Add(-1 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err == nil {
		t.Fatalf("expected error from RunOnce when API returns 500")
	}

	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.JellyfinItemID != "" || dec.JellyfinResolvedAt != nil {
		t.Errorf("row should not be marked when API errors, got id=%q resolved_at=%v", dec.JellyfinItemID, dec.JellyfinResolvedAt)
	}
	if dec.AutoLabel != "" {
		t.Errorf("row should not be auto-labeled on API error, got %q", dec.AutoLabel)
	}
}

func TestSweep_CompleteInventoryInvalidatesCachedPathMissingFromJellyfin(t *testing.T) {
	db := newSweepDB(t)
	libraryDir := t.TempDir()
	targetPath := filepath.Join(libraryDir, "Still On Disk (2026).mkv")
	if err := os.WriteFile(targetPath, []byte("movie"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var ids []int64
	for _, source := range []string{"/dl/older.still.on.disk.2026.mkv", "/dl/still.on.disk.2026.mkv"} {
		id, err := db.InsertDecision(database.ParseDecision{
			SourcePath:      source,
			SourceFilename:  filepath.Base(source),
			EventAt:         time.Now().UTC().Add(-1 * time.Hour),
			TargetPath:      targetPath,
			OrganizeOutcome: "success",
		})
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
		ids = append(ids, id)
		resolvedAt := time.Now().UTC()
		identified := true
		if err := db.UpdateOutcome(id, database.OutcomeUpdate{
			JellyfinItemID:      "jf-stale",
			JellyfinResolvedAt:  &resolvedAt,
			JellyfinIdentified:  &identified,
			JellyfinFirstSeenAt: &resolvedAt,
		}); err != nil {
			t.Fatalf("UpdateOutcome: %v", err)
		}
	}
	// The library is not just this one file: seed survivors so the sweep sees
	// a genuine single-item deletion rather than an empty inventory, which is
	// indistinguishable from a Jellyfin that has not finished starting up.
	survivors := make([]Item, 0, 8)
	cache := []database.JellyfinItem{{
		Path: targetPath, JellyfinItemID: "jf-stale", ItemName: "Still On Disk", ItemType: "Movie",
	}}
	for i := 0; i < 8; i++ {
		p := fmt.Sprintf("/library/Movies/Survivor %d (2026)/Survivor %d (2026).mkv", i, i)
		survivors = append(survivors, Item{ID: fmt.Sprintf("jf-survivor-%d", i), Path: p, Name: fmt.Sprintf("Survivor %d", i), Type: "Movie"})
		cache = append(cache, database.JellyfinItem{
			Path: p, JellyfinItemID: fmt.Sprintf("jf-survivor-%d", i),
			ItemName: fmt.Sprintf("Survivor %d", i), ItemType: "Movie",
		})
	}
	if _, err := db.ReconcileJellyfinItems(cache); err != nil {
		t.Fatalf("ReconcileJellyfinItems: %v", err)
	}

	// Jellyfin now returns every survivor but not targetPath.
	srv, calls := newFakeJellyfinServer(t, survivors)
	sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("expected complete inventory request, got %d calls", *calls)
	}
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("on-disk file should remain: %v", err)
	}

	item, err := db.GetJellyfinItemByPath(targetPath)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath: %v", err)
	}
	if item != nil {
		t.Fatalf("expected stale cache row invalidated, got %+v", item)
	}
	for _, id := range ids {
		dec, err := db.GetDecision(id)
		if err != nil {
			t.Fatalf("GetDecision: %v", err)
		}
		if dec.JellyfinItemID != "" || dec.JellyfinResolvedAt != nil {
			t.Fatalf("expected stale parse outcome cleared, got %+v", dec)
		}
	}

	// The survivors must be untouched by the single-item invalidation.
	for _, s := range survivors {
		cached, err := db.GetJellyfinItemByPath(s.Path)
		if err != nil {
			t.Fatalf("GetJellyfinItemByPath(%s): %v", s.Path, err)
		}
		if cached == nil {
			t.Fatalf("survivor %s must remain cached", s.Path)
		}
	}
}

// A reachable-but-empty Jellyfin (still scanning, storage not mounted, API key
// without library access) must never reconcile as "everything was deleted".
func TestSweep_EmptyInventoryPreservesCachedConfirmations(t *testing.T) {
	db := newSweepDB(t)

	var ids []int64
	var cache []database.JellyfinItem
	for i := 0; i < 5; i++ {
		p := fmt.Sprintf("/library/Movies/Film %d (2026)/Film %d (2026).mkv", i, i)
		id, err := db.InsertDecision(database.ParseDecision{
			SourcePath:      fmt.Sprintf("/dl/film.%d.2026.mkv", i),
			SourceFilename:  fmt.Sprintf("film.%d.2026.mkv", i),
			EventAt:         time.Now().UTC().Add(-1 * time.Hour),
			TargetPath:      p,
			OrganizeOutcome: "success",
		})
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
		ids = append(ids, id)
		now := time.Now().UTC()
		identified := true
		if err := db.UpdateOutcome(id, database.OutcomeUpdate{
			JellyfinItemID:      fmt.Sprintf("jf-%d", i),
			JellyfinResolvedAt:  &now,
			JellyfinIdentified:  &identified,
			JellyfinFirstSeenAt: &now,
		}); err != nil {
			t.Fatalf("UpdateOutcome: %v", err)
		}
		cache = append(cache, database.JellyfinItem{
			Path: p, JellyfinItemID: fmt.Sprintf("jf-%d", i),
			ItemName: fmt.Sprintf("Film %d", i), ItemType: "Movie",
		})
	}
	if _, err := db.ReconcileJellyfinItems(cache); err != nil {
		t.Fatalf("ReconcileJellyfinItems: %v", err)
	}

	srv, _ := newFakeJellyfinServer(t, nil)
	sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err == nil {
		t.Fatal("expected empty inventory to be rejected as implausible")
	}

	items, err := db.ListJellyfinItems()
	if err != nil {
		t.Fatalf("ListJellyfinItems: %v", err)
	}
	if len(items) != len(cache) {
		t.Fatalf("empty inventory must preserve cache, got %d of %d", len(items), len(cache))
	}
	for _, id := range ids {
		dec, err := db.GetDecision(id)
		if err != nil {
			t.Fatalf("GetDecision: %v", err)
		}
		if dec.JellyfinItemID == "" || dec.JellyfinResolvedAt == nil {
			t.Fatalf("empty inventory must preserve outcome, got %+v", dec)
		}
	}
}

// A partial inventory that is well-formed but far smaller than the cached
// snapshot is equally untrustworthy.
func TestSweep_ImplausiblyShrunkInventoryPreservesCachedConfirmations(t *testing.T) {
	db := newSweepDB(t)

	var cache []database.JellyfinItem
	var live []Item
	for i := 0; i < 20; i++ {
		p := fmt.Sprintf("/library/Movies/Film %d (2026)/Film %d (2026).mkv", i, i)
		cache = append(cache, database.JellyfinItem{
			Path: p, JellyfinItemID: fmt.Sprintf("jf-%d", i),
			ItemName: fmt.Sprintf("Film %d", i), ItemType: "Movie",
		})
		// Jellyfin reports only 4 of the 20 cached paths (20% — under the floor).
		if i < 4 {
			live = append(live, Item{ID: fmt.Sprintf("jf-%d", i), Path: p, Name: fmt.Sprintf("Film %d", i), Type: "Movie"})
		}
	}
	if _, err := db.ReconcileJellyfinItems(cache); err != nil {
		t.Fatalf("ReconcileJellyfinItems: %v", err)
	}

	srv, _ := newFakeJellyfinServer(t, live)
	sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err == nil {
		t.Fatal("expected implausibly shrunk inventory to be rejected")
	}

	items, err := db.ListJellyfinItems()
	if err != nil {
		t.Fatalf("ListJellyfinItems: %v", err)
	}
	if len(items) != len(cache) {
		t.Fatalf("shrunk inventory must preserve cache, got %d of %d", len(items), len(cache))
	}
	complete, err := db.IsJellyfinInventoryComplete()
	if err != nil {
		t.Fatalf("IsJellyfinInventoryComplete: %v", err)
	}
	if complete {
		t.Fatal("rejected inventory must leave the snapshot incomplete")
	}
}

// Ordinary attrition stays above the floor and still reconciles.
func TestSweep_ModestShrinkStillReconciles(t *testing.T) {
	db := newSweepDB(t)

	var cache []database.JellyfinItem
	var live []Item
	for i := 0; i < 20; i++ {
		p := fmt.Sprintf("/library/Movies/Film %d (2026)/Film %d (2026).mkv", i, i)
		cache = append(cache, database.JellyfinItem{
			Path: p, JellyfinItemID: fmt.Sprintf("jf-%d", i),
			ItemName: fmt.Sprintf("Film %d", i), ItemType: "Movie",
		})
		if i < 16 {
			live = append(live, Item{ID: fmt.Sprintf("jf-%d", i), Path: p, Name: fmt.Sprintf("Film %d", i), Type: "Movie"})
		}
	}
	if _, err := db.ReconcileJellyfinItems(cache); err != nil {
		t.Fatalf("ReconcileJellyfinItems: %v", err)
	}

	srv, _ := newFakeJellyfinServer(t, live)
	sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	items, err := db.ListJellyfinItems()
	if err != nil {
		t.Fatalf("ListJellyfinItems: %v", err)
	}
	if len(items) != len(live) {
		t.Fatalf("expected cache reconciled to %d live items, got %d", len(live), len(items))
	}
}

func TestSweep_FailedInventoryPreservesCachedConfirmations(t *testing.T) {
	db := newSweepDB(t)
	targetPath := "/library/Movies/Cached (2026)/Cached (2026).mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/cached.2026.mkv",
		SourceFilename:  "cached.2026.mkv",
		EventAt:         time.Now().UTC().Add(-1 * time.Hour),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}
	resolvedAt := time.Now().UTC()
	identified := true
	if err := db.UpdateOutcome(id, database.OutcomeUpdate{
		JellyfinItemID:      "jf-cached",
		JellyfinResolvedAt:  &resolvedAt,
		JellyfinIdentified:  &identified,
		JellyfinFirstSeenAt: &resolvedAt,
	}); err != nil {
		t.Fatalf("UpdateOutcome: %v", err)
	}
	if err := db.UpsertJellyfinItem(targetPath, "jf-cached", "Cached", "Movie"); err != nil {
		t.Fatalf("UpsertJellyfinItem: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err == nil {
		t.Fatal("expected failed inventory error")
	}

	item, err := db.GetJellyfinItemByPath(targetPath)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath: %v", err)
	}
	if item == nil || item.JellyfinItemID != "jf-cached" {
		t.Fatalf("failed inventory must preserve cache, got %+v", item)
	}
	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.JellyfinItemID != "jf-cached" || dec.JellyfinResolvedAt == nil {
		t.Fatalf("failed inventory must preserve outcome, got %+v", dec)
	}
}

func TestSweep_MalformedInventoryPreservesCachedConfirmations(t *testing.T) {
	tests := []struct {
		name string
		item Item
	}{
		{name: "missing path", item: Item{ID: "jf-malformed"}},
		{name: "missing id", item: Item{Path: "/library/Movies/Malformed (2026)/Malformed (2026).mkv"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSweepDB(t)
			targetPath := "/library/Movies/Cached (2026)/Cached (2026).mkv"
			id, err := db.InsertDecision(database.ParseDecision{
				SourcePath:      "/dl/cached.2026.mkv",
				SourceFilename:  "cached.2026.mkv",
				EventAt:         time.Now().UTC().Add(-1 * time.Hour),
				TargetPath:      targetPath,
				OrganizeOutcome: "success",
			})
			if err != nil {
				t.Fatalf("InsertDecision: %v", err)
			}
			resolvedAt := time.Now().UTC()
			identified := true
			if err := db.UpdateOutcome(id, database.OutcomeUpdate{
				JellyfinItemID:      "jf-cached",
				JellyfinResolvedAt:  &resolvedAt,
				JellyfinIdentified:  &identified,
				JellyfinFirstSeenAt: &resolvedAt,
			}); err != nil {
				t.Fatalf("UpdateOutcome: %v", err)
			}
			if _, err := db.ReconcileJellyfinItems([]database.JellyfinItem{{
				Path:           targetPath,
				JellyfinItemID: "jf-cached",
				ItemName:       "Cached",
				ItemType:       "Movie",
			}}); err != nil {
				t.Fatalf("ReconcileJellyfinItems: %v", err)
			}

			srv, _ := newFakeJellyfinServer(t, []Item{tt.item})
			sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
			sweeper.SetPageDelay(0)
			if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err == nil {
				t.Fatal("expected malformed inventory error")
			}

			item, err := db.GetJellyfinItemByPath(targetPath)
			if err != nil {
				t.Fatalf("GetJellyfinItemByPath: %v", err)
			}
			if item == nil || item.JellyfinItemID != "jf-cached" {
				t.Fatalf("malformed inventory must preserve cache, got %+v", item)
			}
			decision, err := db.GetDecision(id)
			if err != nil {
				t.Fatalf("GetDecision: %v", err)
			}
			if decision.JellyfinItemID != "jf-cached" || decision.JellyfinResolvedAt == nil {
				t.Fatalf("malformed inventory must preserve outcome, got %+v", decision)
			}
			complete, err := db.IsJellyfinInventoryComplete()
			if err != nil {
				t.Fatalf("IsJellyfinInventoryComplete: %v", err)
			}
			if complete {
				t.Fatal("malformed inventory must mark prior snapshot incomplete")
			}
		})
	}
}

func TestSweep_InconsistentPaginationPreservesCachedConfirmations(t *testing.T) {
	live := Item{ID: "jf-live", Path: "/library/Movies/Live (2026)/Live (2026).mkv"}
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "negative total",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(ItemsResponse{TotalRecordCount: -1})
			},
		},
		{
			name: "duplicate items",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(ItemsResponse{
					Items:            []Item{live, live},
					TotalRecordCount: 2,
				})
			},
		},
		{
			name: "repeating page",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(ItemsResponse{
					Items:            []Item{live},
					TotalRecordCount: 2,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newSweepDB(t)
			cachedPath := "/library/Movies/Cached (2026)/Cached (2026).mkv"
			if _, err := db.ReconcileJellyfinItems([]database.JellyfinItem{{
				Path:           cachedPath,
				JellyfinItemID: "jf-cached",
				ItemName:       "Cached",
				ItemType:       "Movie",
			}}); err != nil {
				t.Fatalf("ReconcileJellyfinItems: %v", err)
			}

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			sweeper := NewSweeper(NewClient(Config{URL: srv.URL, APIKey: "k"}), db)
			sweeper.SetPageDelay(0)
			if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err == nil {
				t.Fatal("expected inconsistent pagination error")
			}

			item, err := db.GetJellyfinItemByPath(cachedPath)
			if err != nil {
				t.Fatalf("GetJellyfinItemByPath: %v", err)
			}
			if item == nil || item.JellyfinItemID != "jf-cached" {
				t.Fatalf("inconsistent inventory must preserve cache, got %+v", item)
			}
			complete, err := db.IsJellyfinInventoryComplete()
			if err != nil {
				t.Fatalf("IsJellyfinInventoryComplete: %v", err)
			}
			if complete {
				t.Fatal("inconsistent inventory must remain incomplete")
			}
		})
	}
}

func TestSweep_ContextCancellationAbortsPagination(t *testing.T) {
	db := newSweepDB(t)
	// Seed enough rows that the sweep would otherwise paginate.
	for i := 0; i < 5; i++ {
		_, err := db.InsertDecision(database.ParseDecision{
			SourcePath:      "/dl/x.mkv",
			SourceFilename:  "x.mkv",
			EventAt:         time.Now().UTC().Add(-1 * time.Hour),
			TargetPath:      "/library/x" + strconv.Itoa(i) + ".mkv",
			OrganizeOutcome: "success",
		})
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow server: hold long enough that the per-request 30s timeout
		// would normally allow it, but cancellation should abort sooner.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ItemsResponse{Items: nil, TotalRecordCount: 0})
	}))
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := sweeper.RunOnce(ctx, 24*time.Hour, 7*24*time.Hour)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected error on ctx cancellation, got nil")
	}
	if elapsed > 5000*time.Millisecond {
		t.Errorf("expected sweep to abort promptly, took %v", elapsed)
	}
}

func TestSweep_PageDelayIsRespectedAndCancellable(t *testing.T) {
	db := newSweepDB(t)
	for i := 0; i < 3; i++ {
		_, err := db.InsertDecision(database.ParseDecision{
			SourcePath:      "/dl/x.mkv",
			SourceFilename:  "x.mkv",
			EventAt:         time.Now().UTC().Add(-1 * time.Hour),
			TargetPath:      "/library/d" + strconv.Itoa(i) + ".mkv",
			OrganizeOutcome: "success",
		})
		if err != nil {
			t.Fatalf("InsertDecision: %v", err)
		}
	}

	items := []Item{{ID: "a", Path: "/library/d0.mkv"}, {ID: "b", Path: "/library/d1.mkv"}, {ID: "c", Path: "/library/d2.mkv"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startIndex, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
		const limit = 1
		end := startIndex + limit
		if end > len(items) {
			end = len(items)
		}
		var page []Item
		if startIndex < len(items) {
			page = items[startIndex:end]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ItemsResponse{Items: page, TotalRecordCount: len(items)})
	}))
	defer srv.Close()

	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	// Tight delay; verifies pageDelay is honored without slowing the test much.
	sweeper.SetPageDelay(20 * time.Millisecond)

	start := time.Now()
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	elapsed := time.Since(start)
	// 3 items, page size 1 from server -> 3 fetches, 2 sleeps of 20ms = >=40ms.
	if elapsed < 30*time.Millisecond {
		t.Errorf("expected pageDelay to slow pagination, took %v", elapsed)
	}
}

func TestSweep_PathTranslationResolvesContainerPaths(t *testing.T) {
	db := newSweepDB(t)
	// Daemon writes to /mnt/STORAGE5/TVSHOWS/...
	daemonPath := "/mnt/STORAGE5/TVSHOWS/Tracker (2024)/Season 03/Tracker (2024) S03E18.mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/tracker.mkv",
		SourceFilename:  "tracker.mkv",
		EventAt:         time.Now().UTC().Add(-1 * time.Hour),
		TargetPath:      daemonPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	// Jellyfin reports a container-internal path.
	jellyfinPath := "/tv5/Tracker (2024)/Season 03/Tracker (2024) S03E18.mkv"
	srv, _ := newFakeJellyfinServer(t, []Item{
		{ID: "jf-99", Path: jellyfinPath, ProviderIDs: map[string]string{"Imdb": "tt39402011"}},
	})
	client := NewClient(Config{URL: srv.URL, APIKey: "k"})

	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	sweeper.SetPathTranslator(NewPathTranslator([]PathMapping{
		{Jellyfin: "/tv5", Daemon: "/mnt/STORAGE5/TVSHOWS"},
	}))

	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.JellyfinItemID != "jf-99" {
		t.Errorf("expected JellyfinItemID=jf-99 (translation should match), got %q", dec.JellyfinItemID)
	}
	if dec.JellyfinImdbID != "tt39402011" {
		t.Errorf("expected JellyfinImdbID=tt39402011, got %q", dec.JellyfinImdbID)
	}
	if dec.JellyfinResolvedAt == nil {
		t.Error("expected JellyfinResolvedAt to be set")
	}
}

func TestSweep_NoTranslatorMissesContainerPath(t *testing.T) {
	// Regression guard: without a translator, a container-internal path must
	// NOT match a daemon-side target_path. This is the bug the translator fixes.
	db := newSweepDB(t)
	daemonPath := "/mnt/STORAGE5/TVSHOWS/Foo/Foo S01E01.mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/dl/foo.mkv",
		SourceFilename:  "foo.mkv",
		EventAt:         time.Now().UTC().Add(-1 * time.Hour),
		TargetPath:      daemonPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}
	srv, _ := newFakeJellyfinServer(t, []Item{
		{ID: "jf-1", Path: "/tv5/Foo/Foo S01E01.mkv"},
	})
	client := NewClient(Config{URL: srv.URL, APIKey: "k"})
	sweeper := NewSweeper(client, db)
	sweeper.SetPageDelay(0)
	if err := sweeper.RunOnce(context.Background(), 24*time.Hour, 7*24*time.Hour); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	dec, _ := db.GetDecision(id)
	if dec.JellyfinItemID != "" {
		t.Errorf("without translator, expected no match; got %q", dec.JellyfinItemID)
	}
}
