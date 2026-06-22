package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
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
		scanPath       string
		rebuildMovies  bool
		analyze        bool
		jsonOutput     bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan libraries and populate the JellyWatch database",
		Long: `Scan all configured libraries and populate the database.

This command scans your TV and movie libraries to build the JellyWatch database,
which enables instant lookups and conflict detection.

By default, scans the filesystem. Use flags to also sync from Sonarr/Radarr.

Examples:
  jellywatch scan                    # Scan filesystem only
  jellywatch scan --sonarr --radarr  # Also sync from Sonarr and Radarr
  jellywatch scan --analyze          # Scan, then analyze duplicates/scattered media
  jellywatch scan --json             # Emit machine-readable scan summary
  jellywatch scan --stats            # Show database stats after scan`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if rebuildMovies {
				return runRebuildMoviesFromFiles(showStats)
			}
			return runScan(syncSonarr, syncRadarr, syncFilesystem, showStats, scanPath, analyze, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&syncSonarr, "sonarr", false, "Also sync from Sonarr")
	cmd.Flags().BoolVar(&syncRadarr, "radarr", false, "Also sync from Radarr")
	cmd.Flags().BoolVar(&syncFilesystem, "filesystem", true, "Scan filesystem (default: true)")
	cmd.Flags().BoolVar(&showStats, "stats", true, "Show database stats after scan")
	cmd.Flags().StringVar(&scanPath, "path", "", "Scan a specific file or directory under a configured library root")
	cmd.Flags().BoolVar(&rebuildMovies, "rebuild-movies-from-files", false, "Rebuild movie rows from indexed movie media files")
	cmd.Flags().BoolVar(&analyze, "analyze", false, "After scanning, analyze duplicates and scattered media")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON summary")

	return cmd
}

func runRebuildMoviesFromFiles(showStats bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	dbPath := config.GetDatabasePath()
	db, err := database.OpenPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Database: %s\n\n", dbPath)
	rebuild := service.NewCleanupService(db)
	result, err := rebuild.RebuildMoviesFromMediaFiles(cfg.Libraries.Movies)
	if err != nil {
		return err
	}

	if showStats {
		fmt.Printf("Movie media files considered: %d\n", result.FilesConsidered)
		fmt.Printf("Movie rows rebuilt:          %d\n", result.MoviesRebuilt)
	}
	return nil
}

type scanCommandSummary struct {
	Database         string `json:"database"`
	DurationMS       int64  `json:"duration_ms"`
	FilesScanned     int    `json:"files_scanned"`
	FilesAdded       int    `json:"files_added"`
	FilesUpdated     int    `json:"files_updated"`
	FilesSkipped     int    `json:"files_skipped"`
	AITriggered      int    `json:"ai_triggered"`
	AISucceeded      int    `json:"ai_succeeded"`
	AIFailed         int    `json:"ai_failed"`
	NeedsReview      int    `json:"needs_review"`
	TVSeries         int    `json:"tv_series,omitempty"`
	Movies           int    `json:"movies,omitempty"`
	DuplicateGroups  int    `json:"duplicate_groups,omitempty"`
	ScatteredItems   int    `json:"scattered_items,omitempty"`
	ReclaimableBytes int64  `json:"reclaimable_bytes,omitempty"`
}

func runScan(syncSonarr, syncRadarr, syncFilesystem, showStats bool, scanPath string, analyze bool, jsonOutput bool) error {
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

	if !jsonOutput {
		fmt.Printf("Database: %s\n\n", dbPath)
	}

	// Setup logger
	logWriter := io.Writer(os.Stdout)
	if jsonOutput {
		logWriter = io.Discard
	}
	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create sync service
	var sonarrClient *sonarr.Client
	var radarrClient *radarr.Client
	var aiHelper *scanner.AIHelper

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

	// Create AI helper if enabled
	if cfg.AI.Enabled {
		matcher, err := ai.NewMatcher(cfg.AI)
		if err != nil {
			if !jsonOutput {
				fmt.Printf("  Warning: AI matcher initialization failed: %v\n", err)
				fmt.Println("  Continuing without AI auto-trigger")
			}
		} else {
			aiHelper = scanner.NewAIHelper(cfg.AI, db.DB(), matcher)
		}
	}

	syncService := sync.NewSyncService(sync.SyncConfig{
		DB:             db,
		Sonarr:         sonarrClient,
		Radarr:         radarrClient,
		AIHelper:       aiHelper,
		TVLibraries:    cfg.Libraries.TV,
		MovieLibraries: cfg.Libraries.Movies,
		Logger:         logger,
	})

	ctx := context.Background()
	startTime := time.Now()

	if scanPath != "" {
		return runTargetedScan(ctx, db, cfg, aiHelper, scanPath, showStats, startTime, jsonOutput, dbPath)
	}

	// Sync from Sonarr first (lower priority, will be overwritten by filesystem)
	if syncSonarr && cfg.Sonarr.Enabled {
		if !jsonOutput {
			fmt.Println("Syncing from Sonarr...")
		}
		if err := syncService.SyncFromSonarr(ctx); err != nil {
			if !jsonOutput {
				fmt.Printf("  Warning: Sonarr sync failed: %v\n", err)
			}
		} else {
			if !jsonOutput {
				fmt.Println("  Sonarr sync complete")
			}
		}
	}

	// Sync from Radarr
	if syncRadarr && cfg.Radarr.Enabled {
		if !jsonOutput {
			fmt.Println("Syncing from Radarr...")
		}
		if err := syncService.SyncFromRadarr(ctx); err != nil {
			if !jsonOutput {
				fmt.Printf("  Warning: Radarr sync failed: %v\n", err)
			}
		} else {
			if !jsonOutput {
				fmt.Println("  Radarr sync complete")
			}
		}
	}

	// Scan filesystem (higher priority)
	var scanResult *scanner.ScanResult
	if syncFilesystem {
		if !jsonOutput {
			fmt.Printf("Scanning %d TV libraries...\n", len(cfg.Libraries.TV))
			for _, lib := range cfg.Libraries.TV {
				fmt.Printf("  %s\n", lib)
			}
			fmt.Printf("Scanning %d movie libraries...\n", len(cfg.Libraries.Movies))
			for _, lib := range cfg.Libraries.Movies {
				fmt.Printf("  %s\n", lib)
			}
			fmt.Println()
		}

		var err error
		scanResult, err = syncService.SyncFromFilesystem(ctx)
		if err != nil {
			return fmt.Errorf("filesystem sync failed: %w", err)
		}
		if !jsonOutput {
			fmt.Println("Filesystem sync complete")
		}
		if pruned, err := pruneMissingMediaRows(db); err != nil {
			return fmt.Errorf("pruning missing media file rows: %w", err)
		} else if pruned > 0 && !jsonOutput {
			fmt.Printf("Pruned missing media file rows: %d\n", pruned)
		}
		if pruned, err := pruneStaleFilesystemMovieRows(db, ""); err != nil {
			return fmt.Errorf("pruning stale filesystem movie rows: %w", err)
		} else if pruned > 0 && !jsonOutput {
			fmt.Printf("Pruned stale filesystem movie rows: %d\n", pruned)
		}
	}

	duration := time.Since(startTime)
	if !jsonOutput {
		fmt.Printf("\nScan completed in %s\n", duration.Round(time.Millisecond))
	}

	summary := scanCommandSummary{
		Database:   dbPath,
		DurationMS: duration.Milliseconds(),
	}

	// Display scan statistics
	if scanResult != nil {
		summary.FilesScanned = scanResult.FilesScanned
		summary.FilesAdded = scanResult.FilesAdded
		summary.FilesUpdated = scanResult.FilesUpdated
		summary.FilesSkipped = scanResult.FilesSkipped
		summary.AITriggered = scanResult.AITriggered
		summary.AISucceeded = scanResult.AISucceeded
		summary.AIFailed = scanResult.AIFailed
		summary.NeedsReview = scanResult.NeedsReview
		if !jsonOutput {
			fmt.Printf("\n=== Scan Statistics ===\n")
			fmt.Printf("Files scanned: %d\n", scanResult.FilesScanned)
			fmt.Printf("Files added:   %d\n", scanResult.FilesAdded)
			fmt.Printf("Files updated: %d\n", scanResult.FilesUpdated)
		}

		// Show AI statistics if AI was used
		if !jsonOutput && aiHelper != nil && (scanResult.AITriggered > 0 || scanResult.NeedsReview > 0) {
			fmt.Printf("\n=== AI Summary ===\n")
			fmt.Printf("Triggered:   %d\n", scanResult.AITriggered)
			fmt.Printf("Cache hits:  %d\n", scanResult.AICacheHits)
			fmt.Printf("Improved:    %d\n", scanResult.AISucceeded)
			fmt.Printf("Failed:      %d\n", scanResult.AIFailed)
			if scanResult.NeedsReview > 0 {
				fmt.Printf("Needs review: %d (run 'jellywatch audit --generate' to review)\n", scanResult.NeedsReview)
			}
		}
	}

	// Show stats
	if showStats {
		if !jsonOutput {
			fmt.Println("\n=== Database Stats ===")
		}

		// Count series
		var seriesCount int
		for _, lib := range cfg.Libraries.TV {
			count, _ := db.CountSeriesInLibrary(lib)
			seriesCount += count
		}
		summary.TVSeries = seriesCount
		if !jsonOutput {
			fmt.Printf("TV Series: %d\n", seriesCount)
		}

		// Count movies
		var movieCount int
		for _, lib := range cfg.Libraries.Movies {
			count, _ := db.CountMoviesInLibrary(lib)
			movieCount += count
		}
		summary.Movies = movieCount
		if !jsonOutput {
			fmt.Printf("Movies: %d\n", movieCount)
		}

		if shouldRunPostScanAnalysis(showStats, analyze) {
			svc := service.NewCleanupService(db)

			dupAnalysis, err := svc.AnalyzeDuplicates()
			hasDuplicates := err == nil && dupAnalysis.TotalGroups > 0
			if err == nil {
				summary.DuplicateGroups = dupAnalysis.TotalGroups
				summary.ReclaimableBytes = dupAnalysis.ReclaimableBytes
			}

			scatterAnalysis, err := svc.AnalyzeScattered()
			hasScattered := err == nil && scatterAnalysis.TotalItems > 0
			if err == nil {
				summary.ScatteredItems = scatterAnalysis.TotalItems
			}

			if !jsonOutput {
				renderPostScanAnalysis(dupAnalysis, scatterAnalysis, hasDuplicates, hasScattered)
			}
		} else if !jsonOutput {
			fmt.Println("\nPost-scan duplicate/scattered analysis skipped. Run 'jellywatch scan --analyze' or targeted commands when needed.")
		}
	}

	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(summary)
	}
	return nil
}

func shouldRunPostScanAnalysis(showStats, analyze bool) bool {
	return showStats && analyze
}

func renderPostScanAnalysis(dupAnalysis *service.DuplicateAnalysis, scatterAnalysis *service.ScatteredAnalysis, hasDuplicates, hasScattered bool) {
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
			fmt.Println("   jellywatch duplicates generate   # Generate deletion plans")
			fmt.Println("   jellywatch duplicates dry-run    # Preview plans")
			fmt.Println("   jellywatch duplicates execute    # Execute plans")
		}

		if hasScattered {
			step := "1"
			if hasDuplicates {
				step = "2"
			}
			fmt.Printf("\n%s. Then consolidate scattered media:\n", step)
			fmt.Println("   jellywatch consolidate generate  # Generate move plans")
			fmt.Println("   jellywatch consolidate dry-run   # Preview plans")
			fmt.Println("   jellywatch consolidate execute   # Execute plans")
		}

		fmt.Println("\n3. Review files needing classification:")
		fmt.Println("   jellywatch audit --generate      # Generate audit report")

		fmt.Println("\nOr use the interactive wizard:")
		fmt.Println("   jellywatch fix                   # Guided cleanup")
	} else {
		fmt.Println("\n✨ No issues detected - your library is clean!")
	}
}

func runTargetedScan(ctx context.Context, db *database.MediaDB, cfg *config.Config, aiHelper *scanner.AIHelper, path string, showStats bool, startTime time.Time, jsonOutput bool, dbPath string) error {
	libraryRoot, mediaType, err := configuredLibraryForPath(path, cfg)
	if err != nil {
		return err
	}

	fileScanner := scanner.NewFileScanner(db)
	if aiHelper != nil {
		fileScanner = scanner.NewFileScannerWithAI(db, aiHelper)
	}

	if !jsonOutput {
		fmt.Printf("Scanning %s path:\n  %s\nLibrary root:\n  %s\n\n", mediaType, path, libraryRoot)
	}
	scanResult, err := fileScanner.ScanPath(ctx, path, libraryRoot, mediaType)
	if err != nil {
		return fmt.Errorf("targeted filesystem sync failed: %w", err)
	}
	if !jsonOutput {
		fmt.Println("Targeted filesystem sync complete")
	}
	if pruned, err := pruneStaleFilesystemMovieRows(db, path); err != nil {
		return fmt.Errorf("pruning stale filesystem movie rows: %w", err)
	} else if pruned > 0 && !jsonOutput {
		fmt.Printf("Pruned stale filesystem movie rows: %d\n", pruned)
	}

	duration := time.Since(startTime)
	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(scanCommandSummary{
			Database:     dbPath,
			DurationMS:   duration.Milliseconds(),
			FilesScanned: scanResult.FilesScanned,
			FilesAdded:   scanResult.FilesAdded,
			FilesUpdated: scanResult.FilesUpdated,
			FilesSkipped: scanResult.FilesSkipped,
			AITriggered:  scanResult.AITriggered,
			AISucceeded:  scanResult.AISucceeded,
			AIFailed:     scanResult.AIFailed,
			NeedsReview:  scanResult.NeedsReview,
		})
	}
	fmt.Printf("\nScan completed in %s\n", duration.Round(time.Millisecond))
	if showStats {
		fmt.Printf("\n=== Scan Statistics ===\n")
		fmt.Printf("Files scanned: %d\n", scanResult.FilesScanned)
		fmt.Printf("Files added:   %d\n", scanResult.FilesAdded)
		fmt.Printf("Files updated: %d\n", scanResult.FilesUpdated)
		fmt.Printf("Files skipped: %d\n", scanResult.FilesSkipped)
	}

	return nil
}

func pruneStaleFilesystemMovieRows(db *database.MediaDB, path string) (int64, error) {
	if strings.TrimSpace(path) != "" {
		return db.PruneFilesystemMoviesWithoutMediaFilesUnder(path)
	}
	return db.PruneFilesystemMoviesWithoutMediaFiles()
}

func pruneMissingMediaRows(db *database.MediaDB) (int, error) {
	result, err := service.NewCleanupService(db).PruneMissingMediaFiles()
	if err != nil {
		return 0, err
	}
	return result.Pruned, nil
}

func configuredLibraryForPath(path string, cfg *config.Config) (libraryRoot string, mediaType string, err error) {
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolving scan path: %w", err)
	}

	for _, root := range cfg.Libraries.TV {
		if pathIsUnderRoot(cleanPath, root) {
			return root, "episode", nil
		}
	}
	for _, root := range cfg.Libraries.Movies {
		if pathIsUnderRoot(cleanPath, root) {
			return root, "movie", nil
		}
	}

	return "", "", fmt.Errorf("scan path is not under any configured library root: %s", path)
}

func pathIsUnderRoot(path, root string) bool {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
