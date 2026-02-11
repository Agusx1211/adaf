package prompt

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
)

// RolePrompt returns the role-specific system prompt section for a profile.
// An agent NEVER sees its own intelligence rating.
func RolePrompt(profile *config.Profile, globalCfg *config.GlobalConfig) string {
	role := config.EffectiveRole(profile.Role)

	var b strings.Builder

	switch role {
	case config.RoleManager:
		b.WriteString("# Your Role: MANAGER\n\n")
		b.WriteString("You are a MANAGER agent. You do NOT write code directly. Your job is to:\n")
		b.WriteString("- Break down tasks into smaller, well-defined sub-tasks\n")
		b.WriteString("- Spawn developer agents to handle implementation\n")
		b.WriteString("- Review every diff before merging (use `adaf spawn-diff`)\n")
		b.WriteString("- Merge or reject work based on quality\n\n")
		b.WriteString(delegationCommands())
		b.WriteString(spawnableInfo(profile, globalCfg))

	case config.RoleSenior:
		b.WriteString("# Your Role: SENIOR DEVELOPER\n\n")
		b.WriteString("You are a SENIOR DEVELOPER agent. You can both write code AND delegate tasks. You are strongly encouraged to delegate smaller tasks to junior agents when appropriate.\n\n")
		b.WriteString(delegationCommands())
		b.WriteString(spawnableInfo(profile, globalCfg))

	case config.RoleJunior:
		b.WriteString("# Your Role: JUNIOR DEVELOPER\n\n")
		b.WriteString("You are a JUNIOR DEVELOPER agent. Focus exclusively on your assigned task. You cannot spawn sub-agents.\n\n")

	case config.RoleSupervisor:
		b.WriteString("# Your Role: SUPERVISOR\n\n")
		b.WriteString("You are a SUPERVISOR agent. You review progress and provide guidance via notes. You do NOT write code and you do NOT spawn agents.\n\n")
		b.WriteString("## Supervisor Commands\n\n")
		b.WriteString("- `adaf note add --session <N> --note \"guidance text\"` — Send a note to a running agent session\n")
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

func delegationCommands() string {
	var b strings.Builder
	b.WriteString("## Delegation Commands\n\n")
	b.WriteString("- `adaf spawn --profile <name> --task \"task description\" [--read-only] [--wait]` — Spawn a sub-agent\n")
	b.WriteString("- `adaf spawn-status [--spawn-id N]` — Check spawn status\n")
	b.WriteString("- `adaf spawn-wait [--spawn-id N]` — Wait for spawn(s) to complete\n")
	b.WriteString("- `adaf spawn-diff --spawn-id N` — View diff of spawn's changes\n")
	b.WriteString("- `adaf spawn-merge --spawn-id N [--squash]` — Merge spawn's changes\n")
	b.WriteString("- `adaf spawn-reject --spawn-id N` — Reject spawn's changes\n\n")
	return b.String()
}

func spawnableInfo(profile *config.Profile, globalCfg *config.GlobalConfig) string {
	if len(profile.SpawnableProfiles) == 0 || globalCfg == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Profiles to Spawn\n\n")
	for _, name := range profile.SpawnableProfiles {
		p := globalCfg.FindProfile(name)
		if p == nil {
			b.WriteString(fmt.Sprintf("- **%s** (not found in config)\n", name))
			continue
		}
		line := fmt.Sprintf("- **%s** — agent=%s", p.Name, p.Agent)
		if p.Model != "" {
			line += fmt.Sprintf(", model=%s", p.Model)
		}
		role := config.EffectiveRole(p.Role)
		line += fmt.Sprintf(", role=%s", role)
		if p.Intelligence > 0 {
			line += fmt.Sprintf(", intelligence=%d/10", p.Intelligence)
		}
		if p.Description != "" {
			line += fmt.Sprintf(" — %s", p.Description)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")

	if profile.MaxParallel > 0 {
		fmt.Fprintf(&b, "Maximum concurrent sub-agents: %d\n\n", profile.MaxParallel)
	}

	return b.String()
}
