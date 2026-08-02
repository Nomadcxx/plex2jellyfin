package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/postmortem"
)

func TestRootRegistersHiddenPostmortemCommand(t *testing.T) {
	cmd := newRootCmd()
	names := subcommandNames(cmd)
	hidden := hiddenSubcommandMap(cmd)

	if !hasSubcommand("postmortem", names) {
		t.Fatalf("postmortem command not registered")
	}
	if !hidden["postmortem"] {
		t.Fatalf("postmortem should be hidden from normal help")
	}
}

func TestCollectionDueSkipsYoungBundle(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	stamp := now.Add(-24 * time.Hour)
	writeSuccessfulBundle(t, root, stamp)

	due, err := collectionDue(root, now)
	if err != nil {
		t.Fatalf("collectionDue: %v", err)
	}
	if due {
		t.Fatal("expected not due when latest successful bundle is younger than 96h")
	}
}

func TestCollectionDueWhenBundleOlderThanCadence(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	stamp := now.Add(-96 * time.Hour)
	writeSuccessfulBundle(t, root, stamp)

	due, err := collectionDue(root, now)
	if err != nil {
		t.Fatalf("collectionDue: %v", err)
	}
	if !due {
		t.Fatal("expected due when latest successful bundle is at least 96h old")
	}
}

func TestCollectionDueWhenNoBundle(t *testing.T) {
	due, err := collectionDue(t.TempDir(), time.Now().UTC())
	if err != nil {
		t.Fatalf("collectionDue: %v", err)
	}
	if !due {
		t.Fatal("expected due when no successful bundle exists")
	}
}

func TestCollectIfDueSkipsYoungBundleWithoutCollecting(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	stamp := now.Add(-12 * time.Hour)
	writeSuccessfulBundle(t, root, stamp)
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	cmd := newPostmortemCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"collect", "--root", root, "--if-due"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nout=%s", err, buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("skipped")) {
		t.Fatalf("expected skip message, got %q", buf.String())
	}

	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("if-due skip created new entries: before=%d after=%d", len(before), len(after))
	}
}

func writeSuccessfulBundle(t *testing.T, root string, stamp time.Time) {
	t.Helper()
	runID := postmortem.RunID(stamp)
	bundle := postmortem.NewBundlePaths(root, runID)
	if err := os.MkdirAll(bundle.Dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	summary := []byte(`{"run_id":"` + runID + `","generated_at":"` + stamp.UTC().Format(time.RFC3339Nano) + `"}`)
	if err := os.WriteFile(bundle.File("summary.json"), summary, 0o644); err != nil {
		t.Fatalf("WriteFile summary: %v", err)
	}
	if err := os.Symlink(bundle.Dir, filepath.Join(root, "latest")); err != nil {
		t.Fatalf("Symlink latest: %v", err)
	}
}
