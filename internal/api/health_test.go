package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func TestHealthCheckReportsInjectedVersion(t *testing.T) {
	s := NewServer(nil, config.DefaultConfig())
	s.SetVersion("0.1.4-test")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	s.HealthCheck(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if got["version"] != "0.1.4-test" {
		t.Fatalf("version = %q, want injected 0.1.4-test (not hardcoded 1.0.0)", got["version"])
	}
	if got["status"] != "ok" {
		t.Fatalf("status = %q, want ok", got["status"])
	}
}
