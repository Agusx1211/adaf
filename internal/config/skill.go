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

// DefaultSkills returns the built-in skills shipped by default.
func DefaultSkills() []Skill {
	return []Skill{
		{
			ID:    SkillDelegation,
			Short: "You can spawn sub-agents. Run `adaf spawn-info` to see available profiles, roles, cost tiers, and performance data. Run `adaf skill delegation` for full command reference and patterns.",
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
				"**Feedback scoring (MANDATORY after child completion):**\n" +
				"- `adaf spawn-feedback --spawn-id N --difficulty <0-10> --quality <0-10> [--notes \"...\"]`\n" +
				"- Difficulty measures task complexity. Quality measures output correctness.\n" +
				"- This data is tracked across projects and used for profile selection hints.\n\n" +
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
				"If the same profile is allowed with multiple roles, you MUST pass `--role <role>` in the spawn command.\n\n" +
				"## Usage Balance\n\n" +
				"Do not spam the same profile. Distribute work across available profiles — match task difficulty to cost tier (cheap for easy, expensive for hard). Repeatedly using one profile drains that provider's quota and wastes budget. Check the Routing Scoreboard and rotate.\n",
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

// ResolveSkillsForContext returns a copy of skills adjusted for position/role
// and read-only mode.
//
// For non-writing contexts: code_writing is replaced with code_review, commit is removed.
// For read-only mode: commit is removed, read_only is ensured present.
// The input slice is never mutated.
func ResolveSkillsForContext(skills []string, position, role string, readOnly bool, cfg *GlobalConfig) []string {
	canWrite := CanWriteForPositionAndRole(position, role, cfg)

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

// EffectiveStepSkills resolves how loop-step skills should be interpreted:
// - SkillsExplicit=true: use step.Skills exactly (empty means no skills).
// - SkillsExplicit=false with non-empty Skills: preserve existing explicit skills.
// - SkillsExplicit=false with empty Skills: nil => prompt defaults.
func EffectiveStepSkills(step LoopStep) []string {
	if step.SkillsExplicit {
		if len(step.Skills) == 0 {
			return []string{}
		}
		return append([]string(nil), step.Skills...)
	}
	if len(step.Skills) > 0 {
		return append([]string(nil), step.Skills...)
	}
	return nil
}
