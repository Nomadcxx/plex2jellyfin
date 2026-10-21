package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestSelectTVShowLibrary_PreferExistingShow(t *testing.T) {
	// Create temp directories for testing
	tmpDir := t.TempDir()
	storage1 := filepath.Join(tmpDir, "STORAGE1", "TVSHOWS")
	storage2 := filepath.Join(tmpDir, "STORAGE2", "TVSHOWS")
	storage3 := filepath.Join(tmpDir, "STORAGE3", "TVSHOWS")

	// Create library directories
	os.MkdirAll(storage1, 0755)
	os.MkdirAll(storage2, 0755)
	os.MkdirAll(storage3, 0755)

	// Create "Fallout (2024)" show directory in STORAGE2
	falloutDir := filepath.Join(storage2, "Fallout (2024)")
	os.MkdirAll(filepath.Join(falloutDir, "Season 01"), 0755)

	selector := NewSelector([]string{storage1, storage2, storage3})

	// Test: Selecting library for new Fallout episode should pick STORAGE2
	result, err := selector.SelectTVShowLibrary("Fallout", "2024", 1024*1024*100) // 100MB file
	if err != nil {
		t.Fatalf("SelectTVShowLibrary failed: %v", err)
	}

	if result.Library != storage2 {
		t.Errorf("Expected library %s, got %s", storage2, result.Library)
	}

	if result.Reason == "" || result.Reason == "Only library available" {
		t.Errorf("Expected reason to mention existing show, got: %s", result.Reason)
	}

	t.Logf("Selected: %s - Reason: %s", result.Library, result.Reason)
}

func TestSelectTVShowLibrary_NewShow(t *testing.T) {
	tmpDir := t.TempDir()
	storage1 := filepath.Join(tmpDir, "STORAGE1", "TVSHOWS")
	storage2 := filepath.Join(tmpDir, "STORAGE2", "TVSHOWS")

	os.MkdirAll(storage1, 0755)
	os.MkdirAll(storage2, 0755)

	selector := NewSelector([]string{storage1, storage2})

	// Test: New show should go to library with most space
	result, err := selector.SelectTVShowLibrary("Brand New Show", "2025", 1024*1024*100)
	if err != nil {
		t.Fatalf("SelectTVShowLibrary failed: %v", err)
	}

	// Should pick one of them (both have same space in temp dir)
	if result.Library != storage1 && result.Library != storage2 {
		t.Errorf("Expected one of the configured libraries, got %s", result.Library)
	}

	t.Logf("Selected: %s - Reason: %s", result.Library, result.Reason)
}

func TestFindExistingShowDir(t *testing.T) {
	tmpDir := t.TempDir()
	library := filepath.Join(tmpDir, "TVSHOWS")
	os.MkdirAll(library, 0755)

	// Create test show directories
	testShows := []string{
		"For All Mankind (2019)",
		"The White Lotus (2021)",
		"Fallout (2024)",
	}

	for _, show := range testShows {
		os.MkdirAll(filepath.Join(library, show), 0755)
	}

	selector := NewSelector([]string{library})

	tests := []struct {
		showName string
		year     string
		wantFind bool
	}{
		{"For All Mankind", "2019", true},
		{"The White Lotus", "2021", true},
		{"Fallout", "2024", true},
		{"Nonexistent Show", "2020", false},
	}

	for _, tt := range tests {
		t.Run(tt.showName, func(t *testing.T) {
			result := selector.findExistingShowDir(library, tt.showName, tt.year)
			found := result != ""

			if found != tt.wantFind {
				t.Errorf("findExistingShowDir(%q, %q) found=%v, want=%v",
					tt.showName, tt.year, found, tt.wantFind)
			}

			if found {
				t.Logf("Found: %s", result)
			}
		})
	}
}

func TestSelectMovieLibraryCanonicalPathMustBeWithinLibraryRoot(t *testing.T) {
	base := t.TempDir()
	prefixRoot := filepath.Join(base, "movies")
	canonicalRoot := filepath.Join(base, "movies-archive")
	requireDir(t, prefixRoot)
	requireDir(t, canonicalRoot)

	db := openSelectorTestDB(t)
	_, err := db.UpsertMovie(&database.Movie{
		Title:          "Boundary Film",
		Year:           2022,
		CanonicalPath:  filepath.Join(canonicalRoot, "Boundary Film (2022)"),
		LibraryRoot:    canonicalRoot,
		Source:         "plex2jellyfin",
		SourcePriority: 100,
	})
	if err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}

	selector := NewSelectorWithConfig(SelectorConfig{Libraries: []string{prefixRoot, canonicalRoot}, DB: db})
	selection, err := selector.SelectMovieLibrary("Boundary Film", "2022", 1)
	if err != nil {
		t.Fatalf("SelectMovieLibrary: %v", err)
	}
	if selection.Library != canonicalRoot {
		t.Fatalf("selected %q, want canonical root %q", selection.Library, canonicalRoot)
	}
}

func TestSelectMovieLibraryFailsClosedWhenCanonicalRootIsNotConfigured(t *testing.T) {
	base := t.TempDir()
	firstRoot := filepath.Join(base, "movies1")
	secondRoot := filepath.Join(base, "movies2")
	removedRoot := filepath.Join(base, "movies-removed")
	requireDir(t, firstRoot)
	requireDir(t, secondRoot)

	db := openSelectorTestDB(t)
	_, err := db.UpsertMovie(&database.Movie{
		Title:          "Orphaned Canonical Film",
		Year:           2022,
		CanonicalPath:  filepath.Join(removedRoot, "Orphaned Canonical Film (2022)"),
		LibraryRoot:    removedRoot,
		Source:         "plex2jellyfin",
		SourcePriority: 100,
	})
	if err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}

	selector := NewSelectorWithConfig(SelectorConfig{Libraries: []string{firstRoot, secondRoot}, DB: db})
	_, err = selector.SelectMovieLibrary("Orphaned Canonical Film", "2022", 1)
	if err == nil || !strings.Contains(err.Error(), "canonical path") || !strings.Contains(err.Error(), removedRoot) {
		t.Fatalf("SelectMovieLibrary error = %v, want canonical-path error naming removed root %q", err, removedRoot)
	}
}

func TestSelectMovieLibraryUsesLibraryRootWhenCanonicalPathIsEmpty(t *testing.T) {
	base := t.TempDir()
	otherRoot := filepath.Join(base, "movies1")
	canonicalRoot := filepath.Join(base, "movies2")
	removedRoot := filepath.Join(base, "movies-removed")
	requireDir(t, otherRoot)
	requireDir(t, canonicalRoot)

	tests := []struct {
		name        string
		libraryRoot string
		wantLibrary string
		wantError   string
	}{
		{
			name:        "configured library root",
			libraryRoot: filepath.Join(canonicalRoot, "nested"),
			wantLibrary: canonicalRoot,
		},
		{
			name:        "removed library root",
			libraryRoot: removedRoot,
			wantError:   removedRoot,
		},
		{
			name:      "missing library root",
			wantError: "no canonical path or library root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openSelectorTestDB(t)
			_, err := db.UpsertMovie(&database.Movie{
				Title:          "Pathless Film",
				Year:           2022,
				LibraryRoot:    tt.libraryRoot,
				Source:         "radarr",
				SourcePriority: 25,
			})
			if err != nil {
				t.Fatalf("UpsertMovie: %v", err)
			}

			selector := NewSelectorWithConfig(SelectorConfig{Libraries: []string{otherRoot, canonicalRoot}, DB: db})
			selection, err := selector.SelectMovieLibrary("Pathless Film", "2022", 1)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("SelectMovieLibrary error = %v, want error containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectMovieLibrary: %v", err)
			}
			if selection.Library != tt.wantLibrary {
				t.Fatalf("selected %q, want library root %q", selection.Library, tt.wantLibrary)
			}
		})
	}
}

func TestSelectMovieLibraryDoesNotRerouteFullCanonicalMovie(t *testing.T) {
	base := t.TempDir()
	otherRoot := filepath.Join(base, "movies1")
	canonicalRoot := filepath.Join(base, "movies2")
	requireDir(t, otherRoot)
	requireDir(t, canonicalRoot)

	db := openSelectorTestDB(t)
	_, err := db.UpsertMovie(&database.Movie{
		Title:          "Pinned Film",
		Year:           2022,
		CanonicalPath:  filepath.Join(canonicalRoot, "Pinned Film (2022)"),
		LibraryRoot:    canonicalRoot,
		Source:         "plex2jellyfin",
		SourcePriority: 100,
	})
	if err != nil {
		t.Fatalf("UpsertMovie: %v", err)
	}

	selector := NewSelectorWithConfig(SelectorConfig{Libraries: []string{otherRoot, canonicalRoot}, DB: db})
	_, err = selector.SelectMovieLibrary("Pinned Film", "2022", 1<<62)
	if err == nil || !strings.Contains(err.Error(), canonicalRoot) || !strings.Contains(err.Error(), "insufficient space") {
		t.Fatalf("SelectMovieLibrary error = %v, want insufficient-space error naming canonical root %q", err, canonicalRoot)
	}
}

func TestSelectMovieLibraryFailsClosedOnDatabaseError(t *testing.T) {
	base := t.TempDir()
	firstRoot := filepath.Join(base, "movies1")
	secondRoot := filepath.Join(base, "movies2")
	requireDir(t, firstRoot)
	requireDir(t, secondRoot)

	db := openSelectorTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	selector := NewSelectorWithConfig(SelectorConfig{Libraries: []string{firstRoot, secondRoot}, DB: db})
	_, err := selector.SelectMovieLibrary("Database Failure", "2022", 1)
	if err == nil || !strings.Contains(err.Error(), "lookup canonical movie") {
		t.Fatalf("SelectMovieLibrary error = %v, want canonical lookup error", err)
	}
}

func requireDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func openSelectorTestDB(t *testing.T) *database.MediaDB {
	t.Helper()
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "selector.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFindExistingShowDir_PunctuationVariants(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories with punctuation
	os.MkdirAll(filepath.Join(tmpDir, "Chip 'n Dale Rescue Rangers (2022)"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "American Dad! (2005)"), 0755)

	s := &Selector{}

	tests := []struct {
		name      string
		showName  string
		year      string
		expectHit bool
	}{
		{"apostrophe mismatch", "Chip n Dale Rescue Rangers", "2022", true},
		{"exclamation mismatch", "American Dad", "2005", true},
		{"no match", "Nonexistent Show", "2020", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.findExistingShowDir(tmpDir, tt.showName, tt.year)
			if tt.expectHit {
				if result == "" {
					t.Errorf("expected to find show dir for %q, got empty", tt.showName)
				}
			} else {
				if result != "" {
					t.Errorf("expected no match for %q, got %q", tt.showName, result)
				}
			}
		})
	}
}
