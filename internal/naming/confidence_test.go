// internal/naming/confidence_test.go
package naming

import "testing"

func TestCalculateTitleConfidence(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		original string
		wantMin  float64
		wantMax  float64
	}{
		{
			name:     "clean title with year",
			title:    "The Matrix",
			original: "The.Matrix.1999.1080p.BluRay.mkv",
			wantMin:  0.9,
			wantMax:  1.0,
		},
		{
			name:     "garbage title (release group)",
			title:    "RARBG",
			original: "RARBG.mkv",
			wantMin:  0.0,
			wantMax:  0.3,
		},
		{
			name:     "very short title",
			title:    "IT",
			original: "IT.2017.mkv",
			wantMin:  0.4,
			wantMax:  0.7,
		},
		{
			name:     "single word no spaces",
			title:    "Interstellar",
			original: "Interstellar.2014.mkv",
			wantMin:  0.5,
			wantMax:  0.9,
		},
		{
			name:     "duplicate year pattern",
			title:    "Matrix (2001)",
			original: "Matrix (2001) (2001).mkv",
			wantMin:  0.0,
			wantMax:  0.6,
		},
		{
			name:     "codec in title",
			title:    "Movie x264",
			original: "Movie.x264.mkv",
			wantMin:  0.0,
			wantMax:  0.5,
		},
		{
			name:     "resolution in title",
			title:    "Movie 1080p",
			original: "Movie.1080p.mkv",
			wantMin:  0.0,
			wantMax:  0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateTitleConfidence(tt.title, tt.original)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateTitleConfidence(%q, %q) = %v, want between %v and %v",
					tt.title, tt.original, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestHasDuplicateYear(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Matrix (2001) (2001)", true},
		{"Matrix (2001)", false},
		{"2001 A Space Odyssey (2001)", false}, // Title year != release year is OK
		{"Movie 2020 2020", true},
		{"Movie (2020) 2020", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := hasDuplicateYear(tt.input)
			if got != tt.want {
				t.Errorf("hasDuplicateYear(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
