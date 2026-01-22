package main

import (
	"fmt"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newDuplicatesCmd() *cobra.Command {
	var (
		moviesOnly bool
		tvOnly     bool
		showFilter string
	)

	cmd := &cobra.Command{
		Use:   "duplicates [flags]",
		Short: "List duplicate media files",
		Long: `List all duplicate media files found in the database.

Duplicates are files with the same normalized title, year, and episode (for TV shows)
but different quality scores. The CONDOR system identifies which file should be kept
based on quality scoring (Resolution > Source > Size).

Examples:
  jellywatch duplicates              # List all duplicates
  jellywatch duplicates --movies     # Only movies
  jellywatch duplicates --tv         # Only TV episodes
  jellywatch duplicates --show=Silo  # Duplicates for specific show
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDuplicates(moviesOnly, tvOnly, showFilter)
		},
	}

	cmd.Flags().BoolVar(&moviesOnly, "movies", false, "Show only movie duplicates")
	cmd.Flags().BoolVar(&tvOnly, "tv", false, "Show only TV episode duplicates")
	cmd.Flags().StringVar(&showFilter, "show", "", "Filter by show name")

	return cmd
}

func runDuplicates(moviesOnly, tvOnly bool, showFilter string) error {
	// Open database
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	totalGroups := 0
	totalFiles := 0
	totalReclaimable := int64(0)

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
	} else {
		fmt.Println("=== Summary ===")
		fmt.Printf("Duplicate groups: %d\n", totalGroups)
		fmt.Printf("Total files:      %d\n", totalFiles)
		fmt.Printf("Space reclaimable: %s\n", formatBytes(totalReclaimable))
		fmt.Println("\nTo remove duplicates:")
		fmt.Println("  jellywatch consolidate --generate  # Generate cleanup plans")
		fmt.Println("  jellywatch consolidate --dry-run   # Preview actions")
		fmt.Println("  jellywatch consolidate --execute   # Execute cleanup")
	}

	return nil
}
