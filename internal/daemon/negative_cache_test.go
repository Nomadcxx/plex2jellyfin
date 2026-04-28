package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNegativeCache_RecordAndDefer(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Show.S01.mkv")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	c := NewNegativeCache()
	if d, _, _ := c.IsDeferred(p); d {
		t.Fatal("expected fresh path not deferred")
	}

	c.Record(p, "could not extract TV show info from path")
	deferred, remaining, msg := c.IsDeferred(p)
	if !deferred {
		t.Fatal("expected path deferred after Record")
	}
	if remaining <= 0 || remaining > 30*time.Minute {
		t.Fatalf("unexpected remaining: %v", remaining)
	}
	if msg == "" {
		t.Fatal("expected error message preserved")
	}

	c.Forget(p)
	if d, _, _ := c.IsDeferred(p); d {
		t.Fatal("expected forgotten path not deferred")
	}
}

func TestNegativeCache_EvictsMissingSource(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing.mkv")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	c := NewNegativeCache()
	c.Record(p, "obfuscated filename, no episode markers")
	if d, _, _ := c.IsDeferred(p); !d {
		t.Fatal("expected deferred while file present")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if d, _, _ := c.IsDeferred(p); d {
		t.Fatal("expected eviction once source disappears")
	}
}

func TestNegativeCache_BackoffEscalates(t *testing.T) {
	if backoffSchedule(1) != 30*time.Minute {
		t.Fatalf("first failure should defer 30m, got %v", backoffSchedule(1))
	}
	if backoffSchedule(2) != 2*time.Hour {
		t.Fatalf("second failure should defer 2h, got %v", backoffSchedule(2))
	}
	if backoffSchedule(4) != 24*time.Hour {
		t.Fatalf("4th+ failure should defer 24h, got %v", backoffSchedule(4))
	}
}

func TestIsDeterministicUnparseable(t *testing.T) {
	cases := map[string]bool{
		"could not extract TV show info from path: /foo/bar":                               true,
		"transfer failed: all backends failed: rsync: timed out":                           false,
		"unable to parse TV show name: could not extract TV show info from path (obfuscated filename, no episode markers in parent folders)": true,
		"":                            false,
		"open /foo: no such file":     false,
		"unable to parse movie name":  true,
	}
	for msg, want := range cases {
		got := IsDeterministicUnparseable(msg)
		if got != want {
			t.Errorf("IsDeterministicUnparseable(%q) = %v, want %v", msg, got, want)
		}
	}
}
