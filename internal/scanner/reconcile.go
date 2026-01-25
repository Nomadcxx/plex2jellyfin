package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
)

const (
	retryWindowHours  = 24
	cleanupWindowDays = 7
)

func (s *PeriodicScanner) reconcileActivity() (retried int, cleaned int, err error) {
	if s.activityDir == "" {
		return 0, 0, nil
	}

	now := time.Now()
	retryWindow := now.Add(-retryWindowHours * time.Hour)
	cleanupWindow := now.Add(-cleanupWindowDays * 24 * time.Hour)

	entries, err := os.ReadDir(s.activityDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	var toClean []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "activity-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		filePath := filepath.Join(s.activityDir, name)
		fileRetried, fileToClean, fileErr := s.processActivityFile(filePath, retryWindow, cleanupWindow)
		if fileErr != nil {
			s.logger.Warn("scanner", "Error processing activity file",
				logging.F("file", name),
				logging.F("error", fileErr.Error()))
			continue
		}

		retried += fileRetried
		if fileToClean {
			toClean = append(toClean, filePath)
		}
	}

	// Clean files that are entirely old failures
	for _, path := range toClean {
		if err := s.cleanActivityFile(path, cleanupWindow); err != nil {
			s.logger.Warn("scanner", "Error cleaning activity file",
				logging.F("path", path),
				logging.F("error", err.Error()))
		} else {
			cleaned++
		}
	}

	s.logger.Info("scanner", "Activity reconciliation complete",
		logging.F("retried", retried),
		logging.F("cleaned", cleaned))

	return retried, cleaned, nil
}

func (s *PeriodicScanner) processActivityFile(path string, retryWindow, cleanupWindow time.Time) (retried int, shouldClean bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	hasRecentOrSuccess := false
	hasOldFailures := false

	for scanner.Scan() {
		var entry activity.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Success {
			hasRecentOrSuccess = true
			continue
		}

		// Failed entry
		if entry.Timestamp.After(retryWindow) {
			// Recent failure - retry it
			hasRecentOrSuccess = true
			if entry.Source != "" {
				if retryErr := s.retryTransfer(entry); retryErr == nil {
					retried++
				}
			}
		} else if entry.Timestamp.Before(cleanupWindow) {
			// Old failure - mark for cleanup
			hasOldFailures = true
		} else {
			// Between retry and cleanup window - keep but don't retry
			hasRecentOrSuccess = true
		}
	}

	// Only clean if file has old failures and nothing recent/successful
	shouldClean = hasOldFailures && !hasRecentOrSuccess

	return retried, shouldClean, scanner.Err()
}

func (s *PeriodicScanner) retryTransfer(entry activity.Entry) error {
	// Check if source file still exists
	if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
		return err
	}

	s.logger.Info("scanner", "Retrying failed transfer",
		logging.F("source", entry.Source))

	event := watcher.FileEvent{
		Type: watcher.EventCreate,
		Path: entry.Source,
	}

	return s.handler.HandleFileEvent(event)
}

func (s *PeriodicScanner) cleanActivityFile(path string, cleanupWindow time.Time) error {
	// Read all entries, keep only successes and recent failures
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	var keepEntries []activity.Entry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var entry activity.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		// Keep successful entries and failures newer than cleanup window
		if entry.Success || entry.Timestamp.After(cleanupWindow) {
			keepEntries = append(keepEntries, entry)
		}
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return err
	}

	// If nothing to keep, remove the file
	if len(keepEntries) == 0 {
		return os.Remove(path)
	}

	// Rewrite file with kept entries
	tmpPath := path + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, entry := range keepEntries {
		line, _ := json.Marshal(entry)
		tmpFile.Write(append(line, '\n'))
	}
	tmpFile.Close()

	return os.Rename(tmpPath, path)
}
