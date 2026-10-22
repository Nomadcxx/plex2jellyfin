package wizard

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

func TestDuplicateQuitExitsDuplicatePhase(t *testing.T) {
	w := &Wizard{reader: bufio.NewReader(strings.NewReader("y\nq\n")), dryRun: true}
	analysis := &service.DuplicateAnalysis{Groups: []service.DuplicateGroup{
		{Title: "First", BestFileID: 1, Files: []service.MediaFile{{ID: 1}, {ID: 2, Path: "/first.mkv"}}},
		{Title: "Second", BestFileID: 3, Files: []service.MediaFile{{ID: 3}, {ID: 4, Path: "/second.mkv"}}},
	}}

	output, err := captureWizardOutput(t, func() error { return w.handleDuplicates(analysis) })
	if err != nil {
		t.Fatalf("handleDuplicates: %v", err)
	}
	if strings.Contains(output, "Second") || strings.Contains(output, "Would delete: /second.mkv") {
		t.Fatalf("wizard continued to the next duplicate group after quit:\n%s", output)
	}
}

func TestScatteredQuitExitsConsolidationPhase(t *testing.T) {
	w := &Wizard{reader: bufio.NewReader(strings.NewReader("y\nq\n")), dryRun: true}
	analysis := &service.ScatteredAnalysis{Items: []service.ScatteredItem{
		{Title: "First", Locations: []string{"/source1"}, TargetLocation: "/target1"},
		{Title: "Second", Locations: []string{"/source2"}, TargetLocation: "/target2"},
	}}

	output, err := captureWizardOutput(t, func() error { return w.handleScattered(analysis) })
	if err != nil {
		t.Fatalf("handleScattered: %v", err)
	}
	if strings.Contains(output, "Second") || strings.Contains(output, "Would consolidate files") {
		t.Fatalf("wizard continued to the next scattered item after quit:\n%s", output)
	}
}

func captureWizardOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = stdoutW
	runErr := run()
	_ = stdoutW.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(stdoutR)
	_ = stdoutR.Close()
	if err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	return string(output), runErr
}
