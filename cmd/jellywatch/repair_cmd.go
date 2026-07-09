package main

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/spf13/cobra"
)

func newRepairCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Repair historical JellyWatch database state",
		Long:  "Advanced database repair commands for historical metadata problems.",
	}

	cmd.AddCommand(newRepairSeriesDedupeCmd())
	cmd.AddCommand(newRepairUnknownSeasonsCmd())
	return cmd
}

func newRepairSeriesDedupeCmd() *cobra.Command {
	var execute bool
	var limit int

	cmd := &cobra.Command{
		Use:   "series-dedupe",
		Short: "Merge duplicate series rows with the same canonical path",
		Long: `Merge historical duplicate series rows that point at the same canonical folder.

Dry-run is the default. Use --execute to update database relationships and delete
duplicate series rows. This command does not move, rename, or delete media files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := database.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			report, err := db.DedupeSeriesByCanonicalPath(!execute)
			if err != nil {
				return err
			}
			printSeriesDedupeReport(report, limit)
			if !execute && len(report.Groups) > 0 {
				fmt.Println("\nRun 'jellywatch repair series-dedupe --execute' to apply this database repair.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&execute, "execute", false, "apply the repair instead of dry-running")
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum merge groups to print")
	return cmd
}

func printSeriesDedupeReport(report *database.SeriesDedupeReport, limit int) {
	mode := "DRY RUN"
	if !report.DryRun {
		mode = "EXECUTED"
	}
	fmt.Printf("Series canonical-path dedupe repair: %s\n\n", mode)
	fmt.Printf("Groups:              %d\n", len(report.Groups))
	fmt.Printf("Series deleted:      %d\n", report.SeriesDeleted)
	fmt.Printf("Episodes moved:      %d\n", report.EpisodesMoved)
	fmt.Printf("Episodes merged:     %d\n", report.EpisodesMerged)
	fmt.Printf("Media file relinks:  %d\n", report.MediaFilesUpdated)

	if len(report.Groups) == 0 || limit == 0 {
		return
	}

	if limit < 0 || limit > len(report.Groups) {
		limit = len(report.Groups)
	}

	fmt.Println("\nMerge groups:")
	for _, group := range report.Groups[:limit] {
		fmt.Printf("\n%s\n", group.CanonicalPath)
		fmt.Printf("  keep:   #%d %s (%d) source=%s priority=%d episodes=%d\n",
			group.Keeper.ID, group.Keeper.Title, group.Keeper.Year,
			group.Keeper.Source, group.Keeper.SourcePriority, group.Keeper.EpisodeCount)
		for _, duplicate := range group.Duplicates {
			fmt.Printf("  merge:  #%d %s (%d) source=%s priority=%d episodes=%d\n",
				duplicate.ID, duplicate.Title, duplicate.Year,
				duplicate.Source, duplicate.SourcePriority, duplicate.EpisodeCount)
		}
		fmt.Printf("  impact: delete=%d move_episodes=%d merge_episodes=%d relink_files=%d\n",
			group.SeriesDeleted, group.EpisodesMoved, group.EpisodesMerged, group.MediaFilesUpdated)
	}

	if len(report.Groups) > limit {
		fmt.Printf("\n... %d more groups omitted by --limit\n", len(report.Groups)-limit)
	}
}
