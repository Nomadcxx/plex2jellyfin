package wizard

import (
	"fmt"

	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func (w *Wizard) handleScattered(analysis *service.ScatteredAnalysis) error {
	fmt.Println("=== Step 2: Consolidation ===")

	if !w.confirm("Handle scattered media now?") {
		fmt.Println("Skipping consolidation.")
		return nil
	}
	fmt.Println()

	wouldConsolidateCount := 0
	unsupportedCount := 0
	applyAll := false

scatteredItems:
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

		if action == "a" {
			applyAll = true
		}

		switch action {
		case "m", "", "a":
			if w.dryRun {
				fmt.Println("  Would consolidate files to target location")
				wouldConsolidateCount++
			} else {
				fmt.Println("  ⚠️ Move execution is unavailable in the wizard; skipping")
				unsupportedCount++
			}
		case "skip", "s":
			fmt.Println("  Skipped")
		case "q":
			fmt.Println("Quitting consolidation step.")
			break scatteredItems
		}
		fmt.Println()
	}

	if w.dryRun {
		fmt.Printf("✅ Would consolidate %d items\n\n", wouldConsolidateCount)
	} else if unsupportedCount > 0 {
		fmt.Printf("⚠️ No items consolidated. Skipped %d item(s) because move execution is unavailable in the wizard. Run `plex2jellyfin consolidate generate` and then `execute`.\n\n", unsupportedCount)
	} else {
		fmt.Println("No items consolidated.")
	}

	return nil
}
