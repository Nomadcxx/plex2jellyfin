package main

import (
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

func TestInferTypeFromLibraryRoot(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		// TV paths - should return "episode"
		{"uppercase TVSHOWS", "/mnt/STORAGE1/TVSHOWS", "episode"},
		{"lowercase tv", "/data/tv/Breaking Bad", "episode"},
		{"TV Shows with space", "/media/TV Shows/Friends", "episode"},
		{"series keyword", "/storage/Series/The Office", "episode"},
		{"shows keyword", "/nas/Shows/Game of Thrones", "episode"},

		// Movie paths - should return "movie"
		{"uppercase Movies", "/mnt/STORAGE2/Movies", "movie"},
		{"lowercase movies", "/data/movies/The Matrix", "movie"},
		{"film keyword", "/media/Films/Inception", "movie"},
		{"films plural", "/nas/films/Interstellar", "movie"},

		// Unknown paths - should return "unknown"
		{"downloads folder", "/downloads/completed", "unknown"},
		{"generic media", "/media/content", "unknown"},
		{"numbered storage", "/mnt/storage1/data", "unknown"},
		{"empty string", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferTypeFromLibraryRoot(tt.path)
			if result != tt.expected {
				t.Errorf("inferTypeFromLibraryRoot(%q) = %q, want %q",
					tt.path, result, tt.expected)
			}
		})
	}
}

func TestValidateMediaType(t *testing.T) {
	// Config with no APIs enabled (simpler tests)
	cfg := &config.Config{}

	tests := []struct {
		name         string
		file         *database.MediaFile
		aiResult     *ai.Result
		expectValid  bool
		expectReason string // substring to check for in reason
	}{
		{
			name: "TV folder with AI type tv - valid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/TVSHOWS",
				MediaType:   "episode",
			},
			aiResult:     &ai.Result{Type: "tv", Title: "Breaking Bad"},
			expectValid:  true,
			expectReason: "",
		},
		{
			name: "TV folder with AI type movie - invalid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/TVSHOWS",
				MediaType:   "episode",
			},
			aiResult:     &ai.Result{Type: "movie", Title: "The Matrix"},
			expectValid:  false,
			expectReason: "AI suggests movie but file is in episode library",
		},
		{
			name: "Movies folder with AI type movie - valid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/Movies",
				MediaType:   "movie",
			},
			aiResult:     &ai.Result{Type: "movie", Title: "Inception"},
			expectValid:  true,
			expectReason: "",
		},
		{
			name: "Movies folder with AI type tv - invalid",
			file: &database.MediaFile{
				LibraryRoot: "/mnt/Movies",
				MediaType:   "movie",
			},
			aiResult:     &ai.Result{Type: "tv", Title: "Friends"},
			expectValid:  false,
			expectReason: "AI suggests episode but file is in movie library",
		},
		{
			name: "Unknown folder with AI type tv - valid (no validation)",
			file: &database.MediaFile{
				LibraryRoot: "/downloads/completed",
				MediaType:   "episode",
			},
			aiResult:     &ai.Result{Type: "tv", Title: "The Office"},
			expectValid:  true,
			expectReason: "",
		},
		{
			name: "Unknown folder with AI type movie - valid (no validation)",
			file: &database.MediaFile{
				LibraryRoot: "/downloads/completed",
				MediaType:   "movie",
			},
			aiResult:     &ai.Result{Type: "movie", Title: "Avatar"},
			expectValid:  true,
			expectReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, reason := validateMediaType(tt.file, tt.aiResult, cfg)
			if valid != tt.expectValid {
				t.Errorf("validateMediaType() valid = %v, want %v; reason = %q",
					valid, tt.expectValid, reason)
			}
			if tt.expectReason != "" && reason != tt.expectReason {
				t.Errorf("validateMediaType() reason = %q, want %q",
					reason, tt.expectReason)
			}
		})
	}
}
