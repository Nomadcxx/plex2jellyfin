package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/migration"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate path mismatches between database and Sonarr/Radarr",
		Long: `Detect and fix path mismatches between Plex2Jellyfin database and Sonarr/Radarr.

This tool helps users upgrading to the source-of-truth architecture by finding
instances where the database path differs from Sonarr/Radarr paths, and allows
you to choose which path to keep.

Examples:
  plex2jellyfin migrate        # Interactive mode
  plex2jellyfin migrate --dry-run`,
		RunE: runMigrate,
	}

	return cmd
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	var sonarrClient *sonarr.Client
	var radarrClient *radarr.Client

	if cfg.Sonarr.Enabled {
		sonarrClient = sonarr.NewClient(sonarr.Config{
			URL:     cfg.Sonarr.URL,
			APIKey:  cfg.Sonarr.APIKey,
			Timeout: 30 * time.Second,
		})
	}

	if cfg.Radarr.Enabled {
		radarrClient = radarr.NewClient(radarr.Config{
			URL:     cfg.Radarr.URL,
			APIKey:  cfg.Radarr.APIKey,
			Timeout: 30 * time.Second,
		})
	}

	fmt.Println("🔍 Scanning for path mismatches...")

	var allMismatches []migration.PathMismatch

	if sonarrClient != nil {
		seriesMismatches, err := migration.DetectSeriesMismatches(db, sonarrClient)
		if err != nil {
			return fmt.Errorf("detecting series mismatches: %w", err)
		}
		allMismatches = append(allMismatches, seriesMismatches...)
		fmt.Printf("  Found %d series mismatches\n", len(seriesMismatches))
	} else {
		fmt.Println("  Sonarr not enabled, skipping series check")
	}

	if radarrClient != nil {
		movieMismatches, err := migration.DetectMovieMismatches(db, radarrClient)
		if err != nil {
			return fmt.Errorf("detecting movie mismatches: %w", err)
		}
		allMismatches = append(allMismatches, movieMismatches...)
		fmt.Printf("  Found %d movie mismatches\n", len(movieMismatches))
	} else {
		fmt.Println("  Radarr not enabled, skipping movie check")
	}

	if len(allMismatches) == 0 {
		fmt.Println("\n✅ No path mismatches found! Database and Sonarr/Radarr are in sync.")
		return nil
	}

	fmt.Printf("\n📋 Found %d total path mismatches\n\n", len(allMismatches))

	if dryRun {
		printMismatchesDryRun(allMismatches)
		return nil
	}

	return runInteractiveMigration(db, sonarrClient, radarrClient, allMismatches)
}

func printMismatchesDryRun(mismatches []migration.PathMismatch) {
	for i, m := range mismatches {
		fmt.Printf("[%d/%d] %s: %s (%d)\n", i+1, len(mismatches), m.MediaType, m.Title, m.Year)
		fmt.Printf("  Database: %s\n", m.DatabasePath)
		if m.SonarrPath != "" {
			fmt.Printf("  Sonarr:   %s\n", m.SonarrPath)
		}
		if m.RadarrPath != "" {
			fmt.Printf("  Radarr:   %s\n", m.RadarrPath)
		}
		fmt.Println()
	}
}

func runInteractiveMigration(db *database.MediaDB, sonarrClient *sonarr.Client, radarrClient *radarr.Client, mismatches []migration.PathMismatch) error {
	reader := bufio.NewReader(os.Stdin)

	var fixed, skipped, failed int

	for i, m := range mismatches {
		fmt.Printf("\n[%d/%d] %s: %s (%d)\n", i+1, len(mismatches), strings.ToUpper(m.MediaType), m.Title, m.Year)
		fmt.Printf("  Database: %s\n", m.DatabasePath)
		if m.SonarrPath != "" {
			fmt.Printf("  Sonarr:   %s\n", m.SonarrPath)
		}
		if m.RadarrPath != "" {
			fmt.Printf("  Radarr:   %s\n", m.RadarrPath)
		}
		fmt.Println()

		fmt.Println("Choose action:")
		fmt.Println("  [j] Keep Plex2Jellyfin path (update Sonarr/Radarr) - Recommended")
		fmt.Println("  [a] Keep Sonarr/Radarr path (update database)")
		fmt.Println("  [s] Skip this mismatch")
		fmt.Println("  [q] Quit migration")
		fmt.Print("\nChoice [j/a/s/q]: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}

		choice := strings.TrimSpace(strings.ToLower(input))

		switch choice {
		case "j", "plex2jellyfin":
			err := fixMismatch(db, sonarrClient, radarrClient, m, migration.FixChoiceKeepPlex2Jellyfin)
			if err != nil {
				fmt.Printf("  ✗ Error: %v\n", err)
				failed++
			} else {
				fmt.Println("  ✓ Updated Sonarr/Radarr to match database")
				fixed++
			}

		case "a", "arr":
			err := fixMismatch(db, sonarrClient, radarrClient, m, migration.FixChoiceKeepSonarrRadarr)
			if err != nil {
				fmt.Printf("  ✗ Error: %v\n", err)
				failed++
			} else {
				fmt.Println("  ✓ Updated database to match Sonarr/Radarr")
				fixed++
			}

		case "s", "skip":
			fmt.Println("  ⊘ Skipped")
			skipped++

		case "q", "quit":
			fmt.Println("\nMigration stopped by user")
			printSummary(fixed, skipped, failed)
			return nil

		default:
			fmt.Println("  Invalid choice, skipping...")
			skipped++
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	printSummary(fixed, skipped, failed)

	if failed > 0 {
		return fmt.Errorf("%d mismatches failed to fix", failed)
	}

	return nil
}

func fixMismatch(db *database.MediaDB, sonarrClient *sonarr.Client, radarrClient *radarr.Client, mismatch migration.PathMismatch, choice migration.FixChoice) error {
	if mismatch.MediaType == "series" {
		return migration.FixSeriesMismatch(db, sonarrClient, mismatch, choice)
	}
	return migration.FixMovieMismatch(db, radarrClient, mismatch, choice)
}

func printSummary(fixed, skipped, failed int) {
	fmt.Println("\n📊 Migration Summary:")
	fmt.Printf("  ✓ Fixed:   %d\n", fixed)
	fmt.Printf("  ⊘ Skipped: %d\n", skipped)
	if failed > 0 {
		fmt.Printf("  ✗ Failed:  %d\n", failed)
	}
}
