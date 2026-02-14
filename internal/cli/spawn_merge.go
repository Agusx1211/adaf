package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var spawnMergeCmd = &cobra.Command{
	Use:     "spawn-merge",
	Aliases: []string{"spawn_merge", "spawnmerge"},
	Short:   "Merge a spawn's changes into the current branch",
	RunE:    runSpawnMerge,
}

func init() {
	spawnMergeCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	spawnMergeCmd.Flags().Bool("squash", false, "Squash merge instead of merge commit")
	rootCmd.AddCommand(spawnMergeCmd)
}

func runSpawnMerge(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	squash, _ := cmd.Flags().GetBool("squash")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	hash, err := o.Merge(context.Background(), spawnID, squash)
	if err != nil {
		return fmt.Errorf("merge failed: %w", err)
	}
	fmt.Printf("Merged spawn #%d: commit=%s\n", spawnID, hash)
	return nil
}
