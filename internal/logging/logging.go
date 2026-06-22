// Package logging provides structured logging with file output and rotation.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/paths"
)

// Level represents a logging level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a string to a Level
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
	Value interface{}
}

// F creates a new Field (shorthand for structured logging)
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Config holds logger configuration
type Config struct {
	Level      string `mapstructure:"level"`        // debug, info, warn, error
	File       string `mapstructure:"file"`         // log file path (empty = stdout only)
	MaxSizeMB  int    `mapstructure:"max_size_mb"`  // max size before rotation (default: 10)
	MaxBackups int    `mapstructure:"max_backups"`  // number of backups to keep (default: 5)
	MaxAgeDays int    `mapstructure:"max_age_days"` // delete backups older than N days (0 = disabled)
	Compress   bool   `mapstructure:"compress"`     // gzip rotated backups (default: false)
}

// DefaultConfig returns default logging configuration
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		File:       "", // Will be set to ~/.config/jellywatch/logs/jellywatch.log
		MaxSizeMB:  10,
		MaxBackups: 5,
		MaxAgeDays: 0,
		Compress:   false,
	}
}

// Logger provides structured logging with file output
type Logger struct {
	level      Level
	mu         sync.Mutex
	file       *os.File
	filePath   string
	maxSize    int64 // in bytes
	maxBackups int
	maxAgeDays int
	compress   bool
	writers    []io.Writer
}

// New creates a new Logger with the given configuration
func New(cfg Config) (*Logger, error) {
	l := &Logger{
		level:      ParseLevel(cfg.Level),
		maxSize:    int64(cfg.MaxSizeMB) * 1024 * 1024,
		maxBackups: cfg.MaxBackups,
		maxAgeDays: cfg.MaxAgeDays,
		compress:   cfg.Compress,
		writers:    []io.Writer{os.Stdout},
	}

	if l.maxSize == 0 {
		l.maxSize = 10 * 1024 * 1024 // 10MB default
	}
	if l.maxBackups == 0 {
		l.maxBackups = 5
	}

	if cfg.File == "" {
		jwDir, err := paths.JellyWatchDir()
		if err != nil {
			return nil, fmt.Errorf("unable to get jellywatch dir: %w", err)
		}
		cfg.File = filepath.Join(jwDir, "logs", "jellywatch.log")
	}

	// Expand ~ in file path
	if strings.HasPrefix(cfg.File, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("unable to get home dir: %w", err)
		}
		cfg.File = filepath.Join(home, cfg.File[1:])
	}

	l.filePath = cfg.File

	// Create log directory
	logDir := filepath.Dir(cfg.File)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("unable to create log directory: %w", err)
	}

	// Open log file
	if err := l.openFile(); err != nil {
		return nil, err
	}

	return l, nil
}

// openFile opens or creates the log file
func (l *Logger) openFile() error {
	if l.filePath == "" {
		return nil
	}

	f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("unable to open log file: %w", err)
	}

	l.file = f
	l.writers = []io.Writer{os.Stdout, f}
	return nil
}

// checkRotation checks if the log file needs rotation and performs it
func (l *Logger) checkRotation() error {
	if l.file == nil {
		return nil
	}

	info, err := l.file.Stat()
	if err != nil {
		return err
	}

	if info.Size() < l.maxSize {
		return nil
	}

	// Need to rotate
	return l.rotate()
}

// rotate performs log rotation
func (l *Logger) rotate() error {
	// Close current file
	if l.file != nil {
		l.file.Close()
	}

	// Rotate existing backups, optionally compressing the newly-rotated file.
	if err := rotateFiles(l.filePath, l.maxBackups, l.compress); err != nil {
		return err
	}

	// Prune backups older than MaxAgeDays (zero disables).
	if l.maxAgeDays > 0 {
		if err := pruneOldBackups(l.filePath, l.maxAgeDays); err != nil {
			// Don't fail rotation if pruning errors — best-effort.
			fmt.Fprintf(os.Stderr, "log age-pruning error: %v\n", err)
		}
	}

	// Reopen the log file
	return l.openFile()
}

// log writes a log entry
func (l *Logger) log(level Level, component, msg string, err error, fields ...Field) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check rotation before writing
	if rotErr := l.checkRotation(); rotErr != nil {
		fmt.Fprintf(os.Stderr, "log rotation error: %v\n", rotErr)
	}

	// Build log line
	timestamp := time.Now().Format(time.RFC3339)
	var sb strings.Builder

	sb.WriteString(timestamp)
	sb.WriteString(" [")
	sb.WriteString(level.String())
	sb.WriteString("] [")
	sb.WriteString(component)
	sb.WriteString("] ")
	sb.WriteString(msg)

	// Add error if present
	if err != nil {
		sb.WriteString(" | error=")
		sb.WriteString(err.Error())
	}

	// Add fields
	for _, f := range fields {
		sb.WriteString(" | ")
		sb.WriteString(f.Key)
		sb.WriteString("=")
		sb.WriteString(fmt.Sprintf("%v", f.Value))
	}

	sb.WriteString("\n")
	line := sb.String()

	// Write to all writers
	for _, w := range l.writers {
		w.Write([]byte(line))
	}
}

// Debug logs a debug message
func (l *Logger) Debug(component, msg string, fields ...Field) {
	l.log(LevelDebug, component, msg, nil, fields...)
}

// Info logs an info message
func (l *Logger) Info(component, msg string, fields ...Field) {
	l.log(LevelInfo, component, msg, nil, fields...)
}

// Warn logs a warning message
func (l *Logger) Warn(component, msg string, fields ...Field) {
	l.log(LevelWarn, component, msg, nil, fields...)
}

// Error logs an error message with an error
func (l *Logger) Error(component, msg string, err error, fields ...Field) {
	l.log(LevelError, component, msg, err, fields...)
}

// Close closes the log file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() Level {
	return l.level
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// FilePath returns the log file path
func (l *Logger) FilePath() string {
	return l.filePath
}

// Nop returns a no-operation logger that discards all output
func Nop() *Logger {
	return &Logger{
		level:   LevelError + 1, // Higher than any valid level
		writers: []io.Writer{},
	}
}
