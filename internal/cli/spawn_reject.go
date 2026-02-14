package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var spawnRejectCmd = &cobra.Command{
	Use:     "spawn-reject",
	Aliases: []string{"spawn_reject", "spawnreject"},
	Short:   "Reject a spawn's changes and clean up",
	RunE:    runSpawnReject,
}

func init() {
	spawnRejectCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	rootCmd.AddCommand(spawnRejectCmd)
}

func runSpawnReject(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	if err := o.Reject(context.Background(), spawnID); err != nil {
		return err
	}
	fmt.Printf("Rejected spawn #%d\n", spawnID)
	return nil
}
