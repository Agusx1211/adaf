// Package prompt builds system prompts for agent sessions.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

// maxAgentsMDSize is the upper bound (in bytes) for embedding AGENTS.md into
// the prompt. Files larger than this are referenced instead of inlined.
const maxAgentsMDSize = 16 * 1024

const (
	maxLastSessionObjective = 500
	maxLastSessionField     = 300
)

// LoopPromptContext provides loop-specific context for prompt generation.
type LoopPromptContext struct {
	LoopName     string
	Cycle        int
	StepIndex    int
	TotalSteps   int
	Instructions string // step-specific custom instructions
	CanStop      bool
	CanMessage   bool
	CanPushover  bool
	Messages     []store.LoopMessage // unseen messages from other steps
	RunID        int
}

// BuildOpts configures prompt generation.
type BuildOpts struct {
	Store   *store.Store
	Project *store.ProjectConfig

	// Profile is the profile of the agent being prompted.
	Profile *config.Profile

	// GlobalCfg provides access to all profiles (for spawnable info).
	GlobalCfg *config.GlobalConfig

	// Task overrides the objective section when set (used by spawned agents).
	Task string

	// ReadOnly appends read-only instructions.
	ReadOnly bool

	// ParentSessionID, if >0, provides parent context to the child.
	ParentSessionID int

	// SupervisorNotes are injected into the prompt.
	SupervisorNotes []store.SupervisorNote

	// Messages from parent agent (for child prompts).
	Messages []store.SpawnMessage

	// LoopContext provides loop-specific context (nil if not in a loop).
	LoopContext *LoopPromptContext

	// Delegation describes spawn capabilities for this agent's context.
	// If nil, the agent cannot spawn sub-agents.
	Delegation *config.DelegationConfig

	// WaitResults from a previous wait-for-spawns cycle, injected into the prompt.
	WaitResults []WaitResultInfo

	// Handoffs from previous loop step, injected into the prompt.
	Handoffs []store.HandoffInfo
}

// WaitResultInfo describes the result of a spawn that was waited on.
type WaitResultInfo struct {
	SpawnID  int
	Profile  string
	Status   string
	ExitCode int
	Result   string
}

// Build constructs a prompt from project context and role configuration.
func Build(opts BuildOpts) (string, error) {
	var b strings.Builder

	s := opts.Store
	project := opts.Project

	if project == nil {
		return "Explore the codebase and address any open issues.", nil
	}

	plan, _ := s.LoadPlan()
	latest, _ := s.LatestLog()

	// Role-specific header.
	if opts.Profile != nil {
		roleSection := RolePrompt(opts.Profile, opts.GlobalCfg)
		if roleSection != "" {
			b.WriteString(roleSection)
			b.WriteString("\n")
		}
	}

	// Read-only mode.
	if opts.ReadOnly {
		b.WriteString(ReadOnlyPrompt())
		b.WriteString("\n")
	}

	// Objective.
	b.WriteString("# Objective\n\n")
	b.WriteString("Project: " + project.Name + "\n\n")

	if opts.Task != "" {
		b.WriteString(opts.Task + "\n\n")
	} else {
		var currentPhase *store.PlanPhase
		if plan != nil && len(plan.Phases) > 0 {
			for i := range plan.Phases {
				p := &plan.Phases[i]
				if p.Status == "not_started" || p.Status == "in_progress" {
					currentPhase = p
					break
				}
			}
		}

		if currentPhase != nil {
			fmt.Fprintf(&b, "Your task is to work on phase **%s: %s**.\n\n", currentPhase.ID, currentPhase.Title)
			if currentPhase.Description != "" {
				b.WriteString(currentPhase.Description + "\n\n")
			}
		} else if plan != nil && plan.Title != "" {
			b.WriteString("All planned phases are complete. Look for remaining open issues or improvements.\n\n")
		} else {
			b.WriteString("No plan is set. Explore the codebase and address any open issues.\n\n")
		}

		// Neighboring phases.
		if currentPhase != nil && plan != nil && len(plan.Phases) > 1 {
			b.WriteString("## Neighboring Phases\n")
			for i, p := range plan.Phases {
				if p.ID == currentPhase.ID {
					if i > 0 {
						prev := plan.Phases[i-1]
						fmt.Fprintf(&b, "- Previous: [%s] %s: %s\n", prev.Status, prev.ID, prev.Title)
					}
					fmt.Fprintf(&b, "- **Current: [%s] %s: %s**\n", p.Status, p.ID, p.Title)
					if i < len(plan.Phases)-1 {
						next := plan.Phases[i+1]
						fmt.Fprintf(&b, "- Next: [%s] %s: %s\n", next.Status, next.ID, next.Title)
					}
					break
				}
			}
			b.WriteString("\n")
		}
	}

	// Rules.
	b.WriteString("# Rules\n\n")
	b.WriteString("- Write code, run tests, and ensure everything compiles before finishing.\n")
	b.WriteString("- Focus on one coherent unit of work. Stop when the current phase (or a meaningful increment of it) is complete.\n")
	b.WriteString("- Do NOT read or write files inside the `.adaf/` directory directly. " +
		"Use `adaf` CLI commands instead (`adaf issues`, `adaf log`, `adaf plan`, etc.). " +
		"The `.adaf/` directory structure may change and direct access will be restricted in the future.\n")
	b.WriteString("\n")

	// Context.
	b.WriteString("# Context\n\n")

	if latest != nil {
		b.WriteString("## Last Session\n")
		if latest.Objective != "" {
			fmt.Fprintf(&b, "- Objective: %s\n", summarizeForContext(latest.Objective, maxLastSessionObjective))
		}
		if latest.WhatWasBuilt != "" {
			fmt.Fprintf(&b, "- Built: %s\n", summarizeForContext(latest.WhatWasBuilt, maxLastSessionField))
		}
		if latest.NextSteps != "" {
			fmt.Fprintf(&b, "- Next steps: %s\n", summarizeForContext(latest.NextSteps, maxLastSessionField))
		}
		if latest.KnownIssues != "" {
			fmt.Fprintf(&b, "- Known issues: %s\n", summarizeForContext(latest.KnownIssues, maxLastSessionField))
		}
		b.WriteString("\n")
	}

	issues, _ := s.ListIssues()
	var relevant []store.Issue
	for _, iss := range issues {
		if iss.Status == "open" || iss.Status == "in_progress" {
			relevant = append(relevant, iss)
		}
	}
	if len(relevant) > 0 {
		b.WriteString("## Open Issues\n")
		for _, iss := range relevant {
			fmt.Fprintf(&b, "- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description)
		}
		b.WriteString("\n")
	}

	// Supervisor notes.
	if len(opts.SupervisorNotes) > 0 {
		b.WriteString("## Supervisor Notes\n\n")
		for _, note := range opts.SupervisorNotes {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", note.CreatedAt.Format("15:04:05"), note.Author, note.Note)
		}
		b.WriteString("\n")
	}

	// Messages from parent.
	if len(opts.Messages) > 0 {
		b.WriteString("## Messages from Parent\n\n")
		for _, msg := range opts.Messages {
			fmt.Fprintf(&b, "- [%s] %s\n", msg.CreatedAt.Format("15:04:05"), msg.Content)
		}
		b.WriteString("\n")
	}

	// Loop context.
	if lc := opts.LoopContext; lc != nil {
		b.WriteString("# Loop Context\n\n")
		fmt.Fprintf(&b, "You are running in loop %q, cycle %d, step %d of %d",
			lc.LoopName, lc.Cycle+1, lc.StepIndex+1, lc.TotalSteps)
		if opts.Profile != nil {
			fmt.Fprintf(&b, " (profile %q)", opts.Profile.Name)
		}
		b.WriteString(".\n")

		if lc.Instructions != "" {
			b.WriteString("\n" + lc.Instructions + "\n")
		}

		b.WriteString("\n")

		if lc.CanStop {
			b.WriteString("You can stop this loop when objectives are met by running: `adaf loop stop`\n\n")
		}
		if lc.CanMessage {
			b.WriteString("You can send a message to subsequent steps by running: `adaf loop message \"your message\"`\n\n")
		}
		if lc.CanPushover {
			b.WriteString("## Pushover Notifications\n\n")
			b.WriteString("You can send push notifications to the user's device by running:\n")
			b.WriteString("`adaf loop notify \"<title>\" \"<message>\"` — Send a notification (default priority: normal)\n")
			b.WriteString("`adaf loop notify --priority 1 \"<title>\" \"<message>\"` — Send a high-priority notification\n\n")
			b.WriteString("**Character limits:**\n")
			b.WriteString("- Title: max 250 characters (keep it short and descriptive)\n")
			b.WriteString("- Message: max 1024 characters (concise summary of what happened)\n\n")
			b.WriteString("**Priority levels:** -2 (lowest), -1 (low), 0 (normal), 1 (high)\n\n")
			b.WriteString("**When to use:** Send notifications for significant events like task completion, errors requiring attention, or milestones reached. Do NOT spam — only send when genuinely useful.\n\n")
		}

		if len(lc.Messages) > 0 {
			b.WriteString("## Messages from Previous Steps\n\n")
			for _, msg := range lc.Messages {
				fmt.Fprintf(&b, "- [step %d, %s]: %s\n", msg.StepIndex, msg.CreatedAt.Format("15:04:05"), msg.Content)
			}
			b.WriteString("\n")
		}
	}

	// Delegation section.
	b.WriteString(delegationSection(opts.Delegation, opts.GlobalCfg))

	// Wait results from a previous wait-for-spawns cycle.
	if len(opts.WaitResults) > 0 {
		b.WriteString("## Spawn Wait Results\n\n")
		b.WriteString("The spawns you waited for have completed:\n\n")
		for _, wr := range opts.WaitResults {
			fmt.Fprintf(&b, "- Spawn #%d (profile=%s): status=%s, exit_code=%d", wr.SpawnID, wr.Profile, wr.Status, wr.ExitCode)
			if wr.Result != "" {
				fmt.Fprintf(&b, " — %s", wr.Result)
			}
			b.WriteString("\n")
		}
		b.WriteString("\nReview their diffs with `adaf spawn-diff --spawn-id N` and merge or reject as needed.\n\n")
	}

	// Handoff section.
	if len(opts.Handoffs) > 0 {
		b.WriteString("## Inherited Running Agents (Handoff)\n\n")
		b.WriteString("The previous step handed off these running sub-agents to you:\n\n")
		for _, h := range opts.Handoffs {
			fmt.Fprintf(&b, "- Spawn #%d (profile: %s", h.SpawnID, h.Profile)
			if h.Speed != "" {
				fmt.Fprintf(&b, ", speed: %s", h.Speed)
			}
			fmt.Fprintf(&b, ") — Task: %q\n", h.Task)
			fmt.Fprintf(&b, "  Status: %s", h.Status)
			if h.Branch != "" {
				fmt.Fprintf(&b, ", Branch: %s", h.Branch)
			}
			b.WriteString("\n")
			fmt.Fprintf(&b, "  Use `adaf spawn-status --spawn-id %d` to check progress.\n\n", h.SpawnID)
		}
		b.WriteString("You can manage these exactly like your own spawns (wait, diff, merge, reject).\n\n")
	}

	// AGENTS.md.
	workDir := project.RepoPath
	if workDir != "" {
		agentsMD := filepath.Join(workDir, "AGENTS.md")
		if info, err := os.Stat(agentsMD); err == nil {
			if info.Size() <= maxAgentsMDSize {
				if data, err := os.ReadFile(agentsMD); err == nil {
					b.WriteString("# AGENTS.md\n\n")
					b.WriteString("The repository includes an AGENTS.md with instructions for AI agents. Follow these:\n\n")
					b.WriteString(string(data))
					b.WriteString("\n\n")
				}
			} else {
				b.WriteString("# AGENTS.md\n\n")
				fmt.Fprintf(&b, "The repository includes an AGENTS.md file at `%s`. Read it before starting work — it contains important instructions for AI agents.\n\n", agentsMD)
			}
		}
	}

	return b.String(), nil
}

func summarizeForContext(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
