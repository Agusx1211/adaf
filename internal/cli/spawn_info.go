package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/profilescore"
)

var spawnInfoCmd = &cobra.Command{
	Use:   "spawn-info",
	Short: "Show available profiles, roles, and delegation capacity",
	Long: `Display delegation info for the current agent context.

Reads ADAF_DELEGATION_JSON (or falls back to ADAF_LOOP_RUN_ID + ADAF_LOOP_STEP_INDEX)
to show available profiles, roles, max instances, and the maximum parallel spawn limit.
Use this to discover what you can spawn.`,
	Aliases: []string{"spawn_info", "spawninfo"},
	RunE:    runSpawnInfo,
}

func init() {
	rootCmd.AddCommand(spawnInfoCmd)
}

func runSpawnInfo(cmd *cobra.Command, args []string) error {
	parentProfile := os.Getenv("ADAF_PROFILE")
	deleg, err := resolveCurrentDelegation(parentProfile)
	if err != nil {
		fmt.Println("No delegation context available.")
		fmt.Printf("  %v\n", err)
		fmt.Println("This command is intended to be run from within an adaf agent session with delegation capabilities.")
		return nil
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	perfByProfile := loadSpawnInfoPerformance(globalCfg)

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
		if cost := config.NormalizeProfileCost(p.Cost); cost != "" {
			fmt.Printf("    Cost: %s\n", cost)
		}

		speed := dp.Speed
		if speed == "" {
			speed = p.Speed
		}
		if speed != "" {
			fmt.Printf("    Speed: %s\n", speed)
		}

		if dp.MaxInstances > 0 {
			fmt.Printf("    Max instances (this option): %d\n", dp.MaxInstances)
		}
		if p.MaxInstances > 0 {
			fmt.Printf("    Max instances (profile global): %d\n", p.MaxInstances)
		}

		if dp.Handoff {
			fmt.Printf("    Handoff: yes\n")
		}
		if p.Description != "" {
			fmt.Printf("    Description: %s\n", p.Description)
		}
		if perf, ok := perfByProfile[strings.ToLower(strings.TrimSpace(p.Name))]; ok && perf.TotalFeedback > 0 {
			fmt.Printf("    Feedback: %d (quality %.2f/10, difficulty %.2f/10, avg duration %s)\n",
				perf.TotalFeedback, perf.AvgQuality, perf.AvgDifficulty, formatSpawnInfoDuration(perf.AvgDurationSecs))
			if len(perf.RoleBreakdown) > 0 {
				var parts []string
				limit := len(perf.RoleBreakdown)
				if limit > 3 {
					limit = 3
				}
				for i := 0; i < limit; i++ {
					br := perf.RoleBreakdown[i]
					parts = append(parts, fmt.Sprintf("%s=%.2f (%d)", br.Name, br.AvgQuality, br.Count))
				}
				fmt.Printf("    By role (quality): %s\n", strings.Join(parts, ", "))
			}
			if len(perf.ParentBreakdown) > 0 {
				var parts []string
				limit := len(perf.ParentBreakdown)
				if limit > 3 {
					limit = 3
				}
				for i := 0; i < limit; i++ {
					br := perf.ParentBreakdown[i]
					parts = append(parts, fmt.Sprintf("%s=%.2f (%d)", br.Name, br.AvgQuality, br.Count))
				}
				fmt.Printf("    By parent (quality): %s\n", strings.Join(parts, ", "))
			}
			if len(perf.Signals) > 0 {
				fmt.Printf("    Signals: %s\n", strings.Join(perf.Signals, "; "))
			}
		}
		fmt.Println()
	}

	// Show running spawns if store is accessible.
	if s, err := openStore(); err == nil && s.Exists() {
		parentTurnID, _, _, turnErr := getTurnContext()
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

func loadSpawnInfoPerformance(globalCfg *config.GlobalConfig) map[string]profilescore.ProfileSummary {
	out := make(map[string]profilescore.ProfileSummary)
	records, err := profilescore.Default().ListFeedback()
	if err != nil {
		return out
	}
	catalog := make([]profilescore.ProfileCatalogEntry, 0, len(globalCfg.Profiles))
	for _, p := range globalCfg.Profiles {
		catalog = append(catalog, profilescore.ProfileCatalogEntry{Name: p.Name, Cost: config.NormalizeProfileCost(p.Cost)})
	}
	report := profilescore.BuildDashboard(catalog, records)
	for _, summary := range report.Profiles {
		out[strings.ToLower(strings.TrimSpace(summary.Profile))] = summary
	}
	return out
}

func formatSpawnInfoDuration(avgDurationSecs float64) string {
	if avgDurationSecs <= 0 {
		return "n/a"
	}
	d := time.Duration(avgDurationSecs * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", avgDurationSecs)
	}
	return d.Round(time.Second).String()
}
