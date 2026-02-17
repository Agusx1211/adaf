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

Examples:
  adaf spawn --profile developer --task "Write unit tests for auth.go"
  adaf spawn --profile developer --task-file task.md
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
	rootCmd.AddCommand(spawnCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	profileName, _ := cmd.Flags().GetString("profile")
	childRole, _ := cmd.Flags().GetString("role")
	task, _ := cmd.Flags().GetString("task")
	taskFile, _ := cmd.Flags().GetString("task-file")
	issueIDs, _ := cmd.Flags().GetIntSlice("issue")
	readOnly, _ := cmd.Flags().GetBool("read-only")
	childRole = strings.ToLower(strings.TrimSpace(childRole))

	// Bare invocation: no profile, no task → show contextual guide.
	if profileName == "" && task == "" && taskFile == "" {
		return printSpawnGuide()
	}
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
	parentTurnID, parentProfile, parentPosition, err := getTurnContext()
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
			ParentTurnID:   parentTurnID,
			ParentProfile:  parentProfile,
			ParentPosition: parentPosition,
			ChildProfile:   profileName,
			ChildRole:      childRole,
			PlanID:         planID,
			Task:           task,
			IssueIDs:       issueIDs,
			ReadOnly:       readOnly,
			Delegation:     delegation,
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

		fmt.Printf("Spawned sub-agent #%d (%s)\n", resp.SpawnID, spawnedDescriptor(profileName, childRole))
		return nil
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	spawnID, err := o.Spawn(context.Background(), orchestrator.SpawnRequest{
		ParentTurnID:   parentTurnID,
		ParentProfile:  parentProfile,
		ParentPosition: parentPosition,
		ChildProfile:   profileName,
		ChildRole:      childRole,
		PlanID:         planID,
		Task:           task,
		IssueIDs:       issueIDs,
		ReadOnly:       readOnly,
		Delegation:     delegation,
	})
	if err != nil {
		return fmt.Errorf("spawn failed: %w", err)
	}

	fmt.Printf("Spawned sub-agent #%d (%s)\n", spawnID, spawnedDescriptor(profileName, childRole))
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

func getTurnContext() (int, string, string, error) {
	turnStr := os.Getenv("ADAF_TURN_ID")
	profile := os.Getenv("ADAF_PROFILE")
	position := strings.TrimSpace(os.Getenv("ADAF_POSITION"))
	position = strings.ToLower(strings.TrimSpace(position))
	if !config.ValidPosition(position) {
		position = config.PositionLead
	}

	if turnStr == "" || profile == "" {
		return 0, "", "", fmt.Errorf("ADAF_TURN_ID and ADAF_PROFILE environment variables must be set (are you running inside an adaf agent turn?)")
	}

	turnID, err := strconv.Atoi(turnStr)
	if err != nil {
		return 0, "", "", fmt.Errorf("invalid ADAF_TURN_ID: %w", err)
	}

	return turnID, profile, position, nil
}

func spawnedDescriptor(profileName, role string) string {
	parts := []string{"profile=" + profileName}
	if strings.TrimSpace(role) != "" {
		parts = append(parts, "role="+strings.TrimSpace(role))
	}
	return strings.Join(parts, ", ")
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
	if strings.TrimSpace(r.ChildPosition) != "" {
		printField("Position", r.ChildPosition)
	}
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

// printSpawnGuide outputs a compact, LLM-friendly guide for sub-agent delegation.
// It is shown when `adaf spawn` is called with no arguments.
func printSpawnGuide() error {
	fmt.Println("# Sub-Agent Delegation")
	fmt.Println()
	fmt.Println("## Recommended Flow")
	fmt.Println("1. Spawn all independent tasks in parallel:")
	fmt.Println("   adaf spawn --profile <name> --task \"task description\"")
	fmt.Println("   adaf spawn --profile <name> --task \"task description\"")
	fmt.Println("2. Signal that you're waiting:")
	fmt.Println("   adaf wait-for-spawns")
	fmt.Println("   Your turn will be suspended and automatically resumed with results")
	fmt.Println("   when all sub-agents finish. No need to poll or wait manually.")
	fmt.Println()

	// Available Profiles (best-effort).
	parentProfile := os.Getenv("ADAF_PROFILE")
	if deleg, err := resolveCurrentDelegation(parentProfile); err == nil && deleg != nil && len(deleg.Profiles) > 0 {
		globalCfg, cfgErr := config.Load()
		fmt.Printf("## Available Profiles (max %d concurrent)\n", deleg.EffectiveMaxParallel())
		fmt.Println()
		for _, dp := range deleg.Profiles {
			agent := dp.Name
			model := ""
			if cfgErr == nil {
				if p := globalCfg.FindProfile(dp.Name); p != nil {
					agent = p.Agent
					model = p.Model
				}
			}
			label := fmt.Sprintf("  %s (agent: %s", dp.Name, agent)
			if model != "" {
				label += ", model: " + model
			}
			label += ")"
			fmt.Println(label)

			roles, rolesErr := dp.EffectiveRoles()
			if rolesErr == nil && len(roles) > 0 {
				fmt.Printf("    Roles: %s\n", strings.Join(roles, ", "))
			}
		}
		fmt.Println()
	}

	// Running Spawns (best-effort).
	if parentTurnID, _, _, err := getTurnContext(); err == nil && parentTurnID > 0 {
		if s, err := openStore(); err == nil && s.Exists() {
			if records, err := s.SpawnsByParent(parentTurnID); err == nil && len(records) > 0 {
				fmt.Println("## Running Spawns")
				fmt.Println()
				for _, r := range records {
					elapsed := time.Since(r.StartedAt).Round(time.Second).String()
					if !r.CompletedAt.IsZero() {
						elapsed = r.CompletedAt.Sub(r.StartedAt).Round(time.Second).String()
					}
					fmt.Printf("  #%d %s [%s] %s — %q\n", r.ID, r.ChildProfile, r.Status, elapsed, truncate(r.Task, 60))
				}
				fmt.Println()
			} else {
				fmt.Println("## Running Spawns")
				fmt.Println()
				fmt.Println("  No spawns running.")
				fmt.Println()
			}
		}
	}

	fmt.Println("## Other Commands")
	fmt.Println("  adaf spawn-status              # Check all spawn statuses")
	fmt.Println("  adaf spawn-diff --spawn-id N   # View a spawn's code changes")
	fmt.Println("  adaf spawn-merge --spawn-id N  # Merge a completed spawn")
	fmt.Println("  adaf spawn-reject --spawn-id N # Cancel/reject a spawn")
	fmt.Println("  adaf spawn-inspect --spawn-id N # See a spawn's recent activity")

	return nil
}
