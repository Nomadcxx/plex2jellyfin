package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/spf13/cobra"
)

func newRepairUnknownSeasonsCmd() *cobra.Command {
	var execute bool
	var userID string
	var limit int
	var maxRefresh int

	cmd := &cobra.Command{
		Use:   "unknown-seasons",
		Short: "Audit and repair Jellyfin Season Unknown null-index cases",
		Long: `Audit Jellyfin virtual "Season Unknown" containers.

Dry-run is the default. Execute refreshes series with unknown-season episode
paths that include SxxEyy evidence. Pure obfuscated folder-context cases are
reported for manual, source-history, or parse-metadata repair.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepairUnknownSeasons(cmd, userID, limit, maxRefresh, execute)
		},
	}

	cmd.Flags().BoolVar(&execute, "execute", false, "queue Jellyfin metadata refreshes for strict repairable cases")
	cmd.Flags().StringVar(&userID, "user-id", "", "Jellyfin user id for user-scoped library view (default: first admin user)")
	cmd.Flags().IntVar(&limit, "limit", 30, "maximum issues/actions to print")
	cmd.Flags().IntVar(&maxRefresh, "max-refresh", 10, "maximum unique series refreshes to queue")
	return cmd
}

func runRepairUnknownSeasons(cmd *cobra.Command, userID string, limit, maxRefresh int, execute bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.Jellyfin.URL == "" || cfg.Jellyfin.APIKey == "" {
		return fmt.Errorf("jellyfin url/api_key not configured")
	}

	client := jellyfin.NewClient(jellyfin.Config{
		URL:     cfg.Jellyfin.URL,
		APIKey:  cfg.Jellyfin.APIKey,
		Timeout: 30 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if execute {
		report, err := client.RepairUnknownSeasons(ctx, userID, maxRefresh, false)
		if err != nil {
			return err
		}
		printUnknownSeasonReport(cmd.OutOrStdout(), &report.Audit, limit)
		printUnknownSeasonActions(cmd.OutOrStdout(), report.Actions, limit)
		fmt.Fprintf(cmd.OutOrStdout(), "\nRefreshes queued: %d, skipped: %d, errors: %d\n", report.Refreshed, report.Skipped, report.Errors)
		return nil
	}

	report, err := client.AuditUnknownSeasons(ctx, userID)
	if err != nil {
		return err
	}
	printUnknownSeasonReport(cmd.OutOrStdout(), report, limit)
	if report.RefreshCandidateSeasons > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nRun 'plex2jellyfin repair unknown-seasons --execute' to refresh series with parseable unknown-season paths.")
	}
	return nil
}

func printUnknownSeasonReport(out io.Writer, report *jellyfin.UnknownSeasonReport, limit int) {
	fmt.Fprintln(out, "Jellyfin Season Unknown audit")
	fmt.Fprintf(out, "User:                     %s\n", report.UserID)
	fmt.Fprintf(out, "Unknown season containers: %d\n", report.Total)
	fmt.Fprintf(out, "Refresh repairable:       %d seasons / %d episodes\n", report.RefreshRepairableSeasons, report.RefreshRepairableEpisodes)
	fmt.Fprintf(out, "Refresh candidates:       %d seasons / %d parseable episodes\n", report.RefreshCandidateSeasons, report.RefreshCandidateEpisodes)
	fmt.Fprintf(out, "Folder-context/manual:    %d\n", report.FolderContext)
	fmt.Fprintf(out, "Mixed review:             %d\n", report.MixedReview)
	fmt.Fprintf(out, "Manual unknown:           %d\n", report.ManualUnknown)
	fmt.Fprintf(out, "Empty/indexed virtual:    %d\n", report.Empty+report.Indexed)

	if len(report.Issues) == 0 || limit == 0 {
		return
	}
	if limit < 0 || limit > len(report.Issues) {
		limit = len(report.Issues)
	}

	fmt.Fprintln(out, "\nIssues:")
	for _, issue := range report.Issues[:limit] {
		fmt.Fprintf(out, "- [%s] %s (%d episodes, null indexes: %d)\n",
			issue.Class, issue.SeriesName, issue.EpisodeCount, issue.NullNumberCount)
		if issue.ExampleEpisodePath != "" {
			fmt.Fprintf(out, "  example: %s\n", issue.ExampleEpisodePath)
		}
	}
	if len(report.Issues) > limit {
		fmt.Fprintf(out, "\n... %d more omitted by --limit\n", len(report.Issues)-limit)
	}
}

func printUnknownSeasonActions(out io.Writer, actions []jellyfin.UnknownSeasonAction, limit int) {
	if len(actions) == 0 || limit == 0 {
		return
	}
	if limit < 0 || limit > len(actions) {
		limit = len(actions)
	}
	fmt.Fprintln(out, "\nActions:")
	for _, action := range actions[:limit] {
		if action.Error != "" {
			fmt.Fprintf(out, "- [%s] %s (%s): %s\n", action.Action, action.SeriesName, action.SeriesID, action.Error)
			continue
		}
		fmt.Fprintf(out, "- [%s] %s (%s)\n", action.Action, action.SeriesName, action.SeriesID)
	}
	if len(actions) > limit {
		fmt.Fprintf(out, "\n... %d more actions omitted by --limit\n", len(actions)-limit)
	}
}
