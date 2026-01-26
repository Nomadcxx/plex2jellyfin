package main

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/wizard"
	"github.com/spf13/cobra"
)

func newFixCmd() *cobra.Command {
	var (
		dryRun          bool
		autoYes         bool
		duplicatesOnly  bool
		consolidateOnly bool
	)

	cmd := &cobra.Command{
		Use:   "fix",
		Short: "Interactive wizard to clean up your media library",
		Long: `Launch an interactive wizard that guides you through fixing issues
in your media library.

The wizard handles:
  1. Duplicates - Delete inferior quality copies to save space
  2. Scattered media - Consolidate files spread across multiple locations

Examples:
  jellywatch fix                    # Full interactive wizard
  jellywatch fix --dry-run          # Preview what would happen
  jellywatch fix --yes              # Auto-accept all suggestions
  jellywatch fix --duplicates-only  # Only handle duplicates
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFix(dryRun, autoYes, duplicatesOnly, consolidateOnly)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without making changes")
	cmd.Flags().BoolVarP(&autoYes, "yes", "y", false, "Auto-accept all suggestions")
	cmd.Flags().BoolVar(&duplicatesOnly, "duplicates-only", false, "Only handle duplicates")
	cmd.Flags().BoolVar(&consolidateOnly, "consolidate-only", false, "Only handle consolidation")

	return cmd
}

func runFix(dryRun, autoYes, duplicatesOnly, consolidateOnly bool) error {
	db, err := database.Open()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	opts := wizard.Options{
		DryRun:          dryRun,
		AutoYes:         autoYes,
		DuplicatesOnly:  duplicatesOnly,
		ConsolidateOnly: consolidateOnly,
	}

	w := wizard.New(db, opts)
	return w.Run(opts)
}
