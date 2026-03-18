package database

import "testing"

func TestNormalizeForMatch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "The Pitt", "thepitt"},
		{"strip apostrophe", "Chip 'n Dale Rescue Rangers", "chipndalerescuerangers"},
		{"strip colon", "Law & Order: SVU", "lawordersvu"},
		{"strip ampersand", "Law & Order", "laworder"},
		{"strip exclamation", "American Dad!", "americandad"},
		{"dots and underscores", "The.Pitt_2025", "thepitt2025"},
		{"mixed punctuation", "Attenborough's Life in Colour", "attenboroughslifeincolour"},
		{"already normalized", "thepitt", "thepitt"},
		{"empty string", "", ""},
		// Apostrophe variants must match each other
		{"apostrophe match lhs", "Chip 'n Dale", "chipndale"},
		{"apostrophe match rhs", "Chip n Dale", "chipndale"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeForMatch(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeForMatch(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeForMatch_EquivalenceGroups(t *testing.T) {
	// These groups of inputs MUST all normalize to the same value
	groups := [][]string{
		{"Chip 'n Dale Rescue Rangers", "Chip n Dale Rescue Rangers", "chip.n.dale.rescue.rangers"},
		{"Attenborough's Life in Colour", "Attenboroughs Life in Colour"},
		{"American Dad!", "American Dad"},
		{"M*A*S*H", "MASH", "mash"},
	}

	for _, group := range groups {
		t.Run(group[0], func(t *testing.T) {
			expected := NormalizeForMatch(group[0])
			for _, variant := range group[1:] {
				got := NormalizeForMatch(variant)
				if got != expected {
					t.Errorf("NormalizeForMatch(%q) = %q, but NormalizeForMatch(%q) = %q — should be equal",
						group[0], expected, variant, got)
				}
			}
		})
	}
}
