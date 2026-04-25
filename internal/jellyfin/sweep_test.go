package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
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
	if err := sweeper.RunOnce(24*time.Hour, 7*24*time.Hour); err != nil {
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
	// 24h lookback, 7d ttl: row is 48h old, outside lookback but inside TTL.
	if err := sweeper.RunOnce(24*time.Hour, 7*24*time.Hour); err != nil {
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
	if atomic.LoadInt32(calls) != 0 {
		t.Errorf("expected no Jellyfin API calls when no rows in window, got %d", *calls)
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

	if err := sweeper.RunOnce(24*time.Hour, 7*24*time.Hour); err != nil {
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
	if err := sweeper.RunOnce(24*time.Hour, 7*24*time.Hour); err != nil {
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
	if err := sweeper.RunOnce(24*time.Hour, 7*24*time.Hour); err == nil {
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
