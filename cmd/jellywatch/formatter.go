package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
)

//go:embed assets/header.txt
var asciiHeader string

// checkDatabasePopulated verifies that the database exists and has content.
// Returns an error with guidance if the user needs to run scan first.
func checkDatabasePopulated() error {
	dbPath := config.GetDatabasePath()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found at %s\n\n"+
			"You need to run 'jellywatch scan' first to build the database.\n"+
			"This scans your library and populates the database with media information.\n\n"+
			"Run: jellywatch scan", dbPath)
	}

	db, err := database.OpenPath(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	count, err := db.CountMediaFiles()
	if err != nil {
		return fmt.Errorf("failed to query database: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("database is empty - no media files found\n\n" +
			"You need to run 'jellywatch scan' first to populate the database.\n" +
			"This scans your configured library paths and indexes all media files.\n\n" +
			"Run: jellywatch scan")
	}

	return nil
}

// formatBytes converts bytes to human-readable format (e.g., "1.5 GB")
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// printHeader displays the ASCII header with version info
func printHeader(version string) {
	fmt.Println(asciiHeader)
	fmt.Printf("Version: %s\n\n", version)
}
