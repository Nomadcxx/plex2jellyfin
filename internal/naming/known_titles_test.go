package naming

import "testing"

func TestIsKnownMediaTitle(t *testing.T) {
	tests := []struct {
		title string
		want  bool
	}{
		// Known TV shows that conflict with release group names
		{"barry", true},
		{"westworld", true},
		{"ted", true},
		{"lasso", true},
		{"ragnarok", true},
		{"rome", true},
		{"fargo", true},
		{"dexter", true},

		// Actual release groups - should NOT be recognized
		{"rarbg", false},
		{"yify", false},
		{"sparks", false},
		{"flux", false},

		// Random strings
		{"xyzabc123", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := IsKnownMediaTitle(tt.title)
			if got != tt.want {
				t.Errorf("IsKnownMediaTitle(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}
