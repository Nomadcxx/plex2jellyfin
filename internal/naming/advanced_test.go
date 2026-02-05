package naming

import (
	"fmt"
	"testing"
)

func TestCleanMovieName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Hyphen preservation
		{"The.Count.of.Monte-Cristo.2024.WEB-DL.1080p.x264-GROUP", "The Count Of Monte-Cristo (2024)"},
		{"Monte-Cristo.2024.1080p.BluRay.x264-RARBG", "Monte-Cristo (2024)"},

		// Abbreviations
		{"R.I.P.D.2.Rise.of.the.Damned.2022.1080p.BluRay.x264-GROUP", "R.I.P.D. 2 Rise Of The Damned (2022)"},
		{"D.E.B.S.2004.1080p.WEB-DL.AAC2.0.H.264", "D.E.B.S. (2004)"},
		{"U.S.Marshals.1998.1080p.BluRay.x264-SPARKS", "U.S. Marshals (1998)"},

		// Ordinals
		{"The.21st.Century.2024.1080p.BluRay", "The 21st Century (2024)"},
		{"The.1st.Season.Movie.2023.WEB-DL", "The 1st Season Movie (2023)"},

		// Year in title
		{"Blade.Runner.2049.2017.1080p.BluRay.x264", "Blade Runner 2049 (2017)"},
		{"2001.A.Space.Odyssey.1968.2160p.UHD.BluRay", "2001 A Space Odyssey (1968)"},

		// Basic cases
		{"Movie.Name.2024.1080p.BluRay.REMUX.AVC.TrueHD.7.1.Atmos-FGT", "Movie Name (2024)"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := CleanMovieName(tc.input)
			if result != tc.expected {
				t.Errorf("\nInput:    %s\nExpected: %s\nGot:      %s", tc.input, tc.expected, result)
			}
		})
	}
}

func TestIsGarbageTitle(t *testing.T) {
	testCases := []struct {
		title    string
		expected bool
	}{
		{"x264", true},
		{"H264 RARBG", true},
		{"The Matrix", false},
		{"Rome", false},
		{"IT", false},
		{"Monte-Cristo", false},
		{"D3FiL3R", true},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			result := IsGarbageTitle(tc.title)
			if result != tc.expected {
				t.Errorf("IsGarbageTitle(%q) = %v, expected %v", tc.title, result, tc.expected)
			}
		})
	}
}

func TestIsGarbageTitle_KnownMediaTitles(t *testing.T) {
	// These are known media titles that appear in release group blacklist
	// They should NOT be flagged as garbage
	knownTitles := []string{
		"Barry",
		"Westworld",
		"Ted Lasso",
		"Ragnarok",
		"Rome",
		"Fargo",
		"Her",
		"Babylon 5",
	}

	for _, title := range knownTitles {
		t.Run(title, func(t *testing.T) {
			if IsGarbageTitle(title) {
				t.Errorf("IsGarbageTitle(%q) = true, want false (known media title)", title)
			}
		})
	}
}

func TestStripReleaseGroup(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"The.Count.of.Monte-Cristo.2024.WEB-DL.1080p.x264-GROUP", "The Count of Monte-Cristo 2024"},
		{"Movie-Unrated.2023.BluRay", "Movie 2023"},
		{"R.I.P.D.2.2022.1080p", "R.I.P.D. 2 2022"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := StripReleaseGroup(tc.input)
			if result != tc.expected {
				t.Errorf("\nInput:    %s\nExpected: %s\nGot:      %s", tc.input, tc.expected, result)
			}
		})
	}
}

func TestParseMovieNameAdvanced(t *testing.T) {
	testCases := []struct {
		input         string
		expectedTitle string
		expectedYear  string
	}{
		// Simple parser handles most cases (hyphen converted to space is acceptable)
		{"The.Count.of.Monte-Cristo.2024.WEB-DL.1080p.x264-GROUP.mkv", "The Count of Monte Cristo", "2024"},
		// R.I.P.D. triggers advanced parser because simple parser produces garbage
		{"R.I.P.D.2.Rise.of.the.Damned.2022.1080p.BluRay.x264-GROUP.mkv", "R.I.P.D. 2 Rise Of The Damned", "2022"},
		{"Blade.Runner.2049.2017.1080p.BluRay.x264.mkv", "Blade Runner 2049", "2017"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			info, err := ParseMovieName(tc.input)
			if err != nil {
				t.Errorf("ParseMovieName(%s) returned error: %v", tc.input, err)
				return
			}
			if info.Title != tc.expectedTitle {
				t.Errorf("\nInput:    %s\nExpected Title: %s\nGot Title:      %s", tc.input, tc.expectedTitle, info.Title)
			}
			if info.Year != tc.expectedYear {
				t.Errorf("\nInput:    %s\nExpected Year: %s\nGot Year:      %s", tc.input, tc.expectedYear, info.Year)
			}
		})
	}
}

// Print tests for visual inspection during development
func TestPrintCleanMovieName(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping print test in short mode")
	}

	testCases := []string{
		"The.Count.of.Monte-Cristo.2024.WEB-DL.1080p.x264-GROUP",
		"Monte-Cristo.2024.1080p.BluRay.x264-RARBG",
		"R.I.P.D.2.Rise.of.the.Damned.2022.1080p.BluRay.x264-GROUP",
		"D.E.B.S.2004.1080p.WEB-DL.AAC2.0.H.264",
		"U.S.Marshals.1998.1080p.BluRay.x264-SPARKS",
		"The.21st.Century.2024.1080p.BluRay",
		"Blade.Runner.2049.2017.1080p.BluRay.x264",
		"2001.A.Space.Odyssey.1968.2160p.UHD.BluRay",
		"Movie.Name.2024.1080p.BluRay.REMUX.AVC.TrueHD.7.1.Atmos-FGT",
	}

	fmt.Println("\n=== CleanMovieName Results ===")
	for _, tc := range testCases {
		clean := CleanMovieName(tc)
		fmt.Printf("%-60s -> %s\n", tc, clean)
	}
}
