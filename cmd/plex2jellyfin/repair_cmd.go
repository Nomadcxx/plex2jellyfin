package main

import (
	"fmt"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/spf13/cobra"
)

func newRepairCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Repair historical Plex2Jellyfin database state",
		Long:  "Advanced database repair commands for historical metadata problems.",
	}

	cmd.AddCommand(newRepairSeriesDedupeCmd())
	cmd.AddCommand(newRepairNestedSeriesCmd())
	cmd.AddCommand(newRepairUnknownSeasonsCmd())
	return cmd
}

func newRepairNestedSeriesCmd() *cobra.Command {
	var execute bool
	var limit int

	cmd := &cobra.Command{
		Use:   "nested-series",
		Short: "Relink nested release files to existing series rows",
		Long: `Repair series rows created for release folders nested under Season directories.

Dry-run is the default. The repair requires a matching show, season, episode, and
live media file. It changes database relationships only; it does not move or delete files.
Use --execute to apply unambiguous mappings.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := database.Open()
			if err != nil {
				return err
			}
			defer db.Close()

			report, err := db.RepairNestedReleaseSeries(!execute)
			if err != nil {
				return err
			}
			printNestedSeriesRepairReport(report, limit)
			if !execute && report.SeriesDeleted > 0 {
				fmt.Println("\nRun 'plex2jellyfin repair nested-series --execute' to apply the database repair.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&execute, "execute", false, "apply the repair instead of dry-running")
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum candidate groups to print")
	return cmd
}

func printNestedSeriesRepairReport(report *database.NestedSeriesRepairReport, limit int) {
	mode := "DRY RUN"
	if !report.DryRun {
		mode = "EXECUTED"
	}
	fmt.Printf("Nested release series repair: %s\n\n", mode)
	fmt.Printf("Candidates:          %d\n", len(report.Groups))
	fmt.Printf("Ambiguous:           %d\n", report.Ambiguous)
	fmt.Printf("Episodes moved:      %d\n", report.EpisodesMoved)
	fmt.Printf("Episodes merged:     %d\n", report.EpisodesMerged)
	fmt.Printf("Media files relinked: %d\n", report.MediaFilesUpdated)
	seriesAction := "deleted"
	if report.DryRun {
		seriesAction = "to delete"
	}
	fmt.Printf("Synthetic series rows %s: %d\n", seriesAction, report.SeriesDeleted)
	if len(report.Groups) == 0 || limit == 0 {
		return
	}
	if limit < 0 || limit > len(report.Groups) {
		limit = len(report.Groups)
	}
	fmt.Println("\nCandidate mappings:")
	for _, group := range report.Groups[:limit] {
		if group.AmbiguousReason != "" {
			fmt.Printf("  [review] #%d %s: %s\n", group.SourceSeriesID, group.SourcePath, group.AmbiguousReason)
			continue
		}
		fmt.Printf("  #%d %s -> #%d %s (%d media files)\n", group.SourceSeriesID, group.SourcePath, group.TargetSeriesID, group.TargetPath, group.MediaFiles)
	}
	if len(report.Groups) > limit {
		fmt.Printf("  ... %d more groups omitted by --limit\n", len(report.Groups)-limit)
	}
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
				fmt.Println("\nRun 'plex2jellyfin repair series-dedupe --execute' to apply this database repair.")
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
