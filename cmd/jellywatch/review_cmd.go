package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/daemon"
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
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "jellywatch")
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
			if logErr := enhanceLogger.Log(daemon.EnhanceLogEntry{
				Action:  "review_approved",
				File:    item.File,
				AITitle: item.AITitle,
			}); logErr != nil {
				fmt.Fprintf(os.Stderr, "   Warning: failed to log approval: %v\n", logErr)
			}
			fmt.Printf("   Approved: %s\n\n", item.AITitle)
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
