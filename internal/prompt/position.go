package prompt

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
)

func normalizePromptPosition(position string, isSubAgent bool) string {
	pos := strings.ToLower(strings.TrimSpace(position))
	if config.ValidPosition(pos) {
		return pos
	}
	if isSubAgent {
		return config.PositionWorker
	}
	return config.PositionLead
}

// PositionPrompt renders behavior guidance derived from the internal position.
// The prompt intentionally does not expose position as a user-managed concept.
func PositionPrompt(position, workerRole string, hasDelegation, canCallSupervisor bool) string {
	pos := normalizePromptPosition(position, false)
	if !config.ValidPosition(pos) {
		pos = config.PositionLead
	}

	var b strings.Builder
	switch pos {
	case config.PositionSupervisor:
		b.WriteString("# Supervisor Duties\n\n")
		b.WriteString("You are responsible for directional correctness and project control.\n")
		b.WriteString("Do NOT write code and do NOT spawn sub-agents.\n\n")
		b.WriteString("## Required Workflow\n\n")
		b.WriteString("- Review recent execution logs: `adaf log` and `adaf turn show [id]`\n")
		b.WriteString("- Verify plan and issue alignment: `adaf plan` and `adaf issues`\n")
		b.WriteString("- Inspect repository signal (status/history/diff) to detect drift early\n")
		b.WriteString("- If correction is needed, send a concrete instruction to the next step: `adaf loop message \"guidance\"`\n")
		b.WriteString("- Before finishing, you MUST publish a supervisor handoff: `adaf turn finish --built \"...\" --decisions \"...\" --challenges \"...\" --state \"...\" --issues \"...\" --next \"...\"`\n\n")

	case config.PositionManager:
		b.WriteString("# Manager Duties\n\n")
		b.WriteString("You are accountable for output quality and throughput.\n")
		b.WriteString("Do NOT write code directly. Coordinate workers for implementation, investigation, and verification.\n\n")
		b.WriteString("## Required Workflow\n\n")
		b.WriteString("- Break work into parallel worker tasks using `adaf spawn --profile ... --task ...`\n")
		b.WriteString("- Pause with `adaf wait-for-spawns` after launching independent tasks\n")
		b.WriteString("- Track and communicate with workers: `adaf spawn-status`, `adaf spawn-watch`, `adaf spawn-message`, `adaf spawn-reply`\n")
		b.WriteString("- For each writable spawn, review and land work: `adaf spawn-diff --spawn-id N` then `adaf spawn-merge --spawn-id N`\n")
		if canCallSupervisor {
			b.WriteString("- If you need supervisor direction or have no actionable manager work left, escalate: `adaf loop call-supervisor \"status + concrete ask\"`\n")
		}
		b.WriteString("- Keep plan/issues/docs current: `adaf plan`, `adaf issues`, `adaf issue create ...`, `adaf doc ...`\n")
		b.WriteString("- Before finishing, you MUST publish a manager handoff: `adaf turn finish --built \"...\" --decisions \"...\" --challenges \"...\" --state \"...\" --issues \"...\" --next \"...\"`\n\n")

	case config.PositionLead:
		b.WriteString("# Lead Duties\n\n")
		b.WriteString("You are responsible for delivering code and maintaining technical direction.\n")
		b.WriteString("You can implement work directly and, when a team is available, delegate selected sub-tasks.\n\n")
		b.WriteString("## Recommended Workflow\n\n")
		b.WriteString("- Start by checking context: `adaf plan`, `adaf issues`, `adaf log`\n")
		b.WriteString("- Implement and validate changes in your branch\n")
		if hasDelegation {
			b.WriteString("- Delegate parallelizable or investigative work via `adaf spawn --profile ... --task ...`\n")
			b.WriteString("- Merge completed worker branches with `adaf spawn-diff` + `adaf spawn-merge`\n")
		}
		b.WriteString("- Record a precise handoff at the end: `adaf turn finish --built \"...\" --decisions \"...\" --challenges \"...\" --state \"...\" --issues \"...\" --next \"...\"`\n\n")

	default:
		b.WriteString("# Worker Duties\n\n")
		if strings.TrimSpace(workerRole) != "" {
			fmt.Fprintf(&b, "Assigned role: `%s`\n\n", workerRole)
		}
		b.WriteString("Execute the assigned task and report clear results back to the parent.\n")
		b.WriteString("Do not manage loop orchestration.\n\n")
		b.WriteString("## Worker Commands\n\n")
		b.WriteString("- Ask parent for missing context: `adaf parent-ask \"question\"`\n")
		b.WriteString("- Review context/history when needed: `adaf log`, `adaf turn show [id]`\n")
		b.WriteString("- Track issues/docs as required by task: `adaf issues`, `adaf doc ...`\n")
		b.WriteString("- Publish end-of-turn handoff: `adaf turn finish --built \"...\" --decisions \"...\" --challenges \"...\" --state \"...\" --issues \"...\" --next \"...\"`\n\n")
	}

	return b.String()
}
