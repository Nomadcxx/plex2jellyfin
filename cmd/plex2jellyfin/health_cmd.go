package main

import (
	"fmt"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/clitheme"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	"github.com/spf13/cobra"
)

var (
	healthFix    bool
	healthDryRun bool
)

func newHealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check Sonarr/Radarr configuration for plex2jellyfin compatibility",
		Long: `Validates that Sonarr/Radarr have correct settings for plex2jellyfin operation.

Checks:
  - enableCompletedDownloadHandling should be false (plex2jellyfin manages imports)
  - renameEpisodes/renameMovies should be true (canonical naming)

Examples:
  plex2jellyfin health                    # Check configuration
  plex2jellyfin health --fix              # Fix issues (dry-run by default)
  plex2jellyfin health --fix --dry-run=false  # Actually apply fixes`,
		RunE: runHealth,
	}

	cmd.Flags().BoolVar(&healthFix, "fix", false, "Attempt to fix detected issues")
	cmd.Flags().BoolVar(&healthDryRun, "dry-run", true, "Show what would be fixed without changing (only with --fix)")

	return cmd
}

func runHealth(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var allIssues []service.HealthIssue
	var sonarrClient *sonarr.Client
	var radarrClient *radarr.Client

	if cfg.Sonarr.URL != "" && cfg.Sonarr.APIKey != "" {
		clitheme.Section(cmd.OutOrStdout(), "Sonarr")
		sonarrClient = sonarr.NewClient(sonarr.Config{
			URL:     cfg.Sonarr.URL,
			APIKey:  cfg.Sonarr.APIKey,
			Timeout: 30 * time.Second,
		})
		issues, err := service.CheckSonarrConfig(sonarrClient)
		if err != nil {
			clitheme.Warn(cmd.OutOrStdout(), fmt.Sprintf("Sonarr check failed: %v", err))
		} else {
			allIssues = append(allIssues, issues...)
			if len(issues) == 0 {
				clitheme.OK(cmd.OutOrStdout(), "all settings correct")
			}
		}
	} else {
		clitheme.Muted(cmd.OutOrStdout(), "Sonarr: not configured")
	}

	if cfg.Radarr.URL != "" && cfg.Radarr.APIKey != "" {
		clitheme.Section(cmd.OutOrStdout(), "Radarr")
		radarrClient = radarr.NewClient(radarr.Config{
			URL:     cfg.Radarr.URL,
			APIKey:  cfg.Radarr.APIKey,
			Timeout: 30 * time.Second,
		})
		issues, err := service.CheckRadarrConfig(radarrClient)
		if err != nil {
			clitheme.Warn(cmd.OutOrStdout(), fmt.Sprintf("Radarr check failed: %v", err))
		} else {
			allIssues = append(allIssues, issues...)
			if len(issues) == 0 {
				clitheme.OK(cmd.OutOrStdout(), "all settings correct")
			}
		}
	} else {
		clitheme.Muted(cmd.OutOrStdout(), "Radarr: not configured")
	}

	if len(allIssues) == 0 {
		clitheme.OK(cmd.OutOrStdout(), "All arr settings are correctly configured for plex2jellyfin.")
		return nil
	}

	clitheme.Warn(cmd.OutOrStdout(), fmt.Sprintf("Found %d configuration issue(s):", len(allIssues)))
	for i, issue := range allIssues {
		marker := "[warn]"
		if issue.Severity == "critical" {
			marker = "[error]"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s %d. [%s] %s: %s (expected: %s)\n",
			marker, i+1, issue.Service, issue.Setting, issue.Current, issue.Expected)
	}

	if !healthFix {
		clitheme.Muted(cmd.OutOrStdout(), "Run with --fix to attempt automatic remediation.")
		return nil
	}

	if healthDryRun {
		clitheme.Muted(cmd.OutOrStdout(), "[dry-run] Would fix the following issues:")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Applying fixes...")
	}

	var sonarrIssues, radarrIssues []service.HealthIssue
	for _, issue := range allIssues {
		if issue.Service == "sonarr" {
			sonarrIssues = append(sonarrIssues, issue)
		} else {
			radarrIssues = append(radarrIssues, issue)
		}
	}

	var fixedCount int

	if len(sonarrIssues) > 0 && sonarrClient != nil {
		fixed, err := service.FixSonarrIssues(sonarrClient, sonarrIssues, healthDryRun)
		if err != nil {
			clitheme.Err(cmd.OutOrStdout(), fmt.Sprintf("fixing Sonarr issues: %v", err))
		}
		fixedCount += len(fixed)
		for _, f := range fixed {
			if healthDryRun {
				clitheme.Muted(cmd.OutOrStdout(), fmt.Sprintf("Would fix [sonarr] %s", f.Setting))
			} else {
				clitheme.OK(cmd.OutOrStdout(), fmt.Sprintf("Fixed [sonarr] %s", f.Setting))
			}
		}
	}

	if len(radarrIssues) > 0 && radarrClient != nil {
		fixed, err := service.FixRadarrIssues(radarrClient, radarrIssues, healthDryRun)
		if err != nil {
			clitheme.Err(cmd.OutOrStdout(), fmt.Sprintf("fixing Radarr issues: %v", err))
		}
		fixedCount += len(fixed)
		for _, f := range fixed {
			if healthDryRun {
				clitheme.Muted(cmd.OutOrStdout(), fmt.Sprintf("Would fix [radarr] %s", f.Setting))
			} else {
				clitheme.OK(cmd.OutOrStdout(), fmt.Sprintf("Fixed [radarr] %s", f.Setting))
			}
		}
	}

	if healthDryRun {
		clitheme.Muted(cmd.OutOrStdout(), fmt.Sprintf("[dry-run] Would fix %d issue(s). Run with --dry-run=false to apply.", fixedCount))
	} else {
		clitheme.OK(cmd.OutOrStdout(), fmt.Sprintf("Fixed %d issue(s).", fixedCount))
	}

	return nil
}
