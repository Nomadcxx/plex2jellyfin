package ai

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type mockDBProvider struct {
	db *sql.DB
}

func (m *mockDBProvider) DB() *sql.DB {
	return m.db
}

func setupTestDatabase(t *testing.T) *mockDBProvider {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE ai_parse_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			input_normalized TEXT NOT NULL,
			input_type TEXT NOT NULL,
			title TEXT NOT NULL,
			year INTEGER,
			media_type TEXT NOT NULL,
			season INTEGER,
			episodes TEXT,
			absolute_episode INTEGER,
			air_date TEXT,
			confidence REAL NOT NULL,
			model TEXT NOT NULL,
			latency_ms INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			usage_count INTEGER DEFAULT 1,
			UNIQUE(input_normalized, input_type, model)
		)
	`)
	if err != nil {
		t.Fatalf("failed to create ai_parse_cache table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE ai_improvements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id TEXT NOT NULL UNIQUE,
			filename TEXT NOT NULL,
			user_title TEXT NOT NULL,
			user_type TEXT NOT NULL,
			user_year INTEGER,
			ai_title TEXT,
			ai_type TEXT,
			ai_year INTEGER,
			ai_confidence REAL,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER DEFAULT 0,
			error_message TEXT,
			model TEXT,
			original_request TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("failed to create ai_improvements table: %v", err)
	}

	return &mockDBProvider{db: db}
}

func TestNewIntegrator_Disabled(t *testing.T) {
	cfg := config.AIConfig{
		Enabled: false,
	}

	integrator, err := NewIntegrator(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer integrator.Close()

	if integrator.IsEnabled() {
		t.Error("expected integrator to be disabled")
	}

	if integrator.IsAvailable() {
		t.Error("expected disabled integrator to not be available")
	}
}

func TestNewIntegrator_Enabled(t *testing.T) {
	dbProvider := setupTestDatabase(t)
	defer dbProvider.db.Close()

	cfg := config.AIConfig{
		Enabled:             true,
		OllamaEndpoint:      "http://localhost:11434",
		Model:               "llama3.2",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      5,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     5,
			FailureWindowSeconds: 120,
			CooldownSeconds:      30,
		},
		Keepalive: config.KeepaliveConfig{
			Enabled:            false,
			IntervalSeconds:    300,
			IdleTimeoutSeconds: 1800,
		},
	}

	integrator, err := NewIntegrator(cfg, dbProvider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer integrator.Close()

	if !integrator.IsEnabled() {
		t.Error("expected integrator to be enabled")
	}

	if !integrator.IsAvailable() {
		t.Error("expected integrator to be available initially")
	}
}

func TestIntegrator_EnhanceTitle_Disabled(t *testing.T) {
	cfg := config.AIConfig{
		Enabled: false,
	}

	integrator, _ := NewIntegrator(cfg, nil)
	defer integrator.Close()

	title, source, err := integrator.EnhanceTitle("Test Movie", "test.movie.2024.mkv", "movie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if title != "Test Movie" {
		t.Errorf("expected 'Test Movie', got '%s'", title)
	}

	if source != SourceRegex {
		t.Errorf("expected SourceRegex, got %v", source)
	}
}

func TestIntegrator_CircuitBreaker_BlocksWhenOpen(t *testing.T) {
	dbProvider := setupTestDatabase(t)
	defer dbProvider.db.Close()

	cfg := config.AIConfig{
		Enabled:             true,
		OllamaEndpoint:      "http://localhost:11434",
		Model:               "llama3.2",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      1,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     2,
			FailureWindowSeconds: 60,
			CooldownSeconds:      1,
		},
		Keepalive: config.KeepaliveConfig{
			Enabled: false,
		},
	}

	integrator, err := NewIntegrator(cfg, dbProvider)
	if err != nil {
		t.Fatalf("failed to create integrator: %v", err)
	}
	defer integrator.Close()

	integrator.circuit.RecordFailure("test error 1")
	integrator.circuit.RecordFailure("test error 2")

	if integrator.circuit.State() != CircuitOpen {
		t.Fatalf("expected circuit open, got %v", integrator.circuit.State())
	}

	title, source, _ := integrator.EnhanceTitle("Test Title", "test.file.mkv", "movie")

	if source != SourceRegex {
		t.Errorf("expected SourceRegex when circuit open, got %v", source)
	}

	if title != "Test Title" {
		t.Errorf("expected original title, got %s", title)
	}
}

func TestIntegrator_QueueForEnhancement(t *testing.T) {
	dbProvider := setupTestDatabase(t)
	defer dbProvider.db.Close()

	cfg := config.AIConfig{
		Enabled:             true,
		OllamaEndpoint:      "http://localhost:11434",
		Model:               "llama3.2",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      1,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     5,
			FailureWindowSeconds: 120,
			CooldownSeconds:      30,
		},
		Keepalive: config.KeepaliveConfig{
			Enabled: false,
		},
	}

	integrator, err := NewIntegrator(cfg, dbProvider)
	if err != nil {
		t.Fatalf("failed to create integrator: %v", err)
	}
	defer integrator.Close()

	queued := integrator.QueueForEnhancement("req-123", "test.movie.2024.mkv", "Test Movie", "movie")
	if !queued {
		t.Error("expected request to be queued")
	}

	time.Sleep(50 * time.Millisecond)

	status := integrator.Status()
	if status.QueueCapacity != 100 {
		t.Errorf("expected queue capacity 100, got %d", status.QueueCapacity)
	}
}

func TestIntegrator_Status(t *testing.T) {
	dbProvider := setupTestDatabase(t)
	defer dbProvider.db.Close()

	cfg := config.AIConfig{
		Enabled:             true,
		OllamaEndpoint:      "http://localhost:11434",
		Model:               "llama3.2",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      5,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     5,
			FailureWindowSeconds: 120,
			CooldownSeconds:      30,
		},
		Keepalive: config.KeepaliveConfig{
			Enabled: false,
		},
	}

	integrator, err := NewIntegrator(cfg, dbProvider)
	if err != nil {
		t.Fatalf("failed to create integrator: %v", err)
	}
	defer integrator.Close()

	status := integrator.Status()

	if status.CircuitState != CircuitClosed {
		t.Errorf("expected CircuitClosed, got %v", status.CircuitState)
	}

	if !status.ModelAvailable {
		t.Error("expected model to be available")
	}

	if status.ModelName != "llama3.2" {
		t.Errorf("expected model name 'llama3.2', got '%s'", status.ModelName)
	}
}

func TestIntegrator_Close(t *testing.T) {
	dbProvider := setupTestDatabase(t)
	defer dbProvider.db.Close()

	cfg := config.AIConfig{
		Enabled:             true,
		OllamaEndpoint:      "http://localhost:11434",
		Model:               "llama3.2",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      5,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:     5,
			FailureWindowSeconds: 120,
			CooldownSeconds:      30,
		},
		Keepalive: config.KeepaliveConfig{
			Enabled: false,
		},
	}

	integrator, err := NewIntegrator(cfg, dbProvider)
	if err != nil {
		t.Fatalf("failed to create integrator: %v", err)
	}

	err = integrator.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}

	err = integrator.Close()
	if err != nil {
		t.Error("second close should not error")
	}

	queued := integrator.QueueForEnhancement("req-456", "test.mkv", "Test", "movie")
	if queued {
		t.Error("should not queue after shutdown")
	}
}

func TestParseSource_String(t *testing.T) {
	tests := []struct {
		source   ParseSource
		expected string
	}{
		{SourceRegex, "regex"},
		{SourceCache, "cache"},
		{SourceAI, "ai"},
		{ParseSource(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.source.String(); got != tt.expected {
			t.Errorf("ParseSource(%d).String() = %s, want %s", tt.source, got, tt.expected)
		}
	}
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{integratorTestError("connection refused"), true},
		{integratorTestError("no such host"), true},
		{integratorTestError("timeout"), true},
		{integratorTestError("context deadline exceeded"), true},
		{integratorTestError("network is unreachable"), true},
		{integratorTestError("some other error"), false},
	}

	for _, tt := range tests {
		if got := isConnectionError(tt.err); got != tt.expected {
			errStr := "nil"
			if tt.err != nil {
				errStr = tt.err.Error()
			}
			t.Errorf("isConnectionError(%s) = %v, want %v", errStr, got, tt.expected)
		}
	}
}

type integratorTestErr struct {
	msg string
}

func (e *integratorTestErr) Error() string {
	return e.msg
}

func integratorTestError(msg string) error {
	return &integratorTestErr{msg: msg}
}
