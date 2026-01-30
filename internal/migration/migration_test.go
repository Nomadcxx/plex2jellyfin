package migration

import (
	"testing"
)

func TestPathMismatchStructure(t *testing.T) {
	mismatch := PathMismatch{
		MediaType:    "series",
		ID:           1,
		Title:        "Test Series",
		Year:         2024,
		DatabasePath: "/media/TV/Test Series (2024)",
		SonarrPath:   "/old/path/Test Series",
		HasSonarrID:  true,
	}

	if mismatch.MediaType != "series" {
		t.Errorf("expected media type series, got %s", mismatch.MediaType)
	}

	if mismatch.DatabasePath == mismatch.SonarrPath {
		t.Error("paths should be different for a mismatch")
	}
}

func TestFixChoiceConstants(t *testing.T) {
	tests := []struct {
		choice FixChoice
		want   string
	}{
		{FixChoiceKeepJellyWatch, "jellywatch"},
		{FixChoiceKeepSonarrRadarr, "arr"},
		{FixChoiceSkip, "skip"},
	}

	for _, tt := range tests {
		if string(tt.choice) != tt.want {
			t.Errorf("FixChoice constant mismatch: got %s, want %s", tt.choice, tt.want)
		}
	}
}
