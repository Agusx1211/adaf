// Package prompt builds system prompts for agent sessions.
package prompt

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

const maxRecentTurns = 5

func buildSubAgentPrompt(opts BuildOpts) string {
	role := config.EffectiveRole(opts.Role, opts.GlobalCfg)
	if opts.Profile != nil && opts.Profile.Description != "" {
		role = opts.Profile.Description
	}

	var b strings.Builder
	fmt.Fprintf(&b, "You are a sub-agent working as a %s. If you need to communicate with your parent agent use `adaf parent-ask \"question\"`.\n\n", role)

	if opts.ReadOnly {
		b.WriteString("You are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n\n")
	}

	if len(opts.IssueIDs) > 0 && opts.Store != nil {
		b.WriteString("## Assigned Issues\n\n")
		for _, issID := range opts.IssueIDs {
			iss, err := opts.Store.GetIssue(issID)
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description)
		}
		b.WriteString("\n")
	}

	b.WriteString(opts.Task)

	return b.String()
}

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

	// PlanID overrides the plan context for this prompt.
	// If empty and Task is also empty, ActivePlanID from Project is used.
	PlanID string

	// Profile is the profile of the agent being prompted.
	Profile *config.Profile

	// Role overrides the role prompt for this run context.
	// When empty, prompt generation defaults to the configured default role.
	Role string

	// GlobalCfg provides access to all profiles (for spawnable info).
	GlobalCfg *config.GlobalConfig

	// Task overrides the objective section when set (used by spawned agents).
	Task string

	// ReadOnly appends read-only instructions.
	ReadOnly bool

	// ParentTurnID, if >0, provides parent context to the child.
	ParentTurnID int

	// CurrentTurnID, when >0, enables live runtime spawn context in prompts.
	CurrentTurnID int

	// Messages from parent agent (for child prompts).
	Messages []store.SpawnMessage

	// IssueIDs are specific issues assigned to this sub-agent by the parent.
	IssueIDs []int

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
	Summary  string // child's final output
	ReadOnly bool   // whether this was a read-only scout
	Branch   string // worktree branch (empty for read-only)
}

// Build constructs a prompt from project context and role configuration.
func Build(opts BuildOpts) (string, error) {
	if opts.ParentTurnID > 0 {
		return buildSubAgentPrompt(opts), nil
	}

	var b strings.Builder

	s := opts.Store
	project := opts.Project

	if project == nil {
		return "Explore the codebase and address any open issues.", nil
	}

	effectivePlanID := strings.TrimSpace(opts.PlanID)
	if effectivePlanID == "" && opts.Task == "" {
		effectivePlanID = strings.TrimSpace(project.ActivePlanID)
	}

	var plan *store.Plan
	if effectivePlanID == "" && opts.Task == "" {
		// Fallback path: use currently active plan if available.
		loaded, _ := s.LoadPlan()
		if loaded != nil && loaded.ID != "" {
			if loaded.Status == "" {
				loaded.Status = "active"
			}
			if loaded.Status == "active" {
				plan = loaded
				effectivePlanID = loaded.ID
			}
		}
	}
	if effectivePlanID != "" && (plan == nil || plan.ID != effectivePlanID) {
		plan, _ = s.GetPlan(effectivePlanID)
		if plan != nil {
			if plan.Status == "" {
				plan.Status = "active"
			}
			if plan.Status != "active" {
				plan = nil
				effectivePlanID = ""
			}
		} else {
			effectivePlanID = ""
		}
	}

	allTurns, _ := s.ListTurns()

	// Role-specific header.
	if opts.Profile != nil {
		roleSection := RolePrompt(opts.Profile, opts.Role, opts.GlobalCfg)
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

	// Compute effective role for role-conditional prompt sections.
	effectiveRole := ""
	if opts.Profile != nil {
		effectiveRole = config.EffectiveStepRole(opts.Role, opts.GlobalCfg)
	}

	// Rules.
	b.WriteString("# Rules\n\n")
	if opts.ParentTurnID == 0 {
		b.WriteString("- **You are fully autonomous. There is no human in the loop.** No one will answer questions, grant permissions, or provide clarification. " +
			"You must make all decisions yourself. Do not ask for confirmation or direction — decide and act. " +
			"If something is ambiguous, use your best judgment and move forward.\n")
	}
	roleCanWrite := config.CanWriteCode(effectiveRole, opts.GlobalCfg)
	if roleCanWrite {
		b.WriteString("- Write code, run tests, and ensure everything compiles before finishing.\n")
	} else {
		b.WriteString("- Review work, check progress, and provide guidance to running agents. Do NOT write or modify code.\n")
	}
	b.WriteString("- Focus on one coherent unit of work. Stop when the current phase (or a meaningful increment of it) is complete.\n")
	if !opts.ReadOnly && roleCanWrite {
		b.WriteString("- **You own your repository. Commit your work.** Do not leave changes uncommitted. " +
			"Every time you finish a coherent piece of work, create a git commit. " +
			"Uncommitted changes are invisible to scouts, other agents, and future sessions. " +
			"Commit early and often — your worktree is yours alone.\n")
	}
	b.WriteString("- Do NOT read or write files inside the `.adaf/` directory directly. " +
		"Use `adaf` CLI commands instead (`adaf issues`, `adaf log`, `adaf plan`, etc.). " +
		"The `.adaf/` directory structure may change and direct access will be restricted in the future.\n")
	b.WriteString("\n")

	// Context.
	b.WriteString("# Context\n\n")

	isSubAgent := opts.ParentTurnID > 0

	// Recent session logs — only for top-level agents, not sub-agents.
	if !isSubAgent && len(allTurns) > 0 {
		totalTurns := len(allTurns)
		start := totalTurns - maxRecentTurns
		if start < 0 {
			start = 0
		}
		recentTurns := allTurns[start:]

		b.WriteString("## Recent Session Logs\n\n")
		if totalTurns > len(recentTurns) {
			fmt.Fprintf(&b, "There are %d session logs total. Showing the %d most recent:\n\n", totalTurns, len(recentTurns))
		}

		for i, turn := range recentTurns {
			isLatest := i == len(recentTurns)-1

			fmt.Fprintf(&b, "### Turn #%d", turn.ID)
			if !turn.Date.IsZero() {
				fmt.Fprintf(&b, " (%s", turn.Date.Format("2006-01-02"))
				if turn.Agent != "" {
					fmt.Fprintf(&b, ", %s", turn.Agent)
				}
				b.WriteString(")")
			}
			b.WriteString("\n")

			if isLatest {
				// Most recent turn: full detail.
				if turn.Objective != "" {
					fmt.Fprintf(&b, "- Objective: %s\n", turn.Objective)
				}
				if turn.WhatWasBuilt != "" {
					fmt.Fprintf(&b, "- Built: %s\n", turn.WhatWasBuilt)
				}
				if turn.KeyDecisions != "" {
					fmt.Fprintf(&b, "- Key decisions: %s\n", turn.KeyDecisions)
				}
				if turn.Challenges != "" {
					fmt.Fprintf(&b, "- Challenges: %s\n", turn.Challenges)
				}
				if turn.CurrentState != "" {
					fmt.Fprintf(&b, "- Current state: %s\n", turn.CurrentState)
				}
				if turn.KnownIssues != "" {
					fmt.Fprintf(&b, "- Known issues: %s\n", turn.KnownIssues)
				}
				if turn.NextSteps != "" {
					fmt.Fprintf(&b, "- Next steps: %s\n", turn.NextSteps)
				}
				if turn.BuildState != "" {
					fmt.Fprintf(&b, "- Build state: %s\n", turn.BuildState)
				}
			} else {
				// Older turns: condensed view.
				if turn.Objective != "" {
					fmt.Fprintf(&b, "- Objective: %s\n", turn.Objective)
				}
				if turn.WhatWasBuilt != "" {
					fmt.Fprintf(&b, "- Built: %s\n", turn.WhatWasBuilt)
				}
			}
			b.WriteString("\n")
		}
	}

	// Issues section.
	if isSubAgent && len(opts.IssueIDs) > 0 {
		// Sub-agent with assigned issues: show only those.
		b.WriteString("## Assigned Issues\n")
		for _, issID := range opts.IssueIDs {
			iss, err := s.GetIssue(issID)
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description)
		}
		b.WriteString("\n")
	} else if !isSubAgent {
		// Top-level agent: show all open issues.
		var issues []store.Issue
		if effectivePlanID != "" {
			issues, _ = s.ListIssuesForPlan(effectivePlanID)
		} else {
			issues, _ = s.ListSharedIssues()
		}
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
	}

	// Messages from parent.
	if len(opts.Messages) > 0 {
		b.WriteString("## Messages from Parent\n\n")
		for _, msg := range opts.Messages {
			fmt.Fprintf(&b, "- [%s] %s\n", msg.CreatedAt.Format("15:04:05"), msg.Content)
		}
		b.WriteString("\n")
	}

	// Parent communication commands are available whenever this session is a spawned sub-agent.
	if opts.ParentTurnID > 0 {
		b.WriteString(parentCommunicationSection())
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
	var runningSpawns []store.SpawnRecord
	if opts.Store != nil && opts.CurrentTurnID > 0 {
		if records, err := opts.Store.SpawnsByParent(opts.CurrentTurnID); err == nil {
			for _, rec := range records {
				if isDelegationActiveSpawnStatus(rec.Status) {
					runningSpawns = append(runningSpawns, rec)
				}
			}
		}
	}
	b.WriteString(delegationSection(opts.Delegation, opts.GlobalCfg, runningSpawns))

	// Wait results from a previous wait-for-spawns cycle.
	if len(opts.WaitResults) > 0 {
		b.WriteString("## Spawn Wait Results\n\n")
		b.WriteString("The spawns you waited for have completed:\n\n")
		for _, wr := range opts.WaitResults {
			b.WriteString(formatWaitResultInfo(wr))
		}
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

	// Objective — placed last so the agent's immediate focus lands here.
	b.WriteString("# Objective\n\n")
	b.WriteString("Project: " + project.Name + "\n\n")
	if plan != nil {
		fmt.Fprintf(&b, "You are working on plan: **%s**", plan.ID)
		if plan.Title != "" {
			fmt.Fprintf(&b, " — %s", plan.Title)
		}
		b.WriteString("\n\n")
	}

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
			if roleCanWrite {
				fmt.Fprintf(&b, "Your task is to work on phase **%s: %s**.\n\n", currentPhase.ID, currentPhase.Title)
			} else {
				fmt.Fprintf(&b, "Review progress on phase **%s: %s**. Check if agents completed the work correctly. Verify the build passes. Provide guidance or flag issues.\n\n", currentPhase.ID, currentPhase.Title)
			}
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

	return b.String(), nil
}

// formatWaitResultInfo formats a single WaitResultInfo for the prompt.
func formatWaitResultInfo(wr WaitResultInfo) string {
	var b strings.Builder

	fmt.Fprintf(&b, "### Spawn #%d (profile=%s", wr.SpawnID, wr.Profile)
	if wr.ReadOnly {
		b.WriteString(", read-only")
	} else if wr.Branch != "" {
		fmt.Fprintf(&b, ", branch=%s", wr.Branch)
	}
	b.WriteString(") — ")
	b.WriteString(wr.Status)
	if wr.ExitCode != 0 {
		fmt.Fprintf(&b, " (exit_code=%d)", wr.ExitCode)
	}
	b.WriteString("\n\n")

	body := wr.Summary
	if body == "" {
		body = wr.Result
	}
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	} else {
		b.WriteString("(no output captured)\n\n")
	}

	return b.String()
}

func isDelegationActiveSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
		return false
	default:
		return true
	}
}
