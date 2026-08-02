package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/api"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func TestGetDashboard_ReflectsCanonicalDuplicateStats(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	year := 1999
	for i, root := range []string{"/a", "/b"} {
		f := &database.MediaFile{
			Path:            root + "/The Matrix (1999)/The Matrix (1999).mkv",
			Size:            int64(1000 * (i + 1)),
			MediaType:       "movie",
			NormalizedTitle: "the matrix",
			Year:            &year,
			QualityScore:    100 * (i + 1),
			LibraryRoot:     root,
		}
		if err := db.UpsertMediaFile(f); err != nil {
			t.Fatalf("UpsertMediaFile: %v", err)
		}
	}
	if _, err := db.UpsertMovie(&database.Movie{
		Title: "The Matrix", Year: year, CanonicalPath: "/a/The Matrix (1999)",
		LibraryRoot: "/a", Source: "filesystem", SourcePriority: 50,
	}); err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}

	s := &Server{
		cfg:     &config.Config{},
		db:      db,
		service: service.NewCleanupService(db),
	}
	w := httptest.NewRecorder()
	s.GetDashboard(w, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}

	var resp api.DashboardData
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.LibraryStats == nil || resp.LibraryStats.DuplicateGroups == nil {
		t.Fatalf("missing libraryStats.duplicateGroups: %#v", resp.LibraryStats)
	}
	if got := *resp.LibraryStats.DuplicateGroups; got != 1 {
		t.Fatalf("duplicateGroups = %d, want 1", got)
	}
	if resp.LibraryStats.ReclaimableBytes == nil || *resp.LibraryStats.ReclaimableBytes != 1000 {
		t.Fatalf("reclaimableBytes = %v, want 1000", resp.LibraryStats.ReclaimableBytes)
	}
}
