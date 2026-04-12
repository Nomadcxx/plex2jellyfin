package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
)

const (
	barWidth         = 40
	progressInterval = 10
	barFull          = "█"
	barEmpty         = "░"
)

type ScanProgressBar struct {
	label      string
	filesCount int
	aiCount    int
	lastRender int
}

func NewScanProgressBar(label string) *ScanProgressBar {
	fmt.Printf("\nScanning %s\n", label)
	return &ScanProgressBar{label: label}
}

func (b *ScanProgressBar) UpdateDelta(filesInLibrary, aiInLibrary int) {
	b.filesCount = filesInLibrary
	b.aiCount = aiInLibrary
	if b.filesCount-b.lastRender >= progressInterval {
		b.render(false)
		b.lastRender = b.filesCount
	}
}

func (b *ScanProgressBar) Finish(filesScanned, aiCount int) {
	b.filesCount = filesScanned
	b.aiCount = aiCount
	b.render(true)
	fmt.Println()
}

func (b *ScanProgressBar) render(full bool) {
	var bar string
	if full {
		bar = strings.Repeat(barFull, barWidth)
	} else {
		filled := (b.filesCount / progressInterval) % (barWidth + 1)
		if filled > barWidth-1 {
			filled = barWidth - 1
		}
		bar = strings.Repeat(barFull, filled) + strings.Repeat(barEmpty, barWidth-filled)
	}

	aiSuffix := ""
	if b.aiCount > 0 {
		aiSuffix = fmt.Sprintf(" │ AI: %d ✓", b.aiCount)
	}

	status := "..."
	if full {
		status = "100%"
	}

	fmt.Printf("\r  %s %s │ %d files%s", bar, status, b.filesCount, aiSuffix)
}

type ScanSummaryData struct {
	Result          *scanner.ScanResult
	Duration        time.Duration
	DupSets         int
	ScatteredSeries int
	LowConfidence   int
}

func RenderScanSummary(d ScanSummaryData) {
	r := d.Result

	// Build main metrics rows
	var rows [][]string
	rows = append(rows, []string{"Files scanned", fmt.Sprintf("%d", r.FilesScanned)})
	rows = append(rows, []string{"New entries", fmt.Sprintf("%d", r.FilesAdded)})
	rows = append(rows, []string{"Updated", fmt.Sprintf("%d", r.FilesUpdated)})

	if r.AITriggered > 0 {
		aiVal := fmt.Sprintf("%d", r.AITriggered)
		if r.AICacheHits > 0 {
			aiVal += fmt.Sprintf(" (%d cached)", r.AICacheHits)
		}
		rows = append(rows, []string{"AI-assisted", aiVal})
	}

	// Create main table
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(borderStyle).
		Headers("Metric", "Value").
		Rows(rows...)

	fmt.Printf("\n%s\n", t.Render())
	fmt.Printf("⏱  Duration: %.1fs\n", d.Duration.Seconds())

	// Render issues if any
	hasIssues := d.DupSets > 0 || d.ScatteredSeries > 0 || d.LowConfidence > 0
	if hasIssues {
		fmt.Println("\n⚠️  Issues Found:")
		var issueRows [][]string

		step := 1
		if d.DupSets > 0 {
			label := fmt.Sprintf("%d duplicate set", d.DupSets)
			if d.DupSets > 1 {
				label += "s"
			}
			issueRows = append(issueRows, []string{
				fmt.Sprintf("%d.", step),
				label,
				"jellywatch duplicates generate",
			})
			step++
		}
		if d.ScatteredSeries > 0 {
			issueRows = append(issueRows, []string{
				fmt.Sprintf("%d.", step),
				fmt.Sprintf("%d scattered", d.ScatteredSeries),
				"jellywatch consolidate generate",
			})
			step++
		}
		if d.LowConfidence > 0 {
			issueRows = append(issueRows, []string{
				fmt.Sprintf("%d.", step),
				fmt.Sprintf("%d low-confidence", d.LowConfidence),
				"jellywatch audit --generate",
			})
		}

		issueTable := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(borderStyle).
			Headers("Step", "Issue", "Resolution").
			Rows(issueRows...)

		fmt.Printf("%s\n", issueTable.Render())
	} else {
		fmt.Println("\n✓  No issues found")
	}
}

