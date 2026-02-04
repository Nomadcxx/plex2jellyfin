package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func newCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up cruft files and empty directories",
		Long: `Remove non-media files (samples, .nfo, .txt, images) and empty directories
from your media libraries.

Subcommands:
  cruft   - Delete sample files and metadata cruft
  empty   - Delete empty directories only`,
	}

	cmd.AddCommand(newCleanupCruftCmd())
	cmd.AddCommand(newCleanupEmptyCmd())

	return cmd
}

func newCleanupCruftCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "cruft",
		Short: "Delete sample files, .nfo, .txt, and other non-media cruft",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupCruft(dryRun)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview without deleting")
	return cmd
}

func newCleanupEmptyCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "empty",
		Short: "Delete empty directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCleanupEmpty(dryRun)
		},
	}

	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Preview without deleting")
	return cmd
}

// Cruft file extensions that should be deleted
var cruftExtensions = map[string]bool{
	".nfo": true, ".txt": true, ".jpg": true, ".jpeg": true,
	".png": true, ".gif": true, ".srt": true, ".sub": true,
	".idx": true, ".sfv": true, ".md5": true, ".url": true,
}

// Maximum size for sample files (50MB)
const maxSampleSize = 50 * 1024 * 1024

func runCleanupCruft(dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	allRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)
	if len(allRoots) == 0 {
		return fmt.Errorf("no libraries configured")
	}

	if dryRun {
		fmt.Println("Scanning for cruft files (dry run)...")
	} else {
		fmt.Println("Cleaning cruft files...")
	}

	var totalSize int64
	var fileCount int

	for _, root := range allRoots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			if isCruft(path, info) {
				if dryRun {
					fmt.Printf("  [DELETE] %s (%s)\n", path, formatBytes(info.Size()))
				} else {
					if err := os.Remove(path); err != nil {
						fmt.Printf("  Failed: %s: %v\n", path, err)
						return nil
					}
					fmt.Printf("  Deleted: %s\n", filepath.Base(path))
				}
				totalSize += info.Size()
				fileCount++
			}
			return nil
		})
		if err != nil {
			fmt.Printf("Warning: error scanning %s: %v\n", root, err)
		}
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Would delete %d files, reclaiming %s\n", fileCount, formatBytes(totalSize))
		fmt.Println("\nRun without --dry-run to delete these files.")
	} else {
		fmt.Printf("Deleted %d files, reclaimed %s\n", fileCount, formatBytes(totalSize))
	}

	return nil
}

func runCleanupEmpty(dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	allRoots := append(cfg.Libraries.TV, cfg.Libraries.Movies...)
	if len(allRoots) == 0 {
		return fmt.Errorf("no libraries configured")
	}

	if dryRun {
		fmt.Println("Scanning for empty directories (dry run)...")
	} else {
		fmt.Println("Removing empty directories...")
	}

	var dirCount int

	for _, root := range allRoots {
		var dirs []string
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() && path != root {
				dirs = append(dirs, path)
			}
			return nil
		})

		// Process deepest first
		for i := len(dirs) - 1; i >= 0; i-- {
			dir := dirs[i]
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}

			if len(entries) == 0 {
				if dryRun {
					fmt.Printf("  [DELETE] %s/\n", dir)
				} else {
					if err := os.Remove(dir); err == nil {
						fmt.Printf("  Removed: %s/\n", dir)
					}
				}
				dirCount++
			}
		}
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Would remove %d empty directories\n", dirCount)
	} else {
		fmt.Printf("Removed %d empty directories\n", dirCount)
	}

	return nil
}

func isCruft(path string, info os.FileInfo) bool {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	// Known cruft extensions
	if cruftExtensions[ext] {
		return true
	}

	// Sample files (small video files with "sample" in name or path)
	if isVideoFile(path) && info.Size() < maxSampleSize {
		if strings.Contains(name, "sample") ||
			strings.Contains(strings.ToLower(filepath.Dir(path)), "sample") {
			return true
		}
	}

	return false
}
