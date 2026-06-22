package postmortem

import "testing"

func TestClassifySuspiciousNamePollutedReleaseMarker(t *testing.T) {
	got := ClassifySuspiciousName("Ratatouille RoDubbed (2007)", "/mnt/STORAGE1/MOVIES/Ratatouille RoDubbed (2007)")
	if got.Category != "polluted_name" {
		t.Fatalf("Category = %q", got.Category)
	}
	if got.Marker != "RoDubbed" {
		t.Fatalf("Marker = %q", got.Marker)
	}
}

func TestClassifyPathTranslationMismatch(t *testing.T) {
	got := ClassifyPathMismatch("/mnt/STORAGE5/TVSHOWS/The Daily Show/file.mkv", "/tv5/The Daily Show/file.mkv")
	if got.Category != "path_translation_false_positive" {
		t.Fatalf("Category = %q", got.Category)
	}
}
