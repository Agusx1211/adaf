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
		b.WriteString("You are a MANAGER agent. You do NOT write code directly. Your job is to:\n")
		b.WriteString("- Break down tasks into smaller, well-defined sub-tasks\n")
		b.WriteString("- Delegate implementation to sub-agents\n")
		b.WriteString("- Review every diff before merging (use `adaf spawn-diff`)\n")
		b.WriteString("- Merge or reject work based on quality\n\n")
		b.WriteString("Use the Delegation section below to determine whether spawning is enabled in this step.\n\n")

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

	// Wait-for-spawns command.
	b.WriteString("- `adaf wait-for-spawns` — Signal that you want to wait for all spawns to complete, then get results in your next turn\n\n")

	// Task quality guidance.
	b.WriteString("When spawning, write a thorough task description — sub-agents only see what you give them. Include relevant context, goals, constraints, and what \"done\" looks like. Use `--task-file` for anything non-trivial.\n\n")

	// Available profiles.
	if len(deleg.Profiles) > 0 && globalCfg != nil {
		b.WriteString("## Available Profiles to Spawn\n\n")
		for _, dp := range deleg.Profiles {
			p := globalCfg.FindProfile(dp.Name)
			if p == nil {
				b.WriteString(fmt.Sprintf("- **%s** (not found in config)\n", dp.Name))
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
	b.WriteString("- `adaf spawn --profile <name> --task \"task description\" [--read-only] [--wait]` — Spawn a sub-agent\n")
	b.WriteString("- `adaf spawn --profile <name> --task-file <path> [--read-only] [--wait]` — Spawn with detailed task from file\n")
	b.WriteString("- `adaf spawn-status [--spawn-id N]` — Check spawn status\n")
	b.WriteString("- `adaf spawn-wait [--spawn-id N]` — Wait for spawn(s) to complete\n")
	b.WriteString("- `adaf spawn-diff --spawn-id N` — View diff of spawn's changes\n")
	b.WriteString("- `adaf spawn-merge --spawn-id N [--squash]` — Merge spawn's changes\n")
	b.WriteString("- `adaf spawn-reject --spawn-id N` — Reject spawn's changes\n")
	b.WriteString("- `adaf spawn-watch --spawn-id N` — Watch spawn output in real-time\n")
	b.WriteString("- `adaf spawn-reply --spawn-id N \"answer\"` — Reply to child's question\n")
	b.WriteString("- `adaf spawn-message --spawn-id N \"guidance\"` — Send async message to child\n")
	b.WriteString("- `adaf spawn-message --spawn-id N --interrupt \"new priority\"` — Interrupt child's current turn\n\n")
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
