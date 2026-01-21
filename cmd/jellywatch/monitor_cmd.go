package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/spf13/cobra"
)

func newMonitorCmd() *cobra.Command {
	var (
		days        int
		showDetails bool
		filterMethod string
		showErrors  bool
	)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "View activity logs from jellywatchd",
		Long: `View and analyze activity logs from the jellywatchd daemon.

Shows a summary of file organization operations including:
  - Success/failure rates
  - Parse method breakdown (regex vs AI)
  - Data transfer statistics
  - Recent errors

Examples:
  jellywatch monitor              # Show last 3 days
  jellywatch monitor --days 7     # Show last 7 days
  jellywatch monitor --details    # Show detailed JSON entries
  jellywatch monitor --errors     # Show only failed operations
  jellywatch monitor --method ai  # Filter by parse method`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMonitor(days, showDetails, filterMethod, showErrors)
		},
	}

	cmd.Flags().IntVarP(&days, "days", "d", 3, "Days of logs to show")
	cmd.Flags().BoolVar(&showDetails, "details", false, "Show detailed JSON output")
	cmd.Flags().StringVarP(&filterMethod, "method", "m", "", "Filter by parse method (regex|ai|cache)")
	cmd.Flags().BoolVarP(&showErrors, "errors", "e", false, "Show only failed operations")

	return cmd
}

func runMonitor(days int, showDetails bool, filterMethod string, showErrors bool) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "jellywatch")
	activityDir := filepath.Join(configDir, "activity")

	if _, err := os.Stat(activityDir); os.IsNotExist(err) {
		fmt.Println("No activity logs found.")
		fmt.Println("Activity logging starts when jellywatchd processes files.")
		return nil
	}

	entries, err := loadActivityEntries(activityDir, days)
	if err != nil {
		return fmt.Errorf("failed to load activity logs: %w", err)
	}

	if len(entries) == 0 {
		fmt.Printf("No activity entries found in the last %d days.\n", days)
		return nil
	}

	// Apply filters
	entries = filterActivityEntries(entries, filterMethod, showErrors)

	if len(entries) == 0 {
		fmt.Println("No entries match the specified filters.")
		return nil
	}

	if showDetails {
		for _, entry := range entries {
			data, _ := json.MarshalIndent(entry, "", "  ")
			fmt.Println(string(data))
		}
		return nil
	}

	printActivitySummary(entries, days)
	return nil
}

func loadActivityEntries(activityDir string, days int) ([]activity.Entry, error) {
	var entries []activity.Entry

	cutoff := time.Now().AddDate(0, 0, -days)

	dirEntries, err := os.ReadDir(activityDir)
	if err != nil {
		return nil, err
	}

	for _, dirEntry := range dirEntries {
		if !strings.HasPrefix(dirEntry.Name(), "activity-") || !strings.HasSuffix(dirEntry.Name(), ".jsonl") {
			continue
		}

		// Parse date from filename
		datePart := strings.TrimPrefix(dirEntry.Name(), "activity-")
		datePart = strings.TrimSuffix(datePart, ".jsonl")

		fileDate, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}

		if fileDate.Before(cutoff) {
			continue
		}

		filePath := filepath.Join(activityDir, dirEntry.Name())
		fileEntries, err := loadFileEntries(filePath)
		if err != nil {
			continue // Skip files we can't read
		}

		entries = append(entries, fileEntries...)
	}

	// Sort by timestamp
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

func loadFileEntries(filePath string) ([]activity.Entry, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var entries []activity.Entry
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry activity.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed lines
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func filterActivityEntries(entries []activity.Entry, filterMethod string, showErrors bool) []activity.Entry {
	if filterMethod == "" && !showErrors {
		return entries
	}

	var filtered []activity.Entry
	for _, entry := range entries {
		if filterMethod != "" && string(entry.ParseMethod) != filterMethod {
			continue
		}
		if showErrors && entry.Success {
			continue
		}
		filtered = append(filtered, entry)
	}

	return filtered
}

func printActivitySummary(entries []activity.Entry, days int) {
	var totalSuccess, totalFailed int
	var methodCounts = make(map[string]int)
	var totalBytes int64
	var totalDuration int64
	var sonarrNotified, radarrNotified int

	for _, entry := range entries {
		if entry.Success {
			totalSuccess++
			methodCounts[string(entry.ParseMethod)]++
			totalBytes += entry.Bytes
			totalDuration += entry.DurationMs
			if entry.SonarrNotified {
				sonarrNotified++
			}
			if entry.RadarrNotified {
				radarrNotified++
			}
		} else {
			totalFailed++
		}
	}

	total := totalSuccess + totalFailed

	fmt.Printf("\n=== Activity Summary (%d days) ===\n\n", days)
	fmt.Printf("Total operations: %d\n", total)
	fmt.Printf("  Successful: %d\n", totalSuccess)
	fmt.Printf("  Failed:     %d\n", totalFailed)

	if total > 0 {
		successRate := float64(totalSuccess) / float64(total) * 100
		fmt.Printf("  Success rate: %.1f%%\n", successRate)
	}

	if totalSuccess > 0 {
		fmt.Printf("\n--- Parse Methods ---\n")
		methods := []string{"regex", "ai", "cache"}
		for _, method := range methods {
			count := methodCounts[method]
			if count > 0 {
				pct := float64(count) / float64(totalSuccess) * 100
				fmt.Printf("  %s: %d (%.1f%%)\n", method, count, pct)
			}
		}

		fmt.Printf("\n--- Data Transfer ---\n")
		fmt.Printf("  Total: %.2f GB\n", float64(totalBytes)/(1024*1024*1024))
		fmt.Printf("  Avg per op: %.2f MB\n", float64(totalBytes)/float64(totalSuccess)/(1024*1024))

		fmt.Printf("\n--- Performance ---\n")
		avgMs := totalDuration / int64(totalSuccess)
		fmt.Printf("  Avg duration: %d ms\n", avgMs)
		fmt.Printf("  Total time: %s\n", time.Duration(totalDuration)*time.Millisecond)

		fmt.Printf("\n--- Notifications ---\n")
		fmt.Printf("  Sonarr: %d\n", sonarrNotified)
		fmt.Printf("  Radarr: %d\n", radarrNotified)
	}

	// Show recent failures
	var failures []activity.Entry
	for i := len(entries) - 1; i >= 0 && len(failures) < 10; i-- {
		if !entries[i].Success {
			failures = append(failures, entries[i])
		}
	}

	if len(failures) > 0 {
		fmt.Printf("\n--- Recent Failures (last %d) ---\n", len(failures))
		for _, entry := range failures {
			fmt.Printf("  [%s] %s\n", entry.Timestamp.Format("2006-01-02 15:04"), filepath.Base(entry.Source))
			if entry.Error != "" {
				fmt.Printf("    Error: %s\n", entry.Error)
			}
		}
	}

	fmt.Println()
}
