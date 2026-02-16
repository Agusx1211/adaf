package config

import "strings"

// Skill is a granular, togglable instruction block for agent prompts.
type Skill struct {
	ID    string `json:"id"`
	Short string `json:"short"`
	Long  string `json:"long,omitempty"`
}

// Built-in skill IDs.
const (
	SkillAutonomy       = "autonomy"
	SkillCodeWriting    = "code_writing"
	SkillCommit         = "commit"
	SkillFocus          = "focus"
	SkillAdafTools      = "adaf_tools"
	SkillDelegation     = "delegation"
	SkillIssues         = "issues"
	SkillPlan           = "plan"
	SkillSessionContext = "session_context"
	SkillLoopControl    = "loop_control"
	SkillReadOnly       = "read_only"
	SkillPushover       = "pushover"
	SkillCodeReview     = "code_review"
)

func normalizeSkillID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

// DefaultSkills returns the 13 built-in skills.
func DefaultSkills() []Skill {
	return []Skill{
		{
			ID:    SkillAutonomy,
			Short: "You are fully autonomous. There is no human in the loop. Make all decisions yourself. Do not ask for confirmation or direction — decide and act. If something is ambiguous, use your best judgment and move forward.",
			Long: "# Autonomy\n\n" +
				"You are fully autonomous. There is no human in the loop. No one will answer questions, grant permissions, or provide clarification.\n\n" +
				"You must make all decisions yourself. Do not ask for confirmation or direction — decide and act.\n" +
				"If something is ambiguous, use your best judgment and move forward.\n\n" +
				"This means:\n" +
				"- Do not pause for approval\n" +
				"- Do not ask clarifying questions\n" +
				"- Choose the most reasonable interpretation and proceed\n" +
				"- Log your decisions so the next agent (or human) can review them\n",
		},
		{
			ID:    SkillCodeWriting,
			Short: "Write code, run tests, and ensure everything compiles before finishing.",
			Long: "# Code Writing\n\n" +
				"Write code, run tests, and ensure everything compiles before finishing.\n\n" +
				"Guidelines:\n" +
				"- Follow existing code conventions and patterns in the repository\n" +
				"- Run the test suite after making changes\n" +
				"- Fix any compilation errors or test failures you introduce\n" +
				"- Keep changes focused and minimal\n",
		},
		{
			ID:    SkillCodeReview,
			Short: "Review work, check progress, and provide guidance to running agents. Do NOT write or modify code.",
			Long: "# Code Review\n\n" +
				"Review work, check progress, and provide guidance to running agents. Do NOT write or modify code.\n\n" +
				"Guidelines:\n" +
				"- Review diffs and changes made by other agents\n" +
				"- Verify that code follows project conventions and patterns\n" +
				"- Check that tests pass and builds succeed\n" +
				"- Provide constructive feedback and flag issues\n" +
				"- Guide agents toward the correct approach without writing code yourself\n",
		},
		{
			ID:    SkillCommit,
			Short: "You own your repository. Commit your work. Every time you finish a coherent piece of work, create a git commit. Uncommitted changes are invisible to scouts, other agents, and future sessions. Commit early and often.",
			Long: "# Commit Discipline\n\n" +
				"You own your repository. Commit your work. Do not leave changes uncommitted.\n\n" +
				"Every time you finish a coherent piece of work, create a git commit.\n" +
				"Uncommitted changes are invisible to scouts, other agents, and future sessions.\n" +
				"Commit early and often — your worktree is yours alone.\n\n" +
				"Best practices:\n" +
				"- Commit after each logical unit of work\n" +
				"- Use clear, descriptive commit messages\n" +
				"- Do not batch unrelated changes into a single commit\n",
		},
		{
			ID:    SkillFocus,
			Short: "Focus on one coherent unit of work. Stop when the current phase (or a meaningful increment of it) is complete.",
			Long: "# Focus\n\n" +
				"Focus on one coherent unit of work. Stop when the current phase (or a meaningful increment of it) is complete.\n\n" +
				"Avoid scope creep — stay within the bounds of your assigned task.\n" +
				"If you discover related work that needs doing, log it as an issue rather than doing it now.\n",
		},
		{
			ID:    SkillAdafTools,
			Short: "Do NOT read or write files inside the `.adaf/` directory directly. Use `adaf` CLI commands instead (`adaf issues`, `adaf log`, `adaf plan`, etc.). The `.adaf/` directory structure may change and direct access will be restricted in the future.",
			Long: "# ADAF Tools\n\n" +
				"Do NOT read or write files inside the `.adaf/` directory directly.\n" +
				"Use `adaf` CLI commands instead:\n\n" +
				"- `adaf issues` — List and manage issues\n" +
				"- `adaf log` — View session logs\n" +
				"- `adaf plan` — View and manage plans\n" +
				"- `adaf status` — View project status\n" +
				"- `adaf --help` — Full command reference\n\n" +
				"The `.adaf/` directory structure may change and direct access will be restricted in the future.\n",
		},
		{
			ID:    SkillDelegation,
			Short: "You can spawn sub-agents. Run `adaf spawn info` to see available profiles, roles, and capacity. Run `adaf skill delegation` for full command reference and patterns.",
			Long: "# Delegation\n\n" +
				"## Spawn Flow\n\n" +
				"**ALWAYS use this pattern:**\n" +
				"1. Spawn ALL independent tasks at once\n" +
				"2. Call `adaf wait-for-spawns` immediately after\n" +
				"3. Stop immediately after that command. Do not run more commands in this turn — the loop will pause this turn and resume you automatically when children complete.\n\n" +
				"This is critical: `wait-for-spawns` suspends your session with zero token cost.\n\n" +
				"## Communication Style: Downstream Only\n\n" +
				"Communicate primarily to child sessions. Keep direction concrete and executable.\n\n" +
				"- `adaf spawn-message --spawn-id N \"guidance\"` — Send async guidance to a child\n" +
				"- `adaf spawn-message --spawn-id N --interrupt \"new priority\"` — Interrupt child turn with updated direction\n" +
				"- `adaf spawn-reply --spawn-id N \"answer\"` — Answer a child's question\n" +
				"- `adaf spawn-status [--spawn-id N]` / `adaf spawn-watch --spawn-id N` — Monitor children\n\n" +
				"## Scouts (read-only sub-agents)\n\n" +
				"Use `--read-only` spawns for any information gathering:\n" +
				"- Inspecting repo structure, reading files, understanding code\n" +
				"- Reviewing git history, checking test status, analyzing dependencies\n" +
				"- Exploring the codebase before deciding how to break down work\n\n" +
				"Scouts are cheap and fast. Prefer spawning a scout over reading files yourself.\n\n" +
				"**Important:** Read-only scouts run in an isolated worktree snapshot at HEAD. They do NOT see uncommitted or staged changes from the parent. If a scout needs to inspect in-flight work, commit or stash first.\n\n" +
				"## Command Reference\n\n" +
				"**Spawning:**\n" +
				"- `adaf spawn --profile <name> [--role <role>] --task \"...\" [--read-only]` — Spawn a sub-agent (non-blocking)\n" +
				"- `adaf spawn --profile <name> [--role <role>] --task-file <path> [--read-only]` — Spawn with detailed task from file\n" +
				"- `adaf wait-for-spawns` — Suspend until all spawns complete (TOKEN-FREE wait)\n\n" +
				"**Monitoring (use sparingly — prefer wait-for-spawns):**\n" +
				"- `adaf spawn-status [--spawn-id N]` — Check spawn status\n" +
				"- `adaf spawn-watch --spawn-id N` — Watch spawn output in real-time\n\n" +
				"**Review & merge (MANDATORY for writable spawns):**\n" +
				"- `adaf spawn-diff --spawn-id N` — View diff of spawn's changes\n" +
				"- `adaf spawn-merge --spawn-id N [--squash]` — Merge spawn's changes into YOUR branch\n" +
				"- `adaf spawn-reject --spawn-id N` — Reject spawn's changes (destroys branch — see below)\n\n" +
				"**CRITICAL: Writable sub-agents work in isolated worktree branches. Their commits are INVISIBLE to your branch until you explicitly merge them.** A spawn completing with status=completed does NOT mean the work is on your branch. You MUST:\n" +
				"1. Review the diff: `adaf spawn-diff --spawn-id N`\n" +
				"2. Merge: `adaf spawn-merge --spawn-id N`\n" +
				"3. Verify the merge succeeded (spawn a scout if needed)\n" +
				"Skipping the merge means the sub-agent's work is lost — it only exists on an orphaned worktree branch.\n\n" +
				"**Replying to child questions:**\n" +
				"- `adaf spawn-reply --spawn-id N \"answer\"` — Reply to child's question\n\n" +
				"## On Rejecting Work\n\n" +
				"`spawn-reject` destroys the branch entirely. The next spawn starts from scratch. Before rejecting, consider:\n" +
				"- If the issue is minor (e.g. stale files in diff), write a more detailed task description for the next spawn rather than iterating blindly\n" +
				"- If you've already rejected the same task twice, stop and rethink your task description — you are wasting resources\n\n" +
				"## Quick-Start Example\n\n" +
				"```bash\n" +
				"# 1. Spawn a scout to understand the codebase\n" +
				"adaf spawn --profile <name> --role <role> --read-only --task \"Examine the repo structure, summarize key files, list failing tests\"\n" +
				"# 2. Spawn workers for independent tasks\n" +
				"adaf spawn --profile <name> --role <role> --task-file /tmp/task1.md\n" +
				"adaf spawn --profile <name> --role <role> --task-file /tmp/task2.md\n" +
				"# 3. Suspend — costs zero tokens while waiting\n" +
				"adaf wait-for-spawns\n" +
				"```\n\n" +
				"When spawning, write a thorough task description — sub-agents only see what you give them. Include relevant context, goals, constraints, and what \"done\" looks like. Use `--task-file` for anything non-trivial.\n\n" +
				"If the same profile is allowed with multiple roles, you MUST pass `--role <role>` in the spawn command.\n",
		},
		{
			ID:    SkillIssues,
			Short: "Use `adaf issues` to check open work items before starting. Log new issues you discover with `adaf issue create`.",
			Long: "# Issues\n\n" +
				"Track work items via `adaf issues`. Review open issues before starting.\n" +
				"Log new issues you discover during work rather than addressing them immediately.\n\n" +
				"Commands:\n" +
				"- `adaf issues` — List open issues\n" +
				"- `adaf issue create --title \"...\" --priority high` — Create an issue\n" +
				"- `adaf issue close <id>` — Close a resolved issue\n",
		},
		{
			ID:    SkillPlan,
			Short: "Use `adaf plan` to view the active plan, phases, and status. Work on the current phase. Mark phases complete as you finish them.",
			Long: "# Plan\n\n" +
				"Follow the active plan. Work on the current phase.\n\n" +
				"Commands:\n" +
				"- `adaf plan` — View current plan and phases\n" +
				"- `adaf plan phase complete <id>` — Mark a phase complete\n\n" +
				"When your phase is done, mark it complete and move to the next.\n" +
				"If all phases are complete, look for remaining open issues or improvements.\n",
		},
		{
			ID:    SkillSessionContext,
			Short: "Use `adaf log` to review session history before starting. Previous agents left notes about progress, decisions, and next steps.",
			Long: "# Session Context\n\n" +
				"Review recent session logs before starting. Previous agents left notes about:\n" +
				"- What was built\n" +
				"- Key decisions made\n" +
				"- Challenges encountered\n" +
				"- Current state of the project\n" +
				"- Known issues\n" +
				"- Suggested next steps\n\n" +
				"Use `adaf log` to view session history.\n" +
				"Build on previous work rather than starting from scratch.\n",
		},
		{
			ID:    SkillLoopControl,
			Short: "You are running in a loop. Check your loop position and follow step instructions. Use `adaf loop stop` when objectives are met (if allowed). Use `adaf loop message` to communicate with subsequent steps.",
			Long: "# Loop Control\n\n" +
				"You are running in a multi-step loop cycle.\n\n" +
				"Commands:\n" +
				"- `adaf loop stop` — Stop the loop when objectives are met (only available on steps with can_stop)\n" +
				"- `adaf loop message \"text\"` — Send a message to subsequent steps (only available on steps with can_message)\n\n" +
				"Pay attention to:\n" +
				"- Your loop position (cycle, step index)\n" +
				"- Step-specific instructions\n" +
				"- Messages from previous steps\n",
		},
		{
			ID:    SkillReadOnly,
			Short: "You are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze. Return your report in your final assistant message, not in repository files.",
			Long: "# Read-Only Mode\n\n" +
				"You are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n\n" +
				"Do NOT write reports into repository files (for example `*.md`, `*.txt`, or TODO files).\n" +
				"Return your report in your final assistant message.\n",
		},
		{
			ID:    SkillPushover,
			Short: "You can send push notifications to the user's device. Use `adaf loop notify \"<title>\" \"<message>\"` for significant events. Do NOT spam — only send when genuinely useful.",
			Long: "# Pushover Notifications\n\n" +
				"You can send push notifications to the user's device by running:\n" +
				"`adaf loop notify \"<title>\" \"<message>\"` — Send a notification (default priority: normal)\n" +
				"`adaf loop notify --priority 1 \"<title>\" \"<message>\"` — Send a high-priority notification\n\n" +
				"**Character limits:**\n" +
				"- Title: max 250 characters (keep it short and descriptive)\n" +
				"- Message: max 1024 characters (concise summary of what happened)\n\n" +
				"**Priority levels:** -2 (lowest), -1 (low), 0 (normal), 1 (high)\n\n" +
				"**When to use:** Send notifications for significant events like task completion, errors requiring attention, or milestones reached. Do NOT spam — only send when genuinely useful.\n",
		},
	}
}

// EnsureDefaultSkillCatalog seeds default skills into cfg, deduplicates, and normalizes.
// Returns true when it mutates cfg.
func EnsureDefaultSkillCatalog(cfg *GlobalConfig) bool {
	if cfg == nil {
		return false
	}
	if len(cfg.Skills) > 0 {
		// Normalize and deduplicate existing skills.
		seen := make(map[string]struct{}, len(cfg.Skills))
		normalized := make([]Skill, 0, len(cfg.Skills))
		changed := false
		for _, sk := range cfg.Skills {
			id := normalizeSkillID(sk.ID)
			if id == "" {
				changed = true
				continue
			}
			if _, ok := seen[id]; ok {
				changed = true
				continue
			}
			seen[id] = struct{}{}
			if id != sk.ID {
				changed = true
			}
			normalized = append(normalized, Skill{
				ID:    id,
				Short: sk.Short,
				Long:  sk.Long,
			})
		}
		cfg.Skills = normalized
		return changed
	}

	cfg.Skills = DefaultSkills()
	return true
}

// ResolveSkillsForRole returns a copy of skills adjusted for the given role and read-only mode.
// For non-writing roles: code_writing is replaced with code_review, commit is removed.
// For read-only mode: commit is removed, read_only is ensured present.
// The input slice is never mutated.
func ResolveSkillsForRole(skills []string, role string, readOnly bool, cfg *GlobalConfig) []string {
	canWrite := CanWriteCode(role, cfg)

	out := make([]string, 0, len(skills))
	for _, s := range skills {
		sid := normalizeSkillID(s)
		if sid == SkillCodeWriting && !canWrite {
			out = append(out, SkillCodeReview)
			continue
		}
		if sid == SkillCommit && (!canWrite || readOnly) {
			continue
		}
		out = append(out, s)
	}

	if readOnly {
		hasRO := false
		for _, s := range out {
			if normalizeSkillID(s) == SkillReadOnly {
				hasRO = true
				break
			}
		}
		if !hasRO {
			out = append(out, SkillReadOnly)
		}
	}

	return out
}
