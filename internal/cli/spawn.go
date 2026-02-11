package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/orchestrator"
	"github.com/agusx1211/adaf/internal/store"
)

// --- adaf spawn ---

var spawnCmd = &cobra.Command{
	Use:   "spawn",
	Short: "Spawn a sub-agent to work on a task",
	RunE:  runSpawn,
}

func init() {
	spawnCmd.Flags().String("profile", "", "Profile name of the sub-agent to spawn (required)")
	spawnCmd.Flags().String("task", "", "Task description for the sub-agent (required)")
	spawnCmd.Flags().Bool("read-only", false, "Run sub-agent in read-only mode (no worktree)")
	spawnCmd.Flags().Bool("wait", false, "Block until the sub-agent completes")
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Flags().GetString("profile")
	task, _ := cmd.Flags().GetString("task")
	readOnly, _ := cmd.Flags().GetBool("read-only")
	wait, _ := cmd.Flags().GetBool("wait")

	if profileName == "" {
		return fmt.Errorf("--profile is required")
	}
	if task == "" {
		return fmt.Errorf("--task is required")
	}

	parentSessionID, parentProfile, err := getSessionContext()
	if err != nil {
		return err
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	spawnID, err := o.Spawn(context.Background(), orchestrator.SpawnRequest{
		ParentSessionID: parentSessionID,
		ParentProfile:   parentProfile,
		ChildProfile:    profileName,
		Task:            task,
		ReadOnly:        readOnly,
		Wait:            wait,
	})
	if err != nil {
		return fmt.Errorf("spawn failed: %w", err)
	}

	fmt.Printf("Spawned sub-agent #%d (profile=%s)\n", spawnID, profileName)
	if wait {
		result := o.WaitOne(spawnID)
		fmt.Printf("Spawn #%d completed: status=%s exit_code=%d\n", spawnID, result.Status, result.ExitCode)
		if result.Result != "" {
			fmt.Printf("Result: %s\n", result.Result)
		}
	}
	return nil
}

// --- adaf spawn-status ---

var spawnStatusCmd = &cobra.Command{
	Use:   "spawn-status",
	Short: "Show status of spawned sub-agents",
	RunE:  runSpawnStatus,
}

func init() {
	spawnStatusCmd.Flags().Int("spawn-id", 0, "Show status of a specific spawn")
	rootCmd.AddCommand(spawnStatusCmd)
}

func runSpawnStatus(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	parentSessionID, _, err := getSessionContext()
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

	records, err := s.SpawnsByParent(parentSessionID)
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

// --- adaf spawn-wait ---

var spawnWaitCmd = &cobra.Command{
	Use:   "spawn-wait",
	Short: "Wait for spawned sub-agents to complete",
	RunE:  runSpawnWait,
}

func init() {
	spawnWaitCmd.Flags().Int("spawn-id", 0, "Wait for a specific spawn (0 = all)")
	rootCmd.AddCommand(spawnWaitCmd)
}

func runSpawnWait(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	parentSessionID, _, err := getSessionContext()
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

	results := o.Wait(parentSessionID)
	for _, r := range results {
		fmt.Printf("Spawn #%d: status=%s exit_code=%d\n", r.SpawnID, r.Status, r.ExitCode)
	}
	if len(results) == 0 {
		fmt.Println("No spawns to wait for.")
	}
	return nil
}

// --- adaf spawn-diff ---

var spawnDiffCmd = &cobra.Command{
	Use:   "spawn-diff",
	Short: "Show diff of a spawn's changes",
	RunE:  runSpawnDiff,
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

// --- adaf spawn-merge ---

var spawnMergeCmd = &cobra.Command{
	Use:   "spawn-merge",
	Short: "Merge a spawn's changes into the current branch",
	RunE:  runSpawnMerge,
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

// --- adaf spawn-reject ---

var spawnRejectCmd = &cobra.Command{
	Use:   "spawn-reject",
	Short: "Reject a spawn's changes and clean up",
	RunE:  runSpawnReject,
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

// --- Helpers ---

func getSessionContext() (int, string, error) {
	sessionStr := os.Getenv("ADAF_SESSION_ID")
	profile := os.Getenv("ADAF_PROFILE")

	if sessionStr == "" || profile == "" {
		return 0, "", fmt.Errorf("ADAF_SESSION_ID and ADAF_PROFILE environment variables must be set (are you running inside an adaf agent session?)")
	}

	sessionID, err := strconv.Atoi(sessionStr)
	if err != nil {
		return 0, "", fmt.Errorf("invalid ADAF_SESSION_ID: %w", err)
	}

	return sessionID, profile, nil
}

func ensureOrchestrator() (*orchestrator.Orchestrator, error) {
	o := orchestrator.Get()
	if o != nil {
		return o, nil
	}

	// Initialize on demand.
	s, err := openStoreRequired()
	if err != nil {
		return nil, err
	}

	globalCfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		return nil, fmt.Errorf("loading project: %w", err)
	}

	repoRoot := projCfg.RepoPath
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	return orchestrator.Init(s, globalCfg, repoRoot), nil
}

func printSpawnRecord(r *store.SpawnRecord) {
	fmt.Printf("Spawn #%d:\n", r.ID)
	printField("Profile", r.ChildProfile)
	printField("Status", r.Status)
	printField("Task", truncate(r.Task, 80))
	if r.Branch != "" {
		printField("Branch", r.Branch)
	}
	if r.ReadOnly {
		printField("Mode", "read-only")
	}
	if !r.CompletedAt.IsZero() {
		printField("Duration", r.CompletedAt.Sub(r.StartedAt).Round(1).String())
	}
	if r.MergeCommit != "" {
		printField("Merge Commit", r.MergeCommit)
	}
	if r.Result != "" {
		printField("Result", truncate(r.Result, 120))
	}
	fmt.Println()
}
