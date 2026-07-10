package jellystat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSendsTokenAndParsesResponses(t *testing.T) {
	var gotPath, gotToken string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("x-api-token")
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"Name":"Movies","Library_Count":100}]`))
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL + "/", APIKey: "secret-key"})

	raw, err := c.LibraryOverview()
	if err != nil {
		t.Fatalf("LibraryOverview: %v", err)
	}
	if gotPath != "/stats/getLibraryOverview" {
		t.Errorf("path = %s", gotPath)
	}
	if gotToken != "secret-key" {
		t.Errorf("token = %q", gotToken)
	}
	if len(raw) == 0 {
		t.Error("empty response")
	}

	if _, err := c.MostViewedByType(30, "Movie"); err != nil {
		t.Fatalf("MostViewedByType: %v", err)
	}
	if gotPath != "/stats/getMostViewedByType" {
		t.Errorf("path = %s", gotPath)
	}
	if gotBody["type"] != "Movie" || gotBody["days"] != float64(30) {
		t.Errorf("body = %v", gotBody)
	}
}

func TestClientAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(Config{URL: srv.URL, APIKey: "bad"})
	if err := c.Test(); err == nil {
		t.Fatal("expected auth error")
	}
}
