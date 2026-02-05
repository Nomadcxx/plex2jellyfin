package naming

import "testing"

func TestIsJellyfinCompliantFilename(t *testing.T) {
	tests := []struct {
		filename  string
		mediaType string
		want      bool
	}{
		// Compliant TV episodes
		{"Barry.S01E01.mkv", "episode", true},
		{"Barry S01E01.mkv", "episode", true},
		{"Barry - S01E01.mkv", "episode", true},
		{"Barry (2018) S01E01.mkv", "episode", true},
		{"Barry (2018) - S01E01.mkv", "episode", true},
		{"The Office S01E01.mkv", "episode", true},
		{"The Office (US) S01E01.mkv", "episode", true},

		// Non-compliant TV episodes (have release markers)
		{"Barry.S01E01.1080p.WEB-DL.mkv", "episode", false},
		{"Barry.S01E01.HDTV.x264-LOL.mkv", "episode", false},

		// Compliant movies
		{"Interstellar (2014).mkv", "movie", true},
		{"The Matrix (1999).mkv", "movie", true},

		// Non-compliant movies
		{"Interstellar.2014.1080p.BluRay.mkv", "movie", false},
		{"Interstellar (2014) 1080p BluRay.mkv", "movie", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsJellyfinCompliantFilename(tt.filename, tt.mediaType)
			if got != tt.want {
				t.Errorf("IsJellyfinCompliantFilename(%q, %q) = %v, want %v",
					tt.filename, tt.mediaType, got, tt.want)
			}
		})
	}
}
