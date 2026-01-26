package main

import (
	_ "embed"
	"fmt"
)

//go:embed assets/header.txt
var asciiHeader string

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
