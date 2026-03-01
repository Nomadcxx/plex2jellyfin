package main

import (
	"fmt"
	"io"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/spf13/cobra"
)

type orphansClient interface {
	GetOrphanedEpisodes() ([]jellyfin.Item, error)
	RemediateOrphans(orphans []jellyfin.Item, dryRun bool) ([]jellyfin.RemediationResult, error)
}

var (
	orphansLoadConfig    = config.Load
	orphansClientFactory = func(cfg *config.Config) orphansClient {
		return jellyfin.NewClient(jellyfin.Config{URL: cfg.Jellyfin.URL, APIKey: cfg.Jellyfin.APIKey})
	}
)

func newOrphansCmd() *cobra.Command {
	var fix bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "Detect and remediate orphaned Jellyfin episodes",
		Long:  "Find episodes with no series linkage and optionally remediate them via metadata refresh.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrphans(cmd, fix, dryRun)
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "Attempt to remediate all detected orphans")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview remediation without triggering refresh")

	return cmd
}

func runOrphans(cmd *cobra.Command, fix bool, dryRun bool) error {
	out := cmd.OutOrStdout()

	cfg, err := orphansLoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	client := orphansClientFactory(cfg)

	fmt.Fprintln(out, "Scanning for orphaned episodes...")
	orphans, err := client.GetOrphanedEpisodes()
	if err != nil {
		return fmt.Errorf("finding orphans: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Fprintln(out, "No orphaned episodes found.")
		return nil
	}

	fmt.Fprintf(out, "Found %d orphaned episodes:\n", len(orphans))
	for i, orphan := range orphans {
		printOrphan(out, i+1, orphan)
	}

	if !fix {
		fmt.Fprintln(out, "\nRun with --fix to attempt remediation via metadata refresh.")
		return nil
	}

	fmt.Fprintln(out, "\nRemediating...")
	results, err := client.RemediateOrphans(orphans, dryRun)
	if err != nil {
		return fmt.Errorf("remediating orphans: %w", err)
	}

	var refreshed, failed, skipped int
	for _, result := range results {
		switch result.Action {
		case "refreshed":
			refreshed++
		case "failed":
			failed++
			fmt.Fprintf(out, "  FAILED: %s - %v\n", result.ItemName, result.Error)
		case "skipped":
			skipped++
		}
	}

	fmt.Fprintf(out, "\nResults: %d refreshed, %d failed, %d skipped, %d total\n", refreshed, failed, skipped, len(results))
	return nil
}

func printOrphan(out io.Writer, number int, orphan jellyfin.Item) {
	fmt.Fprintf(out, "  %d. %s\n", number, orphan.Name)
	fmt.Fprintf(out, "     Path: %s\n", orphan.Path)
	fmt.Fprintf(out, "     Providers: %v\n", orphan.ProviderIDs)
}
