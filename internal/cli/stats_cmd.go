package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statsFormat string

func init() {
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show profile and loop statistics",
		RunE:  runStatsOverview,
	}

	statsProfileCmd := &cobra.Command{
		Use:   "profile [name]",
		Short: "Show detailed stats for a profile",
		Args:  cobra.ExactArgs(1),
		RunE:  runStatsProfile,
	}

	statsLoopCmd := &cobra.Command{
		Use:   "loop [name]",
		Short: "Show detailed stats for a loop",
		Args:  cobra.ExactArgs(1),
		RunE:  runStatsLoop,
	}

	statsMigrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Retroactively compute stats from existing recordings",
		RunE:  runStatsMigrate,
	}

	statsCmd.PersistentFlags().StringVar(&statsFormat, "format", "table",
		"output format: table or markdown")

	statsCmd.AddCommand(statsProfileCmd, statsLoopCmd, statsMigrateCmd)
	rootCmd.AddCommand(statsCmd)
}

func runStatsOverview(cmd *cobra.Command, args []string) error {
	if isMarkdownFormat(statsFormat) {
		return fmt.Errorf("markdown output is only supported for 'stats profile' and 'stats loop'")
	}
	if !isTableFormat(statsFormat) {
		return fmt.Errorf("unsupported stats format %q (use table or markdown)", statsFormat)
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	profileStats, _ := s.ListProfileStats()
	loopStats, _ := s.ListLoopStats()

	if len(profileStats) == 0 && len(loopStats) == 0 {
		fmt.Println(colorDim + "No stats found. Run 'adaf stats migrate' to compute from existing recordings." + colorReset)
		return nil
	}

	if len(profileStats) > 0 {
		printHeader("Profiles")
		headers := []string{"Profile", "Runs", "Cost", "Last Run"}
		var rows [][]string
		for _, ps := range profileStats {
			lastRun := colorDim + "never" + colorReset
			if !ps.LastRunAt.IsZero() {
				lastRun = formatTimeAgo(ps.LastRunAt)
			}
			rows = append(rows, []string{
				styleBoldWhite + ps.ProfileName + colorReset,
				fmt.Sprintf("%d", ps.TotalRuns),
				fmt.Sprintf("$%.2f", ps.TotalCostUSD),
				lastRun,
			})
		}
		printTable(headers, rows)
	}

	if len(loopStats) > 0 {
		printHeader("Loops")
		headers := []string{"Loop", "Cycles", "Cost", "Last Run"}
		var rows [][]string
		for _, ls := range loopStats {
			lastRun := colorDim + "never" + colorReset
			if !ls.LastRunAt.IsZero() {
				lastRun = formatTimeAgo(ls.LastRunAt)
			}
			rows = append(rows, []string{
				styleBoldWhite + ls.LoopName + colorReset,
				fmt.Sprintf("%d", ls.TotalCycles),
				fmt.Sprintf("$%.2f", ls.TotalCostUSD),
				lastRun,
			})
		}
		printTable(headers, rows)
	}

	return nil
}
