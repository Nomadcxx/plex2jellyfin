package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentDriftWarnings(t *testing.T) {
	dir := t.TempDir()
	build := filepath.Join(dir, "plex2jellyfin.new")
	live := filepath.Join(dir, "plex2jellyfin.live")

	if err := os.WriteFile(build, []byte("new build"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("old build"), 0644); err != nil {
		t.Fatal(err)
	}

	warnings := deploymentDriftWarnings([]deploymentBinary{{
		Name:      "plex2jellyfin",
		BuildPath: build,
		LivePath:  live,
	}})
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "differs from deployed") {
		t.Fatalf("warning %q does not describe deployment drift", warnings[0])
	}
}

func TestDeploymentDriftWarningsQuietForIdenticalOrMissingBuild(t *testing.T) {
	dir := t.TempDir()
	build := filepath.Join(dir, "plex2jellyfin.new")
	live := filepath.Join(dir, "plex2jellyfin.live")

	if err := os.WriteFile(build, []byte("same build"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("same build"), 0644); err != nil {
		t.Fatal(err)
	}

	warnings := deploymentDriftWarnings([]deploymentBinary{
		{Name: "plex2jellyfin", BuildPath: build, LivePath: live},
		{Name: "plex2jellyfin-daemon", BuildPath: filepath.Join(dir, "missing"), LivePath: live},
	})
	if len(warnings) != 0 {
		t.Fatalf("got warnings for identical/missing build artifacts: %v", warnings)
	}
}
