package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show database status and statistics",
		Long: `Display Plex2Jellyfin database status, statistics, and health information.

Shows:
  - Database file location and size
  - Total files, series, and movies
  - Duplicate groups count
  - Unresolved conflicts
  - Last sync information
  - Database health metrics`,
		RunE: runStatus,
	}

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	// Get database path and size
	info, err := os.Stat(dbPath)
	if err != nil {
		return fmt.Errorf("failed to stat database: %w", err)
	}

	// Get statistics
	stats, err := db.GetConsolidationStats()
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	// Get conflicts
	conflicts, err := db.GetUnresolvedConflicts()
	if err != nil {
		return fmt.Errorf("failed to get conflicts: %w", err)
	}

	// Get sync history
	syncLogs, err := db.GetRecentSyncLogs(10) // Last 10 syncs
	if err != nil {
		return fmt.Errorf("failed to get sync history: %w", err)
	}

	// Get actual episode and movie counts
	episodeCount, err := db.CountMediaFilesByType("episode")
	if err != nil {
		episodeCount = 0
	}
	movieCount, err := db.CountMediaFilesByType("movie")
	if err != nil {
		movieCount = 0
	}

	// Display
	fmt.Println("Plex2Jellyfin Database Status")
	fmt.Println("==========================")
	fmt.Printf("Database: %s\n", dbPath)
	fmt.Printf("Size:    %s\n", formatBytes(info.Size()))
	fmt.Printf("Modified: %s\n\n", info.ModTime().Format("2006-01-02 15:04:05"))

	fmt.Println("Statistics")
	fmt.Println("----------")
	fmt.Printf("Total Files:          %d\n", stats.TotalFiles)
	fmt.Printf("TV Episodes:          %d\n", episodeCount)
	fmt.Printf("Movies:               %d\n", movieCount)
	fmt.Printf("Non-Compliant Files:  %d\n", stats.NonCompliantFiles)
	fmt.Println()

	printHousekeepingSummary(os.Stdout, db)
	printDeploymentDrift(os.Stdout)

	duplicateAnalysis, err := service.NewCleanupService(db).AnalyzeDuplicates()
	if err != nil {
		return fmt.Errorf("failed to analyze duplicates: %w", err)
	}
	safeMovieDups := 0
	safeEpisodeDups := 0
	skippedUnsafeMovieDups := 0
	spaceReclaimable := int64(0)
	for _, group := range duplicateAnalysis.Groups {
		if duplicateGroupLooksUnsafeTVMovie(group, cfg) {
			skippedUnsafeMovieDups++
			continue
		}
		switch group.MediaType {
		case "movie":
			safeMovieDups++
		default:
			safeEpisodeDups++
		}
		spaceReclaimable += group.ReclaimableBytes
	}

	fmt.Println("Duplicates (same content exists multiple times)")
	fmt.Println("------------------------------------------------")
	fmt.Printf("Movie duplicates:     %d groups\n", safeMovieDups)
	fmt.Printf("Episode duplicates:   %d groups\n", safeEpisodeDups)
	if skippedUnsafeMovieDups > 0 {
		fmt.Printf("Unsafe groups skipped: %d movie groups under TV library roots\n", skippedUnsafeMovieDups)
	}
	if spaceReclaimable > 0 {
		fmt.Printf("Space reclaimable:    %s\n", formatBytes(spaceReclaimable))
	}
	if safeMovieDups+safeEpisodeDups > 0 {
		fmt.Println("→ Run 'plex2jellyfin duplicates generate' to review")
	}
	fmt.Println()

	// Categorize conflicts
	seriesConflicts := 0
	movieConflicts := 0
	for _, c := range conflicts {
		if c.MediaType == "series" {
			seriesConflicts++
		} else {
			movieConflicts++
		}
	}

	fmt.Println("Scattered TV Series (same show across storage drives)")
	fmt.Println("-----------------------------------------------------")
	fmt.Printf("Series in multiple locations:  %d\n", seriesConflicts)
	if movieConflicts > 0 {
		fmt.Printf("Movies in multiple locations:   %d (tracked, not handled by consolidate)\n", movieConflicts)
	}
	if seriesConflicts > 0 {
		fmt.Println("→ Run 'plex2jellyfin consolidate generate' to review")
	}

	if seriesConflicts > 0 {
		fmt.Println("\nDetails:")
		shown := 0
		for _, c := range conflicts {
			if c.MediaType != "series" {
				continue
			}
			if shown >= 5 {
				fmt.Printf("  ... and %d more\n", seriesConflicts-5)
				break
			}
			yearStr := ""
			if c.Year != nil {
				yearStr = fmt.Sprintf(" (%d)", *c.Year)
			}
			fmt.Printf("  [%s] %s%s - %d locations\n", c.MediaType, c.Title, yearStr, len(c.Locations))
			shown++
		}
	}
	fmt.Println()

	fmt.Println("Sync History")
	fmt.Println("------------")
	if len(syncLogs) == 0 {
		fmt.Println("No sync history")
	} else {
		for _, log := range syncLogs {
			status := "✓"
			if log.Status == "failed" {
				status = "✗"
			}
			completedTime := "running"
			if log.CompletedAt != nil {
				completedTime = log.CompletedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%s %s - %s (%s)\n",
				status,
				log.Source,
				completedTime,
				log.Status)
		}
	}

	return nil
}

func printHousekeepingSummary(w io.Writer, db *database.MediaDB) {
	counts, err := db.CountHousekeepingTasks()
	if err != nil || len(counts) == 0 {
		return
	}
	duplicateManualReview, _ := db.CountDuplicateManualReviewFailures()

	fmt.Fprintln(w, "Housekeeping")
	fmt.Fprintln(w, "------------")
	fmt.Fprintf(w, "Pending:              %d\n", counts[database.TaskStatusPending])
	fmt.Fprintf(w, "Running:              %d\n", counts[database.TaskStatusRunning])
	fmt.Fprintf(w, "Flagged/manual review: %d\n", counts[database.TaskStatusFlagged])
	fmt.Fprintf(w, "Failed:               %d\n", counts[database.TaskStatusFailed])
	if duplicateManualReview > 0 {
		fmt.Fprintf(w, "Duplicate failures:   %d manual-review rows can be collapsed\n", duplicateManualReview)
		fmt.Fprintln(w, "→ Run 'plex2jellyfin database cleanup-housekeeping --execute'")
	}
	fmt.Fprintln(w)
}

type deploymentBinary struct {
	Name      string
	BuildPath string
	LivePath  string
}

func defaultDeploymentBinaries() []deploymentBinary {
	return []deploymentBinary{
		{Name: "plex2jellyfin", BuildPath: "/tmp/plex2jellyfin", LivePath: "/usr/local/bin/plex2jellyfin"},
		{Name: "plex2jellyfin-daemon", BuildPath: "/tmp/plex2jellyfin-daemon", LivePath: "/usr/local/bin/plex2jellyfin-daemon"},
		{Name: "plex2jellyfin-web", BuildPath: "/tmp/plex2jellyfin-web", LivePath: "/usr/local/bin/plex2jellyfin-web"},
	}
}

func deploymentDriftWarnings(binaries []deploymentBinary) []string {
	var warnings []string
	for _, bin := range binaries {
		if bin.BuildPath == "" || bin.LivePath == "" {
			continue
		}
		buildInfo, buildErr := os.Stat(bin.BuildPath)
		liveInfo, liveErr := os.Stat(bin.LivePath)
		if os.IsNotExist(buildErr) {
			continue
		}
		if buildErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: cannot read build artifact %s: %v", bin.Name, bin.BuildPath, buildErr))
			continue
		}
		if liveErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: cannot read deployed binary %s: %v", bin.Name, bin.LivePath, liveErr))
			continue
		}
		same, err := sameFileContent(bin.BuildPath, bin.LivePath, buildInfo, liveInfo)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: cannot compare %s to %s: %v", bin.Name, bin.BuildPath, bin.LivePath, err))
			continue
		}
		if !same {
			warnings = append(warnings, fmt.Sprintf("%s: %s differs from deployed %s", bin.Name, bin.BuildPath, bin.LivePath))
		}
	}
	return warnings
}

func printDeploymentDrift(w io.Writer) {
	warnings := deploymentDriftWarnings(defaultDeploymentBinaries())
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(w, "Deployment")
	fmt.Fprintln(w, "----------")
	fmt.Fprintln(w, "WARNING: rebuilt binaries are not deployed:")
	for _, warning := range warnings {
		fmt.Fprintf(w, "  - %s\n", warning)
	}
	fmt.Fprintln(w, "→ Run: sudo systemctl stop plex2jellyfin-daemon plex2jellyfin-web && sudo cp /tmp/plex2jellyfin /tmp/plex2jellyfin-daemon /tmp/plex2jellyfin-web /usr/local/bin/ && sudo systemctl start plex2jellyfin-daemon plex2jellyfin-web")
	fmt.Fprintln(w)
}

func sameFileContent(a, b string, aInfo, bInfo os.FileInfo) (bool, error) {
	if aInfo.Size() != bInfo.Size() {
		return false, nil
	}
	aHash, err := fileSHA256(a)
	if err != nil {
		return false, err
	}
	bHash, err := fileSHA256(b)
	if err != nil {
		return false, err
	}
	return aHash == bHash, nil
}

func fileSHA256(path string) ([sha256.Size]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [sha256.Size]byte{}, err
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}
