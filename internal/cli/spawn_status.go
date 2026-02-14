package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var spawnStatusCmd = &cobra.Command{
	Use:     "spawn-status",
	Aliases: []string{"spawn_status", "spawnstatus"},
	Short:   "Show status of spawned sub-agents",
	RunE:    runSpawnStatus,
}

func init() {
	spawnStatusCmd.Flags().Int("spawn-id", 0, "Show status of a specific spawn")
	rootCmd.AddCommand(spawnStatusCmd)
}

func runSpawnStatus(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	parentTurnID, _, err := getTurnContext()
	if err != nil {
		return err
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	if spawnID > 0 {
		rec, err := s.GetSpawn(spawnID)
		if err != nil {
			return fmt.Errorf("spawn %d not found: %w", spawnID, err)
		}
		printSpawnRecord(rec)
		return nil
	}

	records, err := s.SpawnsByParent(parentTurnID)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		fmt.Println("No spawns found for this session.")
		return nil
	}
	for _, r := range records {
		printSpawnRecord(&r)
	}
	return nil
}

var spawnWaitCmd = &cobra.Command{
	Use:     "spawn-wait",
	Aliases: []string{"spawn_wait", "spawnwait"},
	Short:   "Wait for spawned sub-agents to complete",
	RunE:    runSpawnWait,
}

func init() {
	spawnWaitCmd.Flags().Int("spawn-id", 0, "Wait for a specific spawn (0 = all)")
	rootCmd.AddCommand(spawnWaitCmd)
}

func runSpawnWait(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	parentTurnID, _, err := getTurnContext()
	if err != nil {
		return err
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	if spawnID > 0 {
		result := o.WaitOne(spawnID)
		fmt.Printf("Spawn #%d: status=%s exit_code=%d\n", result.SpawnID, result.Status, result.ExitCode)
		return nil
	}

	results := o.Wait(parentTurnID)
	for _, r := range results {
		fmt.Printf("Spawn #%d: status=%s exit_code=%d\n", r.SpawnID, r.Status, r.ExitCode)
	}
	if len(results) == 0 {
		fmt.Println("No spawns to wait for.")
	}
	return nil
}

func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
		return true
	default:
		return false
	}
}
