package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifyMediaUpdatedPostsScopedPaths(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, APIKey: "test-key"})
	if err := c.NotifyMediaUpdated([]string{"/movies1/Scary Movie (2026)"}); err != nil {
		t.Fatalf("NotifyMediaUpdated error: %v", err)
	}
	if gotPath != "/Library/Media/Updated" {
		t.Errorf("endpoint = %q, want /Library/Media/Updated", gotPath)
	}
	updates, ok := gotBody["Updates"].([]any)
	if !ok || len(updates) != 1 {
		t.Fatalf("Updates = %v, want 1 entry", gotBody["Updates"])
	}
	first := updates[0].(map[string]any)
	if first["Path"] != "/movies1/Scary Movie (2026)" {
		t.Errorf("Path = %v, want /movies1/Scary Movie (2026)", first["Path"])
	}
	if first["UpdateType"] != "Created" {
		t.Errorf("UpdateType = %v, want Created", first["UpdateType"])
	}
}

func TestNotifyMediaUpdatedEmptyNoCall(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, APIKey: "test-key"})
	if err := c.NotifyMediaUpdated(nil); err != nil {
		t.Fatalf("NotifyMediaUpdated(nil) error: %v", err)
	}
	if called {
		t.Error("expected no HTTP call for empty input")
	}
}