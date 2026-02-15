package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/orchestrator"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

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
  adaf spawn --profile developer --task "Write unit tests for auth.go"
  adaf spawn --profile developer --task-file task.md --wait
  adaf spawn --profile lead-dev --task "Review PR #42" --read-only
  adaf spawn-status                       # Check all spawns
  adaf spawn-diff --spawn-id 3            # View changes
  adaf spawn-merge --spawn-id 3           # Merge changes`,
	RunE: runSpawn,
}

func init() {
	spawnCmd.Flags().String("profile", "", "Profile name of the sub-agent to spawn (required)")
	spawnCmd.Flags().String("role", "", "Role for the sub-agent")
	spawnCmd.Flags().String("task", "", "Task description for the sub-agent")
	spawnCmd.Flags().String("task-file", "", "Path to file containing task description (mutually exclusive with --task)")
	spawnCmd.Flags().IntSlice("issue", nil, "Issue ID(s) to assign to the sub-agent (can be repeated)")
	spawnCmd.Flags().Bool("read-only", false, "Run sub-agent in read-only mode (no worktree)")
	spawnCmd.Flags().Bool("wait", false, "Block until the sub-agent completes")
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Flags().GetString("profile")
	childRole, _ := cmd.Flags().GetString("role")
	task, _ := cmd.Flags().GetString("task")
	taskFile, _ := cmd.Flags().GetString("task-file")
	issueIDs, _ := cmd.Flags().GetIntSlice("issue")
	readOnly, _ := cmd.Flags().GetBool("read-only")
	wait, _ := cmd.Flags().GetBool("wait")
	childRole = strings.ToLower(strings.TrimSpace(childRole))

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
	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if childRole != "" && !config.ValidRole(childRole, globalCfg) {
		return fmt.Errorf("invalid --role %q (valid: %s)", childRole, strings.Join(config.AllRoles(globalCfg), ", "))
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

	if daemonSessionID, ok := currentDaemonSessionID(); ok {
		resp, err := session.RequestSpawn(daemonSessionID, session.WireControlSpawn{
			ParentTurnID:  parentTurnID,
			ParentProfile: parentProfile,
			ChildProfile:  profileName,
			ChildRole:     childRole,
			PlanID:        planID,
			Task:          task,
			IssueIDs:      issueIDs,
			ReadOnly:      readOnly,
			Wait:          wait,
			Delegation:    delegation,
		})
		if err != nil {
			return fmt.Errorf("spawn failed: %w", err)
		}
		if resp == nil || !resp.OK {
			if resp != nil && strings.TrimSpace(resp.Error) != "" {
				return fmt.Errorf("spawn failed: %s", resp.Error)
			}
			return fmt.Errorf("spawn failed: daemon returned an empty response")
		}

		roleSuffix := ""
		if childRole != "" {
			roleSuffix = ", role=" + childRole
		}
		fmt.Printf("Spawned sub-agent #%d (profile=%s%s)\n", resp.SpawnID, profileName, roleSuffix)
		if wait {
			fmt.Printf("Spawn #%d completed: status=%s exit_code=%d\n", resp.SpawnID, resp.Status, resp.ExitCode)
			if strings.TrimSpace(resp.Result) != "" {
				fmt.Printf("Result: %s\n", resp.Result)
			}
		}
		return nil
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	spawnID, err := o.Spawn(context.Background(), orchestrator.SpawnRequest{
		ParentTurnID:  parentTurnID,
		ParentProfile: parentProfile,
		ChildProfile:  profileName,
		ChildRole:     childRole,
		PlanID:        planID,
		Task:          task,
		IssueIDs:      issueIDs,
		ReadOnly:      readOnly,
		Wait:          wait,
		Delegation:    delegation,
	})
	if err != nil {
		return fmt.Errorf("spawn failed: %w", err)
	}

	roleSuffix := ""
	if childRole != "" {
		roleSuffix = ", role=" + childRole
	}
	fmt.Printf("Spawned sub-agent #%d (profile=%s%s)\n", spawnID, profileName, roleSuffix)
	if wait {
		result := o.WaitOne(spawnID)
		fmt.Printf("Spawn #%d completed: status=%s exit_code=%d\n", spawnID, result.Status, result.ExitCode)
		if result.Result != "" {
			fmt.Printf("Result: %s\n", result.Result)
		}
	}
	return nil
}

func resolveCurrentDelegation(parentProfile string) (*config.DelegationConfig, error) {
	if raw := strings.TrimSpace(os.Getenv("ADAF_DELEGATION_JSON")); raw != "" {
		var deleg config.DelegationConfig
		if err := json.Unmarshal([]byte(raw), &deleg); err != nil {
			return nil, fmt.Errorf("invalid ADAF_DELEGATION_JSON: %w", err)
		}
		return deleg.Clone(), nil
	}

	runIDStr := strings.TrimSpace(os.Getenv("ADAF_LOOP_RUN_ID"))
	stepIdxStr := strings.TrimSpace(os.Getenv("ADAF_LOOP_STEP_INDEX"))
	if runIDStr == "" && stepIdxStr == "" {
		return nil, fmt.Errorf("spawning is not allowed in this context: no delegation rules found")
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
	if step.Team == "" {
		return &config.DelegationConfig{}, nil
	}
	team := globalCfg.FindTeam(step.Team)
	if team == nil || team.Delegation == nil {
		return &config.DelegationConfig{}, nil
	}
	return team.Delegation.Clone(), nil
}

func getTurnContext() (int, string, error) {
	turnStr := os.Getenv("ADAF_TURN_ID")
	if turnStr == "" {
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

func currentDaemonSessionID() (int, bool) {
	raw := strings.TrimSpace(os.Getenv("ADAF_SESSION_ID"))
	if raw == "" {
		return 0, false
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func ensureOrchestrator() (*orchestrator.Orchestrator, error) {
	o := orchestrator.Get()
	if o != nil {
		return o, nil
	}

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
	if strings.TrimSpace(r.ChildRole) != "" {
		printField("Role", r.ChildRole)
	}
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
