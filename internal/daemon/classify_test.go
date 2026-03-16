package daemon

import "testing"

func TestClassifyChange_Punctuation(t *testing.T) {
	c := ClassifyChange("Freddys Nightmares", "Freddy's Nightmares", "", "", "tv", "tv")
	if c.Category != ChangePunctuation {
		t.Errorf("got %s, want %s", c.Category, ChangePunctuation)
	}
	if !c.Safe {
		t.Error("punctuation changes should be safe")
	}
	if c.MinConfidence != 0.80 {
		t.Errorf("min confidence = %.2f, want 0.80", c.MinConfidence)
	}
}

func TestClassifyChange_Casing(t *testing.T) {
	c := ClassifyChange("the office", "The Office", "", "", "tv", "tv")
	if c.Category != ChangeCasing {
		t.Errorf("got %s, want %s", c.Category, ChangeCasing)
	}
	if !c.Safe {
		t.Error("casing changes should be safe")
	}
}

func TestClassifyChange_YearAdded(t *testing.T) {
	c := ClassifyChange("The Office", "The Office", "", "2005", "tv", "tv")
	if c.Category != ChangeYearAdded {
		t.Errorf("got %s, want %s", c.Category, ChangeYearAdded)
	}
	if !c.Safe {
		t.Error("year addition should be safe")
	}
	if c.MinConfidence != 0.85 {
		t.Errorf("min confidence = %.2f, want 0.85", c.MinConfidence)
	}
}

func TestClassifyChange_YearCorrected(t *testing.T) {
	c := ClassifyChange("Show", "Show", "2024", "2025", "tv", "tv")
	if c.Category != ChangeYearCorrected {
		t.Errorf("got %s, want %s", c.Category, ChangeYearCorrected)
	}
	if c.MinConfidence != 0.90 {
		t.Errorf("min confidence = %.2f, want 0.90", c.MinConfidence)
	}
}

func TestClassifyChange_DifferentTitle(t *testing.T) {
	c := ClassifyChange("Weird", "Something Completely Different", "", "", "tv", "tv")
	if c.Category != ChangeTitleDifferent {
		t.Errorf("got %s, want %s", c.Category, ChangeTitleDifferent)
	}
	if c.Safe {
		t.Error("different title should NOT be safe")
	}
}

func TestClassifyChange_TypeChange(t *testing.T) {
	c := ClassifyChange("Show", "Show", "", "", "tv", "movie")
	if c.Category != ChangeTypeDifferent {
		t.Errorf("got %s, want %s", c.Category, ChangeTypeDifferent)
	}
	if c.Safe {
		t.Error("type changes should NOT be safe")
	}
}

func TestClassifyChange_NoChange(t *testing.T) {
	c := ClassifyChange("Breaking Bad", "Breaking Bad", "2008", "2008", "tv", "tv")
	if c.Category != ChangeNone {
		t.Errorf("got %s, want %s", c.Category, ChangeNone)
	}
	if !c.Safe {
		t.Error("no change should be safe")
	}
}

func TestClassifyChange_YearRemoved(t *testing.T) {
	c := ClassifyChange("Show", "Show", "2024", "", "tv", "tv")
	if c.Category != ChangeYearRemoved {
		t.Errorf("got %s, want %s", c.Category, ChangeYearRemoved)
	}
	if c.Safe {
		t.Error("year removed by AI should NOT be safe")
	}
}

func TestClassifyChange_PunctuationAndYearCorrected(t *testing.T) {
	c := ClassifyChange("Freddys Nightmares", "Freddy's Nightmares", "2023", "2024", "tv", "tv")
	// Should recognize the year correction, not just the punctuation change
	if c.Category != ChangeYearCorrected {
		t.Errorf("got %s, want %s — year correction should take precedence", c.Category, ChangeYearCorrected)
	}
	if c.Safe {
		t.Error("year correction should NOT be safe (MinConfidence=0.90)")
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"Freddys Nightmares", "Freddy's Nightmares", 0.9, 1.0},
		{"Weird", "Something Completely Different", 0.0, 0.1},
		{"The Office", "The Office", 1.0, 1.0},
		{"Breaking Bad", "Breaking Badly", 0.3, 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got := jaccardWordSimilarity(tt.a, tt.b)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("jaccardWordSimilarity(%q, %q) = %.2f, want [%.2f, %.2f]",
					tt.a, tt.b, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
