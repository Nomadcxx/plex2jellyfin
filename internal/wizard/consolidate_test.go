package wizard

import (
	"bufio"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func TestScatteredMoveReportsSkippedItems(t *testing.T) {
	for _, tc := range []struct {
		name         string
		input        string
		itemCount    int
		wantSummary  string
		wantComplete string
	}{
		{name: "move", input: "y\nm\n", itemCount: 1, wantSummary: "Skipped 1 item", wantComplete: "Consolidated 1 items"},
		{name: "all", input: "y\na\n", itemCount: 2, wantSummary: "Skipped 2 item", wantComplete: "Consolidated 2 items"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := make([]service.ScatteredItem, tc.itemCount)
			for i := range items {
				items[i] = service.ScatteredItem{Title: "Example", Locations: []string{"/source"}, TargetLocation: "/target"}
			}
			w := &Wizard{reader: bufio.NewReader(strings.NewReader(tc.input))}
			output, err := captureWizardOutput(t, func() error {
				return w.handleScattered(&service.ScatteredAnalysis{Items: items})
			})
			if err != nil {
				t.Fatalf("handleScattered: %v", err)
			}
			if !strings.Contains(output, tc.wantSummary) {
				t.Fatalf("summary should report unsupported moves as skipped, got:\n%s", output)
			}
			if strings.Contains(output, tc.wantComplete) {
				t.Fatalf("unsupported moves were reported as completed:\n%s", output)
			}
		})
	}
}
