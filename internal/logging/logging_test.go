package logging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
)

func TestResolveFilePathUsesLoggerDefaultAndExpandsHome(t *testing.T) {
	got, err := ResolveFilePath(Config{})
	if err != nil {
		t.Fatalf("ResolveFilePath default: %v", err)
	}
	base, err := paths.Plex2JellyfinDir()
	if err != nil {
		t.Fatalf("Plex2JellyfinDir: %v", err)
	}
	want := filepath.Join(base, "logs", "plex2jellyfin.log")
	if got != want {
		t.Fatalf("default path = %q, want %q", got, want)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	got, err = ResolveFilePath(Config{File: "~/custom/daemon.log"})
	if err != nil {
		t.Fatalf("ResolveFilePath custom: %v", err)
	}
	want = filepath.Join(home, "custom", "daemon.log")
	if got != want {
		t.Fatalf("custom path = %q, want %q", got, want)
	}
}
