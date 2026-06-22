package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unreadableConfigHome(t *testing.T) string {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("unreadable config tests require a non-root user")
	}

	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "jellywatch")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[libraries]\ntv = [\"/media/tv\"]\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Chmod(cfgPath, 0); err != nil {
		t.Fatalf("chmod config unreadable: %v", err)
	}

	return home
}

func TestConfigRequiredCommandsFailOnUnreadableConfigFirst(t *testing.T) {
	home := unreadableConfigHome(t)
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")

	cases := []struct {
		name string
		args []string
	}{
		{name: "scan", args: []string{"scan", "--filesystem=false", "--stats=false"}},
		{name: "status", args: []string{"status"}},
		{name: "duplicates generate", args: []string{"duplicates", "generate"}},
		{name: "duplicates execute", args: []string{"duplicates", "execute"}},
		{name: "consolidate generate", args: []string{"consolidate", "generate"}},
		{name: "consolidate execute", args: []string{"consolidate", "execute"}},
		{name: "audit generate", args: []string{"audit", "--generate", "--limit", "1"}},
		{name: "cleanup cruft", args: []string{"cleanup", "cruft"}},
		{name: "cleanup empty", args: []string{"cleanup", "empty"}},
		{name: "watch", args: []string{"watch", "/tmp", "--dry-run"}},
		{name: "organize", args: []string{"organize", "/tmp/source.mkv"}},
		{name: "organize-folder", args: []string{"organize-folder", "/tmp/source"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetArgs(tc.args)
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected unreadable config error for %v", tc.args)
			}
			msg := err.Error()
			if !strings.Contains(msg, "failed to load config") || !strings.Contains(msg, "cannot read config file") {
				t.Fatalf("unexpected error for %v:\n%s", tc.args, msg)
			}
		})
	}
}
