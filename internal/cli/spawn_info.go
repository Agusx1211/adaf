package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
)

var spawnInfoCmd = &cobra.Command{
	Use:   "spawn-info",
	Short: "Show available profiles, roles, and delegation capacity",
	Long: `Display delegation info for the current agent context.

Reads ADAF_DELEGATION_JSON to show available profiles, roles, max instances,
and the maximum parallel spawn limit. Use this to discover what you can spawn.`,
	Aliases: []string{"spawn_info", "spawninfo"},
	RunE:    runSpawnInfo,
}

func init() {
	rootCmd.AddCommand(spawnInfoCmd)
}

func runSpawnInfo(cmd *cobra.Command, args []string) error {
	raw := strings.TrimSpace(os.Getenv("ADAF_DELEGATION_JSON"))
	if raw == "" {
		fmt.Println("No delegation context available (ADAF_DELEGATION_JSON not set).")
		fmt.Println("This command is intended to be run from within an adaf agent session with delegation capabilities.")
		return nil
	}

	var deleg config.DelegationConfig
	if err := json.Unmarshal([]byte(raw), &deleg); err != nil {
		return fmt.Errorf("invalid ADAF_DELEGATION_JSON: %w", err)
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fmt.Printf("Maximum concurrent sub-agents: %d\n\n", deleg.EffectiveMaxParallel())

	if style := deleg.DelegationStyleText(); style != "" {
		fmt.Printf("Delegation style: %s\n\n", style)
	}

	if len(deleg.Profiles) == 0 {
		fmt.Println("No profiles available for spawning.")
		return nil
	}

	fmt.Println("Available Profiles:")
	fmt.Println()
	for _, dp := range deleg.Profiles {
		p := globalCfg.FindProfile(dp.Name)
		if p == nil {
			fmt.Printf("  %s (not found in config)\n", dp.Name)
			continue
		}

		fmt.Printf("  %s\n", p.Name)
		fmt.Printf("    Agent: %s\n", p.Agent)

		roles, rolesErr := dp.EffectiveRoles()
		if rolesErr != nil {
			fmt.Printf("    Roles: INVALID (%v)\n", rolesErr)
		} else if len(roles) > 0 {
			fmt.Printf("    Roles: %s\n", strings.Join(roles, ", "))
		}

		if p.Model != "" {
			fmt.Printf("    Model: %s\n", p.Model)
		}
		if p.Intelligence > 0 {
			fmt.Printf("    Intelligence: %d/10\n", p.Intelligence)
		}

		speed := dp.Speed
		if speed == "" {
			speed = p.Speed
		}
		if speed != "" {
			fmt.Printf("    Speed: %s\n", speed)
		}

		maxInstances := p.MaxInstances
		if dp.MaxInstances > 0 {
			maxInstances = dp.MaxInstances
		}
		if maxInstances > 0 {
			fmt.Printf("    Max instances: %d\n", maxInstances)
		}

		if dp.Handoff {
			fmt.Printf("    Handoff: yes\n")
		}
		if dp.Delegation != nil {
			fmt.Printf("    Child spawn profiles: %d\n", len(dp.Delegation.Profiles))
		}
		if p.Description != "" {
			fmt.Printf("    Description: %s\n", p.Description)
		}
		fmt.Println()
	}

	// Show running spawns if store is accessible.
	if s, err := openStore(); err == nil && s.Exists() {
		parentTurnID, _, turnErr := getTurnContext()
		if turnErr == nil && parentTurnID > 0 {
			if records, err := s.SpawnsByParent(parentTurnID); err == nil {
				var running []string
				for _, rec := range records {
					if !isTerminalSpawnStatus(rec.Status) {
						entry := fmt.Sprintf("#%d (profile=%s, status=%s)", rec.ID, rec.ChildProfile, rec.Status)
						running = append(running, entry)
					}
				}
				if len(running) > 0 {
					fmt.Println("Currently Running Spawns:")
					for _, r := range running {
						fmt.Printf("  %s\n", r)
					}
					fmt.Println()
				}
			}
		}
	}

	return nil
}
