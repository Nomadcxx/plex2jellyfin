package wizard

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/service"
)

// Wizard provides an interactive cleanup experience
type Wizard struct {
	db      *database.MediaDB
	service *service.CleanupService
	reader  *bufio.Reader
	dryRun  bool
	autoYes bool
}

// Options configures wizard behavior
type Options struct {
	DryRun          bool
	AutoYes         bool
	DuplicatesOnly  bool
	ConsolidateOnly bool
}

// New creates a new wizard
func New(db *database.MediaDB, opts Options) *Wizard {
	return &Wizard{
		db:      db,
		service: service.NewCleanupService(db),
		reader:  bufio.NewReader(os.Stdin),
		dryRun:  opts.DryRun,
		autoYes: opts.AutoYes,
	}
}

// Run executes the wizard
func (w *Wizard) Run(opts Options) error {
	fmt.Println("🧹 Jellywatch Fix Wizard")
	if w.dryRun {
		fmt.Println("   (DRY RUN - no changes will be made)")
	}
	fmt.Println()

	// Analyze
	dupAnalysis, err := w.service.AnalyzeDuplicates()
	if err != nil {
		return fmt.Errorf("failed to analyze duplicates: %w", err)
	}

	scatterAnalysis, err := w.service.AnalyzeScattered()
	if err != nil {
		return fmt.Errorf("failed to analyze scattered media: %w", err)
	}

	// Summary
	if dupAnalysis.TotalGroups > 0 {
		fmt.Printf("Found %d duplicate groups (%s reclaimable)\n",
			dupAnalysis.TotalGroups, formatBytes(dupAnalysis.ReclaimableBytes))
	}
	if scatterAnalysis.TotalItems > 0 {
		fmt.Printf("Found %d scattered media items (need consolidation)\n",
			scatterAnalysis.TotalItems)
	}

	if dupAnalysis.TotalGroups == 0 && scatterAnalysis.TotalItems == 0 {
		fmt.Println("\n✨ No issues found - your library is clean!")
		return nil
	}

	fmt.Println()

	// Handle duplicates
	if !opts.ConsolidateOnly && dupAnalysis.TotalGroups > 0 {
		if err := w.handleDuplicates(dupAnalysis); err != nil {
			return err
		}
	}

	// Handle scattered
	if !opts.DuplicatesOnly && scatterAnalysis.TotalItems > 0 {
		if err := w.handleScattered(scatterAnalysis); err != nil {
			return err
		}
	}

	return nil
}

// prompt asks for user input
func (w *Wizard) prompt(message string) string {
	fmt.Print(message)
	if w.autoYes {
		fmt.Println("y")
		return "y"
	}
	response, _ := w.reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(response))
}

// confirm asks yes/no question
func (w *Wizard) confirm(message string) bool {
	response := w.prompt(message + " [Y/n]: ")
	return response == "" || response == "y" || response == "yes"
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
