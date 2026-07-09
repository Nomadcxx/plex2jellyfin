package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
)

func TestDatabaseCleanupHousekeepingDryRunReportsCount(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	var out bytes.Buffer
	if err := runDatabaseCleanupHousekeepingWithDB(db, false, &out); err != nil {
		t.Fatalf("runDatabaseCleanupHousekeepingWithDB: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Duplicate manual-review housekeeping failures: 0") {
		t.Fatalf("unexpected dry-run output:\n%s", got)
	}
}

func TestDatabaseCommandRegistersCleanupHousekeeping(t *testing.T) {
	cmd := newDatabaseCmd()
	if !hasSubcommand("cleanup-housekeeping", subcommandNames(cmd)) {
		t.Fatalf("database command did not register cleanup-housekeeping")
	}
}
