package postmortem

import (
	"strings"
	"testing"
)

func TestAgentPromptContainsSafetyRulesAndEvidenceFiles(t *testing.T) {
	prompt := AgentPrompt("/home/nomadx/Documents/jellywatch", "~/.config/jellywatch/reports/latest")

	for _, want := range []string{
		"cd /home/nomadx/Documents/jellywatch",
		"summary.json",
		"unknown-seasons.json",
		"repair-events.json",
		"suspicious-items.json",
		"Do not delete or rename media without explicit user approval",
		"LLM-only repair decisions as suspicious",
		"Obfuscated filenames are folder-context/manual-review candidates",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
