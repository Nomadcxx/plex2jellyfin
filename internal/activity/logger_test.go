package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jellywatch-activity-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewLogger(tmpDir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()

	if logger.GetLogDir() != filepath.Join(tmpDir, "activity") {
		t.Errorf("expected log dir %s, got %s", filepath.Join(tmpDir, "activity"), logger.GetLogDir())
	}
}

func TestLogEntry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jellywatch-activity-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewLogger(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	year := 2024
	confidence := 0.92
	entry := Entry{
		Action:         "organize",
		Source:         "/downloads/test.mkv",
		Target:         "/movies/Test (2024)/test.mkv",
		MediaType:      "movie",
		ParseMethod:    MethodAI,
		ParsedTitle:    "Test Movie",
		ParsedYear:     &year,
		AIConfidence:   &confidence,
		Success:        true,
		Bytes:          1234567,
		DurationMs:     1000,
		SonarrNotified: false,
		RadarrNotified: true,
	}

	if err := logger.Log(entry); err != nil {
		t.Fatalf("failed to log entry: %v", err)
	}

	entries, _ := os.ReadDir(logger.GetLogDir())

	var logFile string
	for _, f := range entries {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".jsonl") {
			logFile = filepath.Join(logger.GetLogDir(), f.Name())
			break
		}
	}

	if logFile == "" {
		t.Fatal("no log file found")
	}

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var logged Entry
	if err := json.Unmarshal(content, &logged); err != nil {
		t.Fatalf("failed to parse logged entry: %v", err)
	}

	if logged.Action != entry.Action {
		t.Errorf("expected action %s, got %s", entry.Action, logged.Action)
	}

	if logged.AIConfidence == nil || *logged.AIConfidence != confidence {
		t.Errorf("expected confidence %f, got %v", confidence, logged.AIConfidence)
	}
}

func TestPruneOld(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "jellywatch-activity-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logger, err := NewLogger(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	oldDate := time.Now().AddDate(0, 0, -10)
	oldFile := filepath.Join(logger.GetLogDir(), "activity-"+oldDate.Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(oldFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	recentDate := time.Now()
	recentFile := filepath.Join(logger.GetLogDir(), "activity-"+recentDate.Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(recentFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := logger.PruneOld(7); err != nil {
		t.Fatalf("failed to prune old logs: %v", err)
	}

	_, oldErr := os.Stat(oldFile)
	_, recentErr := os.Stat(recentFile)

	if !os.IsNotExist(oldErr) {
		t.Error("old file should have been pruned")
	}

	if recentErr != nil {
		t.Error("recent file should still exist: ", recentErr)
	}
}
