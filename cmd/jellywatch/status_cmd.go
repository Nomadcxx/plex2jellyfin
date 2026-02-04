package main

import (
	"fmt"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show database status and statistics",
		Long: `Display HOLDEN database status, statistics, and health information.

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
	fmt.Println("HOLDEN Database Status")
	fmt.Println("======================")
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

	// Get duplicate breakdowns
	movieDups, _ := db.FindDuplicateMovies()
	episodeDups, _ := db.FindDuplicateEpisodes()

	fmt.Println("Duplicates (same content exists multiple times)")
	fmt.Println("------------------------------------------------")
	fmt.Printf("Movie duplicates:     %d groups\n", len(movieDups))
	fmt.Printf("Episode duplicates:   %d groups\n", len(episodeDups))
	if stats.SpaceReclaimable > 0 {
		fmt.Printf("Space reclaimable:    %s\n", formatBytes(stats.SpaceReclaimable))
	}
	if len(movieDups)+len(episodeDups) > 0 {
		fmt.Println("→ Run 'jellywatch duplicates generate' to review")
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

	fmt.Println("Scattered (same title across storage drives)")
	fmt.Println("--------------------------------------------")
	fmt.Printf("Series in multiple locations:  %d\n", seriesConflicts)
	fmt.Printf("Movies in multiple locations:  %d\n", movieConflicts)
	if seriesConflicts > 0 {
		fmt.Println("→ Run 'jellywatch consolidate generate' to review")
	}

	if len(conflicts) > 0 {
		fmt.Println("\nDetails:")
		shown := 0
		for _, c := range conflicts {
			if shown >= 5 {
				fmt.Printf("  ... and %d more\n", len(conflicts)-5)
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
