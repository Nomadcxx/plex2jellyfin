package setup

import (
	"strings"
	"testing"
)

func TestPathMappingsLines(t *testing.T) {
	lines := PathMappingsLines()
	if len(lines) < 5 {
		t.Fatalf("expected shared explanation lines, got %d", len(lines))
	}
	for i, line := range lines {
		if line == "" {
			t.Fatalf("line %d empty", i)
		}
	}
	if !strings.Contains(lines[len(lines)-1], DocsPathMappingsURL) {
		t.Fatalf("last line should cite docs URL, got %q", lines[len(lines)-1])
	}
}

func TestLibraryRoots(t *testing.T) {
	got := LibraryRoots([]string{"/m1", "/m2"}, []string{"/t1"})
	if len(got) != 3 || got[0] != "/m1" || got[2] != "/t1" {
		t.Fatalf("LibraryRoots = %#v", got)
	}
}
