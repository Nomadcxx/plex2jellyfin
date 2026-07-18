package compliance

import (
	"testing"
)

func TestCheckMovie_CompliantFile(t *testing.T) {
	checker := NewChecker("/media/Movies")

	tests := []struct {
		name string
		path string
	}{
		{
			name: "Simple movie with year",
			path: "/media/Movies/Interstellar (2014)/Interstellar (2014).mkv",
		},
		{
			name: "Movie with apostrophe",
			path: "/media/Movies/The King's Speech (2010)/The King's Speech (2010).mkv",
		},
		{
			name: "Movie with comma",
			path: "/media/Movies/Me, Myself & Irene (2000)/Me, Myself & Irene (2000).mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckMovie(tt.path)

			if !result.IsCompliant {
				t.Errorf("expected compliant file, got issues: %v", result.Issues)
			}

			if len(result.Issues) != 0 {
				t.Errorf("expected no issues, got: %v", result.Issues)
			}
		})
	}
}

func TestCheckMovie_NonCompliantFile(t *testing.T) {
	checker := NewChecker("/media/Movies")

	tests := []struct {
		name           string
		path           string
		expectedIssues []string
	}{
		{
			name: "Missing year",
			path: "/media/Movies/Interstellar/Interstellar.mkv",
			expectedIssues: []string{
				IssueMissingYear,
			},
		},
		{
			name: "Release markers",
			path: "/media/Movies/Interstellar (2014)/Interstellar.2014.1080p.BluRay.x264.mkv",
			expectedIssues: []string{
				IssueReleaseMarkers,
				IssueInvalidFilename,
			},
		},
		{
			name: "Wrong year format",
			path: "/media/Movies/Interstellar 2014/Interstellar 2014.mkv",
			expectedIssues: []string{
				IssueInvalidFolderStructure,
				IssueInvalidYearFormat,
			},
		},
		{
			name: "Invalid characters",
			path: "/media/Movies/Movie: The Beginning (2020)/Movie: The Beginning (2020).mkv",
			expectedIssues: []string{
				IssueSpecialCharacters,
			},
		},
		{
			name: "Wrong folder name",
			path: "/media/Movies/Wrong Folder/Interstellar (2014).mkv",
			expectedIssues: []string{
				IssueInvalidFolderStructure,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckMovie(tt.path)

			if result.IsCompliant {
				t.Error("expected non-compliant file, but it passed")
			}

			if len(result.Issues) == 0 {
				t.Error("expected issues, got none")
			}

			// Check that expected issue types are present
			for _, expectedIssue := range tt.expectedIssues {
				found := false
				for _, issue := range result.Issues {
					if containsIssueType(issue, expectedIssue) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue type '%s' not found in: %v", expectedIssue, result.Issues)
				}
			}
		})
	}
}

func TestCheckEpisode_CompliantFile(t *testing.T) {
	checker := NewChecker("/media/TV")

	tests := []struct {
		name string
		path string
	}{
		{
			name: "Standard episode",
			path: "/media/TV/Silo (2023)/Season 01/Silo (2023) S01E01.mkv",
		},
		{
			name: "Double digit season",
			path: "/media/TV/Breaking Bad (2008)/Season 02/Breaking Bad (2008) S02E05.mkv",
		},
		{
			name: "Show with apostrophe",
			path: "/media/TV/The Handmaid's Tale (2017)/Season 01/The Handmaid's Tale (2017) S01E03.mkv",
		},
		{
			// Regression: filenames carrying an episode title (Jellyfin's
			// metadata-gap fallback form) must not be flagged for rename.
			name: "Episode title in filename",
			path: "/media/TV/Show (2026)/Season 01/Show (2026) S01E01 - No Shortcuts.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckEpisode(tt.path)

			if !result.IsCompliant {
				t.Errorf("expected compliant file, got issues: %v", result.Issues)
			}

			if len(result.Issues) != 0 {
				t.Errorf("expected no issues, got: %v", result.Issues)
			}
		})
	}
}

func TestCheckEpisode_NonCompliantFile(t *testing.T) {
	checker := NewChecker("/media/TV")

	tests := []struct {
		name           string
		path           string
		expectedIssues []string
	}{
		{
			name: "Missing season padding",
			path: "/media/TV/Silo (2023)/Season 1/Silo (2023) S01E01.mkv",
			expectedIssues: []string{
				IssueWrongSeasonFolder,
				IssueInvalidPadding,
			},
		},
		{
			name: "Release markers",
			path: "/media/TV/Silo (2023)/Season 01/Silo.2023.S01E01.720p.WEB-DL.mkv",
			expectedIssues: []string{
				IssueReleaseMarkers,
				IssueInvalidFilename,
			},
		},
		{
			name: "Wrong season folder",
			path: "/media/TV/Silo (2023)/S01/Silo (2023) S01E01.mkv",
			expectedIssues: []string{
				IssueWrongSeasonFolder,
			},
		},
		// NOTE: Missing year is no longer a compliance issue for TV shows
		// (Jellyfin recommends but does not require year in TV show folders)
		{
			name: "Wrong show folder",
			path: "/media/TV/Wrong Folder/Season 01/Silo (2023) S01E01.mkv",
			expectedIssues: []string{
				IssueInvalidFolderStructure,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckEpisode(tt.path)

			if result.IsCompliant {
				t.Error("expected non-compliant file, but it passed")
			}

			if len(result.Issues) == 0 {
				t.Error("expected issues, got none")
			}

			// Check that expected issue types are present
			for _, expectedIssue := range tt.expectedIssues {
				found := false
				for _, issue := range result.Issues {
					if containsIssueType(issue, expectedIssue) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue type '%s' not found in: %v", expectedIssue, result.Issues)
				}
			}
		})
	}
}

func TestCheckFile_AutoDetection(t *testing.T) {
	checker := NewChecker("/media")

	tests := []struct {
		name        string
		path        string
		expectValid bool
	}{
		{
			name:        "Valid movie",
			path:        "/media/Movies/Her (2013)/Her (2013).mkv",
			expectValid: true,
		},
		{
			name:        "Valid TV episode",
			path:        "/media/TV/Silo (2023)/Season 01/Silo (2023) S01E01.mkv",
			expectValid: true,
		},
		{
			name:        "Invalid movie with markers",
			path:        "/media/Movies/Her (2013)/Her.2013.1080p.BluRay.mkv",
			expectValid: false,
		},
		{
			name:        "Invalid episode with markers",
			path:        "/media/TV/Silo (2023)/Season 01/Silo.S01E01.720p.mkv",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckFile(tt.path)

			if result.IsCompliant != tt.expectValid {
				t.Errorf("expected IsCompliant=%v, got %v. Issues: %v", tt.expectValid, result.IsCompliant, result.Issues)
			}
		})
	}
}

func TestHasReleaseMarkers(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"Movie (2020).mkv", false},
		{"Movie.2020.1080p.mkv", true},
		{"Movie.2020.BluRay.mkv", true},
		{"Movie.2020.x264.mkv", true},
		{"Movie.2020.WEB-DL.mkv", true},
		{"Movie.2020.REMUX.mkv", true},
		{"Show S01E01.mkv", false},
		{"Show.S01E01.720p.mkv", true},
		{"Show.S01E01.HEVC.mkv", true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := hasReleaseMarkers(tt.filename)
			if result != tt.expected {
				t.Errorf("hasReleaseMarkers(%s) = %v, expected %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestFindInvalidCharacters(t *testing.T) {
	tests := []struct {
		filename string
		expected []string
	}{
		{"Movie (2020).mkv", []string{}},
		{"Movie: The Beginning (2020).mkv", []string{":"}},
		{"Movie/Part 1 (2020).mkv", []string{"/"}},
		{"Movie<Special> (2020).mkv", []string{"<", ">"}},
		{"Movie*.mkv", []string{"*"}},
		{"Movie?.mkv", []string{"?"}},
		{`Movie\Path (2020).mkv`, []string{`\`}},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := findInvalidCharacters(tt.filename)

			if len(result) != len(tt.expected) {
				t.Errorf("findInvalidCharacters(%s) = %v, expected %v", tt.filename, result, tt.expected)
				return
			}

			for i, char := range result {
				if char != tt.expected[i] {
					t.Errorf("findInvalidCharacters(%s)[%d] = %s, expected %s", tt.filename, i, char, tt.expected[i])
				}
			}
		})
	}
}

func TestIsValidSeasonFolder(t *testing.T) {
	tests := []struct {
		folder   string
		expected bool
	}{
		{"Season 01", true},
		{"Season 02", true},
		{"Season 10", true},
		{"Season 99", true},
		{"Season 1", false},   // Not padded
		{"Season 001", false}, // Too many digits
		{"season 01", false},  // Wrong case
		{"S01", false},        // Wrong format
		{"Season01", false},   // No space
	}

	for _, tt := range tests {
		t.Run(tt.folder, func(t *testing.T) {
			result := isValidSeasonFolder(tt.folder)
			if result != tt.expected {
				t.Errorf("isValidSeasonFolder(%s) = %v, expected %v", tt.folder, result, tt.expected)
			}
		})
	}
}

// Helper function to check if an issue string contains an issue type
func containsIssueType(issue, issueType string) bool {
	return len(issue) >= len(issueType) && issue[:len(issueType)] == issueType
}
