package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnhanceLogger_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)

	entry := EnhanceLogEntry{
		Action:     "fast_lane",
		File:       "test.mkv",
		RegexTitle: "Test",
		Confidence: 0.85,
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ai-enhancements.jsonl"))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var parsed EnhanceLogEntry
	if err := json.Unmarshal(data[:len(data)-1], &parsed); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if parsed.Action != "fast_lane" {
		t.Errorf("action = %q, want %q", parsed.Action, "fast_lane")
	}
	if parsed.Ts == "" {
		t.Error("timestamp should be set automatically")
	}
}

func TestEnhanceLogger_Rotation(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)
	logger.maxSize = 100 // rotate after 100 bytes

	// Write enough to trigger rotation
	for i := 0; i < 10; i++ {
		logger.Log(EnhanceLogEntry{
			Action:     "fast_lane",
			File:       "test.mkv",
			RegexTitle: "Some Title That Is Long Enough",
			Confidence: 0.85,
		})
	}

	// Should have created backup files
	entries, _ := os.ReadDir(dir)
	jsonlFiles := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "ai-enhancements") {
			jsonlFiles++
		}
	}
	if jsonlFiles < 2 {
		t.Errorf("expected at least 2 jsonl files after rotation, got %d", jsonlFiles)
	}
}

func TestEnhanceLogger_MaxBackups(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)
	logger.maxSize = 50
	logger.maxBackups = 2

	// Write enough to trigger multiple rotations
	for i := 0; i < 30; i++ {
		logger.Log(EnhanceLogEntry{
			Action:     "fast_lane",
			File:       "test.mkv",
			RegexTitle: "Title",
			Confidence: 0.85,
		})
	}

	entries, _ := os.ReadDir(dir)
	jsonlFiles := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "ai-enhancements") {
			jsonlFiles++
		}
	}
	// current file + maxBackups
	if jsonlFiles > 3 {
		t.Errorf("expected at most 3 files (current + 2 backups), got %d", jsonlFiles)
	}
}

func TestReadFlaggedForReview(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)

	// Write a mix of actions
	logger.Log(EnhanceLogEntry{Action: "fast_lane", File: "a.mkv"})
	logger.Log(EnhanceLogEntry{Action: "flagged_for_review", File: "b.mkv", AITitle: "Better Title"})
	logger.Log(EnhanceLogEntry{Action: "flagged_for_review", File: "c.mkv", AITitle: "Other Title"})
	logger.Log(EnhanceLogEntry{Action: "review_approved", File: "b.mkv"})

	flagged, err := ReadFlaggedForReview(filepath.Join(dir, "ai-enhancements.jsonl"))
	if err != nil {
		t.Fatalf("ReadFlaggedForReview error: %v", err)
	}
	// Only c.mkv should be pending (b.mkv was approved)
	if len(flagged) != 1 {
		t.Fatalf("expected 1 pending review, got %d", len(flagged))
	}
	if flagged[0].File != "c.mkv" {
		t.Errorf("expected c.mkv, got %s", flagged[0].File)
	}
}
