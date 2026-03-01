package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/spf13/cobra"
)

var (
	healthFix    bool
	healthDryRun bool
)

func newHealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check Sonarr/Radarr configuration for jellywatch compatibility",
		Long: `Validates that Sonarr/Radarr have correct settings for jellywatch operation.

Checks:
  - enableCompletedDownloadHandling should be false (jellywatch manages imports)
  - renameEpisodes/renameMovies should be true (canonical naming)

Examples:
  jellywatch health                    # Check configuration
  jellywatch health --fix              # Fix issues (dry-run by default)
  jellywatch health --fix --dry-run=false  # Actually apply fixes`,
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
		fmt.Println("Checking Sonarr...")
		sonarrClient = sonarr.NewClient(sonarr.Config{
			URL:     cfg.Sonarr.URL,
			APIKey:  cfg.Sonarr.APIKey,
			Timeout: 30 * time.Second,
		})
		issues, err := service.CheckSonarrConfig(sonarrClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Sonarr check failed: %v\n", err)
		} else {
			allIssues = append(allIssues, issues...)
			if len(issues) == 0 {
				fmt.Println("  OK - all settings correct")
			}
		}
	} else {
		fmt.Println("Sonarr: not configured")
	}

	if cfg.Radarr.URL != "" && cfg.Radarr.APIKey != "" {
		fmt.Println("Checking Radarr...")
		radarrClient = radarr.NewClient(radarr.Config{
			URL:     cfg.Radarr.URL,
			APIKey:  cfg.Radarr.APIKey,
			Timeout: 30 * time.Second,
		})
		issues, err := service.CheckRadarrConfig(radarrClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Radarr check failed: %v\n", err)
		} else {
			allIssues = append(allIssues, issues...)
			if len(issues) == 0 {
				fmt.Println("  OK - all settings correct")
			}
		}
	} else {
		fmt.Println("Radarr: not configured")
	}

	if len(allIssues) == 0 {
		fmt.Println("\nAll arr settings are correctly configured for jellywatch.")
		return nil
	}

	fmt.Printf("\nFound %d configuration issue(s):\n", len(allIssues))
	for i, issue := range allIssues {
		icon := "!"
		if issue.Severity == "critical" {
			icon = "X"
		}
		fmt.Printf("  %s %d. [%s] %s: %s (expected: %s)\n",
			icon, i+1, issue.Service, issue.Setting, issue.Current, issue.Expected)
	}

	if !healthFix {
		fmt.Println("\nRun with --fix to attempt automatic remediation.")
		return nil
	}

	if healthDryRun {
		fmt.Println("\n[dry-run] Would fix the following issues:")
	} else {
		fmt.Println("\nApplying fixes...")
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
			fmt.Fprintf(os.Stderr, "Error fixing Sonarr issues: %v\n", err)
		}
		fixedCount += len(fixed)
		for _, f := range fixed {
			if healthDryRun {
				fmt.Printf("  Would fix [sonarr] %s\n", f.Setting)
			} else {
				fmt.Printf("  Fixed [sonarr] %s\n", f.Setting)
			}
		}
	}

	if len(radarrIssues) > 0 && radarrClient != nil {
		fixed, err := service.FixRadarrIssues(radarrClient, radarrIssues, healthDryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fixing Radarr issues: %v\n", err)
		}
		fixedCount += len(fixed)
		for _, f := range fixed {
			if healthDryRun {
				fmt.Printf("  Would fix [radarr] %s\n", f.Setting)
			} else {
				fmt.Printf("  Fixed [radarr] %s\n", f.Setting)
			}
		}
	}

	if healthDryRun {
		fmt.Printf("\n[dry-run] Would fix %d issue(s). Run with --dry-run=false to apply.\n", fixedCount)
	} else {
		fmt.Printf("\nFixed %d issue(s).\n", fixedCount)
	}

	return nil
}
