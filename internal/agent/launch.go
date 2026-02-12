package agent

import (
	"strings"

	"github.com/agusx1211/adaf/internal/config"
)

// LaunchSpec is the shared command/args/env launch configuration for a profile.
type LaunchSpec struct {
	Command string
	Args    []string
	Env     map[string]string
}

// BuildLaunchSpec builds agent launch settings from a profile.
//
// This is the single source of truth for translating profile fields
// (model/reasoning/agent type) into CLI args and environment variables.
func BuildLaunchSpec(prof *config.Profile, agentsCfg *AgentsConfig, commandOverride string) LaunchSpec {
	if prof == nil {
		return LaunchSpec{}
	}

	spec := LaunchSpec{
		Command: resolveCommandPath(prof.Agent, agentsCfg, commandOverride),
		Args:    make([]string, 0, 4),
		Env:     make(map[string]string),
	}

	modelOverride := strings.TrimSpace(prof.Model)
	reasoningLevel := strings.TrimSpace(prof.ReasoningLevel)

	switch prof.Agent {
	case "claude":
		if modelOverride != "" {
			spec.Args = append(spec.Args, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			spec.Env["CLAUDE_CODE_EFFORT_LEVEL"] = reasoningLevel
		}
		spec.Args = append(spec.Args, "--dangerously-skip-permissions")

	case "codex":
		if modelOverride != "" {
			spec.Args = append(spec.Args, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			spec.Args = append(spec.Args, "-c", `model_reasoning_effort="`+reasoningLevel+`"`)
		}
		spec.Args = append(spec.Args, "--dangerously-bypass-approvals-and-sandbox")

	case "opencode":
		if modelOverride != "" {
			spec.Args = append(spec.Args, "--model", modelOverride)
		}

	case "gemini":
		if modelOverride != "" {
			spec.Args = append(spec.Args, "--model", modelOverride)
		}
		spec.Args = append(spec.Args, "-y")

	case "vibe":
		// Vibe model selection is env-driven.
		if modelOverride != "" {
			spec.Env["VIBE_ACTIVE_MODEL"] = modelOverride
		}
	}

	if len(spec.Env) == 0 {
		spec.Env = nil
	}
	return spec
}

func resolveCommandPath(agentName string, agentsCfg *AgentsConfig, commandOverride string) string {
	if cmd := strings.TrimSpace(commandOverride); cmd != "" {
		return cmd
	}
	if agentsCfg != nil {
		if rec, ok := agentsCfg.Agents[agentName]; ok {
			if cmd := strings.TrimSpace(rec.Path); cmd != "" {
				return cmd
			}
		}
	}

	// For built-in agents, empty command means "use canonical binary name"
	// from each agent runner.
	switch agentName {
	case "claude", "codex", "vibe", "opencode", "gemini", "generic":
		return ""
	default:
		return agentName
	}
}
