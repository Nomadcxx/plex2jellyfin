package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newDuplicatesCmd() *cobra.Command {
	var (
		moviesOnly bool
		tvOnly     bool
		showFilter string
		execute    bool
	)

	cmd := &cobra.Command{
		Use:   "duplicates [flags]",
		Short: "Find and remove duplicate media files",
		Long: `Find and optionally remove duplicate media files from your library.

IMPORTANT: This command DELETES duplicate files, not moves them.
- Use 'consolidate' to organize scattered episodes into proper folders
- Use 'duplicates --execute' to delete inferior quality duplicates

Duplicates are files with the same normalized title, year, and episode (for TV shows)
but different quality scores. The CONDOR system identifies which file should be kept
based on quality scoring (Resolution > Source > Size).

Examples:
  jellywatch duplicates              # List all duplicates
  jellywatch duplicates --movies     # Only movies
  jellywatch duplicates --tv         # Only TV episodes
  jellywatch duplicates --show=Silo  # Duplicates for specific show
  jellywatch duplicates --execute    # DELETE inferior duplicates (with confirmation)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDuplicates(moviesOnly, tvOnly, showFilter, execute)
		},
	}

	cmd.Flags().BoolVar(&moviesOnly, "movies", false, "Show only movie duplicates")
	cmd.Flags().BoolVar(&tvOnly, "tv", false, "Show only TV episode duplicates")
	cmd.Flags().StringVar(&showFilter, "show", "", "Filter by show name")
	cmd.Flags().BoolVar(&execute, "execute", false, "DELETE inferior duplicate files (requires confirmation)")

	return cmd
}

func runDuplicates(moviesOnly, tvOnly bool, showFilter string, execute bool) error {
	// Open database
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	totalGroups := 0
	totalFiles := 0
	totalReclaimable := int64(0)
	var filesToDelete []string // Track files marked for deletion

	// Show movie duplicates
	if !tvOnly {
		movieGroups, err := db.FindDuplicateMovies()
		if err != nil {
			return fmt.Errorf("failed to find duplicate movies: %w", err)
		}

		if len(movieGroups) > 0 {
			fmt.Println("=== Duplicate Movies ===")

			for _, group := range movieGroups {
				if len(group.Files) < 2 {
					continue
				}

				totalGroups++
				totalFiles += len(group.Files)

				yearStr := ""
				if group.Year != nil {
					yearStr = fmt.Sprintf(" (%d)", *group.Year)
				}

				fmt.Printf("=== %s%s ===\n", group.NormalizedTitle, yearStr)
				fmt.Printf("%-15s | %-8s | %-12s | %-8s | %s\n",
					"Quality Score", "Size", "Resolution", "Source", "Path")
				fmt.Println(strings.Repeat("-", 120))

				for _, file := range group.Files {
					marker := ""
					if group.BestFile != nil && file.ID == group.BestFile.ID {
						marker = "[KEEP]"
					} else {
						marker = "[DELETE]"
						totalReclaimable += file.Size
						filesToDelete = append(filesToDelete, file.Path)
					}

					fmt.Printf("%-15d | %-8s | %-12s | %-8s | %s %s\n",
						file.QualityScore,
						formatBytes(file.Size),
						file.Resolution,
						file.SourceType,
						file.Path,
						marker)
				}

				fmt.Printf("\nSpace reclaimable: %s\n\n", formatBytes(group.SpaceReclaimable))
			}
		}
	}

	// Show TV duplicates
	if !moviesOnly {
		episodeGroups, err := db.FindDuplicateEpisodes()
		if err != nil {
			return fmt.Errorf("failed to find duplicate episodes: %w", err)
		}

		if len(episodeGroups) > 0 {
			fmt.Println("=== Duplicate TV Episodes ===")

			for _, group := range episodeGroups {
				if len(group.Files) < 2 {
					continue
				}

				// Apply show filter if specified
				if showFilter != "" && !strings.Contains(strings.ToLower(group.NormalizedTitle), strings.ToLower(showFilter)) {
					continue
				}

				totalGroups++
				totalFiles += len(group.Files)

				yearStr := ""
				if group.Year != nil {
					yearStr = fmt.Sprintf(" (%d)", *group.Year)
				}

				episodeStr := ""
				if group.Season != nil && group.Episode != nil {
					episodeStr = fmt.Sprintf(" S%02dE%02d", *group.Season, *group.Episode)
				}

				fmt.Printf("=== %s%s%s ===\n", group.NormalizedTitle, yearStr, episodeStr)
				fmt.Printf("%-15s | %-8s | %-12s | %-8s | %s\n",
					"Quality Score", "Size", "Resolution", "Source", "Path")
				fmt.Println(strings.Repeat("-", 120))

				for _, file := range group.Files {
					marker := ""
					if group.BestFile != nil && file.ID == group.BestFile.ID {
						marker = "[KEEP]"
					} else {
						marker = "[DELETE]"
						totalReclaimable += file.Size
						filesToDelete = append(filesToDelete, file.Path)
					}

					fmt.Printf("%-15d | %-8s | %-12s | %-8s | %s %s\n",
						file.QualityScore,
						formatBytes(file.Size),
						file.Resolution,
						file.SourceType,
						file.Path,
						marker)
				}

				fmt.Printf("\nSpace reclaimable: %s\n\n", formatBytes(group.SpaceReclaimable))
			}
		}
	}

	// Summary
	if totalGroups == 0 {
		fmt.Println("✅ No duplicates found!")
		return nil
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Duplicate groups: %d\n", totalGroups)
	fmt.Printf("Total files:      %d\n", totalFiles)
	fmt.Printf("Files to delete:  %d\n", len(filesToDelete))
	fmt.Printf("Space reclaimable: %s\n", formatBytes(totalReclaimable))

	// If execute flag is set, delete the files
	if execute {
		fmt.Println("\n⚠️  WARNING: This will permanently DELETE the files marked [DELETE] above.")
		fmt.Println("This action CANNOT be undone!")
		fmt.Printf("\nAbout to delete %d files to reclaim %s.\n", len(filesToDelete), formatBytes(totalReclaimable))
		fmt.Print("Continue? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("❌ Deletion cancelled.")
			return nil
		}

		fmt.Println("\n🗑️  Deleting duplicate files...")
		deletedCount := 0
		failedCount := 0
		reclaimedBytes := int64(0)

		for i, filePath := range filesToDelete {
			fmt.Printf("[%d/%d] Deleting: %s\n", i+1, len(filesToDelete), filePath)

			// Get file size before deleting
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("  ⚠️  File already gone, skipping\n")
					continue
				}
				fmt.Printf("  ❌ Failed to stat: %v\n", err)
				failedCount++
				continue
			}
			fileSize := fileInfo.Size()

			// Delete the file
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("  ❌ Failed to delete: %v\n", err)
				failedCount++
				continue
			}

			// Remove from database
			if err := db.DeleteMediaFile(filePath); err != nil {
				fmt.Printf("  ⚠️  Deleted file but failed to remove from database: %v\n", err)
			}

			deletedCount++
			reclaimedBytes += fileSize
			fmt.Printf("  ✅ Deleted (%s reclaimed)\n", formatBytes(fileSize))
		}

		fmt.Println("\n=== Deletion Complete ===")
		fmt.Printf("✅ Successfully deleted: %d files\n", deletedCount)
		if failedCount > 0 {
			fmt.Printf("❌ Failed to delete:     %d files\n", failedCount)
		}
		fmt.Printf("💾 Space reclaimed:      %s\n", formatBytes(reclaimedBytes))
	} else {
		fmt.Println("\n💡 To delete these duplicates, run:")
		fmt.Println("  jellywatch duplicates --execute")
		fmt.Println("\nNOTE: 'consolidate' is for organizing scattered episodes, not deleting duplicates!")
	}

	return nil
}
