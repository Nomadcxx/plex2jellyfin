package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const enhanceLogFilename = "ai-enhancements.jsonl"

type EnhanceLogEntry struct {
	Ts           string  `json:"ts"`
	Action       string  `json:"action"`
	File         string  `json:"file,omitempty"`
	RegexTitle   string  `json:"regex_title,omitempty"`
	AITitle      string  `json:"ai_title,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	AIConfidence float64 `json:"ai_confidence,omitempty"`
	Category     string  `json:"category,omitempty"`
	AutoApplied  bool    `json:"auto_applied,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	MediaType    string  `json:"media_type,omitempty"`
	PendingCount int     `json:"pending_count,omitempty"`
	HourlyUsed   int     `json:"hourly_used,omitempty"`
	DailyUsed    int     `json:"daily_used,omitempty"`
}

type EnhanceLogger struct {
	dir        string
	maxSize    int64
	maxBackups int
}

func NewEnhanceLogger(dir string) *EnhanceLogger {
	return &EnhanceLogger{
		dir:        dir,
		maxSize:    10 * 1024 * 1024, // 10MB
		maxBackups: 3,
	}
}

func (l *EnhanceLogger) Log(entry EnhanceLogEntry) error {
	if entry.Ts == "" {
		entry.Ts = time.Now().UTC().Format(time.RFC3339)
	}

	logPath := filepath.Join(l.dir, enhanceLogFilename)

	if err := l.rotateIfNeeded(logPath); err != nil {
		return fmt.Errorf("rotation failed: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func (l *EnhanceLogger) LogPath() string {
	return filepath.Join(l.dir, enhanceLogFilename)
}

func (l *EnhanceLogger) rotateIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil // file doesn't exist yet
	}
	if info.Size() < l.maxSize {
		return nil
	}

	// Shift backups: .3 -> deleted, .2 -> .3, .1 -> .2, current -> .1
	for i := l.maxBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		if i == l.maxBackups {
			os.Remove(src)
			continue
		}
		dst := fmt.Sprintf("%s.%d", path, i+1)
		os.Rename(src, dst)
	}

	return os.Rename(path, path+".1")
}

func ReadFlaggedForReview(logPath string) ([]EnhanceLogEntry, error) {
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	flagged := make(map[string]EnhanceLogEntry)
	resolved := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry EnhanceLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Action {
		case "flagged_for_review":
			flagged[entry.File] = entry
		case "review_approved", "review_rejected":
			resolved[entry.File] = true
		}
	}

	var pending []EnhanceLogEntry
	for file, entry := range flagged {
		if !resolved[file] {
			pending = append(pending, entry)
		}
	}

	return pending, scanner.Err()
}
