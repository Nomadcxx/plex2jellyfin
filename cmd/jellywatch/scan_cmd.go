package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/sync"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var (
		syncSonarr     bool
		syncRadarr     bool
		syncFilesystem bool
		showStats      bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan libraries and populate the HOLDEN database",
		Long: `Scan all configured libraries and populate the database.

This command scans your TV and movie libraries to build the HOLDEN database,
which enables instant lookups and conflict detection.

By default, scans the filesystem. Use flags to also sync from Sonarr/Radarr.

Examples:
  jellywatch scan                    # Scan filesystem only
  jellywatch scan --sonarr --radarr  # Also sync from Sonarr and Radarr
  jellywatch scan --stats            # Show database stats after scan`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(syncSonarr, syncRadarr, syncFilesystem, showStats)
		},
	}

	cmd.Flags().BoolVar(&syncSonarr, "sonarr", false, "Also sync from Sonarr")
	cmd.Flags().BoolVar(&syncRadarr, "radarr", false, "Also sync from Radarr")
	cmd.Flags().BoolVar(&syncFilesystem, "filesystem", true, "Scan filesystem (default: true)")
	cmd.Flags().BoolVar(&showStats, "stats", true, "Show database stats after scan")

	return cmd
}

func runScan(syncSonarr, syncRadarr, syncFilesystem, showStats bool) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Open database
	dbPath := config.GetDatabasePath()
	db, err := database.OpenPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Database: %s\n\n", dbPath)

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create sync service
	var sonarrClient *sonarr.Client
	var radarrClient *radarr.Client

	if syncSonarr && cfg.Sonarr.Enabled {
		sonarrClient = sonarr.NewClient(sonarr.Config{
			URL:    cfg.Sonarr.URL,
			APIKey: cfg.Sonarr.APIKey,
		})
	}

	if syncRadarr && cfg.Radarr.Enabled {
		radarrClient = radarr.NewClient(radarr.Config{
			URL:    cfg.Radarr.URL,
			APIKey: cfg.Radarr.APIKey,
		})
	}

	syncService := sync.NewSyncService(sync.SyncConfig{
		DB:             db,
		Sonarr:         sonarrClient,
		Radarr:         radarrClient,
		TVLibraries:    cfg.Libraries.TV,
		MovieLibraries: cfg.Libraries.Movies,
		Logger:         logger,
	})

	ctx := context.Background()
	startTime := time.Now()

	// Sync from Sonarr first (lower priority, will be overwritten by filesystem)
	if syncSonarr && cfg.Sonarr.Enabled {
		fmt.Println("Syncing from Sonarr...")
		if err := syncService.SyncFromSonarr(ctx); err != nil {
			fmt.Printf("  Warning: Sonarr sync failed: %v\n", err)
		} else {
			fmt.Println("  Sonarr sync complete")
		}
	}

	// Sync from Radarr
	if syncRadarr && cfg.Radarr.Enabled {
		fmt.Println("Syncing from Radarr...")
		if err := syncService.SyncFromRadarr(ctx); err != nil {
			fmt.Printf("  Warning: Radarr sync failed: %v\n", err)
		} else {
			fmt.Println("  Radarr sync complete")
		}
	}

	// Scan filesystem (higher priority)
	if syncFilesystem {
		fmt.Printf("Scanning %d TV libraries...\n", len(cfg.Libraries.TV))
		for _, lib := range cfg.Libraries.TV {
			fmt.Printf("  %s\n", lib)
		}
		fmt.Printf("Scanning %d movie libraries...\n", len(cfg.Libraries.Movies))
		for _, lib := range cfg.Libraries.Movies {
			fmt.Printf("  %s\n", lib)
		}
		fmt.Println()

		if err := syncService.SyncFromFilesystem(ctx); err != nil {
			return fmt.Errorf("filesystem sync failed: %w", err)
		}
		fmt.Println("Filesystem sync complete")
	}

	duration := time.Since(startTime)
	fmt.Printf("\nScan completed in %s\n", duration.Round(time.Millisecond))

	// Show stats
	if showStats {
		fmt.Println("\n=== Database Stats ===")

		// Count series
		var seriesCount int
		for _, lib := range cfg.Libraries.TV {
			count, _ := db.CountSeriesInLibrary(lib)
			seriesCount += count
		}
		fmt.Printf("TV Series: %d\n", seriesCount)

		// Count movies
		var movieCount int
		for _, lib := range cfg.Libraries.Movies {
			count, _ := db.CountMoviesInLibrary(lib)
			movieCount += count
		}
		fmt.Printf("Movies: %d\n", movieCount)

		svc := service.NewCleanupService(db)

		dupAnalysis, err := svc.AnalyzeDuplicates()
		hasDuplicates := err == nil && dupAnalysis.TotalGroups > 0

		scatterAnalysis, err := svc.AnalyzeScattered()
		hasScattered := err == nil && scatterAnalysis.TotalItems > 0

		if hasDuplicates || hasScattered {
			fmt.Println("\n=== Issues Found ===")
		}

		if hasDuplicates {
			fmt.Printf("\n📁 DUPLICATES (same content, different quality): %d groups\n", dupAnalysis.TotalGroups)
			fmt.Printf("   → These have inferior copies that can be DELETED to save %s\n", formatBytes(dupAnalysis.ReclaimableBytes))
		}

		if hasScattered {
			fmt.Printf("\n🔀 SCATTERED MEDIA (same title in multiple locations): %d items\n", scatterAnalysis.TotalItems)
			fmt.Println("   → These need files MOVED to consolidate into one folder")

			for _, item := range scatterAnalysis.Items {
				yearStr := ""
				if item.Year != nil {
					yearStr = fmt.Sprintf(" (%d)", *item.Year)
				}
				fmt.Printf("  [%s] %s%s\n", item.MediaType, item.Title, yearStr)
				for _, loc := range item.Locations {
					fmt.Printf("    - %s\n", loc)
				}
			}
		}

		if hasDuplicates || hasScattered {
			fmt.Println("\n=== What's Next ===")

			if hasDuplicates {
				fmt.Println("\n1. Handle duplicates first (recommended):")
				fmt.Println("   jellywatch fix              # Interactive wizard (handles all)")
				fmt.Println("   jellywatch duplicates       # Review duplicates only")
			}

			if hasScattered {
				step := "1"
				if hasDuplicates {
					step = "2"
				}
				fmt.Printf("\n%s. Then consolidate scattered media:\n", step)
				fmt.Println("   jellywatch consolidate      # Review and organize")
			}
		} else {
			fmt.Println("\n✨ No issues detected - your library is clean!")
		}
	}

	return nil
}
