package naming

import "testing"

func TestNormalizeMediaNameStopwordCasing(t *testing.T) {
	tests := []struct {
		name  string
		title string
		year  string
		want  string
	}{
		{"lowercase release", "masters of the universe", "2026", "Masters of the Universe (2026)"},
		{"lowercase article", "the lord of the rings", "2001", "The Lord of the Rings (2001)"},
		{"mixed case preserves lowercase stopword", "The Mandalorian and Grogu", "2026", "The Mandalorian and Grogu (2026)"},
		{"colon lost preserves uppercase article", "Kingsman The Secret Service", "2014", "Kingsman The Secret Service (2014)"},
		{"canonical casing stays intact", "Guardians of the Galaxy Vol 2", "2017", "Guardians of the Galaxy Vol 2 (2017)"},
		{"numeric prefix preserves article", "2001 A Space Odyssey", "1968", "2001 A Space Odyssey (1968)"},
		{"colon preserves article", "Star Wars: The Empire Strikes Back", "1980", "Star Wars: The Empire Strikes Back (1980)"},
		{"single word", "Us", "2019", "Us (2019)"},
		{"final word stays capitalized", "hell of a summer", "2023", "Hell of a Summer (2023)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeMediaName(tc.title, tc.year); got != tc.want {
				t.Fatalf("NormalizeMediaName(%q, %q) = %q, want %q", tc.title, tc.year, got, tc.want)
			}
		})
	}
}
