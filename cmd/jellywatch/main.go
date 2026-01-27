package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/organizer"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/Nomadcxx/jellywatch/internal/validator"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	version        = "dev" // Set by build flags: -ldflags="-X main.version=1.0.0"
	cfgFile        string
	dryRun         bool
	verbose        bool
	keepSource     bool
	forceOverwrite bool
	timeout        time.Duration
	verifyChecksum bool
	libraryPath    string
	recursive      bool
	sonarrURL      string
	sonarrAPIKey   string
	radarrURL      string
	radarrAPIKey   string
	backendName    string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "jellywatch",
		Short: "Media file organizer for Jellyfin libraries",
		Long: `JellyWatch monitors download directories and automatically organizes
media files according to Jellyfin naming conventions.

Features:
  - Robust file transfers with timeout handling (won't hang on failing disks)
  - Automatic TV show and movie detection
  - Jellyfin-compliant naming: "Show Name (Year) S01E01.ext"
  - Sonarr integration for queue management`,
	}

	// Add custom help function to show ASCII header
	originalHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd.Name() == "jellywatch" {
			printHeader(version)
		}
		originalHelpFunc(cmd, args)
	})

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/jellywatch/config.toml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "n", false, "preview changes without moving files")

	rootCmd.AddCommand(newOrganizeCmd())
	rootCmd.AddCommand(newOrganizeFolderCmd())
	rootCmd.AddCommand(newConsolidateCmd())
	rootCmd.AddCommand(newDuplicatesCmd())
	rootCmd.AddCommand(newScanCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newValidateCmd())
	rootCmd.AddCommand(newSonarrCmd())
	rootCmd.AddCommand(newRadarrCmd())
	rootCmd.AddCommand(newWatchCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newDatabaseCmd())
	rootCmd.AddCommand(newMonitorCmd())
	rootCmd.AddCommand(newFixCmd())
	rootCmd.AddCommand(newServeCmd())
	rootCmd.AddCommand(newAuditCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newOrganizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organize <source> [target-library]",
		Short: "Organize media files according to Jellyfin standards",
		Long: `Organize media files from source directory to target library.

If source is a file, organizes that single file.
If source is a directory, organizes all media files within.

Examples:
  jellywatch organize /downloads/Silo.S02E02.mkv /media/TV
  jellywatch organize /downloads/tv/ --library /media/TV
  jellywatch organize /downloads/movies/ --library /media/Movies --dry-run`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runOrganize,
	}

	cmd.Flags().StringVarP(&libraryPath, "library", "l", "", "target library path")
	cmd.Flags().BoolVarP(&keepSource, "keep-source", "k", false, "copy instead of move (keep source files)")
	cmd.Flags().BoolVarP(&forceOverwrite, "force", "f", false, "overwrite existing files regardless of quality")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "process subdirectories")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 5*time.Minute, "transfer timeout")
	cmd.Flags().BoolVar(&verifyChecksum, "checksum", false, "verify checksum after transfer")
	cmd.Flags().StringVarP(&backendName, "backend", "b", "auto", "transfer backend: auto, pv, rsync, native")

	return cmd
}

func runOrganize(cmd *cobra.Command, args []string) error {
	source := args[0]

	target := libraryPath
	if len(args) > 1 {
		target = args[1]
	}

	if target == "" {
		cfg, err := config.Load()
		if err == nil && len(cfg.Libraries.TV) > 0 {
			target = cfg.Libraries.TV[0]
		} else {
			return fmt.Errorf("no target library specified (use --library or config file)")
		}
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("cannot access source: %w", err)
	}

	org, err := organizer.NewOrganizer(
		[]string{target},
		organizer.WithDryRun(dryRun),
		organizer.WithKeepSource(keepSource),
		organizer.WithForceOverwrite(forceOverwrite),
		organizer.WithTimeout(timeout),
		organizer.WithChecksumVerify(verifyChecksum),
		organizer.WithBackend(transfer.ParseBackend(backendName)),
	)
	if err != nil {
		return fmt.Errorf("failed to create organizer: %w", err)
	}

	if info.IsDir() {
		return organizeDirectory(org, source, target)
	}
	_, err = organizeFile(org, source, target)
	return err
}

func organizeFile(org *organizer.Organizer, source, target string) (*organizer.OrganizationResult, error) {
	filename := filepath.Base(source)

	if naming.IsTVEpisodeFilename(filename) {
		result, err := org.OrganizeTVEpisode(source, target)
		if err != nil {
			return nil, err
		}
		printResult(result)
		return result, nil
	}

	if naming.IsMovieFilename(filename) {
		result, err := org.OrganizeMovie(source, target)
		if err != nil {
			return nil, err
		}
		printResult(result)
		return result, nil
	}

	return nil, fmt.Errorf("cannot determine media type for: %s", filename)
}

func organizeDirectory(org *organizer.Organizer, source, target string) error {
	var processed, succeeded, failed, skipped int

	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if !recursive && path != source {
				return filepath.SkipDir
			}
			return nil
		}

		if !isMediaFileCheck(filepath.Ext(path)) {
			return nil
		}

		processed++
		result, err := organizeFile(org, path, target)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", filepath.Base(path), err)
		} else if result.Skipped {
			skipped++
		} else if result.Success {
			succeeded++
		} else {
			failed++
		}

		return nil
	})

	fmt.Printf("\nSummary: %d processed, %d succeeded, %d skipped, %d failed\n", processed, succeeded, skipped, failed)
	return err
}

func newOrganizeFolderCmd() *cobra.Command {
	var keepExtras bool

	cmd := &cobra.Command{
		Use:   "organize-folder <folder> [library]",
		Short: "Intelligently organize a download folder",
		Long: `Analyze and organize a download folder containing media and related files.

This command analyzes the folder to:
  - Detect the main media file (largest video)
  - Identify sample files and junk (nfo, txt, etc.)
  - Copy subtitles alongside the media
  - Remove samples and junk files after successful transfer
  - Detect incomplete archives (RAR files without extracted media)

Examples:
  jellywatch organize-folder /downloads/Movie.2024.1080p/ /media/Movies
  jellywatch organize-folder /downloads/Show.S01E05/ /media/TV --keep-extras
  jellywatch organize-folder /downloads/folder/ --dry-run`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]

			target := libraryPath
			if len(args) > 1 {
				target = args[1]
			}

			if target == "" {
				cfg, err := config.Load()
				if err == nil && len(cfg.Libraries.Movies) > 0 {
					target = cfg.Libraries.Movies[0]
				} else {
					return fmt.Errorf("no target library specified (use --library or config file)")
				}
			}

			org, err := organizer.NewOrganizer(
				[]string{target},
				organizer.WithDryRun(dryRun),
				organizer.WithKeepSource(keepSource),
				organizer.WithForceOverwrite(forceOverwrite),
				organizer.WithTimeout(timeout),
				organizer.WithChecksumVerify(verifyChecksum),
				organizer.WithBackend(transfer.ParseBackend(backendName)),
			)
			if err != nil {
				return fmt.Errorf("failed to create organizer: %w", err)
			}

			result, err := org.OrganizeFolder(source, target, keepExtras)
			if err != nil {
				return fmt.Errorf("organization failed: %w", err)
			}

			printFolderResult(result, keepExtras)

			if result.Error != nil {
				return result.Error
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&libraryPath, "library", "l", "", "target library path")
	cmd.Flags().BoolVarP(&keepSource, "keep-source", "k", false, "copy instead of move (keep source files)")
	cmd.Flags().BoolVarP(&forceOverwrite, "force", "f", false, "overwrite existing files regardless of quality")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 5*time.Minute, "transfer timeout")
	cmd.Flags().BoolVar(&verifyChecksum, "checksum", false, "verify checksum after transfer")
	cmd.Flags().StringVarP(&backendName, "backend", "b", "auto", "transfer backend: auto, pv, rsync, native")
	cmd.Flags().BoolVar(&keepExtras, "keep-extras", false, "preserve extra files (trailers, featurettes)")

	return cmd
}

func printFolderResult(result *organizer.FolderOrganizationResult, keepExtras bool) {
	if result.Analysis != nil {
		fmt.Printf("📁 Analyzed: %s\n", result.Analysis.Path)
		fmt.Printf("   Type: %s\n", result.Analysis.MediaType.String())
		if result.Analysis.MainMediaFile != nil {
			fmt.Printf("   Main: %s\n", result.Analysis.MainMediaFile.Name)
		}
		fmt.Printf("   Files: %d media, %d samples, %d junk, %d subtitles\n",
			len(result.Analysis.MediaFiles),
			len(result.Analysis.SampleFiles),
			len(result.Analysis.JunkFiles),
			len(result.Analysis.SubtitleFiles))
	}

	if result.MediaResult != nil {
		printResult(result.MediaResult)
	}

	if len(result.SubtitlesCopied) > 0 {
		fmt.Printf("📝 Subtitles copied: %d\n", len(result.SubtitlesCopied))
		if verbose {
			for _, s := range result.SubtitlesCopied {
				fmt.Printf("   - %s\n", s)
			}
		}
	}

	if len(result.JunkRemoved) > 0 {
		fmt.Printf("🗑  Junk removed: %d\n", len(result.JunkRemoved))
		if verbose {
			for _, j := range result.JunkRemoved {
				fmt.Printf("   - %s\n", j)
			}
		}
	}

	if len(result.SamplesRemoved) > 0 {
		fmt.Printf("🗑  Samples removed: %d\n", len(result.SamplesRemoved))
	}

	if len(result.ExtrasSkipped) > 0 && !keepExtras {
		fmt.Printf("⏭  Extras skipped: %d\n", len(result.ExtrasSkipped))
		if verbose {
			for _, e := range result.ExtrasSkipped {
				fmt.Printf("   - %s\n", e)
			}
		}
	}

	if result.Error != nil {
		fmt.Printf("✗ Error: %v\n", result.Error)
	}
}

func printResult(result *organizer.OrganizationResult) {
	if result.Skipped {
		fmt.Printf("⊘ skipped %s\n", filepath.Base(result.SourcePath))
		if verbose {
			fmt.Printf("  Reason: %s\n", result.SkipReason)
		}
		return
	}

	if result.Success {
		action := "moved"
		if keepSource {
			action = "copied"
		}
		if dryRun {
			action = "would " + action[:len(action)-1]
		}
		fmt.Printf("✓ %s %s\n", action, filepath.Base(result.SourcePath))
		if verbose {
			fmt.Printf("  → %s\n", result.TargetPath)
			if result.BytesCopied > 0 {
				fmt.Printf("  %s in %s\n", formatBytes(result.BytesCopied), result.Duration)
			}
			if result.SourceQuality != nil {
				fmt.Printf("  Quality: %s\n", result.SourceQuality.String())
			}
		}
	} else {
		fmt.Printf("✗ %s: %v\n", filepath.Base(result.SourcePath), result.Error)
	}
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <path>",
		Short: "Validate media files follow Jellyfin naming conventions",
		Long: `Check if media files follow Jellyfin naming conventions.

Reports issues like:
  - Missing year in parentheses
  - Release group markers in filename
  - Wrong season folder structure

Examples:
  jellywatch validate /media/TV/Silo/
  jellywatch validate /media/Movies/ --recursive`,
		Args: cobra.ExactArgs(1),
		RunE: runValidate,
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", true, "validate subdirectories")

	return cmd
}

func runValidate(cmd *cobra.Command, args []string) error {
	path := args[0]

	v := validator.NewValidator()

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access path: %w", err)
	}

	var valid, invalid int

	validate := func(filePath string) {
		result, err := v.ValidateFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "? %s: %v\n", filepath.Base(filePath), err)
			return
		}

		if result.Valid {
			valid++
			if verbose {
				fmt.Printf("✓ %s\n", filepath.Base(filePath))
			}
		} else {
			invalid++
			fmt.Printf("✗ %s\n", filepath.Base(filePath))
			for _, issue := range result.Issues {
				fmt.Printf("  - %s\n", issue)
			}
			if result.ExpectedName != "" {
				fmt.Printf("  Expected: %s\n", result.ExpectedName)
			}
		}
	}

	if !info.IsDir() {
		validate(path)
	} else {
		filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !recursive && filepath.Dir(p) != path {
				return nil
			}
			if isMediaFileCheck(filepath.Ext(p)) {
				validate(p)
			}
			return nil
		})
	}

	fmt.Printf("\nValidation: %d valid, %d invalid\n", valid, invalid)

	if invalid > 0 {
		return fmt.Errorf("%d files have naming issues", invalid)
	}
	return nil
}

func newSonarrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sonarr",
		Short: "Sonarr integration commands",
		Long:  `Commands for interacting with Sonarr API.`,
	}

	cmd.PersistentFlags().StringVar(&sonarrURL, "url", "http://localhost:8989", "Sonarr URL")
	cmd.PersistentFlags().StringVar(&sonarrAPIKey, "api-key", "", "Sonarr API key")

	cmd.AddCommand(newSonarrStatusCmd())
	cmd.AddCommand(newSonarrQueueCmd())
	cmd.AddCommand(newSonarrClearCmd())
	cmd.AddCommand(newSonarrImportCmd())

	return cmd
}

func getSonarrClient(cmd *cobra.Command) (*sonarr.Client, error) {
	apiKey := sonarrAPIKey
	url := sonarrURL

	if apiKey == "" {
		apiKey = os.Getenv("SONARR_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("Sonarr API key required (--api-key or SONARR_API_KEY env)")
	}

	return sonarr.NewClient(sonarr.Config{
		URL:     url,
		APIKey:  apiKey,
		Timeout: 30 * time.Second,
	}), nil
}

func newSonarrStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check Sonarr connection status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getSonarrClient(cmd)
			if err != nil {
				return err
			}

			status, err := client.GetSystemStatus()
			if err != nil {
				return fmt.Errorf("cannot connect to Sonarr: %w", err)
			}

			fmt.Printf("✓ Connected to %s\n", status.AppName)
			fmt.Printf("  Version: %s\n", status.Version)
			fmt.Printf("  Branch: %s\n", status.Branch)
			fmt.Printf("  URL: %s\n", sonarrURL)
			return nil
		},
	}
}

func newSonarrQueueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "queue",
		Short: "List Sonarr download queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getSonarrClient(cmd)
			if err != nil {
				return err
			}

			queue, err := client.GetQueue(1, 50)
			if err != nil {
				return err
			}

			if queue.TotalRecords == 0 {
				fmt.Println("Queue is empty")
				return nil
			}

			fmt.Printf("Queue: %d items\n\n", queue.TotalRecords)
			for _, item := range queue.Records {
				status := item.Status
				if item.TrackedDownloadStatus != "" {
					status = item.TrackedDownloadStatus
				}

				icon := "○"
				if status == "completed" || status == "ok" {
					icon = "✓"
				} else if status == "warning" || status == "error" {
					icon = "✗"
				}

				fmt.Printf("%s [%d] %s (%s)\n", icon, item.ID, item.Title, status)

				if verbose && len(item.StatusMessages) > 0 {
					for _, msg := range item.StatusMessages {
						fmt.Printf("    %s\n", msg.Title)
						for _, m := range msg.Messages {
							fmt.Printf("      - %s\n", m)
						}
					}
				}
			}
			return nil
		},
	}
}

func newSonarrClearCmd() *cobra.Command {
	var blocklist bool

	cmd := &cobra.Command{
		Use:   "clear-stuck",
		Short: "Clear stuck items from Sonarr queue",
		Long: `Remove items with import errors from Sonarr queue.

These are typically items that failed to import due to permission errors,
disk issues, or other problems that JellyWatch can handle.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getSonarrClient(cmd)
			if err != nil {
				return err
			}

			stuck, err := client.GetStuckItems()
			if err != nil {
				return err
			}

			if len(stuck) == 0 {
				fmt.Println("No stuck items in queue")
				return nil
			}

			fmt.Printf("Found %d stuck items:\n", len(stuck))
			for _, item := range stuck {
				fmt.Printf("  - [%d] %s (%s)\n", item.ID, item.Title, item.TrackedDownloadStatus)
			}

			if dryRun {
				fmt.Println("\n[dry-run] Would remove these items")
				return nil
			}

			count, err := client.ClearStuckItems(blocklist)
			if err != nil {
				return err
			}

			fmt.Printf("\n✓ Cleared %d items from queue\n", count)
			return nil
		},
	}

	cmd.Flags().BoolVar(&blocklist, "blocklist", false, "add releases to blocklist")

	return cmd
}

func newSonarrImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <path>",
		Short: "Trigger Sonarr import scan for a path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getSonarrClient(cmd)
			if err != nil {
				return err
			}

			path := args[0]
			fmt.Printf("Triggering import scan for: %s\n", path)

			resp, err := client.TriggerDownloadedEpisodesScan(path)
			if err != nil {
				return err
			}

			fmt.Printf("✓ Command queued (ID: %d)\n", resp.ID)

			if verbose {
				fmt.Println("Waiting for completion...")
				if err := client.WaitForCommand(resp.ID, 5*time.Minute); err != nil {
					return fmt.Errorf("import failed: %w", err)
				}
				fmt.Println("✓ Import completed")
			}

			return nil
		},
	}
}

func newRadarrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "radarr",
		Short: "Radarr integration commands",
		Long:  `Commands for interacting with Radarr API.`,
	}

	cmd.PersistentFlags().StringVar(&radarrURL, "url", "http://localhost:7878", "Radarr URL")
	cmd.PersistentFlags().StringVar(&radarrAPIKey, "api-key", "", "Radarr API key")

	cmd.AddCommand(newRadarrStatusCmd())
	cmd.AddCommand(newRadarrQueueCmd())
	cmd.AddCommand(newRadarrClearCmd())
	cmd.AddCommand(newRadarrImportCmd())
	cmd.AddCommand(newRadarrMoviesCmd())

	return cmd
}

func getRadarrClient(cmd *cobra.Command) (*radarr.Client, error) {
	apiKey := radarrAPIKey
	url := radarrURL

	if apiKey == "" {
		apiKey = os.Getenv("RADARR_API_KEY")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("Radarr API key required (--api-key or RADARR_API_KEY env)")
	}

	return radarr.NewClient(radarr.Config{
		URL:     url,
		APIKey:  apiKey,
		Timeout: 30 * time.Second,
	}), nil
}

func newRadarrStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check Radarr connection status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getRadarrClient(cmd)
			if err != nil {
				return err
			}

			status, err := client.GetSystemStatus()
			if err != nil {
				return fmt.Errorf("cannot connect to Radarr: %w", err)
			}

			fmt.Printf("✓ Connected to Radarr\n")
			fmt.Printf("  Version: %s\n", status.Version)
			fmt.Printf("  Branch: %s\n", status.Branch)
			fmt.Printf("  URL: %s\n", radarrURL)
			return nil
		},
	}
}

func newRadarrQueueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "queue",
		Short: "List Radarr download queue",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getRadarrClient(cmd)
			if err != nil {
				return err
			}

			queue, err := client.GetQueue(1, 50)
			if err != nil {
				return err
			}

			if queue.TotalRecords == 0 {
				fmt.Println("Queue is empty")
				return nil
			}

			fmt.Printf("Queue: %d items\n\n", queue.TotalRecords)
			for _, item := range queue.Records {
				status := item.Status
				if item.TrackedDownloadStatus != "" {
					status = item.TrackedDownloadStatus
				}

				icon := "○"
				if status == "completed" || status == "ok" {
					icon = "✓"
				} else if status == "warning" || status == "error" {
					icon = "✗"
				}

				fmt.Printf("%s [%d] %s (%s)\n", icon, item.ID, item.Title, status)

				if verbose && len(item.StatusMessages) > 0 {
					for _, msg := range item.StatusMessages {
						fmt.Printf("    %s\n", msg.Title)
						for _, m := range msg.Messages {
							fmt.Printf("      - %s\n", m)
						}
					}
				}
			}
			return nil
		},
	}
}

func newRadarrClearCmd() *cobra.Command {
	var blocklist bool

	cmd := &cobra.Command{
		Use:   "clear-stuck",
		Short: "Clear stuck items from Radarr queue",
		Long: `Remove items with import errors from Radarr queue.

These are typically items that failed to import due to permission errors,
disk issues, or other problems that JellyWatch can handle.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getRadarrClient(cmd)
			if err != nil {
				return err
			}

			stuck, err := client.GetStuckItems()
			if err != nil {
				return err
			}

			if len(stuck) == 0 {
				fmt.Println("No stuck items in queue")
				return nil
			}

			fmt.Printf("Found %d stuck items:\n", len(stuck))
			for _, item := range stuck {
				fmt.Printf("  - [%d] %s (%s)\n", item.ID, item.Title, item.TrackedDownloadStatus)
			}

			if dryRun {
				fmt.Println("\n[dry-run] Would remove these items")
				return nil
			}

			count, err := client.ClearStuckItems(blocklist)
			if err != nil {
				return err
			}

			fmt.Printf("\n✓ Cleared %d items from queue\n", count)
			return nil
		},
	}

	cmd.Flags().BoolVar(&blocklist, "blocklist", false, "add releases to blocklist")

	return cmd
}

func newRadarrImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <path>",
		Short: "Trigger Radarr import scan for a path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getRadarrClient(cmd)
			if err != nil {
				return err
			}

			path := args[0]
			fmt.Printf("Triggering import scan for: %s\n", path)

			resp, err := client.TriggerDownloadedMoviesScan(path)
			if err != nil {
				return err
			}

			fmt.Printf("✓ Command queued (ID: %d)\n", resp.ID)

			if verbose {
				fmt.Println("Waiting for completion...")
				if err := client.WaitForCommand(resp.ID, 5*time.Minute); err != nil {
					return fmt.Errorf("import failed: %w", err)
				}
				fmt.Println("✓ Import completed")
			}

			return nil
		},
	}
}

func newRadarrMoviesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "movies",
		Short: "List movies in Radarr library",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := getRadarrClient(cmd)
			if err != nil {
				return err
			}

			movies, err := client.GetMovies()
			if err != nil {
				return err
			}

			fmt.Printf("Movies: %d total\n\n", len(movies))

			for _, movie := range movies {
				icon := "○"
				if movie.HasFile {
					icon = "✓"
				}
				fmt.Printf("%s %s (%d)\n", icon, movie.Title, movie.Year)
				if verbose {
					fmt.Printf("    Path: %s\n", movie.Path)
					if movie.HasFile {
						fmt.Printf("    Size: %s\n", formatBytes(movie.SizeOnDisk))
					}
				}
			}
			return nil
		},
	}
}

func newWatchCmd() *cobra.Command {
	var tvLibrary string
	var movieLibrary string
	var debounce time.Duration

	cmd := &cobra.Command{
		Use:   "watch <directory>",
		Short: "Watch directory for new media files",
		Long: `Monitor a directory and automatically organize new media files.

When a new media file is detected, it will be organized according to
Jellyfin naming conventions and moved to the appropriate library.

Examples:
  jellywatch watch /downloads/tv --tv-library /media/TV
  jellywatch watch /downloads --tv-library /media/TV --movie-library /media/Movies
  jellywatch watch /downloads -n  # dry-run mode`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			watchDir := args[0]

			cfg, _ := config.Load()

			tvLibs := []string{}
			if tvLibrary != "" {
				tvLibs = []string{tvLibrary}
			} else if cfg != nil && len(cfg.Libraries.TV) > 0 {
				tvLibs = cfg.Libraries.TV
			}

			movieLibs := []string{}
			if movieLibrary != "" {
				movieLibs = []string{movieLibrary}
			} else if cfg != nil && len(cfg.Libraries.Movies) > 0 {
				movieLibs = cfg.Libraries.Movies
			}

			if len(tvLibs) == 0 && len(movieLibs) == 0 {
				return fmt.Errorf("no libraries configured (use --tv-library or --movie-library, or set in config)")
			}

			handler, err := daemon.NewMediaHandler(daemon.MediaHandlerConfig{
				TVLibraries:  tvLibs,
				MovieLibs:    movieLibs,
				DebounceTime: debounce,
				DryRun:       dryRun,
				Timeout:      timeout,
				Backend:      transfer.ParseBackend(backendName),
			})
			if err != nil {
				return fmt.Errorf("failed to create media handler: %w", err)
			}
			defer handler.Shutdown()

			w, err := watcher.NewWatcher(handler, dryRun)
			if err != nil {
				return fmt.Errorf("creating watcher: %w", err)
			}
			defer w.Close()

			if err := w.Watch([]string{watchDir}); err != nil {
				return fmt.Errorf("setting up watch: %w", err)
			}

			fmt.Printf("Watching: %s\n", watchDir)
			if len(tvLibs) > 0 {
				fmt.Printf("TV Library: %s\n", tvLibs[0])
			}
			if len(movieLibs) > 0 {
				fmt.Printf("Movie Library: %s\n", movieLibs[0])
			}
			if dryRun {
				fmt.Println("Mode: DRY RUN (no files will be moved)")
			}
			fmt.Println("\nPress Ctrl+C to stop")

			return w.Start()
		},
	}

	cmd.Flags().StringVar(&tvLibrary, "tv-library", "", "target TV library path")
	cmd.Flags().StringVar(&movieLibrary, "movie-library", "", "target movie library path")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "transfer timeout")
	cmd.Flags().DurationVar(&debounce, "debounce", 10*time.Second, "debounce time before processing")
	cmd.Flags().StringVarP(&backendName, "backend", "b", "auto", "transfer backend: auto, pv, rsync, native")

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			printHeader(version)
		},
	}
}

// isMediaFileCheck checks if extension is a media file type
func isMediaFileCheck(ext string) bool {
	ext = strings.ToLower(ext)
	mediaExts := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
		".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
	}
	return mediaExts[ext]
}

// newConsolidateCmd creates the consolidate command
func newConsolidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "consolidate",
		Short: "Consolidate scattered media files",
		Long: `Find and consolidate media files scattered across multiple locations.

This command identifies when the same title exists in multiple library folders
and generates plans to consolidate them into a single location.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "--generate",
		Short: "Generate consolidation plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Consolidate generate - not yet implemented")
			return nil
		},
	})

	return cmd
}

// newDuplicatesCmd creates the duplicates command
func newDuplicatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "duplicates",
		Short: "Find and manage duplicate files",
		Long: `Identify duplicate media files across your libraries.

This command finds files that are duplicates of each other and
can help you clean up wasted space.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "--generate",
		Short: "Generate duplicate plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Duplicates generate - not yet implemented")
			return nil
		},
	})

	return cmd
}

