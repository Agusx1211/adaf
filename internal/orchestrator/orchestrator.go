// Package orchestrator manages hierarchical agent spawning, delegation, and merging.
package orchestrator

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/loop"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/worktree"
)

// SpawnRequest describes a request to spawn a sub-agent.
type SpawnRequest struct {
	ParentSessionID int
	ParentProfile   string
	ChildProfile    string
	Task            string
	ReadOnly        bool
	Wait            bool // if true, Spawn blocks until child completes
}

// SpawnResult is the outcome of a completed spawn.
type SpawnResult struct {
	SpawnID  int
	Status   string
	ExitCode int
	Result   string
}

type activeSpawn struct {
	record *store.SpawnRecord
	cancel context.CancelFunc
	done   chan struct{}
}

type pendingSpawn struct {
	req SpawnRequest
	ch  chan spawnOutcome
}

type spawnOutcome struct {
	spawnID int
	err     error
}

// Orchestrator manages sub-agent lifecycle.
type Orchestrator struct {
	store     *store.Store
	globalCfg *config.GlobalConfig
	worktrees *worktree.Manager
	repoRoot  string

	mu        sync.Mutex
	running   map[string]int        // parent profile -> count of running spawns
	instances map[string]int        // child profile -> count of running instances
	queue     []*pendingSpawn
	spawns    map[int]*activeSpawn
}

// New creates an Orchestrator.
func New(s *store.Store, globalCfg *config.GlobalConfig, repoRoot string) *Orchestrator {
	return &Orchestrator{
		store:     s,
		globalCfg: globalCfg,
		worktrees: worktree.NewManager(repoRoot),
		repoRoot:  repoRoot,
		running:   make(map[string]int),
		instances: make(map[string]int),
		spawns:    make(map[int]*activeSpawn),
	}
}

// Spawn starts (or queues) a sub-agent.
func (o *Orchestrator) Spawn(ctx context.Context, req SpawnRequest) (int, error) {
	// Validate parent profile can spawn.
	parentProf := o.globalCfg.FindProfile(req.ParentProfile)
	if parentProf == nil {
		return 0, fmt.Errorf("parent profile %q not found", req.ParentProfile)
	}
	if !config.CanSpawn(parentProf.Role) {
		return 0, fmt.Errorf("profile %q (role=%s) cannot spawn sub-agents", req.ParentProfile, config.EffectiveRole(parentProf.Role))
	}

	// Validate child profile exists and is in spawnable list.
	childProf := o.globalCfg.FindProfile(req.ChildProfile)
	if childProf == nil {
		return 0, fmt.Errorf("child profile %q not found", req.ChildProfile)
	}
	if !isSpawnable(parentProf, req.ChildProfile) {
		return 0, fmt.Errorf("profile %q is not in spawnable_profiles of %q", req.ChildProfile, req.ParentProfile)
	}

	o.mu.Lock()

	// Check child profile instance limit.
	if childProf.MaxInstances > 0 {
		currentInstances := o.instances[req.ChildProfile]
		if currentInstances >= childProf.MaxInstances {
			// Queue the spawn (will be released when an instance of this profile completes).
			ch := make(chan spawnOutcome, 1)
			o.queue = append(o.queue, &pendingSpawn{req: req, ch: ch})
			o.mu.Unlock()

			select {
			case outcome := <-ch:
				return outcome.spawnID, outcome.err
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
	}

	// Check parent concurrency limit.
	maxPar := parentProf.MaxParallel
	if maxPar <= 0 {
		maxPar = 4 // sensible default
	}
	currentRunning := o.running[req.ParentProfile]
	if currentRunning >= maxPar {
		// Queue the spawn.
		ch := make(chan spawnOutcome, 1)
		o.queue = append(o.queue, &pendingSpawn{req: req, ch: ch})
		o.mu.Unlock()

		// Wait for queue slot.
		select {
		case outcome := <-ch:
			return outcome.spawnID, outcome.err
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	o.running[req.ParentProfile]++
	o.instances[req.ChildProfile]++
	o.mu.Unlock()

	return o.startSpawn(ctx, req, parentProf, childProf)
}

func (o *Orchestrator) startSpawn(ctx context.Context, req SpawnRequest, parentProf, childProf *config.Profile) (int, error) {
	// Create spawn record.
	rec := &store.SpawnRecord{
		ParentSessionID: req.ParentSessionID,
		ParentProfile:   req.ParentProfile,
		ChildProfile:    req.ChildProfile,
		Task:            req.Task,
		ReadOnly:        req.ReadOnly,
		Status:          "running",
	}

	var wtPath string
	if !req.ReadOnly {
		branchName := worktree.BranchName(req.ParentSessionID, req.ChildProfile)
		var err error
		wtPath, err = o.worktrees.Create(ctx, branchName)
		if err != nil {
			o.decrementRunning(req.ParentProfile)
			return 0, fmt.Errorf("creating worktree: %w", err)
		}
		rec.Branch = branchName
		rec.WorktreePath = wtPath
	}

	if err := o.store.CreateSpawn(rec); err != nil {
		if wtPath != "" {
			o.worktrees.RemoveWithBranch(ctx, wtPath, rec.Branch)
		}
		o.decrementRunning(req.ParentProfile)
		return 0, fmt.Errorf("creating spawn record: %w", err)
	}

	// Resolve agent.
	agentInstance, ok := agent.Get(childProf.Agent)
	if !ok {
		rec.Status = "failed"
		rec.Result = "agent not found: " + childProf.Agent
		o.store.UpdateSpawn(rec)
		o.decrementRunning(req.ParentProfile)
		return rec.ID, fmt.Errorf("agent %q not found", childProf.Agent)
	}

	// Build child prompt.
	projCfg, _ := o.store.LoadProject()
	childPrompt, _ := promptpkg.Build(promptpkg.BuildOpts{
		Store:           o.store,
		Project:         projCfg,
		Profile:         childProf,
		GlobalCfg:       o.globalCfg,
		Task:            req.Task,
		ReadOnly:        req.ReadOnly,
		ParentSessionID: req.ParentSessionID,
	})

	workDir := o.repoRoot
	if wtPath != "" {
		workDir = wtPath
	}

	// Build agent args (similar to startAgent in TUI).
	var agentArgs []string
	agentEnv := map[string]string{
		"ADAF_SESSION_ID":     fmt.Sprintf("%d", rec.ID),
		"ADAF_PROFILE":        childProf.Name,
		"ADAF_PARENT_SESSION": fmt.Sprintf("%d", req.ParentSessionID),
	}
	modelOverride := childProf.Model
	switch childProf.Agent {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if childProf.ReasoningLevel != "" {
			agentEnv["CLAUDE_CODE_EFFORT_LEVEL"] = childProf.ReasoningLevel
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if childProf.ReasoningLevel != "" {
			agentArgs = append(agentArgs, "-c", `model_reasoning_effort="`+childProf.ReasoningLevel+`"`)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	}

	// Look up custom command path.
	agentsCfg, _ := agent.LoadAgentsConfig(o.store.Root())
	var customCmd string
	if agentsCfg != nil {
		if arec, ok := agentsCfg.Agents[childProf.Agent]; ok && arec.Path != "" {
			customCmd = arec.Path
		}
	}
	if customCmd == "" {
		switch childProf.Agent {
		case "claude", "codex", "vibe", "opencode", "generic":
		default:
			customCmd = childProf.Agent
		}
	}

	agentCfg := agent.Config{
		Name:    childProf.Agent,
		Command: customCmd,
		Args:    agentArgs,
		Env:     agentEnv,
		WorkDir: workDir,
		Prompt:  childPrompt,
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	}

	childCtx, childCancel := context.WithCancel(ctx)
	done := make(chan struct{})

	as := &activeSpawn{
		record: rec,
		cancel: childCancel,
		done:   done,
	}

	o.mu.Lock()
	o.spawns[rec.ID] = as
	o.mu.Unlock()

	// Run the child agent in a goroutine.
	go func() {
		defer close(done)
		defer o.onSpawnComplete(ctx, rec, req.ParentProfile)

		l := &loop.Loop{
			Store:  o.store,
			Agent:  agentInstance,
			Config: agentCfg,
			OnStart: func(sessionID int) {
				rec.ChildSessionID = sessionID
				o.store.UpdateSpawn(rec)
			},
			PromptFunc: func(sessionID int) string {
				msgs, _ := o.store.UnreadMessages(rec.ID, "parent_to_child")
				for _, m := range msgs {
					o.store.MarkMessageRead(m.SpawnID, m.ID)
				}
				newPrompt, _ := promptpkg.Build(promptpkg.BuildOpts{
					Store:           o.store,
					Project:         projCfg,
					Profile:         childProf,
					GlobalCfg:       o.globalCfg,
					Task:            req.Task,
					ReadOnly:        req.ReadOnly,
					ParentSessionID: req.ParentSessionID,
					Messages:        msgs,
				})
				return newPrompt
			},
		}

		err := l.Run(childCtx)
		rec.CompletedAt = time.Now().UTC()
		if err != nil && err != context.Canceled {
			rec.Status = "failed"
			rec.Result = err.Error()
			rec.ExitCode = 1
		} else {
			rec.Status = "completed"
			rec.ExitCode = 0
		}
		o.store.UpdateSpawn(rec)
	}()

	if req.Wait {
		<-done
	}

	return rec.ID, nil
}

func (o *Orchestrator) onSpawnComplete(ctx context.Context, rec *store.SpawnRecord, parentProfile string) {
	o.mu.Lock()
	delete(o.spawns, rec.ID)
	o.decrementRunningLocked(parentProfile)
	o.decrementInstancesLocked(rec.ChildProfile)

	// Check queue for next pending spawn that can now run.
	// A queued spawn becomes eligible when both the parent's MaxParallel
	// and the child's MaxInstances limits have room.
	for i, pending := range o.queue {
		parentProf := o.globalCfg.FindProfile(pending.req.ParentProfile)
		childProf := o.globalCfg.FindProfile(pending.req.ChildProfile)
		if parentProf == nil || childProf == nil {
			// Remove invalid entry.
			o.queue = append(o.queue[:i], o.queue[i+1:]...)
			o.mu.Unlock()
			pending.ch <- spawnOutcome{err: fmt.Errorf("profile not found")}
			return
		}

		// Check both limits.
		maxPar := parentProf.MaxParallel
		if maxPar <= 0 {
			maxPar = 4
		}
		if o.running[pending.req.ParentProfile] >= maxPar {
			continue
		}
		if childProf.MaxInstances > 0 && o.instances[pending.req.ChildProfile] >= childProf.MaxInstances {
			continue
		}

		// This one can run.
		o.queue = append(o.queue[:i], o.queue[i+1:]...)
		o.running[pending.req.ParentProfile]++
		o.instances[pending.req.ChildProfile]++
		o.mu.Unlock()

		spawnID, err := o.startSpawn(ctx, pending.req, parentProf, childProf)
		pending.ch <- spawnOutcome{spawnID: spawnID, err: err}
		return
	}
	o.mu.Unlock()
}

func (o *Orchestrator) decrementRunning(profile string) {
	o.mu.Lock()
	o.decrementRunningLocked(profile)
	o.mu.Unlock()
}

func (o *Orchestrator) decrementRunningLocked(profile string) {
	o.running[profile]--
	if o.running[profile] <= 0 {
		delete(o.running, profile)
	}
}

func (o *Orchestrator) decrementInstancesLocked(profile string) {
	o.instances[profile]--
	if o.instances[profile] <= 0 {
		delete(o.instances, profile)
	}
}

// Wait blocks until all spawns for the given parent session are done.
func (o *Orchestrator) Wait(parentSessionID int) []SpawnResult {
	// Collect active spawns for this parent.
	o.mu.Lock()
	var toWait []*activeSpawn
	for _, as := range o.spawns {
		if as.record.ParentSessionID == parentSessionID {
			toWait = append(toWait, as)
		}
	}
	o.mu.Unlock()

	for _, as := range toWait {
		<-as.done
	}

	// Return results from store.
	records, _ := o.store.SpawnsByParent(parentSessionID)
	var results []SpawnResult
	for _, r := range records {
		results = append(results, SpawnResult{
			SpawnID:  r.ID,
			Status:   r.Status,
			ExitCode: r.ExitCode,
			Result:   r.Result,
		})
	}
	return results
}

// WaitOne blocks until a specific spawn completes.
func (o *Orchestrator) WaitOne(spawnID int) SpawnResult {
	o.mu.Lock()
	as, ok := o.spawns[spawnID]
	o.mu.Unlock()

	if ok {
		<-as.done
	}

	rec, err := o.store.GetSpawn(spawnID)
	if err != nil {
		return SpawnResult{SpawnID: spawnID, Status: "unknown"}
	}
	return SpawnResult{
		SpawnID:  rec.ID,
		Status:   rec.Status,
		ExitCode: rec.ExitCode,
		Result:   rec.Result,
	}
}

// Cancel cancels a running spawn.
func (o *Orchestrator) Cancel(spawnID int) error {
	o.mu.Lock()
	as, ok := o.spawns[spawnID]
	o.mu.Unlock()

	if !ok {
		return fmt.Errorf("spawn %d not found or already completed", spawnID)
	}
	as.cancel()
	return nil
}

// Merge merges a completed spawn's branch into the current branch.
func (o *Orchestrator) Merge(ctx context.Context, spawnID int, squash bool) (string, error) {
	rec, err := o.store.GetSpawn(spawnID)
	if err != nil {
		return "", fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}
	if rec.Status != "completed" {
		return "", fmt.Errorf("spawn %d is %s, not completed", spawnID, rec.Status)
	}
	if rec.Branch == "" {
		return "", fmt.Errorf("spawn %d has no branch (read-only?)", spawnID)
	}

	var hash string
	msg := fmt.Sprintf("Merge spawn #%d (%s): %s", spawnID, rec.ChildProfile, rec.Task)
	if squash {
		hash, err = o.worktrees.MergeSquash(ctx, rec.Branch, msg)
	} else {
		hash, err = o.worktrees.Merge(ctx, rec.Branch, msg)
	}
	if err != nil {
		return "", err
	}

	// Clean up worktree.
	if rec.WorktreePath != "" {
		o.worktrees.RemoveWithBranch(ctx, rec.WorktreePath, rec.Branch)
	}

	rec.Status = "merged"
	rec.MergeCommit = hash
	o.store.UpdateSpawn(rec)

	return hash, nil
}

// Reject rejects a spawn's work and cleans up.
func (o *Orchestrator) Reject(ctx context.Context, spawnID int) error {
	rec, err := o.store.GetSpawn(spawnID)
	if err != nil {
		return fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}

	// Cancel if still running.
	o.mu.Lock()
	if as, ok := o.spawns[spawnID]; ok {
		as.cancel()
	}
	o.mu.Unlock()

	// Clean up worktree.
	if rec.WorktreePath != "" {
		o.worktrees.RemoveWithBranch(ctx, rec.WorktreePath, rec.Branch)
	}

	rec.Status = "rejected"
	return o.store.UpdateSpawn(rec)
}

// Diff returns the diff for a spawn's branch.
func (o *Orchestrator) Diff(ctx context.Context, spawnID int) (string, error) {
	rec, err := o.store.GetSpawn(spawnID)
	if err != nil {
		return "", fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}
	if rec.Branch == "" {
		return "", fmt.Errorf("spawn %d has no branch", spawnID)
	}
	return o.worktrees.Diff(ctx, rec.Branch)
}

// Status returns spawn records for a parent session.
func (o *Orchestrator) Status(parentSessionID int) []store.SpawnRecord {
	records, _ := o.store.SpawnsByParent(parentSessionID)
	return records
}

// CleanupAll cleans up all active worktrees.
func (o *Orchestrator) CleanupAll(ctx context.Context) error {
	return o.worktrees.CleanupAll(ctx)
}

func isSpawnable(parent *config.Profile, childName string) bool {
	if len(parent.SpawnableProfiles) == 0 {
		return true // no restriction = can spawn anything
	}
	for _, name := range parent.SpawnableProfiles {
		if name == childName {
			return true
		}
	}
	return false
}

// --- Singleton ---

var (
	globalOrch   *Orchestrator
	globalOrchMu sync.Mutex
)

// Init initializes the global orchestrator singleton.
func Init(s *store.Store, globalCfg *config.GlobalConfig, repoRoot string) *Orchestrator {
	globalOrchMu.Lock()
	defer globalOrchMu.Unlock()
	globalOrch = New(s, globalCfg, repoRoot)
	return globalOrch
}

// Get returns the global orchestrator singleton, or nil if not initialized.
func Get() *Orchestrator {
	globalOrchMu.Lock()
	defer globalOrchMu.Unlock()
	return globalOrch
}
