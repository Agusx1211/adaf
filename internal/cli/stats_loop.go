package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

func runStatsLoop(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	name := args[0]
	if isMarkdownFormat(statsFormat) {
		return runStatsLoopMarkdown(s, name)
	}
	if !isTableFormat(statsFormat) {
		return fmt.Errorf("unsupported stats format %q (use table or markdown)", statsFormat)
	}

	ls, err := s.GetLoopStats(name)
	if err != nil {
		return err
	}

	if ls.TotalRuns == 0 {
		fmt.Printf("%sNo stats found for loop %q. Run 'adaf stats migrate' to compute from existing recordings.%s\n",
			colorDim, name, colorReset)
		return nil
	}

	printHeader(fmt.Sprintf("Loop: %s", name))
	printField("Runs", fmt.Sprintf("%d", ls.TotalRuns))
	printField("Total Cycles", fmt.Sprintf("%d", ls.TotalCycles))
	printField("Total Cost", fmt.Sprintf("$%.2f", ls.TotalCostUSD))
	printField("Total Duration", formatDuration(ls.TotalDuration))

	if len(ls.StepStats) > 0 {
		var parts []string
		for profile, count := range ls.StepStats {
			parts = append(parts, fmt.Sprintf("%s (%d)", profile, count))
		}
		printField("Step Profiles", strings.Join(parts, ", "))
	}

	if len(ls.TurnIDs) > 0 {
		recent := ls.TurnIDs
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		var ids []string
		for _, id := range recent {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		printField("Recent Turns", strings.Join(ids, ", "))
	}

	if !ls.LastRunAt.IsZero() {
		printField("Last Run", formatTimeAgo(ls.LastRunAt))
	}

	return nil
}

func runStatsLoopMarkdown(s *store.Store, name string) error {
	globalCfg, _ := config.Load()
	if globalCfg == nil {
		return fmt.Errorf("global config not found")
	}

	loopDef := globalCfg.FindLoop(name)
	if loopDef == nil {
		return fmt.Errorf("loop %q not found", name)
	}

	ls, err := s.GetLoopStats(name)
	if err != nil {
		return err
	}

	fmt.Printf("# Loop Report: %s\n\n", name)

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

	if ls.TotalRuns > 0 {
		fmt.Println("## Aggregate Statistics")
		fmt.Printf("- Total Runs: %d\n", ls.TotalRuns)
		fmt.Printf("- Total Cycles: %d\n", ls.TotalCycles)
		fmt.Printf("- Total Cost: $%.2f\n", ls.TotalCostUSD)
		fmt.Printf("- Total Duration: %s\n", formatDuration(ls.TotalDuration))
		fmt.Printf("- Average Cost/Run: $%.2f\n", ls.TotalCostUSD/float64(ls.TotalRuns))
		fmt.Printf("- Average Cycles/Run: %.1f\n", float64(ls.TotalCycles)/float64(ls.TotalRuns))
		fmt.Println()
	} else {
		fmt.Println("## Aggregate Statistics")
		fmt.Println("- No runs recorded yet.")
		fmt.Println()
	}

	if len(ls.StepStats) > 0 {
		fmt.Println("## Per-Step Breakdown")
		for profile, count := range ls.StepStats {
			fmt.Printf("- %s: %d total runs\n", profile, count)
		}
		fmt.Println()
	}

	if len(ls.TurnIDs) > 0 {
		fmt.Println("## Turn History")
		recent := ls.TurnIDs
		if len(recent) > 20 {
			recent = recent[len(recent)-20:]
		}
		for _, tid := range recent {
			turn, err := s.GetTurn(tid)
			if err != nil {
				continue
			}
			metrics, _ := stats.ExtractFromRecording(s, tid)

			outcome := "unknown"
			if turn.BuildState == "success" {
				outcome = "success"
			} else if turn.BuildState != "" {
				outcome = turn.BuildState
			}

			fmt.Printf("- Turn #%d (%s, %s, %s)",
				tid,
				turn.Date.Format("2006-01-02"),
				formatDuration(turn.DurationSecs),
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
