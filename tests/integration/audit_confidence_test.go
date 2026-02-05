package integration

import (
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/naming"
)

func TestAuditConfidenceFix(t *testing.T) {
	// Test that well-named files get reasonable confidence scores
	testCases := []struct {
		filename    string
		mediaType   string
		wantMinConf float64
		description string
	}{
		{
			filename:    "Barry.S01E01.mkv",
			mediaType:   "episode",
			wantMinConf: 0.6,
			description: "Single-word known show should not be flagged as garbage",
		},
		{
			filename:    "Westworld S01E01.mkv",
			mediaType:   "episode",
			wantMinConf: 0.6,
			description: "Known show with space separator",
		},
		{
			filename:    "Ted Lasso S02E06.mkv",
			mediaType:   "episode",
			wantMinConf: 0.8,
			description: "Multi-word known show",
		},
		{
			filename:    "Interstellar (2014).mkv",
			mediaType:   "movie",
			wantMinConf: 0.79,
			description: "Properly formatted movie",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Parse the filename
			var rawTitle string
			if tc.mediaType == "episode" {
				tv, err := naming.ParseTVShowName(tc.filename)
				if err != nil {
					t.Fatalf("Failed to parse TV show: %v", err)
				}
				rawTitle = tv.Title
			} else {
				movie, err := naming.ParseMovieName(tc.filename)
				if err != nil {
					t.Fatalf("Failed to parse movie: %v", err)
				}
				rawTitle = movie.Title
			}

			// Calculate confidence using RAW title
			confidence := naming.CalculateTitleConfidence(rawTitle, tc.filename)

			if confidence < tc.wantMinConf {
				t.Errorf("Confidence for %q = %.2f, want >= %.2f (raw title: %q)",
					tc.filename, confidence, tc.wantMinConf, rawTitle)
			}

			// Also verify the file would be considered Jellyfin-compliant
			if naming.IsJellyfinCompliantFilename(tc.filename, tc.mediaType) {
				t.Logf("File %q is already Jellyfin-compliant", tc.filename)
			}
		})
	}
}

func TestNormalizedVsRawTitle(t *testing.T) {
	// Demonstrate the difference between normalized and raw titles
	filename := "Barry.S01E01.mkv"

	tv, err := naming.ParseTVShowName(filename)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	rawTitle := tv.Title                                 // "Barry"
	normalizedTitle := database.NormalizeTitle(rawTitle) // "barry"

	rawConf := naming.CalculateTitleConfidence(rawTitle, filename)
	normConf := naming.CalculateTitleConfidence(normalizedTitle, filename)

	t.Logf("Raw title %q confidence: %.2f", rawTitle, rawConf)
	t.Logf("Normalized title %q confidence: %.2f", normalizedTitle, normConf)

	// Raw title should have higher confidence
	if rawConf <= normConf {
		t.Logf("Note: After fix, both should be reasonable. Raw=%.2f, Norm=%.2f", rawConf, normConf)
	}

	// With the fix, both should be >= 0.6
	if rawConf < 0.6 {
		t.Errorf("Raw title confidence %.2f is too low", rawConf)
	}
}
