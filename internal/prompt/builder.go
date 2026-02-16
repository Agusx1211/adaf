// Package prompt builds system prompts for agent sessions.
package prompt

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

const maxRecentTurns = 5

func buildSubAgentPrompt(opts BuildOpts) (string, error) {
	role := config.EffectiveRole(opts.Role, opts.GlobalCfg)

	var b strings.Builder

	// Wrap system context in a supra-code block so the model clearly distinguishes
	// it from the task that follows. Same pattern as buildStandaloneChatContext.
	b.WriteString("Context: `````\n")

	fmt.Fprintf(&b, "You are a sub-agent working as a %s.", role)
	b.WriteString(" You were spawned by a parent agent to complete a specific task.\n\n")

	if opts.ReadOnly {
		b.WriteString("You are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n\n")
	} else {
		b.WriteString("Commit your work when you finish.\n\n")
	}

	b.WriteString("If you need to communicate with your parent agent use `adaf parent-ask \"question\"`.\n\n")

	if opts.Delegation != nil && len(opts.Delegation.Profiles) > 0 {
		b.WriteString(delegationSection(opts.Delegation, opts.GlobalCfg, nil))
	}

	b.WriteString("Your task below is your PRIMARY directive. ")
	b.WriteString("Any project-level instructions (CLAUDE.md, AGENTS.md, etc.) are background context — ")
	b.WriteString("they must NOT override or expand your task. Do exactly what the task says, nothing more.\n")

	b.WriteString("`````\n\n")

	// Task and issues go OUTSIDE the context block so the model treats them
	// as the primary instruction, not secondary context.
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

	return b.String(), nil
}

// LoopPromptContext provides loop-specific context for prompt generation.
type LoopPromptContext struct {
	LoopName      string
	Cycle         int
	StepIndex     int
	TotalSteps    int
	Instructions  string // step-specific custom instructions
	InitialPrompt string // general objective injected across all steps
	CanStop       bool
	CanMessage    bool
	CanPushover   bool
	Messages      []store.LoopMessage // unseen messages from other steps
	RunID         int
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

	// StandaloneChat enables the minimal interactive chat prompt path.
	// When true, Build() short-circuits to a focused prompt without
	// autonomous rules, session logs, or loop context.
	StandaloneChat bool

	// Skills lists which skill IDs are active for this prompt.
	// When nil (not empty slice), Build() falls back to the legacy prompt path.
	// When non-nil (including empty), Build() uses the skills-driven path.
	Skills []string
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
// When opts.Skills is non-nil, the skills-driven prompt path is used.
// When opts.Skills is nil, the legacy prompt path is used for backward compatibility.
func Build(opts BuildOpts) (string, error) {
	if opts.ParentTurnID > 0 {
		return buildSubAgentPrompt(opts)
	}

	if opts.StandaloneChat {
		return buildStandaloneChatContext(opts)
	}

	if opts.Skills != nil {
		return buildSkillsPrompt(opts)
	}

	return buildLegacyPrompt(opts)
}

// hasSkill reports whether the given skill ID is active in the skills list.
func hasSkill(skills []string, id string) bool {
	for _, s := range skills {
		if strings.EqualFold(s, id) {
			return true
		}
	}
	return false
}

// buildSkillsPrompt constructs a prompt driven by explicit skill IDs.
func buildSkillsPrompt(opts BuildOpts) (string, error) {
	var b strings.Builder

	project := opts.Project

	if project == nil {
		return "Explore the codebase and address any open issues.", nil
	}

	_, plan := resolvePlan(opts)

	// Role header (slim: title + identity + description only).
	if opts.Profile != nil {
		roleSection := RolePromptSlim(opts.Profile, opts.Role, opts.GlobalCfg)
		if roleSection != "" {
			b.WriteString(roleSection)
			b.WriteString("\n")
		}
	}

	// Resolve skills for role and read-only mode.
	effectiveRole := ""
	if opts.Profile != nil {
		effectiveRole = config.EffectiveStepRole(opts.Role, opts.GlobalCfg)
	}
	resolvedSkills := config.ResolveSkillsForRole(opts.Skills, effectiveRole, opts.ReadOnly, opts.GlobalCfg)

	// Skills section.
	if len(resolvedSkills) > 0 && opts.GlobalCfg != nil {
		b.WriteString("# Skills\n\n")
		for _, skillID := range resolvedSkills {
			sk := opts.GlobalCfg.FindSkill(skillID)
			if sk == nil {
				continue
			}
			fmt.Fprintf(&b, "## %s\n%s\n\n", sk.ID, sk.Short)
		}
	}

	// Read-only mode.
	if opts.ReadOnly && hasSkill(resolvedSkills, config.SkillReadOnly) {
		b.WriteString(ReadOnlyPrompt())
		b.WriteString("\n")
	}

	// Dynamic context sections gated by active skills.

	// Loop control.
	if hasSkill(resolvedSkills, config.SkillLoopControl) && opts.LoopContext != nil {
		b.WriteString(renderLoopContext(opts))
	}

	// Pushover.
	if hasSkill(resolvedSkills, config.SkillPushover) && opts.LoopContext != nil && opts.LoopContext.CanPushover {
		b.WriteString(renderPushover())
	}

	// Loop messages (always render if loop context has messages, regardless of skills).
	if opts.LoopContext != nil && len(opts.LoopContext.Messages) > 0 {
		b.WriteString(renderLoopMessages(opts.LoopContext.Messages))
	}

	// Delegation section.
	if hasSkill(resolvedSkills, config.SkillDelegation) {
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
	}

	// Runtime data (always, when present): wait results, handoffs.
	b.WriteString(renderWaitResults(opts.WaitResults))
	b.WriteString(renderHandoffs(opts.Handoffs))

	// Project context (lightweight — agents discover details via CLI).
	b.WriteString(renderContextSection(opts, project, plan))

	return b.String(), nil
}

// buildLegacyPrompt is the original Build() logic, used when opts.Skills is nil.
func buildLegacyPrompt(opts BuildOpts) (string, error) {
	var b strings.Builder

	s := opts.Store
	project := opts.Project

	if project == nil {
		return "Explore the codebase and address any open issues.", nil
	}

	effectivePlanID, plan := resolvePlan(opts)

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

	// Recent session logs.
	if len(allTurns) > 0 {
		b.WriteString(renderSessionLogs(allTurns))
	}

	// Issues section.
	b.WriteString(renderIssues(s, effectivePlanID))

	// Loop context.
	if opts.LoopContext != nil {
		b.WriteString(renderLoopContext(opts))

		if opts.LoopContext.CanPushover {
			b.WriteString(renderPushover())
		}

		if len(opts.LoopContext.Messages) > 0 {
			b.WriteString(renderLoopMessages(opts.LoopContext.Messages))
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

	// Wait results and handoffs.
	b.WriteString(renderWaitResults(opts.WaitResults))
	b.WriteString(renderHandoffs(opts.Handoffs))

	// Objective.
	var legacySkills []string
	if roleCanWrite {
		legacySkills = []string{config.SkillCodeWriting}
	}
	b.WriteString(renderObjective(opts, project, plan, legacySkills))

	return b.String(), nil
}

// resolvePlan resolves the effective plan ID and loads the plan.
func resolvePlan(opts BuildOpts) (string, *store.Plan) {
	effectivePlanID := strings.TrimSpace(opts.PlanID)
	if effectivePlanID == "" && opts.Task == "" && opts.Project != nil {
		effectivePlanID = strings.TrimSpace(opts.Project.ActivePlanID)
	}

	var plan *store.Plan
	if effectivePlanID == "" && opts.Task == "" && opts.Store != nil {
		loaded, _ := opts.Store.LoadPlan()
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
	if effectivePlanID != "" && (plan == nil || plan.ID != effectivePlanID) && opts.Store != nil {
		plan, _ = opts.Store.GetPlan(effectivePlanID)
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
	return effectivePlanID, plan
}

// renderSessionLogs formats recent session logs for the prompt.
func renderSessionLogs(allTurns []store.Turn) string {
	var b strings.Builder
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
			if turn.Objective != "" {
				fmt.Fprintf(&b, "- Objective: %s\n", turn.Objective)
			}
			if turn.WhatWasBuilt != "" {
				fmt.Fprintf(&b, "- Built: %s\n", turn.WhatWasBuilt)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderIssues formats the issues section.
func renderIssues(s *store.Store, effectivePlanID string) string {
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
	if len(relevant) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Open Issues\n")
	for _, iss := range relevant {
		fmt.Fprintf(&b, "- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description)
	}
	b.WriteString("\n")
	return b.String()
}

// renderLoopContext formats the loop context section.
func renderLoopContext(opts BuildOpts) string {
	lc := opts.LoopContext
	if lc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Loop Context\n\n")
	fmt.Fprintf(&b, "You are running in loop %q, cycle %d, step %d of %d",
		lc.LoopName, lc.Cycle+1, lc.StepIndex+1, lc.TotalSteps)
	if opts.Profile != nil {
		fmt.Fprintf(&b, " (profile %q)", opts.Profile.Name)
	}
	b.WriteString(".\n")

	if lc.InitialPrompt != "" {
		b.WriteString("\n## General Objective\n\n")
		b.WriteString(lc.InitialPrompt + "\n")
	}

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
	return b.String()
}

// renderPushover formats the pushover notifications section.
func renderPushover() string {
	var b strings.Builder
	b.WriteString("## Pushover Notifications\n\n")
	b.WriteString("You can send push notifications to the user's device by running:\n")
	b.WriteString("`adaf loop notify \"<title>\" \"<message>\"` — Send a notification (default priority: normal)\n")
	b.WriteString("`adaf loop notify --priority 1 \"<title>\" \"<message>\"` — Send a high-priority notification\n\n")
	b.WriteString("**Character limits:**\n")
	b.WriteString("- Title: max 250 characters (keep it short and descriptive)\n")
	b.WriteString("- Message: max 1024 characters (concise summary of what happened)\n\n")
	b.WriteString("**Priority levels:** -2 (lowest), -1 (low), 0 (normal), 1 (high)\n\n")
	b.WriteString("**When to use:** Send notifications for significant events like task completion, errors requiring attention, or milestones reached. Do NOT spam — only send when genuinely useful.\n\n")
	return b.String()
}

// renderLoopMessages formats loop messages from previous steps.
func renderLoopMessages(messages []store.LoopMessage) string {
	var b strings.Builder
	b.WriteString("## Messages from Previous Steps\n\n")
	for _, msg := range messages {
		fmt.Fprintf(&b, "- [step %d, %s]: %s\n", msg.StepIndex, msg.CreatedAt.Format("15:04:05"), msg.Content)
	}
	b.WriteString("\n")
	return b.String()
}

// renderWaitResults formats wait-for-spawns results.
func renderWaitResults(results []WaitResultInfo) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Spawn Wait Results\n\n")
	b.WriteString("The spawns you waited for have completed:\n\n")
	for _, wr := range results {
		b.WriteString(formatWaitResultInfo(wr))
	}
	return b.String()
}

// renderHandoffs formats the handoff section.
func renderHandoffs(handoffs []store.HandoffInfo) string {
	if len(handoffs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Inherited Running Agents (Handoff)\n\n")
	b.WriteString("The previous step handed off these running sub-agents to you:\n\n")
	for _, h := range handoffs {
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
	return b.String()
}

// renderContextSection formats a lightweight project context section for the skills-driven path.
// Agents discover plan details, issues, and session history via CLI commands.
func renderContextSection(opts BuildOpts, project *store.ProjectConfig, plan *store.Plan) string {
	var b strings.Builder
	b.WriteString("# Project\n\n")
	b.WriteString("Project: " + project.Name + "\n\n")

	if plan != nil {
		fmt.Fprintf(&b, "Active plan: **%s**", plan.ID)
		if plan.Title != "" {
			fmt.Fprintf(&b, " — %s", plan.Title)
		}
		b.WriteString("\n\n")
	}

	if opts.Task != "" {
		b.WriteString(opts.Task + "\n\n")
	}

	return b.String()
}

// renderObjective formats the objective section.
func renderObjective(opts BuildOpts, project *store.ProjectConfig, plan *store.Plan, skills []string) string {
	roleCanWrite := hasSkill(skills, config.SkillCodeWriting)
	var b strings.Builder
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
	return b.String()
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

// buildStandaloneChatContext generates a minimal prompt for interactive chat sessions.
// It includes only role identity, project name, tool pointers, and the conversation —
// no autonomous rules, session logs, issues, loop context, or delegation docs.
func buildStandaloneChatContext(opts BuildOpts) (string, error) {
	var b strings.Builder

	// Wrap context in a supra-code block so the model clearly distinguishes
	// system context from the conversation / instructions that follow.
	b.WriteString("Context: `````\n")

	// Role identity (brief).
	if opts.Profile != nil {
		role := config.EffectiveStepRole(opts.Role, opts.GlobalCfg)
		roles := config.DefaultRoleDefinitions()
		if opts.GlobalCfg != nil {
			config.EnsureDefaultRoleCatalog(opts.GlobalCfg)
			roles = opts.GlobalCfg.Roles
		}
		roleTitle := strings.ToUpper(role)
		roleIdentity := ""
		for _, def := range roles {
			if strings.EqualFold(def.Name, role) {
				if strings.TrimSpace(def.Title) != "" {
					roleTitle = strings.TrimSpace(def.Title)
				}
				roleIdentity = strings.TrimSpace(def.Identity)
				break
			}
		}
		fmt.Fprintf(&b, "# %s\n", roleTitle)
		if roleIdentity != "" {
			b.WriteString(roleIdentity + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("You are in an interactive chat session with the user. Respond directly to their messages.\n")
	b.WriteString("When asked to do work (write code, fix bugs, explore the codebase, etc.), use your tools.\n\n")

	// Project name.
	if opts.Project != nil && opts.Project.Name != "" {
		fmt.Fprintf(&b, "Project: %s\n\n", opts.Project.Name)
	}

	// Tools pointer.
	b.WriteString("## Tools\n")
	b.WriteString("The `adaf` CLI provides project management tools. Run `adaf --help` for available commands.\n")
	b.WriteString("Do not access the `.adaf/` directory directly — use `adaf` commands.\n\n")

	// Delegation pointer (brief).
	if opts.Delegation != nil && len(opts.Delegation.Profiles) > 0 {
		b.WriteString("You can delegate work to sub-agents. Run `adaf spawn` for details.\n\n")
	}

	b.WriteString("`````\n\n")

	// General objective for standalone chat.
	if lc := opts.LoopContext; lc != nil && lc.InitialPrompt != "" {
		b.WriteString("## General Objective\n\n")
		b.WriteString(lc.InitialPrompt)
		b.WriteString("\n\n")
	}

	// Step instructions (standalone profile instructions + current message).
	if lc := opts.LoopContext; lc != nil && lc.Instructions != "" {
		b.WriteString(lc.Instructions)
		b.WriteString("\n")
	}

	return b.String(), nil
}

func isDelegationActiveSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
		return false
	default:
		return true
	}
}
