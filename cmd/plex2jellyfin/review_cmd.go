package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/daemon"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/organizer"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	var listOnly bool

	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review AI enhancement suggestions",
		Long:  "Review flagged AI title enhancement suggestions and approve or reject them.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(listOnly)
		},
	}

	cmd.Flags().BoolVar(&listOnly, "list", false, "List pending reviews without prompting")
	return cmd
}

func runReview(listOnly bool) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "plex2jellyfin")
	logPath := filepath.Join(configDir, "ai-enhancements.jsonl")

	flagged, err := daemon.ReadFlaggedForReview(logPath)
	if err != nil {
		return fmt.Errorf("failed to read enhancement log: %w", err)
	}

	if len(flagged) == 0 {
		fmt.Println("No items flagged for review.")
		return nil
	}

	fmt.Printf("%d items flagged for review:\n\n", len(flagged))

	enhanceLogger := daemon.NewEnhanceLogger(configDir)
	reader := bufio.NewReader(os.Stdin)

	// Load config once so approvals can actually organize files. A missing
	// or broken config is not a fatal error for listing — we still want to
	// show pending items — but it does block any approval from running.
	var cfg *config.Config
	var cfgErr error
	if !listOnly {
		cfg, cfgErr = config.Load()
		if cfgErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load config (%v); approvals will be log-only\n", cfgErr)
		}
	}

	for i, item := range flagged {
		fmt.Printf("%d. %s\n", i+1, item.File)
		fmt.Printf("   Regex: %q (confidence: %.2f)\n", item.RegexTitle, item.Confidence)
		fmt.Printf("   AI suggests: %q (confidence: %.2f)\n", item.AITitle, item.AIConfidence)
		fmt.Printf("   Category: %s\n", item.Category)

		if listOnly {
			fmt.Println()
			continue
		}

		fmt.Print("   [a]pprove  [r]eject  [s]kip: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a", "approve":
			if err := applyApproved(cfg, item); err != nil {
				fmt.Fprintf(os.Stderr, "   Approve failed: %v\n", err)
				if logErr := enhanceLogger.Log(daemon.EnhanceLogEntry{
					Action:  "review_approve_failed",
					File:    item.File,
					AITitle: item.AITitle,
					Reason:  err.Error(),
				}); logErr != nil {
					fmt.Fprintf(os.Stderr, "   Warning: failed to log error: %v\n", logErr)
				}
				fmt.Println()
				continue
			}
			if logErr := enhanceLogger.Log(daemon.EnhanceLogEntry{
				Action:  "review_approved",
				File:    item.File,
				AITitle: item.AITitle,
			}); logErr != nil {
				fmt.Fprintf(os.Stderr, "   Warning: failed to log approval: %v\n", logErr)
			}
			fmt.Printf("   Approved & organized: %s\n\n", item.AITitle)
		case "r", "reject":
			if logErr := enhanceLogger.Log(daemon.EnhanceLogEntry{
				Action: "review_rejected",
				File:   item.File,
				Reason: "user rejected",
			}); logErr != nil {
				fmt.Fprintf(os.Stderr, "   Warning: failed to log rejection: %v\n", logErr)
			}
			fmt.Printf("   Rejected.\n\n")
		default:
			fmt.Printf("   Skipped.\n\n")
		}
	}

	return nil
}

// applyApproved reconstructs an organize call from the flagged log entry
// and runs it via a locally-constructed Organizer. The entry must carry
// SourcePath, MediaType, AITitle and (for TV) AISeason/AIEpisode — the
// daemon writes these at flag time. Older entries without SourcePath are
// rejected with a clear error.
func applyApproved(cfg *config.Config, item daemon.EnhanceLogEntry) error {
	if cfg == nil {
		return fmt.Errorf("config unavailable; cannot organize")
	}
	if item.SourcePath == "" {
		return fmt.Errorf("log entry missing source_path (flagged by older daemon version); re-queue and try again")
	}
	if _, err := os.Stat(item.SourcePath); err != nil {
		return fmt.Errorf("source file no longer accessible: %w", err)
	}

	yearStr := ""
	if item.AIYear != nil {
		yearStr = fmt.Sprintf("%d", *item.AIYear)
	}

	switch strings.ToLower(item.MediaType) {
	case "tv":
		if len(cfg.Libraries.TV) == 0 {
			return fmt.Errorf("no TV libraries configured")
		}
		targetLib := item.TargetLib
		if targetLib == "" {
			targetLib = cfg.Libraries.TV[0]
		}
		org, err := organizer.NewOrganizer(cfg.Libraries.TV)
		if err != nil {
			return fmt.Errorf("build organizer: %w", err)
		}
		tv := naming.TVShowInfo{Title: item.AITitle, Year: yearStr}
		if item.AISeason != nil {
			tv.Season = *item.AISeason
		}
		if item.AIEpisode != nil {
			tv.Episode = *item.AIEpisode
		}
		result, err := org.OrganizeTVWithParsed(item.SourcePath, targetLib, tv)
		if err != nil {
			return err
		}
		if result != nil && result.Success {
			fmt.Printf("   → %s\n", result.TargetPath)
			return nil
		}
		if result != nil && result.Error != nil {
			return result.Error
		}
		return fmt.Errorf("organize returned no result")
	case "movie", "":
		if len(cfg.Libraries.Movies) == 0 {
			return fmt.Errorf("no movie libraries configured")
		}
		targetLib := item.TargetLib
		if targetLib == "" {
			targetLib = cfg.Libraries.Movies[0]
		}
		org, err := organizer.NewOrganizer(cfg.Libraries.Movies)
		if err != nil {
			return fmt.Errorf("build organizer: %w", err)
		}
		mv := naming.MovieInfo{Title: item.AITitle, Year: yearStr}
		result, err := org.OrganizeMovieWithParsed(item.SourcePath, targetLib, mv)
		if err != nil {
			return err
		}
		if result != nil && result.Success {
			fmt.Printf("   → %s\n", result.TargetPath)
			return nil
		}
		if result != nil && result.Error != nil {
			return result.Error
		}
		return fmt.Errorf("organize returned no result")
	default:
		return fmt.Errorf("unknown media type %q", item.MediaType)
	}
}
