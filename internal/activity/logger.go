package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ParseMethod string

const (
	MethodRegex ParseMethod = "regex"
	MethodAI    ParseMethod = "ai"
	MethodCache ParseMethod = "cache"
)

type Entry struct {
	Timestamp      time.Time   `json:"ts"`
	Action         string      `json:"action"`
	Source         string      `json:"source"`
	Target         string      `json:"target,omitempty"`
	MediaType      string      `json:"media_type"`
	ParseMethod    ParseMethod `json:"parse_method"`
	ParsedTitle    string      `json:"parsed_title"`
	ParsedYear     *int        `json:"parsed_year,omitempty"`
	AIConfidence   *float64    `json:"ai_confidence,omitempty"`
	Success        bool        `json:"success"`
	Bytes          int64       `json:"bytes,omitempty"`
	DurationMs     int64       `json:"duration_ms,omitempty"`
	SonarrNotified bool        `json:"sonarr_notified"`
	RadarrNotified bool        `json:"radarr_notified"`
	Error          string      `json:"error,omitempty"`
}

type Logger struct {
	mu          sync.Mutex
	configDir   string
	logDir      string
	currentFile *os.File
	currentDate string
}

func NewLogger(configDir string) (*Logger, error) {
	logDir := filepath.Join(configDir, "activity")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	return &Logger{
		configDir: configDir,
		logDir:    logDir,
	}, nil
}

func (l *Logger) Log(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry.Timestamp = time.Now()

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	today := time.Now().Format("2006-01-02")

	if l.currentDate != today || l.currentFile == nil {
		if err := l.rotateFile(today); err != nil {
			return err
		}
	}

	if l.currentFile == nil {
		return nil
	}

	_, err = l.currentFile.Write(append(line, '\n'))
	return err
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		return l.currentFile.Close()
	}
	return nil
}

func (l *Logger) PruneOld(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "activity-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		if entry.IsDir() {
			continue
		}

		name := strings.TrimPrefix(entry.Name(), "activity-")
		name = strings.TrimSuffix(name, ".jsonl")

		fileDate, err := time.Parse("2006-01-02", name)
		if err != nil {
			continue
		}

		if fileDate.Before(cutoff) {
			os.Remove(filepath.Join(l.logDir, entry.Name()))
		}
	}

	return nil
}

func (l *Logger) rotateFile(date string) error {
	if l.currentFile != nil {
		l.currentFile.Close()
	}

	filePath := filepath.Join(l.logDir, "activity-"+date+".jsonl")

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	l.currentFile = file
	l.currentDate = date

	return nil
}

func (l *Logger) GetLogDir() string {
	return l.logDir
}
