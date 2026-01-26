package wizard

import (
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/service"
)

func (w *Wizard) handleScattered(analysis *service.ScatteredAnalysis) error {
	fmt.Println("=== Step 2: Consolidation ===")

	if !w.confirm("Handle scattered media now?") {
		fmt.Println("Skipping consolidation.")
		return nil
	}
	fmt.Println()

	consolidatedCount := 0
	applyAll := false

	for i, item := range analysis.Items {
		yearStr := ""
		if item.Year != nil {
			yearStr = fmt.Sprintf(" (%d)", *item.Year)
		}

		fmt.Printf("[%d/%d] %s%s - scattered across %d locations\n",
			i+1, len(analysis.Items), item.Title, yearStr, len(item.Locations))

		fmt.Printf("  Target: %s\n", item.TargetLocation)
		for _, loc := range item.Locations {
			if loc != item.TargetLocation {
				fmt.Printf("  Move from: %s\n", loc)
			}
		}
		fmt.Println()

		var action string
		if applyAll {
			action = "m"
		} else {
			action = w.prompt("  [M]ove / [S]kip / [A]ll remaining / [Q]uit: ")
		}

		switch action {
		case "m", "":
			if w.dryRun {
				fmt.Println("  Would consolidate files to target location")
			} else {
				// TODO: Implement actual move using consolidate package
				fmt.Println("  ⚠️ Consolidation execution not yet implemented")
			}
			consolidatedCount++
		case "skip", "s":
			fmt.Println("  Skipped")
		case "a":
			applyAll = true
			if w.dryRun {
				fmt.Println("  Would consolidate files to target location")
			} else {
				fmt.Println("  ⚠️ Consolidation execution not yet implemented")
			}
			consolidatedCount++
		case "q":
			fmt.Println("Quitting consolidation step.")
			break
		}
		fmt.Println()
	}

	if w.dryRun {
		fmt.Printf("✅ Would consolidate %d items\n\n", consolidatedCount)
	} else {
		fmt.Printf("✅ Consolidated %d items\n\n", consolidatedCount)
	}

	return nil
}
