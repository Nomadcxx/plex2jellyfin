package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

func TestServerHealth(t *testing.T) {
	handler := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	defer handler.Shutdown()

	server := NewServer(handler, nil, ":0", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status healthy, got %s", resp.Status)
	}
}

func TestServerHealthUnhealthy(t *testing.T) {
	handler := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	defer handler.Shutdown()

	server := NewServer(handler, nil, ":0", nil)
	server.SetHealthy(false)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("expected status unhealthy, got %s", resp.Status)
	}
}

func TestServerReady(t *testing.T) {
	handler := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	defer handler.Shutdown()

	server := NewServer(handler, nil, ":0", nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "ready" {
		t.Errorf("expected body 'ready', got '%s'", w.Body.String())
	}
}

func TestServerMetrics(t *testing.T) {
	handler := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	defer handler.Shutdown()

	handler.stats.RecordMovie(1024 * 1024 * 100)
	handler.stats.RecordTV(1024 * 1024 * 50)
	handler.stats.RecordError()

	server := NewServer(handler, nil, ":0", nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp MetricsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.MoviesProcessed != 1 {
		t.Errorf("expected 1 movie, got %d", resp.MoviesProcessed)
	}
	if resp.TVProcessed != 1 {
		t.Errorf("expected 1 TV, got %d", resp.TVProcessed)
	}
	if resp.TotalProcessed != 2 {
		t.Errorf("expected 2 total, got %d", resp.TotalProcessed)
	}
	if resp.Errors != 1 {
		t.Errorf("expected 1 error, got %d", resp.Errors)
	}
	if resp.BytesTransferMB < 149 || resp.BytesTransferMB > 151 {
		t.Errorf("expected ~150 MB, got %.2f", resp.BytesTransferMB)
	}
}

func TestStats(t *testing.T) {
	stats := NewStats()

	stats.RecordMovie(100)
	stats.RecordTV(200)
	stats.RecordError()

	snap := stats.Snapshot()

	if snap.MoviesProcessed != 1 {
		t.Errorf("expected 1 movie, got %d", snap.MoviesProcessed)
	}
	if snap.TVProcessed != 1 {
		t.Errorf("expected 1 TV, got %d", snap.TVProcessed)
	}
	if snap.BytesTransferred != 300 {
		t.Errorf("expected 300 bytes, got %d", snap.BytesTransferred)
	}
	if snap.Errors != 1 {
		t.Errorf("expected 1 error, got %d", snap.Errors)
	}
	if snap.Uptime <= 0 {
		t.Error("expected positive uptime")
	}
}
