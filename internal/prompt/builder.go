// Package prompt builds system prompts for agent sessions.
package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

const maxRecentTurns = 5

func buildSubAgentPrompt(opts BuildOpts) (string, error) {
	position := normalizePromptPosition(opts.Position, true)
	workerRole := config.EffectiveWorkerRoleForPosition(position, opts.Role, opts.GlobalCfg)
	roleLabel := workerRole
	if strings.TrimSpace(roleLabel) == "" {
		roleLabel = "worker"
	}

	var b strings.Builder

	// Wrap system context in a fenced code block so the model clearly
	// distinguishes it from the task that follows, and markdown renderers
	// display it as a visually separate block instead of swallowing the task.
	b.WriteString("Context:\n```\n")

	fmt.Fprintf(&b, "You are a sub-agent working as a %s.", roleLabel)
	b.WriteString(" You were spawned by a parent agent to complete a specific task.\n")

	if opts.ReadOnly {
		b.WriteString("You are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n")
	} else {
		b.WriteString("Commit your work when you finish.\n")
	}

	b.WriteString("If you need to communicate with your parent agent use: adaf parent-ask \"question\"\n")
	b.WriteString("Do NOT manage turn logs with `adaf turn ...`; your parent agent owns turn handoff publication.\n")

	if opts.Delegation != nil && len(opts.Delegation.Profiles) > 0 {
		b.WriteString(delegationSection(opts.Delegation, opts.GlobalCfg, nil))
	}

	b.WriteString("Your task below is your primary directive.\n")

	b.WriteString("```\n\n")

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
	LoopName          string
	Cycle             int
	StepIndex         int
	TotalSteps        int
	Instructions      string // step-specific custom instructions
	InitialPrompt     string // general objective injected across all steps
	CanStop           bool
	CanMessage        bool
	CanCallSupervisor bool
	CanPushover       bool
	Messages          []store.LoopMessage // unseen messages from other steps
	RunID             int
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

	// Position overrides the built-in execution position for this run context.
	// Valid values: supervisor, manager, lead, worker.
	Position string

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
	// When nil, Build() uses the full configured skill catalog.
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

// Build constructs a prompt from project context and role/position configuration.
func Build(opts BuildOpts) (string, error) {
	if opts.ParentTurnID > 0 {
		return buildSubAgentPrompt(opts)
	}

	if opts.StandaloneChat {
		return buildStandaloneChatContext(opts)
	}

	if opts.Skills == nil {
		opts.Skills = defaultPromptSkillIDs(opts.GlobalCfg)
	}
	return buildSkillsPrompt(opts)
}

func defaultPromptSkillIDs(globalCfg *config.GlobalConfig) []string {
	_ = globalCfg
	defaults := config.DefaultSkills()
	out := make([]string, 0, len(defaults))
	for _, sk := range defaults {
		id := strings.TrimSpace(sk.ID)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
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

	effectivePlanID, plan := resolvePlan(opts)

	effectivePosition := normalizePromptPosition(opts.Position, false)
	workerRole := config.EffectiveWorkerRoleForPosition(effectivePosition, opts.Role, opts.GlobalCfg)
	hasDelegation := opts.Delegation != nil && len(opts.Delegation.Profiles) > 0
	canCallSupervisor := true
	if opts.LoopContext != nil && effectivePosition == config.PositionManager {
		canCallSupervisor = opts.LoopContext.CanCallSupervisor
	}

	// Position header and duties.
	if opts.Profile != nil {
		posSection := PositionPrompt(effectivePosition, workerRole, hasDelegation, canCallSupervisor)
		if posSection != "" {
			b.WriteString(posSection)
			b.WriteString("\n")
		}
	}

	// Worker-role header (slim role identity), only for worker contexts.
	if opts.Profile != nil && workerRole != "" {
		roleSection := RolePromptSlim(opts.Profile, workerRole, opts.GlobalCfg)
		if roleSection != "" {
			b.WriteString(roleSection)
			b.WriteString("\n")
		}
	}

	// Resolve skills for position/role and read-only mode.
	resolvedSkills := config.ResolveSkillsForContext(opts.Skills, effectivePosition, workerRole, opts.ReadOnly, opts.GlobalCfg)
	roleCanWrite := config.CanWriteForPositionAndRole(effectivePosition, workerRole, opts.GlobalCfg)

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

	// Core operating rules.
	if opts.ParentTurnID == 0 && (opts.LoopContext != nil || hasSkill(resolvedSkills, config.SkillAutonomy)) {
		b.WriteString("You are fully autonomous. There is no human in the loop.\n\n")
	}
	if opts.ParentTurnID == 0 && !opts.ReadOnly && roleCanWrite && hasSkill(resolvedSkills, config.SkillCommit) {
		b.WriteString("You own your repository. Commit your work.\n\n")
	}

	// Dynamic context sections gated by active skills.

	// Loop context.
	if opts.LoopContext != nil {
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

	// Session logs and issues.
	if opts.ParentTurnID == 0 && opts.Store != nil {
		if hasSkill(resolvedSkills, config.SkillSessionContext) {
			if allTurns, err := opts.Store.ListTurns(); err == nil && len(allTurns) > 0 {
				b.WriteString(renderSessionLogs(allTurns))
			}
		}
		if hasSkill(resolvedSkills, config.SkillIssues) {
			b.WriteString(renderIssues(opts.Store, effectivePlanID))
		}
	}

	// Project context (lightweight — agents discover details via CLI).
	b.WriteString(renderContextSection(opts, project, plan))
	b.WriteString(renderObjective(opts, project, plan, effectivePlanID, effectivePosition))

	// Turn handoff logging.
	if hasSkill(resolvedSkills, config.SkillSessionContext) {
		b.WriteString(renderTurnHandoffInstructions(config.PositionMustWriteTurnLog(effectivePosition)))
	}

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
	if lc.CanCallSupervisor {
		b.WriteString("If you need supervisor direction or have no actionable work left, escalate with: `adaf loop call-supervisor \"status + concrete ask\"`\n\n")
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

	// Count writable spawns that need merging.
	writableCount := 0
	for _, wr := range results {
		if !wr.ReadOnly && wr.Branch != "" && (wr.Status == "completed" || wr.Status == "canceled") {
			writableCount++
		}
	}
	if writableCount > 0 {
		b.WriteString("**IMPORTANT: Writable spawns completed. Their work is on isolated branches — NOT on your branch yet. You MUST `adaf spawn-diff` and `adaf spawn-merge` each writable spawn to land the changes on your branch. Skipping merge means the work is lost.**\n\n")
	}

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

func renderTurnHandoffInstructions(required bool) string {
	var b strings.Builder
	b.WriteString("## Turn Handoff\n\n")
	if required {
		b.WriteString("Before finishing your turn, you MUST publish a complete handoff so the next agent can continue without re-discovery.\n")
	} else {
		b.WriteString("Before finishing your turn, publish a complete handoff so the next agent can continue without re-discovery.\n")
	}
	b.WriteString("Run `adaf turn finish --built \"...\" --decisions \"...\" --challenges \"...\" --state \"...\" --issues \"...\" --next \"...\"`.\n")
	b.WriteString("`adaf turn finish` fails if any required section is missing.\n")
	b.WriteString("If `ADAF_TURN_ID` is set, `adaf turn finish` targets the current turn automatically.\n\n")
	return b.String()
}

// renderObjective formats the objective section.
func renderObjective(opts BuildOpts, project *store.ProjectConfig, plan *store.Plan, effectivePlanID, position string) string {
	roleCanWrite := config.CanWriteForPositionAndRole(position, opts.Role, opts.GlobalCfg)
	effectivePosition := normalizePromptPosition(position, false)
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
		if effectivePosition == config.PositionManager || effectivePosition == config.PositionSupervisor {
			if plan != nil && strings.TrimSpace(plan.ID) != "" {
				fmt.Fprintf(&b, "Use `adaf plan show %s` to inspect the active plan goals, rationale, and scope.\n", plan.ID)
			} else {
				b.WriteString("Use `adaf plan` to inspect active plan goals, rationale, and scope.\n")
			}
			b.WriteString("Use `adaf issues` and `adaf log` to validate progress and then publish concrete guidance for the next step.\n\n")
			return b.String()
		}

		var issues []store.Issue
		if opts.Store != nil {
			if effectivePlanID != "" {
				issues, _ = opts.Store.ListIssuesForPlan(effectivePlanID)
			} else {
				issues, _ = opts.Store.ListSharedIssues()
			}
		}
		openIssues := make([]store.Issue, 0, len(issues))
		issuesByID := make(map[int]store.Issue, len(issues))
		for _, iss := range issues {
			issuesByID[iss.ID] = iss
			if store.IsOpenIssueStatus(iss.Status) {
				openIssues = append(openIssues, iss)
			}
		}
		sortIssueQueue(openIssues)

		type blockedIssue struct {
			issue   store.Issue
			waiting []int
		}
		ready := make([]store.Issue, 0, len(openIssues))
		blocked := make([]blockedIssue, 0, len(openIssues))
		for _, iss := range openIssues {
			waiting := unresolvedIssueDependencies(iss, issuesByID)
			if len(waiting) == 0 {
				ready = append(ready, iss)
				continue
			}
			blocked = append(blocked, blockedIssue{issue: iss, waiting: waiting})
		}

		if len(ready) > 0 {
			currentIssue := ready[0]
			if roleCanWrite {
				fmt.Fprintf(&b, "Your task is to work on issue **#%d: %s**.\n\n", currentIssue.ID, currentIssue.Title)
			} else {
				fmt.Fprintf(&b, "Review progress on issue **#%d: %s**. Check if the implementation is correct, verify tests/build, and provide guidance.\n\n", currentIssue.ID, currentIssue.Title)
			}
			if strings.TrimSpace(currentIssue.Description) != "" {
				b.WriteString(currentIssue.Description + "\n\n")
			}

			b.WriteString("## Ready Issues\n")
			limit := 3
			if len(ready) < limit {
				limit = len(ready)
			}
			for i := 0; i < limit; i++ {
				iss := ready[i]
				fmt.Fprintf(&b, "- #%d [%s] %s\n", iss.ID, iss.Priority, iss.Title)
			}
			b.WriteString("\n")

			if len(blocked) > 0 {
				b.WriteString("## Blocked Issues\n")
				limitBlocked := 3
				if len(blocked) < limitBlocked {
					limitBlocked = len(blocked)
				}
				for i := 0; i < limitBlocked; i++ {
					bi := blocked[i]
					fmt.Fprintf(&b, "- #%d waits on %s\n", bi.issue.ID, formatIssueDependencyIDs(bi.waiting))
				}
				b.WriteString("\n")
			}
		} else if len(blocked) > 0 {
			b.WriteString("All open issues are currently blocked by dependencies. Focus on resolving blockers first.\n\n")
			b.WriteString("## Blocked Issues\n")
			limitBlocked := 5
			if len(blocked) < limitBlocked {
				limitBlocked = len(blocked)
			}
			for i := 0; i < limitBlocked; i++ {
				bi := blocked[i]
				fmt.Fprintf(&b, "- #%d waits on %s\n", bi.issue.ID, formatIssueDependencyIDs(bi.waiting))
			}
			b.WriteString("\n")
		} else if plan != nil && plan.Title != "" {
			b.WriteString("No open issues are tracked for this plan. Look for gaps, file issues, or refine the plan details.\n\n")
		} else {
			b.WriteString("No plan is set. Explore the codebase and address any open issues.\n\n")
		}
	}
	return b.String()
}

func unresolvedIssueDependencies(issue store.Issue, byID map[int]store.Issue) []int {
	if len(issue.DependsOn) == 0 {
		return nil
	}

	waiting := make([]int, 0, len(issue.DependsOn))
	for _, depID := range store.NormalizeIssueDependencyIDs(issue.DependsOn) {
		dep, ok := byID[depID]
		if !ok || !store.IsTerminalIssueStatus(dep.Status) {
			waiting = append(waiting, depID)
		}
	}
	if len(waiting) == 0 {
		return nil
	}
	return waiting
}

func sortIssueQueue(issues []store.Issue) {
	sort.Slice(issues, func(i, j int) bool {
		pi := issuePriorityRank(issues[i].Priority)
		pj := issuePriorityRank(issues[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return issues[i].ID < issues[j].ID
	})
}

func issuePriorityRank(priority string) int {
	switch strings.TrimSpace(strings.ToLower(priority)) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

func formatIssueDependencyIDs(dependsOn []int) string {
	if len(dependsOn) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(dependsOn))
	for _, depID := range dependsOn {
		if depID <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("#%d", depID))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
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

	// Remind parent to merge writable spawns.
	if !wr.ReadOnly && wr.Branch != "" && (wr.Status == "completed" || wr.Status == "canceled") {
		fmt.Fprintf(&b, "**Action required:** Review and merge this spawn's work: `adaf spawn-diff --spawn-id %d` then `adaf spawn-merge --spawn-id %d`\n\n", wr.SpawnID, wr.SpawnID)
	}

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
		position := normalizePromptPosition(opts.Position, false)
		role := config.EffectiveWorkerRoleForPosition(position, opts.Role, opts.GlobalCfg)
		if role == "" {
			role = strings.TrimSpace(opts.Role)
		}
		if role == "" {
			role = config.DefaultWorkerRole(opts.GlobalCfg)
		}
		fmt.Fprintf(&b, "# %s\n", strings.ToUpper(role))
		if role != "" {
			fmt.Fprintf(&b, "Assigned role: %s\n", role)
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
	b.WriteString("Do not access the adaf project store directly (for example `~/.adaf/projects/<id>/`) — use `adaf` commands.\n\n")

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
	return !store.IsTerminalSpawnStatus(status)
}
