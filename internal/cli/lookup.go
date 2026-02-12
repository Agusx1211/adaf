package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var lookupCmd = &cobra.Command{
	Use:     "lookup <hex-id>",
	Aliases: []string{"find", "id"},
	Short:   "Look up a record by hex ID",
	Long: `Search all loop runs and turns for a matching hex ID.

The hex ID can match a loop run, a loop step, or an individual turn.

Examples:
  adaf lookup a3f2b1c9`,
	Args: cobra.ExactArgs(1),
	RunE: runLookup,
}

func init() {
	rootCmd.AddCommand(lookupCmd)
}

func runLookup(cmd *cobra.Command, args []string) error {
	hexID := strings.TrimSpace(args[0])
	if hexID == "" {
		return fmt.Errorf("hex ID is required")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	found := false

	// Search loop runs.
	runs, err := s.ListLoopRuns()
	if err != nil {
		return fmt.Errorf("listing loop runs: %w", err)
	}
	for _, run := range runs {
		if run.HexID == hexID {
			found = true
			printHeader("Loop Run (matched hex_id)")
			printField("Run ID", fmt.Sprintf("#%d", run.ID))
			printField("Hex ID", run.HexID)
			printField("Loop", run.LoopName)
			printField("Status", run.Status)
			printField("Cycle", fmt.Sprintf("%d", run.Cycle+1))
			printField("Started", run.StartedAt.Format("2006-01-02 15:04:05"))
			printField("Turns", fmt.Sprintf("%d", len(run.TurnIDs)))
			fmt.Println()
		}
		for key, stepHex := range run.StepHexIDs {
			if stepHex == hexID {
				found = true
				printHeader("Loop Step (matched step_hex_id)")
				printField("Run ID", fmt.Sprintf("#%d", run.ID))
				if run.HexID != "" {
					printField("Run Hex ID", run.HexID)
				}
				printField("Step Key", key)
				printField("Step Hex ID", stepHex)
				printField("Loop", run.LoopName)
				printField("Status", run.Status)
				fmt.Println()
			}
		}
	}

	// Search turns.
	turns, err := s.ListTurns()
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}
	for _, turn := range turns {
		matched := false
		matchType := ""
		if turn.HexID == hexID {
			matched = true
			matchType = "hex_id"
		} else if turn.LoopRunHexID == hexID {
			matched = true
			matchType = "loop_run_hex_id"
		} else if turn.StepHexID == hexID {
			matched = true
			matchType = "step_hex_id"
		}
		if matched {
			found = true
			printHeader(fmt.Sprintf("Turn (matched %s)", matchType))
			printField("Turn ID", fmt.Sprintf("#%d", turn.ID))
			if turn.HexID != "" {
				printField("Hex ID", turn.HexID)
			}
			if turn.LoopRunHexID != "" {
				printField("Loop Run Hex ID", turn.LoopRunHexID)
			}
			if turn.StepHexID != "" {
				printField("Step Hex ID", turn.StepHexID)
			}
			printField("Agent", turn.Agent)
			printField("Date", turn.Date.Format("2006-01-02 15:04:05"))
			printField("Objective", truncateLookup(turn.Objective, 80))
			fmt.Println()
		}
	}

	if !found {
		fmt.Printf("  %sNo records found for hex ID %q.%s\n", colorDim, hexID, colorReset)
	}

	return nil
}

func truncateLookup(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
