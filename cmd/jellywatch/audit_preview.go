package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Nomadcxx/jellywatch/internal/plans"
)

const auditCardWidth = 88

func truncateRunes(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func confidenceToken(conf float64) string {
	return fmt.Sprintf("%.0f%%", conf*100)
}

func renderAuditCard(index int, total int, item *plans.AuditItem, action *plans.AuditAction) string {
	innerWidth := auditCardWidth - 4 // border + padding
	lineStyle := lipgloss.NewStyle().Width(innerWidth)

	beforeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	afterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	labelStyle := lipgloss.NewStyle().Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)

	statusLabel := okStyle.Render("OK")
	if item.SkipReason != "" {
		statusLabel = skipStyle.Render("SKIP")
	} else if action == nil {
		statusLabel = warnStyle.Render("WARN")
	}

	beforeFile := truncateRunes(filepath.Base(item.Path), innerWidth-10)
	beforeTitle := truncateRunes(item.Title, innerWidth-16)
	beforeConfidence := confidenceToken(item.Confidence)

	afterFile := beforeFile
	afterTitle := beforeTitle
	afterConfidence := beforeConfidence
	if action != nil {
		afterFile = truncateRunes(filepath.Base(action.NewPath), innerWidth-10)
		afterTitle = truncateRunes(action.NewTitle, innerWidth-16)
		afterConfidence = confidenceToken(action.Confidence)
	}

	lines := []string{
		lineStyle.Render(fmt.Sprintf("%d/%d  %s", index+1, total, statusLabel)),
		lineStyle.Render(labelStyle.Render("BEFORE")),
		lineStyle.Render(beforeStyle.Render(fmt.Sprintf("File:  %s", beforeFile))),
		lineStyle.Render(beforeStyle.Render(fmt.Sprintf("Title: %s (%s)", beforeTitle, beforeConfidence))),
		lineStyle.Render(strings.Repeat("-", innerWidth)),
		lineStyle.Render(labelStyle.Render("AFTER")),
		lineStyle.Render(afterStyle.Render(fmt.Sprintf("File:  %s", afterFile))),
		lineStyle.Render(afterStyle.Render(fmt.Sprintf("Title: %s (%s)", afterTitle, afterConfidence))),
	}

	if item.SkipReason != "" {
		reason := truncateRunes(item.SkipReason, innerWidth-8)
		lines = append(lines, lineStyle.Render(skipStyle.Render(fmt.Sprintf("SKIP: %s", reason))))
	} else if action != nil && action.Reasoning != "" {
		reason := truncateRunes(action.Reasoning, innerWidth-9)
		lines = append(lines, lineStyle.Render(afterStyle.Render(fmt.Sprintf("Reason: %s", reason))))
	}

	cardStyle := lipgloss.NewStyle().
		Width(auditCardWidth).
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)

	return cardStyle.Render(strings.Join(lines, "\n"))
}

func renderAuditPlanDetails(plan *plans.AuditPlan, showAllFiles bool) string {
	var b strings.Builder
	b.WriteString("\nFiles:\n")

	actionIdx := 0
	changesShown := 0

	for i, item := range plan.Items {
		var action *plans.AuditAction
		hasAction := item.SkipReason == "" && actionIdx < len(plan.Actions)
		isSkipped := item.SkipReason != ""

		if hasAction {
			action = &plan.Actions[actionIdx]
			actionIdx++
			changesShown++
		}

		// Hide items that have neither action nor skip reason unless user asked for all.
		if !showAllFiles && !hasAction && !isSkipped {
			continue
		}

		b.WriteString(renderAuditCard(i, len(plan.Items), &item, action))
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("\nSummary: %d changes to apply\n", changesShown))
	b.WriteString(fmt.Sprintf("AI candidates: %d | AI calls: %d | AI errors: %d\n",
		plan.Summary.AICandidateCount, plan.Summary.AITotalCalls, plan.Summary.AIErrorCount))
	if plan.Summary.DeterministicSkipped > 0 || plan.Summary.ManualReviewSkipped > 0 {
		b.WriteString(fmt.Sprintf("Pre-AI skipped: %d deterministic | %d manual-review\n",
			plan.Summary.DeterministicSkipped, plan.Summary.ManualReviewSkipped))
	}
	if !showAllFiles && plan.Summary.FilesToSkip > 0 {
		unchanged := plan.Summary.TotalFiles - plan.Summary.FilesToRename - plan.Summary.FilesToSkip
		b.WriteString(fmt.Sprintf("Unchanged: %d | Skipped: %d (use --show-all to include all files)\n", unchanged, plan.Summary.FilesToSkip))
	}
	if plan.Summary.FilesToRename > 0 {
		b.WriteString("\nRun 'jellywatch audit execute' to apply changes\n")
	}

	return b.String()
}

type auditPreviewModel struct {
	viewport viewport.Model
	content  string
	ready    bool
}

func newAuditPreviewModel(content string) auditPreviewModel {
	return auditPreviewModel{content: content}
}

func (m auditPreviewModel) Init() tea.Cmd {
	return nil
}

func (m auditPreviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		headerHeight := 2
		footerHeight := 1
		height := msg.Height - headerHeight - footerHeight
		if height < 1 {
			height = 1
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width, height)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = height
		}
		m.viewport.SetContent(m.content)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c", "enter":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m auditPreviewModel) View() string {
	if !m.ready {
		return "Loading audit preview..."
	}

	header := lipgloss.NewStyle().Bold(true).Render("Audit Dry Run Preview")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Use arrows/PageUp/PageDown to scroll. Press q to exit.")
	return header + "\n" + m.viewport.View() + "\n" + help
}

func isInteractiveTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func runAuditDryRunPreview(content string) error {
	if !isInteractiveTerminal() {
		return fmt.Errorf("non-interactive terminal")
	}

	program := tea.NewProgram(newAuditPreviewModel(content))
	_, err := program.Run()
	return err
}
