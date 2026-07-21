package tmdb

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

// newTestVerifier builds a Verifier backed by a fresh self-cleaning MediaDB so
// lookupLocal/lookupCached return real rows without network calls. Cache rows
// are inserted into tmdb_lookups with fetched_at=now so cacheMaxAge is happy.
func newTestVerifier(t *testing.T, cacheRows map[string][]Match) *Verifier {
	t.Helper()
	dir := t.TempDir()
	db, err := database.OpenPath(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for title, matches := range cacheRows {
		b, _ := json.Marshal(matches)
		if _, err := db.DB().Exec(
			`INSERT INTO tmdb_lookups (normalized_title, media_kind, source, results_json, fetched_at)
			 VALUES (?, 'movie', 'test', ?, ?)`,
			title, string(b), time.Now().UTC(),
		); err != nil {
			t.Fatalf("seed tmdb_lookups %q: %v", title, err)
		}
	}
	return &Verifier{db: db, cacheMaxAge: 30 * 24 * time.Hour}
}

func TestLookupExactRequiresExactYear(t *testing.T) {
	v := newTestVerifier(t, map[string][]Match{
		"avatar": {{ID: "19995", Title: "Avatar", Year: "2009"}},
	})
	if m := v.LookupExact(context.Background(), KindMovie, "Avatar", "2025"); m != nil {
		t.Errorf("LookupExact returned %+v for year 2025, want nil", m)
	}
	if m := v.LookupExact(context.Background(), KindMovie, "Avatar", "2009"); m == nil || m.ID != "19995" {
		t.Errorf("LookupExact = %+v, want Avatar 2009", m)
	}
}

func TestLookupExactRejectsPlusMinusOneYear(t *testing.T) {
	v := newTestVerifier(t, map[string][]Match{
		"some film": {{ID: "1", Title: "Some Film", Year: "2020"}},
	})
	if m := v.LookupExact(context.Background(), KindMovie, "Some Film", "2021"); m != nil {
		t.Errorf("LookupExact returned %+v for ±1 year, want nil (strict)", m)
	}
}

func TestLookupExactEmptyInputReturnsNil(t *testing.T) {
	v := newTestVerifier(t, nil)
	if m := v.LookupExact(context.Background(), KindMovie, "", "2025"); m != nil {
		t.Errorf("LookupExact empty title = %+v, want nil", m)
	}
	if m := v.LookupExact(context.Background(), KindMovie, "Avatar", ""); m != nil {
		t.Errorf("LookupExact empty year = %+v, want nil", m)
	}
}