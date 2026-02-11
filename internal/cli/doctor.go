package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

func init() {
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Export profile or loop history for LLM analysis",
	}

	doctorProfileCmd := &cobra.Command{
		Use:   "profile [name]",
		Short: "Export profile history as markdown for LLM analysis",
		Args:  cobra.ExactArgs(1),
		RunE:  runDoctorProfile,
	}

	doctorLoopCmd := &cobra.Command{
		Use:   "loop [name]",
		Short: "Export loop history as markdown for LLM analysis",
		Args:  cobra.ExactArgs(1),
		RunE:  runDoctorLoop,
	}

	doctorCmd.AddCommand(doctorProfileCmd, doctorLoopCmd)
	rootCmd.AddCommand(doctorCmd)
}

func runDoctorProfile(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	name := args[0]
	globalCfg, _ := config.Load()

	prof := globalCfg.FindProfile(name)
	if prof == nil {
		return fmt.Errorf("profile %q not found", name)
	}

	ps, err := s.GetProfileStats(name)
	if err != nil {
		return err
	}

	// Header
	fmt.Printf("# Profile Doctor Report: %s\n\n", name)

	// Configuration
	fmt.Println("## Configuration")
	fmt.Printf("- Agent: %s\n", prof.Agent)
	if prof.Model != "" {
		fmt.Printf("- Model: %s\n", prof.Model)
	}
	if prof.Role != "" {
		fmt.Printf("- Role: %s\n", prof.Role)
	}
	if prof.Intelligence > 0 {
		fmt.Printf("- Intelligence: %d/10\n", prof.Intelligence)
	}
	if prof.Description != "" {
		fmt.Printf("- Description: %s\n", prof.Description)
	}
	if prof.ReasoningLevel != "" {
		fmt.Printf("- Reasoning Level: %s\n", prof.ReasoningLevel)
	}
	if len(prof.SpawnableProfiles) > 0 {
		fmt.Printf("- Spawnable: %s\n", strings.Join(prof.SpawnableProfiles, ", "))
	}
	fmt.Println()

	// Aggregate Statistics
	if ps.TotalRuns > 0 {
		fmt.Println("## Aggregate Statistics")
		fmt.Printf("- Total Runs: %d (%d success, %d failure)\n", ps.TotalRuns, ps.SuccessCount, ps.FailureCount)
		fmt.Printf("- Total Cost: $%.2f\n", ps.TotalCostUSD)
		fmt.Printf("- Total Tokens: %s input, %s output\n", formatTokens(ps.TotalInputTok), formatTokens(ps.TotalOutputTok))
		fmt.Printf("- Total Duration: %s\n", formatDuration(ps.TotalDuration))
		if ps.TotalRuns > 0 {
			fmt.Printf("- Average Cost/Run: $%.2f\n", ps.TotalCostUSD/float64(ps.TotalRuns))
			successRate := float64(ps.SuccessCount) / float64(ps.TotalRuns) * 100
			fmt.Printf("- Success Rate: %.0f%%\n", successRate)
		}
		fmt.Println()
	}

	// Tool Usage Patterns
	if len(ps.ToolCalls) > 0 {
		fmt.Println("## Tool Usage Patterns")
		type toolCount struct {
			name  string
			count int
		}
		var sorted []toolCount
		for name, count := range ps.ToolCalls {
			sorted = append(sorted, toolCount{name, count})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		for _, tc := range sorted {
			fmt.Printf("- %s: %d calls\n", tc.name, tc.count)
		}
		fmt.Println()
	}

	// Session History
	logs, _ := s.ListLogs()
	profileLogs := filterLogsByProfile(logs, name, prof.Agent)

	if len(profileLogs) > 0 {
		fmt.Println("## Session History")
		fmt.Println()

		// Show most recent sessions (up to 20)
		start := 0
		if len(profileLogs) > 20 {
			start = len(profileLogs) - 20
		}

		for _, log := range profileLogs[start:] {
			metrics, _ := stats.ExtractFromRecording(s, log.ID)

			outcome := "unknown"
			if log.BuildState == "success" {
				outcome = "success"
			} else if log.BuildState != "" {
				outcome = log.BuildState
			}

			fmt.Printf("### Session #%d (%s, %s, %s)\n",
				log.ID,
				log.Date.Format("2006-01-02"),
				formatDuration(log.DurationSecs),
				outcome)

			if log.Objective != "" {
				obj := log.Objective
				if len(obj) > 200 {
					obj = obj[:200] + "..."
				}
				fmt.Printf("Objective: %s\n", obj)
			}
			if log.WhatWasBuilt != "" {
				fmt.Printf("Outcome: %s\n", log.WhatWasBuilt)
			}

			if metrics != nil {
				fmt.Printf("Cost: $%.4f, Tokens: %s in / %s out\n",
					metrics.TotalCostUSD,
					formatTokens(metrics.InputTokens),
					formatTokens(metrics.OutputTokens))

				if len(metrics.ToolCalls) > 0 {
					var parts []string
					for tool, count := range metrics.ToolCalls {
						parts = append(parts, fmt.Sprintf("%s(%d)", tool, count))
					}
					fmt.Printf("Tools: %s\n", strings.Join(parts, ", "))
				}
			}

			if log.CommitHash != "" {
				fmt.Printf("Git commit: %s\n", log.CommitHash)
			}

			if log.KnownIssues != "" {
				fmt.Printf("Issues: %s\n", log.KnownIssues)
			}

			fmt.Println()
		}
	}

	// Spawn Relationships
	spawns, _ := s.ListSpawns()
	if len(spawns) > 0 {
		childCounts := make(map[string]int)
		parentCounts := make(map[string]int)
		for _, sp := range spawns {
			if sp.ParentProfile == name {
				childCounts[sp.ChildProfile]++
			}
			if sp.ChildProfile == name {
				parentCounts[sp.ParentProfile]++
			}
		}
		if len(childCounts) > 0 || len(parentCounts) > 0 {
			fmt.Println("## Spawn Relationships")
			for child, count := range childCounts {
				fmt.Printf("- Spawned %s %d times\n", child, count)
			}
			for parent, count := range parentCounts {
				fmt.Printf("- Was spawned by %s %d times\n", parent, count)
			}
			fmt.Println()
		}
	}

	// Observations
	if ps.TotalRuns > 0 {
		fmt.Println("## Observations")
		if ps.TotalRuns > 0 {
			avgDuration := ps.TotalDuration / ps.TotalRuns
			fmt.Printf("- Average session duration: %s\n", formatDuration(avgDuration))
		}
		if ps.TotalRuns > 0 {
			successRate := float64(ps.SuccessCount) / float64(ps.TotalRuns) * 100
			fmt.Printf("- Success rate: %.0f%%\n", successRate)
		}
		if ps.TotalRuns > 0 {
			avgCost := ps.TotalCostUSD / float64(ps.TotalRuns)
			fmt.Printf("- Cost trend: $%.2f/run average\n", avgCost)
		}
		fmt.Println()
	}

	return nil
}

func runDoctorLoop(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	name := args[0]
	globalCfg, _ := config.Load()

	loopDef := globalCfg.FindLoop(name)
	if loopDef == nil {
		return fmt.Errorf("loop %q not found", name)
	}

	ls, err := s.GetLoopStats(name)
	if err != nil {
		return err
	}

	// Header
	fmt.Printf("# Loop Doctor Report: %s\n\n", name)

	// Configuration
	fmt.Println("## Configuration")
	fmt.Printf("- Steps: %d\n", len(loopDef.Steps))
	for i, step := range loopDef.Steps {
		turns := step.Turns
		if turns == 0 {
			turns = 1
		}
		fmt.Printf("  %d. %s (turns: %d", i+1, step.Profile, turns)
		if step.CanStop {
			fmt.Print(", can_stop")
		}
		if step.CanMessage {
			fmt.Print(", can_message")
		}
		fmt.Println(")")
		if step.Instructions != "" {
			fmt.Printf("     Instructions: %s\n", step.Instructions)
		}
	}
	fmt.Println()

	// Aggregate Statistics
	if ls.TotalRuns > 0 {
		fmt.Println("## Aggregate Statistics")
		fmt.Printf("- Total Runs: %d\n", ls.TotalRuns)
		fmt.Printf("- Total Cycles: %d\n", ls.TotalCycles)
		fmt.Printf("- Total Cost: $%.2f\n", ls.TotalCostUSD)
		fmt.Printf("- Total Duration: %s\n", formatDuration(ls.TotalDuration))
		if ls.TotalRuns > 0 {
			fmt.Printf("- Average Cost/Run: $%.2f\n", ls.TotalCostUSD/float64(ls.TotalRuns))
			fmt.Printf("- Average Cycles/Run: %.1f\n", float64(ls.TotalCycles)/float64(ls.TotalRuns))
		}
		fmt.Println()
	}

	// Per-Step Breakdown
	if len(ls.StepStats) > 0 {
		fmt.Println("## Per-Step Breakdown")
		for profile, count := range ls.StepStats {
			fmt.Printf("- %s: %d total runs\n", profile, count)
		}
		fmt.Println()
	}

	// Session History
	if len(ls.SessionIDs) > 0 {
		fmt.Println("## Session History")
		recent := ls.SessionIDs
		if len(recent) > 20 {
			recent = recent[len(recent)-20:]
		}
		for _, sid := range recent {
			log, err := s.GetLog(sid)
			if err != nil {
				continue
			}
			metrics, _ := stats.ExtractFromRecording(s, sid)

			outcome := "unknown"
			if log.BuildState == "success" {
				outcome = "success"
			} else if log.BuildState != "" {
				outcome = log.BuildState
			}

			fmt.Printf("- Session #%d (%s, %s, %s)",
				sid,
				log.Date.Format("2006-01-02"),
				formatDuration(log.DurationSecs),
				outcome)

			if metrics != nil && metrics.TotalCostUSD > 0 {
				fmt.Printf(" cost=$%.4f", metrics.TotalCostUSD)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	return nil
}

// filterLogsByProfile returns session logs matching a profile name or agent name.
func filterLogsByProfile(logs []store.SessionLog, profileName, agentName string) []store.SessionLog {
	var result []store.SessionLog
	for _, log := range logs {
		if log.ProfileName == profileName {
			result = append(result, log)
		} else if log.ProfileName == "" && log.Agent == agentName {
			result = append(result, log)
		}
	}
	return result
}
