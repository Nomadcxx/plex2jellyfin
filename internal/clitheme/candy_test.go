package clitheme

import "testing"

func TestCandyBarEndpoints(t *testing.T) {
	empty := CandyBar(0, 10)
	if empty != "[oooooooooo]" {
		t.Fatalf("queued bar should be all pellets: %q", empty)
	}
	full := CandyBar(1, 10)
	if full[len(full)-2] != 'C' {
		t.Fatalf("pacman should finish at right: %q", full)
	}
	mid := CandyBar(0.5, 10)
	if !containsByte(mid, 'C') || !containsByte(mid, 'o') || !containsByte(mid, '-') {
		t.Fatalf("mid bar should have trail/pacman/pellets: %q", mid)
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

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}
