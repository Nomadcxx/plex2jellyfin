package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected. The config commands print
// their report directly rather than through cobra's output writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// seedingOnlyConfigHome writes a config whose only TV intake is a seeding root.
func seedingOnlyConfigHome(t *testing.T) (home, seedDir, libDir string) {
	t.Helper()
	home = t.TempDir()
	seedDir = filepath.Join(home, "torrents", "complete", "tv")
	libDir = filepath.Join(home, "library", "tv")
	for _, dir := range []string{seedDir, libDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfgDir := filepath.Join(home, ".config", "plex2jellyfin")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`[watch]
movies = []
tv = []
seeding_movies = []
seeding_tv = ["%s"]

[libraries]
movies = []
tv = ["%s"]

[daemon]
enabled = false
scan_frequency = "5m"

[options]
delete_source = true
`, seedDir, libDir)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, seedDir, libDir
}

func runConfigSubcommand(t *testing.T, home string, args ...string) string {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("SUDO_USER", "")
	return captureStdout(t, func() {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		_ = cmd.Execute()
	})
}

func TestConfigShowListsSeedingWatchRoots(t *testing.T) {
	home, seedDir, _ := seedingOnlyConfigHome(t)
	out := runConfigSubcommand(t, home, "config", "show")

	if !strings.Contains(out, seedDir) {
		t.Fatalf("config show omits the seeding TV root %q:\n%s", seedDir, out)
	}
	if !strings.Contains(strings.ToLower(out), "seeding") {
		t.Fatalf("config show does not label seeding roots:\n%s", out)
	}
}

func TestConfigTestChecksSeedingRootsAndDoesNotWarnWhenOnlySeedingIsConfigured(t *testing.T) {
	home, seedDir, _ := seedingOnlyConfigHome(t)
	out := runConfigSubcommand(t, home, "config", "test")

	if strings.Contains(out, "No watch directories configured") {
		t.Fatalf("seeding-only intake must count as a watch directory:\n%s", out)
	}
	if !strings.Contains(out, seedDir) {
		t.Fatalf("config test never checked the seeding TV root %q:\n%s", seedDir, out)
	}
}
