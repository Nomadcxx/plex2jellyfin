package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"For All Mankind", "forallmankind"},
		{"For All Mankind (2019)", "forallmankind"},
		{"The White Lotus (2021)", "thewhitelotus"},
		{"Fallout (2024)", "fallout"},
		{"M*A*S*H", "mash"},
		{"The.Daily.Show", "thedailyshow"},
		{"Star-Trek_Discovery", "startrekdiscovery"},
		{"It's Always Sunny", "itsalwayssunny"},
		{"Law & Order: SVU", "lawordersvu"},
		{"FALLOUT", "fallout"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeTitle(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCountEpisodesInShow(t *testing.T) {
	tmpDir := t.TempDir()

	// Create show structure with episodes
	showDir := filepath.Join(tmpDir, "Test Show (2024)")
	season1 := filepath.Join(showDir, "Season 01")
	season2 := filepath.Join(showDir, "Season 02")

	os.MkdirAll(season1, 0755)
	os.MkdirAll(season2, 0755)

	// Create video files
	videoFiles := []string{
		filepath.Join(season1, "Test Show S01E01.mkv"),
		filepath.Join(season1, "Test Show S01E02.mkv"),
		filepath.Join(season1, "Test Show S01E03.mp4"),
		filepath.Join(season2, "Test Show S02E01.avi"),
		filepath.Join(season2, "Test Show S02E02.m4v"),
	}

	for _, f := range videoFiles {
		os.WriteFile(f, []byte("fake video"), 0644)
	}

	// Create non-video files (should be ignored)
	os.WriteFile(filepath.Join(season1, "subtitle.srt"), []byte("subs"), 0644)
	os.WriteFile(filepath.Join(season1, "poster.jpg"), []byte("image"), 0644)
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte("metadata"), 0644)

	count := countEpisodesInShow(showDir)

	if count != 5 {
		t.Errorf("countEpisodesInShow() = %d, want 5", count)
	}
}

func TestCountEpisodesInShow_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	showDir := filepath.Join(tmpDir, "Empty Show")
	os.MkdirAll(showDir, 0755)

	count := countEpisodesInShow(showDir)

	if count != 0 {
		t.Errorf("countEpisodesInShow(empty) = %d, want 0", count)
	}
}

func TestCountEpisodesInShow_NonexistentDirectory(t *testing.T) {
	count := countEpisodesInShow("/nonexistent/path")

	if count != 0 {
		t.Errorf("countEpisodesInShow(nonexistent) = %d, want 0", count)
	}
}

func TestFindShowDirInLibrary(t *testing.T) {
	tmpDir := t.TempDir()
	library := filepath.Join(tmpDir, "TVSHOWS")
	os.MkdirAll(library, 0755)

	// Create test show directories
	testShows := map[string]string{
		"For All Mankind (2019)": "forallmankind",
		"The White Lotus (2021)": "thewhitelotus",
		"Fallout (2024)":         "fallout",
		"Breaking Bad":           "breakingbad",
		"Star Trek Discovery":    "startrekdiscovery",
	}

	for showName := range testShows {
		os.MkdirAll(filepath.Join(library, showName), 0755)
	}

	selector := &Selector{libraries: []string{library}}

	tests := []struct {
		showName  string
		year      string
		wantFound bool
	}{
		{"For All Mankind", "2019", true},
		{"The White Lotus", "2021", true},
		{"Fallout", "2024", true},
		{"Breaking Bad", "", true},
		{"Star Trek Discovery", "", true},
		{"Nonexistent Show", "2020", false},
		{"Breaking", "", false}, // Partial match shouldn't work
	}

	for _, tt := range tests {
		t.Run(tt.showName, func(t *testing.T) {
			normalized := normalizeTitle(tt.showName)
			result := selector.findShowDirInLibrary(library, normalized, tt.year)
			found := result != ""

			if found != tt.wantFound {
				t.Errorf("findShowDirInLibrary(%q, %q) found=%v, want=%v",
					tt.showName, tt.year, found, tt.wantFound)
			}

			if found {
				if !strings.Contains(result, library) {
					t.Errorf("Result path %q doesn't contain library %q", result, library)
				}
			}
		})
	}
}

func TestFindAllShowLocations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple libraries
	storage1 := filepath.Join(tmpDir, "STORAGE1", "TVSHOWS")
	storage2 := filepath.Join(tmpDir, "STORAGE2", "TVSHOWS")
	storage3 := filepath.Join(tmpDir, "STORAGE3", "TVSHOWS")

	os.MkdirAll(storage1, 0755)
	os.MkdirAll(storage2, 0755)
	os.MkdirAll(storage3, 0755)

	// Create "Fallout (2024)" in STORAGE1 and STORAGE2 with different episode counts
	fallout1 := filepath.Join(storage1, "Fallout (2024)", "Season 01")
	fallout2 := filepath.Join(storage2, "Fallout (2024)", "Season 01")

	os.MkdirAll(fallout1, 0755)
	os.MkdirAll(fallout2, 0755)

	// STORAGE1: 2 episodes
	os.WriteFile(filepath.Join(fallout1, "Fallout S01E01.mkv"), []byte("video"), 0644)
	os.WriteFile(filepath.Join(fallout1, "Fallout S01E02.mkv"), []byte("video"), 0644)

	// STORAGE2: 5 episodes
	for i := 1; i <= 5; i++ {
		filename := filepath.Join(fallout2, filepath.Join(fallout2, filepath.Base(fallout2), filepath.Base(fallout2)))
		os.WriteFile(filepath.Join(fallout2, filepath.Base(filename)), []byte("video"), 0644)
	}
	os.WriteFile(filepath.Join(fallout2, "Fallout S01E01.mkv"), []byte("video"), 0644)
	os.WriteFile(filepath.Join(fallout2, "Fallout S01E02.mkv"), []byte("video"), 0644)
	os.WriteFile(filepath.Join(fallout2, "Fallout S01E03.mkv"), []byte("video"), 0644)
	os.WriteFile(filepath.Join(fallout2, "Fallout S01E04.mkv"), []byte("video"), 0644)
	os.WriteFile(filepath.Join(fallout2, "Fallout S01E05.mkv"), []byte("video"), 0644)

	selector := &Selector{
		libraries: []string{storage1, storage2, storage3},
	}

	locations := selector.findAllShowLocations("Fallout", "2024")

	// Should find 2 locations (STORAGE1 and STORAGE2)
	if len(locations) != 2 {
		t.Fatalf("findAllShowLocations() found %d locations, want 2", len(locations))
	}

	// Find which is which by episode count
	var loc1, loc2 ShowLocation
	for _, loc := range locations {
		if loc.EpisodeCount == 2 {
			loc1 = loc
		} else if loc.EpisodeCount == 5 {
			loc2 = loc
		}
	}

	if loc1.Library != storage1 {
		t.Errorf("Location with 2 episodes should be in STORAGE1, got %s", loc1.Library)
	}

	if loc2.Library != storage2 {
		t.Errorf("Location with 5 episodes should be in STORAGE2, got %s", loc2.Library)
	}

	if loc1.EpisodeCount != 2 {
		t.Errorf("STORAGE1 episode count = %d, want 2", loc1.EpisodeCount)
	}

	if loc2.EpisodeCount != 5 {
		t.Errorf("STORAGE2 episode count = %d, want 5", loc2.EpisodeCount)
	}
}

func TestFindAllShowLocations_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()
	library := filepath.Join(tmpDir, "TVSHOWS")
	os.MkdirAll(library, 0755)

	// Create some other shows
	os.MkdirAll(filepath.Join(library, "Breaking Bad"), 0755)
	os.MkdirAll(filepath.Join(library, "The Wire"), 0755)

	selector := &Selector{libraries: []string{library}}

	locations := selector.findAllShowLocations("Nonexistent Show", "2024")

	if len(locations) != 0 {
		t.Errorf("findAllShowLocations(nonexistent) found %d locations, want 0", len(locations))
	}
}

func TestFindShowDirInLibrary_YearAware(t *testing.T) {
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "library")
	seasonDir := filepath.Join(libDir, "Dracula (2020)")
	seasonDir2025 := filepath.Join(libDir, "Dracula (2025)")

	os.MkdirAll(seasonDir, 0755)
	os.MkdirAll(seasonDir2025, 0755)

	selector := &Selector{
		libraries: []string{libDir},
	}

	result := selector.findShowDirInLibrary(libDir, "dracula", "2020")
	if result != seasonDir {
		t.Errorf("year 2020: got %q, want %q", result, seasonDir)
	}

	result = selector.findShowDirInLibrary(libDir, "dracula", "2025")
	if result != seasonDir2025 {
		t.Errorf("year 2025: got %q, want %q", result, seasonDir2025)
	}

	result = selector.findShowDirInLibrary(libDir, "dracula", "")
	if result == "" {
		t.Errorf("no year: should match any, got empty")
	}
}
