package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPreflightReportsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": path, "kind": "watch"})
	req := httptest.NewRequest("POST", "/paths/preflight", bytes.NewReader(body))
	w := httptest.NewRecorder()
	(PreflightHandler{}).ServeHTTP(w, req)

	var got preflightResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Readable {
		t.Errorf("expected readable, got %+v", got)
	}
	if !got.Writable {
		t.Errorf("expected writable watch directory, got %+v", got)
	}
}

func TestPreflightReportsMissing(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"path": "/nonexistent/zzz", "kind": "watch"})
	req := httptest.NewRequest("POST", "/paths/preflight", bytes.NewReader(body))
	w := httptest.NewRecorder()
	(PreflightHandler{}).ServeHTTP(w, req)

	var got preflightResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Readable {
		t.Errorf("expected not readable")
	}
}
