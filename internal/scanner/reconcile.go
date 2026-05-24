package scanner

import (
	"bufio"
	"encoding/json"
	"fmt"
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
	// deterministicRetryDays is the minimum interval between retries of a
	// deterministic failure (e.g. unparseable filename) when the source
	// file hasn't changed. Short enough to recover after a parser fix ships,
	// long enough to not re-log the same failure every scan cycle.
	deterministicRetryDays = 7
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

	// Collect the latest failure entry per source within this file. Multiple
	// failure entries for the same path accumulate over a day; retrying each
	// independently produces the classic retry-amplification pattern. We
	// dedup to the latest entry so a given source is retried at most once
	// per reconciliation pass.
	latestFailure := make(map[string]activity.Entry)

	for scanner.Scan() {
		var entry activity.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Success {
			hasRecentOrSuccess = true
			// A later success for this source clears any pending retry.
			if entry.Source != "" {
				delete(latestFailure, entry.Source)
			}
			continue
		}

		if entry.Timestamp.Before(cleanupWindow) {
			hasOldFailures = true
			continue
		}

		// Within retention window — consider for retry. Keep the latest
		// (by timestamp) failure per source.
		hasRecentOrSuccess = true
		if entry.Source == "" {
			continue
		}
		if prev, ok := latestFailure[entry.Source]; !ok || entry.Timestamp.After(prev.Timestamp) {
			latestFailure[entry.Source] = entry
		}
	}

	now := time.Now()
	deterministicCutoff := now.Add(-deterministicRetryDays * 24 * time.Hour)

	for _, entry := range latestFailure {
		// Only retry failures within the standard retry window unless the
		// entry is deterministic and due for a periodic re-check.
		withinRetryWindow := entry.Timestamp.After(retryWindow)

		if entry.Deterministic || isDeterministicActivityFailure(entry) {
			if !s.deterministicDueForRetry(entry, deterministicCutoff) {
				continue
			}
		} else if !withinRetryWindow {
			continue
		}

		if retryErr := s.retryTransfer(entry); retryErr == nil {
			retried++
		}
	}

	// Only clean if file has old failures and nothing recent/successful
	shouldClean = hasOldFailures && !hasRecentOrSuccess

	return retried, shouldClean, scanner.Err()
}

// deterministicDueForRetry returns true when a deterministic failure should
// be retried this pass. Two triggers (OR): the source mtime has changed
// since the failure was recorded (file was re-downloaded or renamed), OR
// the failure is older than deterministicRetryDays (in case a parser fix
// has shipped that might now handle it).
func (s *PeriodicScanner) deterministicDueForRetry(entry activity.Entry, cutoff time.Time) bool {
	if entry.Timestamp.Before(cutoff) {
		return true
	}
	if entry.Source == "" {
		return false
	}
	if entry.SourceMtime == 0 {
		return false
	}
	st, err := os.Stat(entry.Source)
	if err != nil {
		return false
	}
	return st.ModTime().Unix() != entry.SourceMtime
}

func isDeterministicActivityFailure(entry activity.Entry) bool {
	if entry.Error == "" {
		return false
	}
	msg := strings.ToLower(entry.Error)
	patterns := []string{
		"could not extract tv show info from path",
		"obfuscated filename, no episode markers",
		"no episode markers in parent folders",
		"season_pack_unresolved",
		"unable to parse tv show name",
		"could not extract movie info from path",
		"unable to parse movie name",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func (s *PeriodicScanner) retryTransfer(entry activity.Entry) error {
	// Check if source file still exists
	if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
		return err
	}

	// Only retry files inside configured watch paths
	insideWatch := false
	for _, wp := range s.watchPaths {
		if strings.HasPrefix(entry.Source, wp) {
			insideWatch = true
			break
		}
	}
	if !insideWatch {
		return fmt.Errorf("source %s is outside watch roots, skipping retry", entry.Source)
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
