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
		expectReason string
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

func TestProgressBar(t *testing.T) {
	t.Run("tracks progress correctly", func(t *testing.T) {
		pb := NewProgressBar(100)

		if pb.total != 100 {
			t.Errorf("total = %d, want 100", pb.total)
		}
		if pb.current != 0 {
			t.Errorf("current = %d, want 0", pb.current)
		}

		pb.Update(5, 1)
		if pb.current != 1 {
			t.Errorf("after Update, current = %d, want 1", pb.current)
		}
		if pb.actionsCreated != 5 {
			t.Errorf("actionsCreated = %d, want 5", pb.actionsCreated)
		}
	})

	t.Run("handles zero total", func(t *testing.T) {
		pb := NewProgressBar(0)
		pb.Update(0, 0)
		pb.Finish()
	})
}

func TestAuditStats(t *testing.T) {
	stats := NewAuditStats()

	// Record some AI calls
	stats.RecordAICall(true)
	stats.RecordAICall(true)
	stats.RecordAICall(false)

	if stats.AITotalCalls != 3 {
		t.Errorf("AITotalCalls = %d, want 3", stats.AITotalCalls)
	}
	if stats.AISuccessCount != 2 {
		t.Errorf("AISuccessCount = %d, want 2", stats.AISuccessCount)
	}
	if stats.AIErrorCount != 1 {
		t.Errorf("AIErrorCount = %d, want 1", stats.AIErrorCount)
	}

	// Record skips
	stats.RecordSkip("AI suggests movie but file is in episode library")
	stats.RecordSkip("confidence too low")
	stats.RecordSkip("title unchanged")

	if stats.TypeMismatches != 1 {
		t.Errorf("TypeMismatches = %d, want 1", stats.TypeMismatches)
	}
	if stats.ConfidenceTooLow != 1 {
		t.Errorf("ConfidenceTooLow = %d, want 1", stats.ConfidenceTooLow)
	}
	if stats.TitleUnchanged != 1 {
		t.Errorf("TitleUnchanged = %d, want 1", stats.TitleUnchanged)
	}

	// Record action
	stats.RecordAction()
	if stats.ActionsCreated != 1 {
		t.Errorf("ActionsCreated = %d, want 1", stats.ActionsCreated)
	}
}
