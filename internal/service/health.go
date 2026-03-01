package service

import (
	"fmt"
	"log/slog"

	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// HealthIssue represents a detected configuration issue.
type HealthIssue struct {
	Service  string // "sonarr", "radarr"
	Setting  string // "enableCompletedDownloadHandling", "renameEpisodes", etc.
	Current  string // current value
	Expected string // expected value
	Severity string // "critical", "warning"
	FixCmd   string // suggested fix command or empty
}

// HealthReport contains all detected health issues.
type HealthReport struct {
	Issues  []HealthIssue
	Healthy bool
}

// CheckSonarrConfig validates Sonarr settings for jellywatch compatibility.
func CheckSonarrConfig(client *sonarr.Client) ([]HealthIssue, error) {
	var issues []HealthIssue

	dlCfg, err := client.GetDownloadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("checking sonarr download client config: %w", err)
	}
	if dlCfg.EnableCompletedDownloadHandling {
		issues = append(issues, HealthIssue{
			Service:  "sonarr",
			Setting:  "enableCompletedDownloadHandling",
			Current:  "true",
			Expected: "false",
			Severity: "critical",
			FixCmd:   "jellywatch health --fix",
		})
	}

	nameCfg, err := client.GetNamingConfig()
	if err != nil {
		return nil, fmt.Errorf("checking sonarr naming config: %w", err)
	}
	if !nameCfg.RenameEpisodes {
		issues = append(issues, HealthIssue{
			Service:  "sonarr",
			Setting:  "renameEpisodes",
			Current:  "false",
			Expected: "true",
			Severity: "warning",
			FixCmd:   "jellywatch health --fix",
		})
	}

	return issues, nil
}

// CheckRadarrConfig validates Radarr settings for jellywatch compatibility.
func CheckRadarrConfig(client *radarr.Client) ([]HealthIssue, error) {
	var issues []HealthIssue

	dlCfg, err := client.GetDownloadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("checking radarr download client config: %w", err)
	}
	if dlCfg.EnableCompletedDownloadHandling {
		issues = append(issues, HealthIssue{
			Service:  "radarr",
			Setting:  "enableCompletedDownloadHandling",
			Current:  "true",
			Expected: "false",
			Severity: "critical",
			FixCmd:   "jellywatch health --fix",
		})
	}

	nameCfg, err := client.GetNamingConfig()
	if err != nil {
		return nil, fmt.Errorf("checking radarr naming config: %w", err)
	}
	if !nameCfg.RenameMovies {
		issues = append(issues, HealthIssue{
			Service:  "radarr",
			Setting:  "renameMovies",
			Current:  "false",
			Expected: "true",
			Severity: "warning",
			FixCmd:   "jellywatch health --fix",
		})
	}

	return issues, nil
}

// FixSonarrIssues attempts to fix detected Sonarr configuration issues.
func FixSonarrIssues(client *sonarr.Client, issues []HealthIssue, dryRun bool) ([]HealthIssue, error) {
	var fixed []HealthIssue

	for _, issue := range issues {
		if issue.Service != "sonarr" {
			continue
		}

		if dryRun {
			slog.Info("dry-run: would fix", "service", issue.Service, "setting", issue.Setting)
			fixed = append(fixed, issue)
			continue
		}

		switch issue.Setting {
		case "enableCompletedDownloadHandling":
			cfg, err := client.GetDownloadClientConfig()
			if err != nil {
				return fixed, err
			}
			cfg.EnableCompletedDownloadHandling = false
			if _, err := client.UpdateDownloadClientConfig(*cfg); err != nil {
				return fixed, fmt.Errorf("fixing %s: %w", issue.Setting, err)
			}
			fixed = append(fixed, issue)
			slog.Info("fixed", "service", "sonarr", "setting", issue.Setting, "value", false)

		case "renameEpisodes":
			cfg, err := client.GetNamingConfig()
			if err != nil {
				return fixed, err
			}
			cfg.RenameEpisodes = true
			if err := client.UpdateNamingConfig(cfg); err != nil {
				return fixed, fmt.Errorf("fixing %s: %w", issue.Setting, err)
			}
			fixed = append(fixed, issue)
			slog.Info("fixed", "service", "sonarr", "setting", issue.Setting, "value", true)
		}
	}

	return fixed, nil
}

// FixRadarrIssues attempts to fix detected Radarr configuration issues.
func FixRadarrIssues(client *radarr.Client, issues []HealthIssue, dryRun bool) ([]HealthIssue, error) {
	var fixed []HealthIssue

	for _, issue := range issues {
		if issue.Service != "radarr" {
			continue
		}

		if dryRun {
			slog.Info("dry-run: would fix", "service", issue.Service, "setting", issue.Setting)
			fixed = append(fixed, issue)
			continue
		}

		switch issue.Setting {
		case "enableCompletedDownloadHandling":
			cfg, err := client.GetDownloadClientConfig()
			if err != nil {
				return fixed, err
			}
			cfg.EnableCompletedDownloadHandling = false
			if _, err := client.UpdateDownloadClientConfig(*cfg); err != nil {
				return fixed, fmt.Errorf("fixing %s: %w", issue.Setting, err)
			}
			fixed = append(fixed, issue)
			slog.Info("fixed", "service", "radarr", "setting", issue.Setting, "value", false)

		case "renameMovies":
			cfg, err := client.GetNamingConfig()
			if err != nil {
				return fixed, err
			}
			cfg.RenameMovies = true
			if err := client.UpdateNamingConfig(cfg); err != nil {
				return fixed, fmt.Errorf("fixing %s: %w", issue.Setting, err)
			}
			fixed = append(fixed, issue)
			slog.Info("fixed", "service", "radarr", "setting", issue.Setting, "value", true)
		}
	}

	return fixed, nil
}
