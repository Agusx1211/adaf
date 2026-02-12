package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/orchestrator"
	"github.com/agusx1211/adaf/internal/store"
)

// --- adaf spawn ---

var spawnCmd = &cobra.Command{
	Use:     "spawn",
	Aliases: []string{"fork", "sub-agent", "sub_agent", "subagent"},
	Short:   "Spawn a sub-agent to work on a task",
	Long: `Spawn a child agent to work on a subtask in an isolated git worktree.

The child agent runs in its own branch and can be monitored, messaged,
and eventually merged or rejected. Use --read-only for analysis tasks
that don't need a separate worktree.

Must be called from within an adaf agent turn (ADAF_TURN_ID or ADAF_SESSION_ID set).

Examples:
  adaf spawn --profile junior --task "Write unit tests for auth.go"
  adaf spawn --profile junior --task-file task.md --wait
  adaf spawn --profile senior --task "Review PR #42" --read-only
  adaf spawn-status                       # Check all spawns
  adaf spawn-diff --spawn-id 3            # View changes
  adaf spawn-merge --spawn-id 3           # Merge changes`,
	RunE: runSpawn,
}

func init() {
	spawnCmd.Flags().String("profile", "", "Profile name of the sub-agent to spawn (required)")
	spawnCmd.Flags().String("task", "", "Task description for the sub-agent")
	spawnCmd.Flags().String("task-file", "", "Path to file containing task description (mutually exclusive with --task)")
	spawnCmd.Flags().Bool("read-only", false, "Run sub-agent in read-only mode (no worktree)")
	spawnCmd.Flags().Bool("wait", false, "Block until the sub-agent completes")
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Flags().GetString("profile")
	task, _ := cmd.Flags().GetString("task")
	taskFile, _ := cmd.Flags().GetString("task-file")
	readOnly, _ := cmd.Flags().GetBool("read-only")
	wait, _ := cmd.Flags().GetBool("wait")

	if profileName == "" {
		return fmt.Errorf("--profile is required")
	}
	if task != "" && taskFile != "" {
		return fmt.Errorf("--task and --task-file are mutually exclusive")
	}
	if task == "" && taskFile == "" {
		return fmt.Errorf("--task or --task-file is required")
	}
	if taskFile != "" {
		data, err := os.ReadFile(taskFile)
		if err != nil {
			return fmt.Errorf("reading task file: %w", err)
		}
		if len(data) > 100*1024 {
			fmt.Fprintf(os.Stderr, "Warning: task file is %dKB (>100KB), this may be very large for a prompt\n", len(data)/1024)
		}
		task = string(data)
	}

	parentTurnID, parentProfile, err := getTurnContext()
	if err != nil {
		return err
	}
	planID := strings.TrimSpace(os.Getenv("ADAF_PLAN_ID"))

	delegation, err := resolveCurrentDelegation(parentProfile)
	if err != nil {
		return err
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	spawnID, err := o.Spawn(context.Background(), orchestrator.SpawnRequest{
		ParentTurnID: parentTurnID,
		ParentProfile:   parentProfile,
		ChildProfile:    profileName,
		PlanID:          planID,
		Task:            task,
		ReadOnly:        readOnly,
		Wait:            wait,
		Delegation:      delegation,
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

// --- adaf spawn-wait ---

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

// --- adaf spawn-diff ---

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

// --- adaf spawn-merge ---

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

// --- adaf spawn-reject ---

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

// --- adaf spawn-watch ---

var spawnWatchCmd = &cobra.Command{
	Use:     "spawn-watch",
	Aliases: []string{"spawn_watch", "spawnwatch"},
	Short:   "Watch spawn output in real-time",
	RunE:    runSpawnWatch,
}

func init() {
	spawnWatchCmd.Flags().Int("spawn-id", 0, "Spawn ID to watch (required)")
	spawnWatchCmd.Flags().Bool("raw", false, "Print raw NDJSON without formatting")
	rootCmd.AddCommand(spawnWatchCmd)
}

func runSpawnWatch(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	raw, _ := cmd.Flags().GetBool("raw")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	// Wait for child session ID to be set.
	var rec *store.SpawnRecord
	for i := 0; i < 50; i++ { // poll up to 10 seconds
		rec, err = s.GetSpawn(spawnID)
		if err != nil {
			return fmt.Errorf("spawn %d not found: %w", spawnID, err)
		}
		if rec.ChildTurnID > 0 {
			break
		}
		if rec.Status == "completed" || rec.Status == "failed" || rec.Status == "merged" || rec.Status == "rejected" {
			fmt.Printf("Spawn #%d is already %s\n", spawnID, rec.Status)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if rec.ChildTurnID == 0 {
		return fmt.Errorf("spawn %d has not started a session yet", spawnID)
	}

	// Find events file.
	eventsPath := filepath.Join(s.Root(), "records", fmt.Sprintf("%d", rec.ChildTurnID), "events.jsonl")

	// Tail the events file.
	var offset int64
	for {
		// Check if spawn is terminal.
		rec, _ = s.GetSpawn(spawnID)
		terminal := rec != nil && (rec.Status == "completed" || rec.Status == "failed" || rec.Status == "merged" || rec.Status == "rejected")

		f, err := os.Open(eventsPath)
		if err != nil {
			if os.IsNotExist(err) && !terminal {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			if terminal {
				return nil
			}
			return err
		}

		if offset > 0 {
			f.Seek(offset, 0)
		}

		buf := make([]byte, 64*1024)
		n, readErr := f.Read(buf)
		if n > 0 {
			offset += int64(n)
			chunk := string(buf[:n])
			lines := strings.Split(chunk, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if raw {
					fmt.Println(line)
				} else {
					formatEventLine(line)
				}
			}
		}
		f.Close()

		if readErr != nil && terminal {
			return nil
		}

		if n == 0 {
			if terminal {
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// --- adaf spawn-inspect ---

var spawnInspectCmd = &cobra.Command{
	Use:     "spawn-inspect",
	Aliases: []string{"spawn_inspect", "spawninspect"},
	Short:   "Inspect a running spawn's recent activity",
	Long: `Show the child agent's recent stream events, formatted for consumption
by a parent agent. Shows the last few tool calls, reasoning blocks, and output.`,
	RunE: runSpawnInspect,
}

func init() {
	spawnInspectCmd.Flags().Int("spawn-id", 0, "Spawn ID to inspect (required)")
	spawnInspectCmd.Flags().Int("last", 20, "Number of recent events to show")
	rootCmd.AddCommand(spawnInspectCmd)
}

func runSpawnInspect(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	last, _ := cmd.Flags().GetInt("last")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	// Fast path: if this process owns the running orchestrator, inspect in-memory events.
	if o := orchestrator.Get(); o != nil {
		events, err := o.InspectSpawn(spawnID)
		if err == nil {
			if len(events) == 0 {
				fmt.Println("No events recorded yet.")
				return nil
			}

			// Show only the last N events.
			if last > 0 && len(events) > last {
				events = events[len(events)-last:]
			}

			fmt.Printf("Recent activity for spawn #%d (%d events):\n\n", spawnID, len(events))
			for _, ev := range events {
				if ev.Text != "" {
					fmt.Printf("[output] %s\n", truncate(ev.Text, 200))
				} else if ev.Parsed.Type != "" {
					summary := ev.Parsed.Type
					if ev.Parsed.Subtype != "" {
						summary += "/" + ev.Parsed.Subtype
					}
					// Extract a brief content preview from the assistant message.
					content := ""
					if ev.Parsed.AssistantMessage != nil {
						for _, cb := range ev.Parsed.AssistantMessage.Content {
							if cb.Text != "" {
								content = truncate(cb.Text, 150)
								break
							}
						}
					}
					if content != "" {
						fmt.Printf("[%s] %s\n", summary, content)
					} else {
						raw := string(ev.Raw)
						if len(raw) > 200 {
							raw = raw[:200] + "..."
						}
						fmt.Printf("[%s] %s\n", summary, raw)
					}
				}
			}
			return nil
		}
	}

	// Cross-process fallback: inspect persisted recording events.
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	rec, err := s.GetSpawn(spawnID)
	if err != nil {
		return fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}
	if rec.ChildTurnID == 0 {
		fmt.Println("Spawn has not started a child session yet.")
		return nil
	}

	eventsPath := filepath.Join(s.Root(), "records", fmt.Sprintf("%d", rec.ChildTurnID), "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No recorded events yet.")
			return nil
		}
		return fmt.Errorf("reading events: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty = append(nonEmpty, line)
	}
	if len(nonEmpty) == 0 {
		fmt.Println("No recorded events yet.")
		return nil
	}
	if last > 0 && len(nonEmpty) > last {
		nonEmpty = nonEmpty[len(nonEmpty)-last:]
	}

	fmt.Printf("Recent activity for spawn #%d (%d events):\n\n", spawnID, len(nonEmpty))
	for _, line := range nonEmpty {
		formatEventLine(line)
	}
	return nil
}

// formatEventLine formats a single NDJSON recording event for display.
func formatEventLine(line string) {
	var ev store.RecordingEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		fmt.Println(line)
		return
	}

	prefix := fmt.Sprintf("[%s] %s: ", ev.Timestamp.Format("15:04:05"), ev.Type)
	data := ev.Data
	if len(data) > 200 {
		data = data[:200] + "..."
	}
	fmt.Printf("%s%s\n", prefix, data)
}

// --- Helpers ---

func resolveCurrentDelegation(parentProfile string) (*config.DelegationConfig, error) {
	runIDStr := strings.TrimSpace(os.Getenv("ADAF_LOOP_RUN_ID"))
	stepIdxStr := strings.TrimSpace(os.Getenv("ADAF_LOOP_STEP_INDEX"))
	if runIDStr == "" && stepIdxStr == "" {
		// Non-loop session: keep legacy role/profile behavior.
		return nil, nil
	}
	if runIDStr == "" || stepIdxStr == "" {
		return nil, fmt.Errorf("ADAF_LOOP_RUN_ID and ADAF_LOOP_STEP_INDEX must both be set when spawning from a loop step")
	}

	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ADAF_LOOP_RUN_ID: %s", runIDStr)
	}
	stepIdx, err := strconv.Atoi(stepIdxStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ADAF_LOOP_STEP_INDEX: %s", stepIdxStr)
	}

	s, err := openStoreRequired()
	if err != nil {
		return nil, err
	}
	run, err := s.GetLoopRun(runID)
	if err != nil {
		return nil, fmt.Errorf("loading loop run %d: %w", runID, err)
	}

	globalCfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading global config: %w", err)
	}
	loopDef := globalCfg.FindLoop(run.LoopName)
	if loopDef == nil {
		return nil, fmt.Errorf("loop %q not found in global config", run.LoopName)
	}
	if stepIdx < 0 || stepIdx >= len(loopDef.Steps) {
		return nil, fmt.Errorf("loop %q has %d steps; step index %d is out of range", run.LoopName, len(loopDef.Steps), stepIdx)
	}

	step := loopDef.Steps[stepIdx]
	if step.Profile != "" && !strings.EqualFold(step.Profile, parentProfile) {
		return nil, fmt.Errorf("loop step %d profile mismatch: env profile=%q, loop profile=%q", stepIdx, parentProfile, step.Profile)
	}
	if step.Delegation == nil {
		// Explicitly disallow spawn in this step.
		return &config.DelegationConfig{}, nil
	}

	// Return a shallow copy so request-scoped code cannot mutate global config.
	deleg := *step.Delegation
	deleg.Profiles = append([]config.DelegationProfile(nil), step.Delegation.Profiles...)
	return &deleg, nil
}

func getTurnContext() (int, string, error) {
	turnStr := os.Getenv("ADAF_TURN_ID")
	if turnStr == "" {
		// Backward compat: fall back to ADAF_SESSION_ID
		turnStr = os.Getenv("ADAF_SESSION_ID")
	}
	profile := os.Getenv("ADAF_PROFILE")

	if turnStr == "" || profile == "" {
		return 0, "", fmt.Errorf("ADAF_TURN_ID and ADAF_PROFILE environment variables must be set (are you running inside an adaf agent turn?)")
	}

	turnID, err := strconv.Atoi(turnStr)
	if err != nil {
		return 0, "", fmt.Errorf("invalid ADAF_TURN_ID: %w", err)
	}

	return turnID, profile, nil
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
	if r.Status == "running" || r.Status == "awaiting_input" {
		printField("Elapsed", time.Since(r.StartedAt).Round(time.Second).String())
	}
	if !r.CompletedAt.IsZero() {
		printField("Duration", r.CompletedAt.Sub(r.StartedAt).Round(time.Second).String())
	}
	if r.MergeCommit != "" {
		printField("Merge Commit", r.MergeCommit)
	}
	if r.Result != "" {
		printField("Result", truncate(r.Result, 120))
	}
	if r.Status == "awaiting_input" {
		s, err := openStoreRequired()
		if err == nil {
			if ask, err := s.PendingAsk(r.ID); err == nil && ask != nil {
				printField("Question", truncate(ask.Content, 120))
			}
		}
	}
	fmt.Println()
}
