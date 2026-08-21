package jellyfin

import "testing"

func TestNormalizeItemID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"dashed guid from plugin", "4bceeac3-77fe-a6fa-39e9-7850c6b55e35", "4bceeac377fea6fa39e97850c6b55e35"},
		{"compact guid from rest api", "4bceeac377fea6fa39e97850c6b55e35", "4bceeac377fea6fa39e97850c6b55e35"},
		{"uppercase is folded", "4BCEEAC3-77FE-A6FA-39E9-7850C6B55E35", "4bceeac377fea6fa39e97850c6b55e35"},
		{"surrounding whitespace", "  4bceeac377fea6fa39e97850c6b55e35  ", "4bceeac377fea6fa39e97850c6b55e35"},
		{"empty stays empty", "", ""},
		{"non-guid passes through", "jf-123", "jf-123"},
		{"wrong length passes through", "4bceeac377fe", "4bceeac377fe"},
		{"non-hex guid shape passes through", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeItemID(tc.in); got != tc.want {
				t.Fatalf("NormalizeItemID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
