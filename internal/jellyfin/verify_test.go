package jellyfin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewVerifier(t *testing.T) {
	client := &Client{}
	verifier := NewVerifier(client)

	if verifier == nil {
		t.Fatal("NewVerifier returned nil")
	}

	if verifier.client != client {
		t.Error("verifier.client does not match input client")
	}
}

func TestVerifierVerifyLibrary(t *testing.T) {
	// Create temporary test directory
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "movie.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Items":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Items":[{"Id":"123","Name":"Test Movie","Path":"` + testFile + `","Type":"Movie"}]}`))
		case "/Library/VirtualFolders":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Name":"Movies","Locations":["` + tempDir + `"],"ItemId":"lib-1"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	verifier := NewVerifier(client)
	result, err := verifier.VerifyLibrary("lib-1")
	if err != nil {
		t.Fatalf("VerifyLibrary error: %v", err)
	}

	if result == nil {
		t.Fatal("VerifyLibrary returned nil result")
	}

	if result.ScannedCount != 1 {
		t.Errorf("expected ScannedCount 1, got %d", result.ScannedCount)
	}
}

func TestVerifierGetUnidentifiedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Items" {
			w.Header().Set("Content-Type", "application/json")
			// Return items with empty ProviderIds (unidentified)
			w.Write([]byte(`{"Items":[{"Id":"123","Name":"Unknown Movie","Path":"/movies/unknown.mkv","Type":"Movie","ProviderIds":{}}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	verifier := NewVerifier(client)
	items, err := verifier.GetUnidentifiedItems("lib-1")
	if err != nil {
		t.Fatalf("GetUnidentifiedItems error: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("expected 1 unidentified item, got %d", len(items))
	}
}

func TestVerifierFindOrphanedFiles(t *testing.T) {
	// Create temp dir with file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "exists.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Mock server returning a path that doesn't exist
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Items" {
			w.Header().Set("Content-Type", "application/json")
			// Return item with non-existent path
			w.Write([]byte(`{"Items":[{"Id":"123","Name":"Orphaned Movie","Path":"/nonexistent/orphaned.mkv","Type":"Movie"}]}`))
			return
		}
		if r.URL.Path == "/Library/VirtualFolders" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Name":"Movies","Locations":["` + tempDir + `"],"ItemId":"lib-1"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	verifier := NewVerifier(client)
	orphaned, err := verifier.FindOrphanedFiles("lib-1")
	if err != nil {
		t.Fatalf("FindOrphanedFiles error: %v", err)
	}

	if len(orphaned) != 1 {
		t.Errorf("expected 1 orphaned file, got %d", len(orphaned))
	}

	if orphaned[0].Issue != IssueOrphaned {
		t.Errorf("expected issue type 'orphaned', got '%s'", orphaned[0].Issue)
	}
}

func TestVerifierFindMissingFromJellyfin(t *testing.T) {
	// Create temp dir with file that exists on filesystem
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "missing.mkv")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Mock server returning empty items (Jellyfin doesn't know about the file)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Items" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Items":[]}`))
			return
		}
		if r.URL.Path == "/Library/VirtualFolders" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"Name":"Movies","Locations":["` + tempDir + `"],"ItemId":"lib-1"}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{
		URL:    server.URL,
		APIKey: "test-key",
	})

	verifier := NewVerifier(client)
	missing, err := verifier.FindMissingFromJellyfin(tempDir)
	if err != nil {
		t.Fatalf("FindMissingFromJellyfin error: %v", err)
	}

	if len(missing) != 1 {
		t.Errorf("expected 1 missing file, got %d", len(missing))
	}

	if missing[0].Issue != IssueMissing {
		t.Errorf("expected issue type 'missing', got '%s'", missing[0].Issue)
	}
}

func TestVerificationResultCounts(t *testing.T) {
	result := VerificationResult{
		LibraryPath:       "/test",
		ScannedCount:      10,
		IdentifiedCount:   8,
		UnidentifiedCount: 2,
		Mismatches: []Mismatch{
			{Path: "/test/file1.mkv", Issue: IssueOrphaned},
			{Path: "/test/file2.mkv", Issue: IssueMissing},
		},
		LastRun: time.Now(),
	}

	if result.ScannedCount != 10 {
		t.Errorf("expected ScannedCount 10, got %d", result.ScannedCount)
	}

	if len(result.Mismatches) != 2 {
		t.Errorf("expected 2 mismatches, got %d", len(result.Mismatches))
	}
}

func TestIssueTypeString(t *testing.T) {
	tests := []struct {
		issue    IssueType
		expected string
	}{
		{IssueOrphaned, "orphaned"},
		{IssueMissing, "missing"},
		{IssueUnidentified, "unidentified"},
	}

	for _, tt := range tests {
		if string(tt.issue) != tt.expected {
			t.Errorf("expected IssueType '%s', got '%s'", tt.expected, string(tt.issue))
		}
	}
}
