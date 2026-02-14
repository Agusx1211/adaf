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

func runStatsProfile(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	name := args[0]
	if isMarkdownFormat(statsFormat) {
		return runStatsProfileMarkdown(s, name)
	}
	if !isTableFormat(statsFormat) {
		return fmt.Errorf("unsupported stats format %q (use table or markdown)", statsFormat)
	}

	ps, err := s.GetProfileStats(name)
	if err != nil {
		return err
	}

	if ps.TotalRuns == 0 {
		fmt.Printf("%sNo stats found for profile %q. Run 'adaf stats migrate' to compute from existing recordings.%s\n",
			colorDim, name, colorReset)
		return nil
	}

	printHeader(fmt.Sprintf("Profile: %s", name))
	printField("Runs", fmt.Sprintf("%d (%d success, %d failure)", ps.TotalRuns, ps.SuccessCount, ps.FailureCount))
	printField("Total Cost", fmt.Sprintf("$%.2f", ps.TotalCostUSD))
	printField("Total Tokens", fmt.Sprintf("%s input, %s output", formatTokens(ps.TotalInputTok), formatTokens(ps.TotalOutputTok)))
	printField("Total Duration", formatDuration(ps.TotalDuration))

	if len(ps.ToolCalls) > 0 {
		printField("Top Tools", formatTopTools(ps.ToolCalls))
	}

	if ps.SpawnsCreated > 0 {
		printField("Sub-agents", fmt.Sprintf("%d created", ps.SpawnsCreated))
	}
	if len(ps.SpawnedBy) > 0 {
		var parts []string
		for parent, count := range ps.SpawnedBy {
			parts = append(parts, fmt.Sprintf("%s (%d)", parent, count))
		}
		printField("Triggered By", strings.Join(parts, ", "))
	}

	if len(ps.TurnIDs) > 0 {
		recent := ps.TurnIDs
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		var ids []string
		for _, id := range recent {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		printField("Recent Turns", strings.Join(ids, ", "))
	}

	if !ps.LastRunAt.IsZero() {
		printField("Last Run", formatTimeAgo(ps.LastRunAt))
	}

	return nil
}

func runStatsProfileMarkdown(s *store.Store, name string) error {
	globalCfg, _ := config.Load()
	if globalCfg == nil {
		return fmt.Errorf("global config not found")
	}

	prof := globalCfg.FindProfile(name)
	if prof == nil {
		return fmt.Errorf("profile %q not found", name)
	}

	ps, err := s.GetProfileStats(name)
	if err != nil {
		return err
	}

	fmt.Printf("# Profile Report: %s\n\n", name)

	fmt.Println("## Configuration")
	fmt.Printf("- Agent: %s\n", prof.Agent)
	if prof.Model != "" {
		fmt.Printf("- Model: %s\n", prof.Model)
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
	if prof.Speed != "" {
		fmt.Printf("- Speed: %s\n", prof.Speed)
	}
	fmt.Println()

	if ps.TotalRuns > 0 {
		fmt.Println("## Aggregate Statistics")
		fmt.Printf("- Total Runs: %d (%d success, %d failure)\n", ps.TotalRuns, ps.SuccessCount, ps.FailureCount)
		fmt.Printf("- Total Cost: $%.2f\n", ps.TotalCostUSD)
		fmt.Printf("- Total Tokens: %s input, %s output\n", formatTokens(ps.TotalInputTok), formatTokens(ps.TotalOutputTok))
		fmt.Printf("- Total Duration: %s\n", formatDuration(ps.TotalDuration))
		fmt.Printf("- Average Cost/Run: $%.2f\n", ps.TotalCostUSD/float64(ps.TotalRuns))
		successRate := float64(ps.SuccessCount) / float64(ps.TotalRuns) * 100
		fmt.Printf("- Success Rate: %.0f%%\n", successRate)
		fmt.Println()
	} else {
		fmt.Println("## Aggregate Statistics")
		fmt.Println("- No runs recorded yet.")
		fmt.Println()
	}

	if len(ps.ToolCalls) > 0 {
		fmt.Println("## Tool Usage Patterns")
		type toolCount struct {
			name  string
			count int
		}
		var sorted []toolCount
		for toolName, count := range ps.ToolCalls {
			sorted = append(sorted, toolCount{name: toolName, count: count})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		for _, tc := range sorted {
			fmt.Printf("- %s: %d calls\n", tc.name, tc.count)
		}
		fmt.Println()
	}

	turns, _ := s.ListTurns()
	profileTurns := filterTurnsByProfile(turns, name, prof.Agent)
	if len(profileTurns) > 0 {
		fmt.Println("## Turn History")
		fmt.Println()

		start := 0
		if len(profileTurns) > 20 {
			start = len(profileTurns) - 20
		}

		for _, turn := range profileTurns[start:] {
			metrics, _ := stats.ExtractFromRecording(s, turn.ID)

			outcome := "unknown"
			if turn.BuildState == "success" {
				outcome = "success"
			} else if turn.BuildState != "" {
				outcome = turn.BuildState
			}

			fmt.Printf("### Turn #%d (%s, %s, %s)\n",
				turn.ID,
				turn.Date.Format("2006-01-02"),
				formatDuration(turn.DurationSecs),
				outcome)

			if turn.Objective != "" {
				obj := turn.Objective
				if len(obj) > 200 {
					obj = obj[:200] + "..."
				}
				fmt.Printf("Objective: %s\n", obj)
			}
			if turn.WhatWasBuilt != "" {
				fmt.Printf("Outcome: %s\n", turn.WhatWasBuilt)
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

			if turn.CommitHash != "" {
				fmt.Printf("Git commit: %s\n", turn.CommitHash)
			}
			if turn.KnownIssues != "" {
				fmt.Printf("Issues: %s\n", turn.KnownIssues)
			}
			fmt.Println()
		}
	}

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

	return nil
}

func filterTurnsByProfile(turns []store.Turn, profileName, agentName string) []store.Turn {
	var result []store.Turn
	for _, t := range turns {
		if t.ProfileName == profileName {
			result = append(result, t)
		} else if t.ProfileName == "" && t.Agent == agentName {
			result = append(result, t)
		}
	}
	return result
}
