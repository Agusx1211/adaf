package prompt

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
)

// RolePrompt returns the role-specific system prompt section for a profile.
// An agent NEVER sees its own intelligence rating.
// Spawning capabilities are no longer emitted here — they come from delegationSection().
func RolePrompt(profile *config.Profile, stepRole string, globalCfg *config.GlobalConfig) string {
	role := config.EffectiveStepRole(stepRole, profile)

	var b strings.Builder

	switch role {
	case config.RoleManager:
		b.WriteString("# Your Role: MANAGER\n\n")
		b.WriteString("You are a MANAGER agent. You do NOT write code or run tests directly. Your entire value comes from effective delegation and review.\n\n")
		b.WriteString("## Core Principles\n\n")
		b.WriteString("1. **Delegate aggressively.** Every piece of work — coding, investigation, testing, review — should be done by a sub-agent. Spawn early, spawn often, spawn in parallel.\n")
		b.WriteString("2. **Prefer scouts over doing it yourself.** For reading files, checking git history, running tests, or inspecting the repo, spawn `--read-only` scouts. Your context window is expensive — save it for decisions. You can read a file directly when truly needed, but default to delegation.\n")
		b.WriteString("3. **Maximize parallelism.** Spawn all independent tasks at once, then `wait-for-spawns`. Sequential spawning wastes time. If you have 3 tasks, spawn 3 agents simultaneously.\n")
		b.WriteString("4. **Review every diff** with `adaf spawn-diff` before merging. When work needs corrections, prefer sending feedback via `spawn-message --interrupt` (if still running) or writing a precise corrective task rather than blindly rejecting and re-spawning.\n\n")
		b.WriteString("## Anti-Patterns (avoid these)\n\n")
		b.WriteString("- Spawning one agent at a time with `--wait` — this burns tokens while you idle. Use `wait-for-spawns`\n")
		b.WriteString("- Doing 3 sequential spawn-reject-respawn cycles for the same issue — give better instructions upfront, or use `spawn-message` mid-flight\n")
		b.WriteString("- Writing or editing any file yourself\n\n")
		b.WriteString("Use the Delegation section below for available profiles and commands.\n\n")

	case config.RoleSenior:
		b.WriteString("# Your Role: LEAD DEVELOPER\n\n")
		b.WriteString("You are an expert LEAD DEVELOPER agent. You write high-quality code and are expected to deliver excellent, well-tested solutions.\n\n")
		b.WriteString("If delegation is available in this step, you may use it strategically for parallel work.\n\n")
		b.WriteString(communicationCommands())

	case config.RoleJunior:
		b.WriteString("# Your Role: DEVELOPER\n\n")
		b.WriteString("You are a skilled DEVELOPER agent. Focus exclusively on delivering high-quality, well-tested code for your assigned task.\n\n")
		b.WriteString("If delegation is available in this step, only use it when it clearly improves delivery quality or speed.\n\n")
		b.WriteString(communicationCommands())

	case config.RoleSupervisor:
		b.WriteString("# Your Role: SUPERVISOR\n\n")
		b.WriteString("You are a SUPERVISOR agent. You review progress and provide guidance via notes. You do NOT write code.\n\n")
		b.WriteString("## Supervisor Commands\n\n")
		b.WriteString("- `adaf note add [--session <N>] --note \"guidance text\"` — Send a note to a running agent session\n")
		b.WriteString("- `adaf note list [--session <N>]` — List supervisor notes\n\n")
	}

	if profile.Description != "" {
		b.WriteString("## Your Description\n\n")
		b.WriteString(profile.Description + "\n\n")
	}

	return b.String()
}

// ReadOnlyPrompt returns the read-only mode prompt section.
func ReadOnlyPrompt() string {
	return "# READ-ONLY MODE\n\nYou are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n"
}

// delegationSection builds the delegation/spawning prompt section from a DelegationConfig.
func delegationSection(deleg *config.DelegationConfig, globalCfg *config.GlobalConfig) string {
	if deleg == nil {
		return "You cannot spawn sub-agents.\n\n"
	}

	var b strings.Builder

	b.WriteString("# Delegation\n\n")

	// Style guidance.
	if style := deleg.DelegationStyleText(); style != "" {
		b.WriteString("**Delegation style:** " + style + "\n\n")
	}

	// Delegation commands.
	b.WriteString(delegationCommands())

	// Task quality guidance.
	b.WriteString("When spawning, write a thorough task description — sub-agents only see what you give them. Include relevant context, goals, constraints, and what \"done\" looks like. Use `--task-file` for anything non-trivial.\n\n")

	// Quick-start example.
	b.WriteString("### Quick-Start Example\n\n")
	b.WriteString("```bash\n")
	b.WriteString("# 1. Spawn a scout to understand the codebase\n")
	b.WriteString("adaf spawn --profile <name> --read-only --task \"Examine the repo structure, summarize key files, list failing tests\"\n")
	b.WriteString("# 2. Spawn workers for independent tasks\n")
	b.WriteString("adaf spawn --profile <name> --task-file /tmp/task1.md\n")
	b.WriteString("adaf spawn --profile <name> --task-file /tmp/task2.md\n")
	b.WriteString("# 3. Suspend — costs zero tokens while waiting\n")
	b.WriteString("adaf wait-for-spawns\n")
	b.WriteString("```\n\n")

	// Available profiles.
	if len(deleg.Profiles) > 0 && globalCfg != nil {
		b.WriteString("## Available Profiles to Spawn\n\n")
		for _, dp := range deleg.Profiles {
			p := globalCfg.FindProfile(dp.Name)
			if p == nil {
				fmt.Fprintf(&b, "- **%s** (not found in config)\n", dp.Name)
				continue
			}
			line := fmt.Sprintf("- **%s** — agent=%s", p.Name, p.Agent)
			if p.Model != "" {
				line += fmt.Sprintf(", model=%s", p.Model)
			}
			if p.Intelligence > 0 {
				line += fmt.Sprintf(", intelligence=%d/10", p.Intelligence)
			}
			speed := dp.Speed
			if speed == "" {
				speed = p.Speed
			}
			if speed != "" {
				line += fmt.Sprintf(", speed=%s", speed)
			}
			if dp.Handoff {
				line += " [handoff]"
			}
			if p.Description != "" {
				line += fmt.Sprintf(" — %s", p.Description)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	maxPar := deleg.EffectiveMaxParallel()
	fmt.Fprintf(&b, "Maximum concurrent sub-agents: %d\n\n", maxPar)

	return b.String()
}

func delegationCommands() string {
	var b strings.Builder
	b.WriteString("## Delegation Commands\n\n")

	// Spawn flow — strong guidance.
	b.WriteString("### Spawn Flow\n\n")
	b.WriteString("**ALWAYS use this pattern:**\n")
	b.WriteString("1. Spawn ALL independent tasks at once (without `--wait`)\n")
	b.WriteString("2. Call `adaf wait-for-spawns` immediately after\n")
	b.WriteString("3. Finish your current turn — you will be resumed automatically when all children complete\n\n")
	b.WriteString("This is critical: `wait-for-spawns` suspends your session with zero token cost. ")
	b.WriteString("Using `--wait` keeps your session alive and burns tokens while you idle. ")
	b.WriteString("**Only use `--wait` when you absolutely need a child's output before you can spawn the next task in the same turn** (rare).\n\n")

	// Scout pattern.
	b.WriteString("### Scouts (read-only sub-agents)\n\n")
	b.WriteString("Use `--read-only` spawns for any information gathering:\n")
	b.WriteString("- Inspecting repo structure, reading files, understanding code\n")
	b.WriteString("- Reviewing git history, checking test status, analyzing dependencies\n")
	b.WriteString("- Exploring the codebase before deciding how to break down work\n\n")
	b.WriteString("Scouts are cheap and fast. Prefer spawning a scout over reading files yourself.\n\n")
	b.WriteString("**Important:** Read-only scouts run in an isolated worktree snapshot at HEAD. They do NOT see uncommitted or staged changes from the parent. If a scout needs to inspect in-flight work, commit or stash first.\n\n")

	// Command reference.
	b.WriteString("### Command Reference\n\n")
	b.WriteString("**Spawning:**\n")
	b.WriteString("- `adaf spawn --profile <name> --task \"...\" [--read-only]` — Spawn a sub-agent (non-blocking)\n")
	b.WriteString("- `adaf spawn --profile <name> --task-file <path> [--read-only]` — Spawn with detailed task from file\n")
	b.WriteString("- `adaf wait-for-spawns` — Suspend until all spawns complete (TOKEN-FREE wait)\n\n")
	b.WriteString("**Monitoring (use sparingly — prefer wait-for-spawns):**\n")
	b.WriteString("- `adaf spawn-status [--spawn-id N]` — Check spawn status\n")
	b.WriteString("- `adaf spawn-watch --spawn-id N` — Watch spawn output in real-time\n\n")
	b.WriteString("**Review & merge:**\n")
	b.WriteString("- `adaf spawn-diff --spawn-id N` — View diff of spawn's changes\n")
	b.WriteString("- `adaf spawn-merge --spawn-id N [--squash]` — Merge spawn's changes\n")
	b.WriteString("- `adaf spawn-reject --spawn-id N` — Reject spawn's changes (destroys branch — see below)\n\n")
	b.WriteString("**Mid-flight guidance (while child is still running):**\n")
	b.WriteString("- `adaf spawn-message --spawn-id N \"guidance\"` — Send async guidance to child\n")
	b.WriteString("- `adaf spawn-message --spawn-id N --interrupt \"new priority\"` — Interrupt child's current turn with new instructions\n")
	b.WriteString("- `adaf spawn-reply --spawn-id N \"answer\"` — Reply to child's question\n\n")

	// Reject guidance.
	b.WriteString("### On Rejecting Work\n\n")
	b.WriteString("`spawn-reject` destroys the branch entirely. The next spawn starts from scratch. Before rejecting, consider:\n")
	b.WriteString("- If the child is still running, use `spawn-message --interrupt` to course-correct instead\n")
	b.WriteString("- If the issue is minor (e.g. stale files in diff), write a more detailed task description for the next spawn rather than iterating blindly\n")
	b.WriteString("- If you've already rejected the same task twice, stop and rethink your task description — you are wasting resources\n\n")

	return b.String()
}

func communicationCommands() string {
	var b strings.Builder
	b.WriteString("## Communication with Parent\n\n")
	b.WriteString("- `adaf parent-ask \"question\"` — Ask your parent a question (blocks until answered)\n")
	b.WriteString("- `adaf parent-notify \"status update\"` — Send a non-blocking notification to parent\n")
	b.WriteString("- `adaf spawn-read-messages` — Read messages from parent\n\n")
	return b.String()
}
