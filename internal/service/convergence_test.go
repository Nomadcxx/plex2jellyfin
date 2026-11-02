package service

import "testing"

func TestGroupConfidenceDoesNotAssumeEqualSizeMeansIdentical(t *testing.T) {
	group := DuplicateGroup{
		BestFileID: 1,
		Files: []MediaFile{
			{ID: 1, Size: 100, Resolution: "1080p", QualityScore: 50},
			{ID: 2, Size: 100, Resolution: "1080p", QualityScore: 50},
		},
	}

	level, reasons := (&CleanupService{}).GroupConfidence(group)
	if level != ConfidenceLow || len(reasons) == 0 {
		t.Fatalf("equal-size quality tie confidence = %q, reasons = %v; want low confidence with a reason", level, reasons)
	}
}
