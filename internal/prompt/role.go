package prompt

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

// RolePrompt returns the role-specific system prompt section for a profile.
// An agent NEVER sees its own intelligence rating.
// Spawning capabilities are no longer emitted here — they come from delegationSection().
func RolePrompt(profile *config.Profile, stepRole string, globalCfg *config.GlobalConfig) string {
	role := config.EffectiveStepRole(stepRole, globalCfg)

	roles := config.DefaultRoleDefinitions()
	rules := config.DefaultPromptRules()
	if globalCfg != nil {
		config.EnsureDefaultRoleCatalog(globalCfg)
		roles = globalCfg.Roles
		rules = globalCfg.PromptRules
	}

	ruleBodies := make(map[string]string, len(rules))
	for _, rule := range rules {
		ruleID := strings.ToLower(strings.TrimSpace(rule.ID))
		if ruleID == "" {
			continue
		}
		ruleBodies[ruleID] = strings.TrimSpace(rule.Body)
	}

	roleTitle := strings.ToUpper(role)
	roleIdentity := ""
	roleDesc := ""
	var ruleIDs []string
	for _, def := range roles {
		if strings.EqualFold(def.Name, role) {
			if strings.TrimSpace(def.Title) != "" {
				roleTitle = strings.TrimSpace(def.Title)
			}
			roleIdentity = strings.TrimSpace(def.Identity)
			roleDesc = strings.TrimSpace(def.Description)
			ruleIDs = append([]string(nil), def.RuleIDs...)
			break
		}
	}

	var b strings.Builder
	b.WriteString("# Your Role: " + roleTitle + "\n\n")
	if roleIdentity != "" {
		b.WriteString(roleIdentity + "\n\n")
	}
	if roleDesc != "" {
		b.WriteString(roleDesc + "\n\n")
	}
	for _, ruleID := range ruleIDs {
		// Communication rules are now runtime-contextual: upstream is injected
		// for spawned sub-agents only, downstream is injected only when the
		// agent actually has delegation capability (see delegationSection).
		if strings.EqualFold(strings.TrimSpace(ruleID), config.RuleCommunicationUpstream) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ruleID), config.RuleCommunicationDownstream) {
			continue
		}
		body := ruleBodies[strings.ToLower(strings.TrimSpace(ruleID))]
		if body == "" {
			continue
		}
		b.WriteString(body + "\n\n")
	}

	if profile.Description != "" {
		b.WriteString("## Your Description\n\n")
		b.WriteString(profile.Description + "\n\n")
	}

	return b.String()
}

// ReadOnlyPrompt returns the read-only mode prompt section.
func ReadOnlyPrompt() string {
	return "# READ-ONLY MODE\n\nYou are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n\nDo NOT write reports into repository files (for example `*.md`, `*.txt`, or TODO files). Return your report in your final assistant message.\n"
}

// delegationSection builds the delegation/spawning prompt section from a DelegationConfig.
func delegationSection(deleg *config.DelegationConfig, globalCfg *config.GlobalConfig, runningSpawns []store.SpawnRecord) string {
	if deleg == nil || len(deleg.Profiles) == 0 {
		return "You cannot spawn sub-agents.\n\n"
	}

	var b strings.Builder
	runningByProfile := make(map[string]int)
	for _, rec := range runningSpawns {
		runningByProfile[strings.ToLower(strings.TrimSpace(rec.ChildProfile))]++
	}

	b.WriteString("# Delegation\n\n")

	// Downstream communication style — only shown when delegation is available.
	if globalCfg != nil {
		if rule := globalCfg.FindPromptRule(config.RuleCommunicationDownstream); rule != nil && rule.Body != "" {
			b.WriteString(rule.Body + "\n\n")
		}
	}

	// Style guidance.
	if style := deleg.DelegationStyleText(); style != "" {
		b.WriteString("**Delegation style:** " + style + "\n\n")
	}

	// Delegation commands.
	b.WriteString(delegationCommands())

	// Task quality guidance.
	b.WriteString("When spawning, write a thorough task description — sub-agents only see what you give them. Include relevant context, goals, constraints, and what \"done\" looks like. Use `--task-file` for anything non-trivial.\n\n")
	b.WriteString("If the same profile is allowed with multiple roles, you MUST pass `--role <role>` in the spawn command.\n\n")

	// Quick-start example.
	b.WriteString("### Quick-Start Example\n\n")
	b.WriteString("```bash\n")
	b.WriteString("# 1. Spawn a scout to understand the codebase\n")
	b.WriteString("adaf spawn --profile <name> --role <role> --read-only --task \"Examine the repo structure, summarize key files, list failing tests\"\n")
	b.WriteString("# 2. Spawn workers for independent tasks\n")
	b.WriteString("adaf spawn --profile <name> --role <role> --task-file /tmp/task1.md\n")
	b.WriteString("adaf spawn --profile <name> --role <role> --task-file /tmp/task2.md\n")
	b.WriteString("# 3. Suspend — costs zero tokens while waiting\n")
	b.WriteString("adaf wait-for-spawns\n")
	b.WriteString("```\n\n")

	if len(runningSpawns) > 0 {
		b.WriteString("## Currently Running Spawns\n\n")
		for _, rec := range runningSpawns {
			line := fmt.Sprintf("- Spawn #%d — profile=%s", rec.ID, rec.ChildProfile)
			if strings.TrimSpace(rec.ChildRole) != "" {
				line += fmt.Sprintf(", role=%s", rec.ChildRole)
			}
			line += fmt.Sprintf(", status=%s", rec.Status)
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

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
			roles, rolesErr := dp.EffectiveRoles()
			if rolesErr != nil {
				line += fmt.Sprintf(", roles=INVALID(%v)", rolesErr)
			} else if len(roles) == 1 {
				line += fmt.Sprintf(", role=%s", roles[0])
			} else if len(roles) > 1 {
				line += fmt.Sprintf(", roles=%s", strings.Join(roles, "/"))
			}
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
			maxInstances := p.MaxInstances
			if dp.MaxInstances > 0 {
				maxInstances = dp.MaxInstances
			}
			if maxInstances > 0 {
				line += fmt.Sprintf(", max_instances=%d", maxInstances)
			}
			running := runningByProfile[strings.ToLower(strings.TrimSpace(p.Name))]
			if maxInstances > 0 {
				line += fmt.Sprintf(", running=%d/%d", running, maxInstances)
				if running >= maxInstances {
					line += " [at-cap]"
				}
			} else if running > 0 {
				line += fmt.Sprintf(", running=%d", running)
			}
			if dp.Handoff {
				line += " [handoff]"
			}
			if dp.Delegation != nil {
				line += fmt.Sprintf(" [child-spawn:%d]", len(dp.Delegation.Profiles))
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
	b.WriteString("3. Stop immediately after that command. Do not run more commands in this turn — the loop will pause this turn and resume you automatically when children complete.\n\n")
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
	b.WriteString("- `adaf spawn --profile <name> [--role <role>] --task \"...\" [--read-only]` — Spawn a sub-agent (non-blocking)\n")
	b.WriteString("- `adaf spawn --profile <name> [--role <role>] --task-file <path> [--read-only]` — Spawn with detailed task from file\n")
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
