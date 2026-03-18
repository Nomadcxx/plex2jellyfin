package library

import (
	"os"
	"path/filepath"
	"testing"
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
