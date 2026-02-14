package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
)

// ProfileResolveOpts holds variant-specific options for profile resolution.
type ProfileResolveOpts struct {
	Prefix         string // e.g. "ask" or "pm" — used in synthetic profile name
	CustomCmd      string // value from --command flag (empty if not supported)
	ReasoningLevel string // value from --reasoning-level flag (empty if not supported)
}

// resolveProfile resolves an agent profile from --profile or --agent/--model flags.
func resolveProfile(cmd *cobra.Command, opts ProfileResolveOpts) (*config.Profile, *config.GlobalConfig, string, error) {
	profileName, _ := cmd.Flags().GetString("profile")
	agentName, _ := cmd.Flags().GetString("agent")
	modelFlag, _ := cmd.Flags().GetString("model")

	modelFlag = strings.TrimSpace(modelFlag)
	profileName = strings.TrimSpace(profileName)
	customCmd := strings.TrimSpace(opts.CustomCmd)
	reasoningLevel := strings.TrimSpace(opts.ReasoningLevel)

	globalCfg, err := config.Load()
	if err != nil {
		return nil, nil, "", fmt.Errorf("loading global config: %w", err)
	}

	agentsCfg, err := agent.LoadAgentsConfig()
	if err != nil {
		return nil, nil, "", fmt.Errorf("loading agent configuration: %w", err)
	}

	// If --profile is given, use that profile directly.
	if profileName != "" {
		prof := globalCfg.FindProfile(profileName)
		if prof == nil {
			return nil, nil, "", fmt.Errorf("profile %q not found", profileName)
		}
		// Allow --model to override the profile's model.
		if modelFlag != "" {
			prof.Model = modelFlag
		}
		if reasoningLevel != "" {
			prof.ReasoningLevel = reasoningLevel
		}
		cmdOverride := customCmd
		if cmdOverride == "" {
			if rec, ok := agentsCfg.Agents[prof.Agent]; ok && strings.TrimSpace(rec.Path) != "" {
				cmdOverride = strings.TrimSpace(rec.Path)
			}
		}
		return prof, globalCfg, cmdOverride, nil
	}

	// Build a profile from --agent/--model flags.
	if _, ok := agent.Get(agentName); !ok {
		return nil, nil, "", fmt.Errorf("unknown agent %q (valid: %s)", agentName, strings.Join(agentNames(), ", "))
	}

	if rec, ok := agentsCfg.Agents[agentName]; ok && customCmd == "" && strings.TrimSpace(rec.Path) != "" {
		customCmd = strings.TrimSpace(rec.Path)
	}

	modelOverride := agent.ResolveModelOverride(agentsCfg, globalCfg, agentName)
	if modelFlag != "" {
		modelOverride = modelFlag
	}

	prof := &config.Profile{
		Name:           fmt.Sprintf("%s:%s", opts.Prefix, agentName),
		Agent:          agentName,
		Model:          modelOverride,
		ReasoningLevel: reasoningLevel,
	}

	return prof, globalCfg, customCmd, nil
}
