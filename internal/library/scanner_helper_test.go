package library

import (
	"os"
	"testing"
)

func TestExtractYearFromDir(t *testing.T) {
	tests := []struct {
		dirName  string
		expected string
	}{
		{"Dracula (2020)", "2020"},
		{"The Matrix (1999)", "1999"},
		{"No Year Folder", ""},
		{"Movie Name 2024", ""}, // No parentheses
	}

	for _, tt := range tests {
		result := extractYearFromDir(tt.dirName)
		if result != tt.expected {
			t.Errorf("extractYearFromDir(%q) = %q, want %q", tt.dirName, result, tt.expected)
		}
	}
}

func TestFindShowDirInLibrary_YearAware(t *testing.T) {
	// Create a mock library with entries
	tmpDir := t.TempDir()
	libDir := tmpDir + "/library"
	seasonDir := libDir + "/Dracula (2020)"
	seasonDir2025 := libDir + "/Dracula (2025)"

	// Create directories
	os.MkdirAll(seasonDir, 0755)
	os.MkdirAll(seasonDir2025, 0755)

	// Create Selector
	selector := &Selector{
		libraries: []string{libDir},
	}

	// Test 1: When looking for year "2020", should match "Dracula (2020)"
	result := selector.findShowDirInLibrary(libDir, "dracula", "2020")
	if result == "" {
		t.Errorf("Should match year 2020, got empty")
	}
	if result != seasonDir {
		t.Errorf("Should match 'Dracula (2020)', got: %s, expected: %s", result, seasonDir)
	}

	// Test 2: When looking for year "2025", should match "Dracula (2025)"
	result = selector.findShowDirInLibrary(libDir, "dracula", "2025")
	if result == "" {
		t.Errorf("Should match year 2025, got empty")
	}
	if result != seasonDir2025 {
		t.Errorf("Should match 'Dracula (2025)', got: %s, expected: %s", result, seasonDir2025)
	}

	// Test 3: No year parameter should match alphabetically first
	result = selector.findShowDirInLibrary(libDir, "dracula", "")
	if result == "" {
		t.Errorf("No year should match any, got empty")
	}

	// Cleanup
	os.RemoveAll(tmpDir)
}
