package service

import (
	"fmt"
	"log/slog"

	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

// HealthIssue represents a detected configuration issue.
type HealthIssue struct {
	Service  string `json:"service"`  // "sonarr", "radarr"
	Setting  string `json:"setting"`  // "enableCompletedDownloadHandling", "renameEpisodes", etc.
	Current  string `json:"current"`  // current value
	Expected string `json:"expected"` // expected value
	Severity string `json:"severity"` // "critical", "warning"
	FixCmd   string `json:"fix_cmd"`  // suggested fix command or empty
}

// HealthReport contains all detected health issues.
type HealthReport struct {
	Issues  []HealthIssue
	Healthy bool
}

// CheckSonarrConfig validates Sonarr settings for plex2jellyfin compatibility.
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
			FixCmd:   "plex2jellyfin health --fix --dry-run=false",
		})
	}

	nameCfg, err := client.GetNamingConfig()
	if err != nil {
		return nil, fmt.Errorf("checking sonarr naming config: %w", err)
	}
	if nameCfg.RenameEpisodes {
		issues = append(issues, HealthIssue{
			Service:  "sonarr",
			Setting:  "renameEpisodes",
			Current:  "true",
			Expected: "false",
			Severity: "critical",
			FixCmd:   "plex2jellyfin health --fix --dry-run=false",
		})
	}

	return issues, nil
}

// CheckRadarrConfig validates Radarr settings for plex2jellyfin compatibility.
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
			FixCmd:   "plex2jellyfin health --fix --dry-run=false",
		})
	}

	nameCfg, err := client.GetNamingConfig()
	if err != nil {
		return nil, fmt.Errorf("checking radarr naming config: %w", err)
	}
	if nameCfg.RenameMovies {
		issues = append(issues, HealthIssue{
			Service:  "radarr",
			Setting:  "renameMovies",
			Current:  "true",
			Expected: "false",
			Severity: "critical",
			FixCmd:   "plex2jellyfin health --fix --dry-run=false",
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
			cfg.RenameEpisodes = false
			if err := client.UpdateNamingConfig(cfg); err != nil {
				return fixed, fmt.Errorf("fixing %s: %w", issue.Setting, err)
			}
			fixed = append(fixed, issue)
			slog.Info("fixed", "service", "sonarr", "setting", issue.Setting, "value", false)
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
			cfg.RenameMovies = false
			if err := client.UpdateNamingConfig(cfg); err != nil {
				return fixed, fmt.Errorf("fixing %s: %w", issue.Setting, err)
			}
			fixed = append(fixed, issue)
			slog.Info("fixed", "service", "radarr", "setting", issue.Setting, "value", false)
		}
	}

	return fixed, nil
}

// EnsureSonarrConfig applies the hands-off policy and verifies Sonarr retained it.
func EnsureSonarrConfig(client *sonarr.Client) ([]HealthIssue, error) {
	issues, err := CheckSonarrConfig(client)
	if err != nil {
		return nil, err
	}
	fixed, err := FixSonarrIssues(client, issues, false)
	if err != nil {
		return fixed, err
	}
	remaining, err := CheckSonarrConfig(client)
	if err != nil {
		return fixed, fmt.Errorf("verifying sonarr compatibility: %w", err)
	}
	if len(remaining) > 0 {
		return fixed, fmt.Errorf("sonarr still incompatible: %s=%s", remaining[0].Setting, remaining[0].Current)
	}
	return fixed, nil
}

// EnsureRadarrConfig applies the hands-off policy and verifies Radarr retained it.
func EnsureRadarrConfig(client *radarr.Client) ([]HealthIssue, error) {
	issues, err := CheckRadarrConfig(client)
	if err != nil {
		return nil, err
	}
	fixed, err := FixRadarrIssues(client, issues, false)
	if err != nil {
		return fixed, err
	}
	remaining, err := CheckRadarrConfig(client)
	if err != nil {
		return fixed, fmt.Errorf("verifying radarr compatibility: %w", err)
	}
	if len(remaining) > 0 {
		return fixed, fmt.Errorf("radarr still incompatible: %s=%s", remaining[0].Setting, remaining[0].Current)
	}
	return fixed, nil
}
