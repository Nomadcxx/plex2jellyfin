package postmortem

import "fmt"

func ContextMarkdown() string {
	return `# JellyWatch Postmortem Context

- CLI is for mass processing; WebUI is for targeted review.
- Auto-repair must be conservative and auditable.
- LLM output is advisory, not authoritative.
- Obfuscated filenames are folder-context or manual-review candidates, not AI rename candidates.
- Parser drift fixes should become deterministic tests.
- Every repair must have evidence, outcome, and rollback context.
`
}

func AgentPrompt(workspace, bundle string) string {
	return fmt.Sprintf(`# JellyWatch Scheduled Postmortem

Workspace:
cd %s

Objective:
Analyze the latest JellyWatch report bundle and determine:
1. What worked.
2. What failed.
3. Whether parser drift, metadata drift, duplicate pollution, or bad auto-repair occurred.
4. Which issues need patches.
5. Which auto-repairs were safe, unsafe, or should be adjusted.

Evidence bundle:
%s

Read these first:
- context.md
- summary.json
- repair-events.json
- suspicious-items.json
- jellyfin-diff.json
- parse-decisions.json
- housekeeping.json
- daemon-log-excerpt.txt
- report.md

Safety rules:
- Do not delete or rename media without explicit user approval.
- Treat LLM-only repair decisions as suspicious unless corroborated by deterministic parser, folder context, Jellyfin/provider metadata, or duplicate evidence.
- Obfuscated filenames are folder-context/manual-review candidates, not AI rename candidates.
- Parser drift should identify the rule or release marker that caused the bad parse.

Expected output:
- Findings ordered by severity.
- Concrete examples with file paths.
- False positives to suppress.
- Patch recommendations.
- Whether safe auto-repair thresholds should be tightened or relaxed.
`, workspace, bundle)
}
