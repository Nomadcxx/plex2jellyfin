package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/clitheme"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/Nomadcxx/plex2jellyfin/internal/scanner"
	setupdomain "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

// runSetupInitialScan indexes every configured library with per-library
// ILoveCandy bars, then chowns the config dir like the TUI installer.
func runSetupInitialScan(ctx context.Context, stdout io.Writer, draft setupdomain.Draft) error {
	tv := draft.Libraries.TV
	movies := draft.Libraries.Movies
	libs := append(append([]string{}, tv...), movies...)
	if len(libs) == 0 {
		clitheme.Muted(stdout, "No library paths configured — skipping initial scan.")
		return paths.ChownConfigDir(draft.Runtime.Permissions.User, draft.Runtime.Permissions.Group)
	}

	clitheme.Section(stdout, "Initial library scan")
	clitheme.Muted(stdout, "Indexing libraries (same step as the TUI installer). Pac-Man eats the dots.")

	dbPath, err := paths.DatabasePath()
	if err != nil {
		return err
	}
	db, err := database.OpenPath(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ui := &clitheme.LibraryCandyUI{Out: stdout, Libs: libs, Width: 22}
	fileScanner := scanner.NewFileScanner(db)
	result, scanErr := fileScanner.ScanWithOptions(ctx, scanner.ScanOptions{
		TVLibraries:    tv,
		MovieLibraries: movies,
		OnProgress: func(p scanner.ScanProgress) {
			ui.Render(p.LibrariesDone, p.FilesScanned, p.Library)
		},
	})

	// TUI order: chown even when the scan fails (DB artifacts already exist).
	if err := paths.ChownConfigDir(draft.Runtime.Permissions.User, draft.Runtime.Permissions.Group); err != nil {
		clitheme.Warn(stdout, fmt.Sprintf("could not fix config-dir ownership: %v", err))
		clitheme.Muted(stdout, "Fix with: sudo chown -R \"$USER:$USER\" ~/.config/plex2jellyfin")
	}

	if scanErr != nil {
		return scanErr
	}
	if result == nil {
		return nil
	}

	tvCount, _ := db.CountMediaFilesByType("episode")
	movieCount, _ := db.CountMediaFilesByType("movie")
	fmt.Fprintln(stdout)
	clitheme.OK(stdout, fmt.Sprintf("Scan finished in %s", result.Duration.Round(100*time.Millisecond)))
	clitheme.Muted(stdout, fmt.Sprintf("  files indexed: %d  (added %d, updated %d, skipped %d)",
		result.FilesScanned, result.FilesAdded, result.FilesUpdated, result.FilesSkipped))
	clitheme.Muted(stdout, fmt.Sprintf("  episode rows: %d   movie rows: %d", tvCount, movieCount))
	if len(result.Errors) > 0 {
		clitheme.Warn(stdout, fmt.Sprintf("%d path errors during indexing (see logs)", len(result.Errors)))
	}
	return nil
}
