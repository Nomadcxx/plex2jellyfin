package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/plans"
	"github.com/spf13/cobra"
)

func newDuplicatesCmd() *cobra.Command {
	var (
		moviesOnly bool
		tvOnly     bool
		showFilter string
		generate   bool
		dryRun     bool
		execute    bool
	)

	cmd := &cobra.Command{
		Use:   "duplicates [flags]",
		Short: "Find and remove duplicate media files",
		Long: `Find and optionally remove duplicate media files from your library.

Workflow:
  1. jellywatch duplicates              # Show duplicate analysis
  2. jellywatch duplicates --generate   # Generate deletion plans
  3. jellywatch duplicates --dry-run    # Preview pending plans
  4. jellywatch duplicates --execute    # Execute plans (delete files)

Examples:
  jellywatch duplicates              # List all duplicates
  jellywatch duplicates --movies     # Only movies
  jellywatch duplicates --tv         # Only TV episodes
  jellywatch duplicates --generate   # Generate deletion plans
  jellywatch duplicates --dry-run    # Preview what would be deleted
  jellywatch duplicates --execute    # DELETE inferior duplicates
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDuplicates(moviesOnly, tvOnly, showFilter, generate, dryRun, execute)
		},
	}

	cmd.Flags().BoolVar(&moviesOnly, "movies", false, "Show only movie duplicates")
	cmd.Flags().BoolVar(&tvOnly, "tv", false, "Show only TV episode duplicates")
	cmd.Flags().StringVar(&showFilter, "show", "", "Filter by show name")
	cmd.Flags().BoolVar(&generate, "generate", false, "Generate deletion plans")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview pending plans")
	cmd.Flags().BoolVar(&execute, "execute", false, "Execute pending plans (delete files)")

	return cmd
}

func runDuplicates(moviesOnly, tvOnly bool, showFilter string, generate, dryRun, execute bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Generate plans
	if generate {
		return runDuplicatesGenerate(db, moviesOnly, tvOnly, showFilter)
	}

	// Preview plans
	if dryRun {
		return runDuplicatesDryRun()
	}

	// Execute plans
	if execute {
		return runDuplicatesExecute(db)
	}

	// Default: show analysis
	return runDuplicatesAnalysis(db, moviesOnly, tvOnly, showFilter)
}

func generateGroupID(title string, year *int, season, episode *int) string {
	parts := []string{strings.ToLower(title)}
	if year != nil {
		parts = append(parts, fmt.Sprintf("%d", *year))
	}
	if season != nil {
		parts = append(parts, fmt.Sprintf("s%d", *season))
	}
	if episode != nil {
		parts = append(parts, fmt.Sprintf("e%d", *episode))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "-")))
	return fmt.Sprintf("%x", hash[:8])
}

func runDuplicatesAnalysis(db *database.MediaDB, moviesOnly, tvOnly bool, showFilter string) error {
	totalGroups := 0
	totalReclaimable := int64(0)

	// Movie duplicates
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
				totalReclaimable += group.SpaceReclaimable
				printDuplicateGroup(group.NormalizedTitle, group.Year, nil, nil, group.Files, group.BestFile)
			}
		}
	}

	// TV duplicates
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
				if showFilter != "" && !strings.Contains(strings.ToLower(group.NormalizedTitle), strings.ToLower(showFilter)) {
					continue
				}
				totalGroups++
				totalReclaimable += group.SpaceReclaimable
				printDuplicateGroup(group.NormalizedTitle, group.Year, group.Season, group.Episode, group.Files, group.BestFile)
			}
		}
	}

	if totalGroups == 0 {
		fmt.Println("✅ No duplicates found!")
		return nil
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Duplicate groups:   %d\n", totalGroups)
	fmt.Printf("Space reclaimable: %s\n", formatBytes(totalReclaimable))
	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch duplicates --generate   # Generate deletion plans")
	fmt.Println("  jellywatch duplicates --dry-run    # Preview plans")
	fmt.Println("  jellywatch duplicates --execute    # Execute plans")

	return nil
}

func printDuplicateGroup(title string, year, season, episode *int, files []*database.MediaFile, bestFile *database.MediaFile) {
	yearStr := ""
	if year != nil {
		yearStr = fmt.Sprintf(" (%d)", *year)
	}
	episodeStr := ""
	if season != nil && episode != nil {
		episodeStr = fmt.Sprintf(" S%02dE%02d", *season, *episode)
	}

	fmt.Printf("=== %s%s%s ===\n", title, yearStr, episodeStr)
	fmt.Printf("%-15s | %-8s | %-12s | %-8s | %s\n", "Quality Score", "Size", "Resolution", "Source", "Path")
	fmt.Println(strings.Repeat("-", 120))

	for _, file := range files {
		marker := "[DELETE]"
		if bestFile != nil && file.ID == bestFile.ID {
			marker = "[KEEP]"
		}
		fmt.Printf("%-15d | %-8s | %-12s | %-8s | %s %s\n",
			file.QualityScore, formatBytes(file.Size), file.Resolution, file.SourceType, file.Path, marker)
	}
	fmt.Println()
}

func runDuplicatesGenerate(db *database.MediaDB, moviesOnly, tvOnly bool, showFilter string) error {
	fmt.Println("🔍 Analyzing duplicates...")

	plan := &plans.DuplicatePlan{
		CreatedAt: time.Now(),
		Command:   "duplicates",
		Summary:   plans.DuplicateSummary{},
		Plans:     []plans.DuplicateGroup{},
	}

	// Movie duplicates
	if !tvOnly {
		movieGroups, err := db.FindDuplicateMovies()
		if err != nil {
			return fmt.Errorf("failed to find duplicate movies: %w", err)
		}

		for _, group := range movieGroups {
			if len(group.Files) < 2 || group.BestFile == nil {
				continue
			}

			for _, file := range group.Files {
				if file.ID == group.BestFile.ID {
					continue
				}

				plan.Plans = append(plan.Plans, plans.DuplicateGroup{
					GroupID:   generateGroupID(group.NormalizedTitle, group.Year, nil, nil),
					Title:     group.NormalizedTitle,
					Year:      group.Year,
					MediaType: "movie",
					Keep: plans.FileInfo{
						ID:           group.BestFile.ID,
						Path:         group.BestFile.Path,
						Size:         group.BestFile.Size,
						QualityScore: group.BestFile.QualityScore,
						Resolution:   group.BestFile.Resolution,
						SourceType:   group.BestFile.SourceType,
					},
					Delete: plans.FileInfo{
						ID:           file.ID,
						Path:         file.Path,
						Size:         file.Size,
						QualityScore: file.QualityScore,
						Resolution:   file.Resolution,
						SourceType:   file.SourceType,
					},
				})

				plan.Summary.FilesToDelete++
				plan.Summary.SpaceReclaimable += file.Size
			}
			plan.Summary.TotalGroups++
		}
	}

	// TV duplicates
	if !moviesOnly {
		episodeGroups, err := db.FindDuplicateEpisodes()
		if err != nil {
			return fmt.Errorf("failed to find duplicate episodes: %w", err)
		}

		for _, group := range episodeGroups {
			if len(group.Files) < 2 || group.BestFile == nil {
				continue
			}

			if showFilter != "" && !strings.Contains(strings.ToLower(group.NormalizedTitle), strings.ToLower(showFilter)) {
				continue
			}

			for _, file := range group.Files {
				if file.ID == group.BestFile.ID {
					continue
				}

				plan.Plans = append(plan.Plans, plans.DuplicateGroup{
					GroupID:   generateGroupID(group.NormalizedTitle, group.Year, group.Season, group.Episode),
					Title:     group.NormalizedTitle,
					Year:      group.Year,
					MediaType: "series",
					Season:    group.Season,
					Episode:   group.Episode,
					Keep: plans.FileInfo{
						ID:           group.BestFile.ID,
						Path:         group.BestFile.Path,
						Size:         group.BestFile.Size,
						QualityScore: group.BestFile.QualityScore,
						Resolution:   group.BestFile.Resolution,
						SourceType:   group.BestFile.SourceType,
					},
					Delete: plans.FileInfo{
						ID:           file.ID,
						Path:         file.Path,
						Size:         file.Size,
						QualityScore: file.QualityScore,
						Resolution:   file.Resolution,
						SourceType:   file.SourceType,
					},
				})

				plan.Summary.FilesToDelete++
				plan.Summary.SpaceReclaimable += file.Size
			}
			plan.Summary.TotalGroups++
		}
	}

	if len(plan.Plans) == 0 {
		fmt.Println("✅ No duplicates found!")
		return nil
	}

	if err := plans.SaveDuplicatePlans(plan); err != nil {
		return fmt.Errorf("failed to save plans: %w", err)
	}

	fmt.Println("\n✅ Plans generated successfully!")
	fmt.Printf("\nDuplicate Summary:\n")
	fmt.Printf("  Duplicate groups:   %d\n", plan.Summary.TotalGroups)
	fmt.Printf("  Files to delete:    %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("  Space reclaimable: %s\n", formatBytes(plan.Summary.SpaceReclaimable))

	plansDir, _ := plans.GetPlansDir()
	fmt.Printf("\nPlan saved to: %s/duplicates.json\n", plansDir)

	fmt.Println("\nNext steps:")
	fmt.Println("  jellywatch duplicates --dry-run    # Preview what will be deleted")
	fmt.Println("  jellywatch duplicates --execute    # Execute the plans")

	return nil
}

func runDuplicatesDryRun() error {
	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch duplicates --generate' first to create plans.")
		return nil
	}

	fmt.Println("🔍 DRY RUN - No changes will be made")
	fmt.Printf("\nPlan created: %s\n", plan.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Files to delete: %d\n", plan.Summary.FilesToDelete)
	fmt.Printf("Space to reclaim: %s\n\n", formatBytes(plan.Summary.SpaceReclaimable))

	for i, p := range plan.Plans {
		yearStr := ""
		if p.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *p.Year)
		}
		episodeStr := ""
		if p.Season != nil && p.Episode != nil {
			episodeStr = fmt.Sprintf(" S%02dE%02d", *p.Season, *p.Episode)
		}

		fmt.Printf("[%d] %s%s%s\n", i+1, p.Title, yearStr, episodeStr)
		fmt.Printf("    KEEP:   %s (%s, score %d)\n", p.Keep.Path, formatBytes(p.Keep.Size), p.Keep.QualityScore)
		fmt.Printf("    DELETE: %s (%s, score %d)\n\n", p.Delete.Path, formatBytes(p.Delete.Size), p.Delete.QualityScore)
	}

	fmt.Println("To execute these plans, run:")
	fmt.Println("  jellywatch duplicates --execute")

	return nil
}

func runDuplicatesExecute(db *database.MediaDB) error {
	plan, err := plans.LoadDuplicatePlans()
	if err != nil {
		return fmt.Errorf("failed to load plans: %w", err)
	}

	if plan == nil {
		fmt.Println("No pending plans found.")
		fmt.Println("Run 'jellywatch duplicates --generate' first to create plans.")
		return nil
	}

	fmt.Printf("⚠️  WARNING: This will permanently DELETE %d files.\n", plan.Summary.FilesToDelete)
	fmt.Printf("Space to reclaim: %s\n", formatBytes(plan.Summary.SpaceReclaimable))
	fmt.Println("This action CANNOT be undone!")
	fmt.Print("\nContinue? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("❌ Execution cancelled.")
		return nil
	}

	fmt.Println("\n🗑️  Deleting duplicate files...")

	deletedCount := 0
	failedCount := 0
	reclaimedBytes := int64(0)

	for i, p := range plan.Plans {
		fmt.Printf("[%d/%d] Deleting: %s\n", i+1, len(plan.Plans), p.Delete.Path)

		fileInfo, err := os.Stat(p.Delete.Path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("  ⚠️  File already gone, skipping")
				continue
			}
			fmt.Printf("  ❌ Failed to stat: %v\n", err)
			failedCount++
			continue
		}
		fileSize := fileInfo.Size()

		if err := os.Remove(p.Delete.Path); err != nil {
			fmt.Printf("  ❌ Failed to delete: %v\n", err)
			failedCount++
			continue
		}

		if err := db.DeleteMediaFile(p.Delete.Path); err != nil {
			fmt.Printf("  ⚠️  Deleted but failed to update database: %v\n", err)
		}

		deletedCount++
		reclaimedBytes += fileSize
		fmt.Printf("  ✅ Deleted (%s reclaimed)\n", formatBytes(fileSize))
	}

	// Delete plans file on success
	if failedCount == 0 {
		if err := plans.DeleteDuplicatePlans(); err != nil {
			fmt.Printf("⚠️  Failed to clean up plans file: %v\n", err)
		}
	}

	fmt.Println("\n=== Execution Complete ===")
	fmt.Printf("✅ Successfully deleted: %d files\n", deletedCount)
	if failedCount > 0 {
		fmt.Printf("❌ Failed to delete:     %d files\n", failedCount)
	}
	fmt.Printf("💾 Space reclaimed:      %s\n", formatBytes(reclaimedBytes))

	return nil
}
