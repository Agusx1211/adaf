package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var spawnDiffCmd = &cobra.Command{
	Use:     "spawn-diff",
	Aliases: []string{"spawn_diff", "spawndiff"},
	Short:   "Show diff of a spawn's changes",
	RunE:    runSpawnDiff,
}

func init() {
	spawnDiffCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	rootCmd.AddCommand(spawnDiffCmd)
}

func runSpawnDiff(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	diff, err := o.Diff(context.Background(), spawnID)
	if err != nil {
		return err
	}
	fmt.Print(diff)
	return nil
}
