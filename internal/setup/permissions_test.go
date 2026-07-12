package setup

import "testing"

func TestDefaultPermissions(t *testing.T) {
	p := DefaultPermissions()
	if p.FileMode != "0664" {
		t.Fatalf("FileMode = %q, want 0664", p.FileMode)
	}
	if p.DirMode != "0775" {
		t.Fatalf("DirMode = %q, want 0775", p.DirMode)
	}
	if p.Group == "" {
		t.Fatal("Group should default to a shared group name")
	}
}
