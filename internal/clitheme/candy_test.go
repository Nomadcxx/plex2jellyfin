package clitheme

import (
	"strings"
	"testing"
)

func TestCandyBarEndpoints(t *testing.T) {
	empty := CandyBar(0, 10)
	if !strings.Contains(empty, "oooooooooo") {
		t.Fatalf("queued bar should be all pellets: %q", empty)
	}
	full := CandyBar(1, 10)
	if !strings.Contains(full, "C") || !strings.Contains(full, "---------") {
		t.Fatalf("pacman should finish at right with trail: %q", full)
	}
	mid := CandyBar(0.5, 10)
	if !strings.Contains(mid, "C") || !strings.Contains(mid, "o") || !strings.Contains(mid, "-") {
		t.Fatalf("mid bar should have trail/pacman/pellets: %q", mid)
	}
}

func TestLibraryLabelDistinguishesSiblingMounts(t *testing.T) {
	a := LibraryLabel("/mnt/STORAGE1/TVSHOWS", 28)
	b := LibraryLabel("/mnt/STORAGE2/TVSHOWS", 28)
	if a != "STORAGE1/TVSHOWS" || b != "STORAGE2/TVSHOWS" {
		t.Fatalf("got %q / %q", a, b)
	}
	if a == b {
		t.Fatal("sibling mounts must not share the same label")
	}
}

func TestSoftCandyPctMonotonic(t *testing.T) {
	prev := SoftCandyPct(0)
	for n := 1; n < 500; n += 17 {
		got := SoftCandyPct(n)
		if got < prev {
			t.Fatalf("not monotonic at %d: %v < %v", n, got, prev)
		}
		if got >= 1 {
			t.Fatalf("soft pct must stay < 1 before done: %v", got)
		}
		prev = got
	}
}
