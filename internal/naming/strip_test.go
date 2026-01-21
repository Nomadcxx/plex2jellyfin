package naming

import (
    "strings"
    "testing"
)

func TestStripReleaseMarkersChained(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"2.Guns.2013.1080p.BluRay.x264-SPARKS-postbot", "2 Guns 2013"},
        {"Sinister.2012.720p.BluRay.x264-AMIABLE-WhiteRevtmp", "Sinister 2012"},
        {"Batman.1989.2160p.BDRip.AAC.7.1.HDR10.x265.10bit-MarkII", "Batman 1989"},
        {"Movie.2020.1080p-GROUP", "Movie 2020"},
        {"Movie.2020.1080p-One-Two-Three", "Movie 2020"},
    }
    
    for _, tc := range tests {
        result := stripReleaseMarkers(tc.input)
        // Normalize spaces and trim for comparison
        result = strings.TrimSpace(normalizeSpaces(result))
        expected := strings.TrimSpace(normalizeSpaces(tc.expected))
        if result != expected {
            t.Errorf("stripReleaseMarkers(%q)\n  got:      %q\n  expected: %q", tc.input, result, expected)
        }
    }
}

func TestParseMovieNameChainedReleaseGroups(t *testing.T) {
    tests := []struct {
        filename       string
        expectedTitle  string
        expectedYear   string
    }{
        {"2.Guns.2013.1080p.BluRay.x264-SPARKS-postbot.mkv", "2 Guns", "2013"},
        {"Sinister.2012.720p.BluRay.x264-AMIABLE-WhiteRevtmp.mkv", "Sinister", "2012"},
    }
    
    for _, tc := range tests {
        info, err := ParseMovieName(tc.filename)
        if err != nil {
            t.Errorf("ParseMovieName(%q) error: %v", tc.filename, err)
            continue
        }
        if info.Title != tc.expectedTitle {
            t.Errorf("ParseMovieName(%q) title = %q, want %q", tc.filename, info.Title, tc.expectedTitle)
        }
        if info.Year != tc.expectedYear {
            t.Errorf("ParseMovieName(%q) year = %q, want %q", tc.filename, info.Year, tc.expectedYear)
        }
    }
}
