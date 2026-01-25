package naming

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestParseMovieName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
		wantYear  string
		wantErr   bool
	}{
		{
			name:      "Standard format",
			input:     "Dune Part Two (2024).mkv",
			wantTitle: "Dune Part Two",
			wantYear:  "2024",
			wantErr:   false,
		},
		{
			name:      "With release markers",
			input:     "Dune.Part.Two.2024.1080p.BluRay.x264-GROUP.mkv",
			wantTitle: "Dune Part Two",
			wantYear:  "2024",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMovieName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMovieName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Title != tt.wantTitle {
				t.Errorf("ParseMovieName() title = %v, want %v", got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("ParseMovieName() year = %v, want %v", got.Year, tt.wantYear)
			}
		})
	}
}

func TestParseTVShowName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTitle  string
		wantSeason int
		wantEp     int
		wantErr    bool
	}{
		{
			name:       "Standard format",
			input:      "Breaking Bad S01E01.mkv",
			wantTitle:  "Breaking Bad",
			wantSeason: 1,
			wantEp:     1,
			wantErr:    false,
		},
		{
			name:       "With release markers",
			input:      "Breaking.Bad.S01E01.1080p.WEB-DL.x264.mkv",
			wantTitle:  "Breaking Bad",
			wantSeason: 1,
			wantEp:     1,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTVShowName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTVShowName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Title != tt.wantTitle {
				t.Errorf("ParseTVShowName() title = %v, want %v", got.Title, tt.wantTitle)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("ParseTVShowName() season = %v, want %v", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEp {
				t.Errorf("ParseTVShowName() episode = %v, want %v", got.Episode, tt.wantEp)
			}
		})
	}
}

func TestIsTVEpisodeFilename_DatePatterns(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		// Date patterns - should be TV
		{"The.Daily.Show.2026.01.22.Guest.Name.1080p.WEB.mkv", true},
		{"Colbert.2026-01-22.1080p.WEB.mkv", true},
		{"SNL.2026_01_22.Host.1080p.mkv", true},
		{"Last.Week.Tonight.2026.01.19.1080p.mkv", true},

		// Standard patterns still work
		{"Show.S01E05.mkv", true},
		{"Show.1x05.mkv", true},

		// Movies - should not be TV
		{"Movie.Name.2026.1080p.mkv", false},
		{"Another.Movie.2024.BluRay.mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsTVEpisodeFilename(tt.filename)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsTVEpisodeFromPath_SourceHint(t *testing.T) {
	tests := []struct {
		path string
		hint SourceHint
		want bool
	}{
		// SourceTV forces TV classification
		{"/downloads/movie.mkv", SourceTV, true},
		{"/downloads/no.pattern.mkv", SourceTV, true},

		// SourceMovie forces Movie classification
		{"/downloads/Show.S01E05.mkv", SourceMovie, false},
		{"/downloads/Daily.Show.2026.01.22.mkv", SourceMovie, false},

		// SourceUnknown uses filename detection
		{"/downloads/Show.S01E05.mkv", SourceUnknown, true},
		{"/downloads/Daily.Show.2026.01.22.mkv", SourceUnknown, true},
		{"/downloads/Movie.2024.mkv", SourceUnknown, false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_hint_%v", filepath.Base(tt.path), tt.hint)
		t.Run(name, func(t *testing.T) {
			got := IsTVEpisodeFromPath(tt.path, tt.hint)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFromPath(%q, %v) = %v, want %v", tt.path, tt.hint, got, tt.want)
			}
		})
	}
}
