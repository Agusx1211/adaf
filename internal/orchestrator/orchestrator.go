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
	"github.com/agusx1211/adaf/internal/stream"
	"github.com/agusx1211/adaf/internal/worktree"
)

// SpawnRequest describes a request to spawn a sub-agent.
type SpawnRequest struct {
	ParentTurnID  int
	ParentProfile string
	ChildProfile  string
	PlanID        string
	Task          string
	ReadOnly      bool
	Wait          bool                     // if true, Spawn blocks until child completes
	Delegation    *config.DelegationConfig // explicit delegation config (required for spawning)
}

// SpawnResult is the outcome of a completed spawn.
type SpawnResult struct {
	SpawnID  int
	Status   string
	ExitCode int
	Result   string
}

type activeSpawn struct {
	record      *store.SpawnRecord
	cancel      context.CancelFunc
	done        chan struct{}
	eventBuffer *eventRingBuffer // circular buffer of recent events
	interruptCh chan string      // signals the child loop about an interrupt
}

// eventRingBuffer is a thread-safe circular buffer of recent stream events.
type eventRingBuffer struct {
	mu     sync.RWMutex
	events []stream.RawEvent
	size   int
	pos    int
	full   bool
}

func newEventRingBuffer(size int) *eventRingBuffer {
	return &eventRingBuffer{
		events: make([]stream.RawEvent, size),
		size:   size,
	}
}

func (rb *eventRingBuffer) Add(ev stream.RawEvent) {
	rb.mu.Lock()
	rb.events[rb.pos] = ev
	rb.pos = (rb.pos + 1) % rb.size
	if rb.pos == 0 {
		rb.full = true
	}
	rb.mu.Unlock()
}

func (rb *eventRingBuffer) Snapshot() []stream.RawEvent {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if !rb.full {
		result := make([]stream.RawEvent, rb.pos)
		copy(result, rb.events[:rb.pos])
		return result
	}
	// Buffer is full — return in order starting from pos.
	result := make([]stream.RawEvent, rb.size)
	copy(result, rb.events[rb.pos:])
	copy(result[rb.size-rb.pos:], rb.events[:rb.pos])
	return result
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
	running   map[string]int // parent profile -> count of running spawns
	instances map[string]int // child profile -> count of running instances
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
	// Validate parent profile exists.
	parentProf := o.globalCfg.FindProfile(req.ParentProfile)
	if parentProf == nil {
		return 0, fmt.Errorf("parent profile %q not found", req.ParentProfile)
	}

	// Delegation config is required for spawning.
	deleg := req.Delegation
	strictDelegation := deleg != nil
	if deleg == nil {
		// Backward compatibility: if no delegation config, try legacy role-based check
		// and build a delegation config from SpawnableProfiles.
		if !config.CanSpawn(parentProf.Role) {
			return 0, fmt.Errorf("profile %q cannot spawn sub-agents (no delegation config and role=%s)", req.ParentProfile, config.EffectiveRole(parentProf.Role))
		}
		deleg = legacyDelegation(parentProf)
		req.Delegation = deleg
	}

	// Validate child profile exists and is in delegation profiles list.
	childProf := o.globalCfg.FindProfile(req.ChildProfile)
	if childProf == nil {
		return 0, fmt.Errorf("child profile %q not found", req.ChildProfile)
	}
	if strictDelegation && !deleg.HasProfile(req.ChildProfile) {
		return 0, fmt.Errorf("profile %q is not in delegation profiles of %q", req.ChildProfile, req.ParentProfile)
	}
	if !strictDelegation && !legacyCanSpawn(deleg, req.ChildProfile, parentProf) {
		return 0, fmt.Errorf("profile %q is not in delegation profiles of %q", req.ChildProfile, req.ParentProfile)
	}

	o.mu.Lock()

	// Check child profile instance limit.
	// Use delegation profile's MaxInstances if set, otherwise fall back to child profile's.
	maxInst := childProf.MaxInstances
	if dp := deleg.FindProfile(req.ChildProfile); dp != nil && dp.MaxInstances > 0 {
		maxInst = dp.MaxInstances
	}
	if maxInst > 0 {
		currentInstances := o.instances[req.ChildProfile]
		if currentInstances >= maxInst {
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

	// Check parent concurrency limit from delegation config.
	maxPar := deleg.EffectiveMaxParallel()
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

// legacyDelegation builds a DelegationConfig from legacy Profile fields for backward compatibility.
func legacyDelegation(prof *config.Profile) *config.DelegationConfig {
	deleg := &config.DelegationConfig{
		MaxParallel: prof.MaxParallel,
	}
	for _, name := range prof.SpawnableProfiles {
		deleg.Profiles = append(deleg.Profiles, config.DelegationProfile{Name: name})
	}
	return deleg
}

// legacyCanSpawn checks whether a child profile is allowed under legacy or delegation rules.
// In legacy mode (empty SpawnableProfiles), any profile can be spawned.
func legacyCanSpawn(deleg *config.DelegationConfig, childName string, parentProf *config.Profile) bool {
	if deleg.HasProfile(childName) {
		return true
	}
	// Legacy: empty SpawnableProfiles means "can spawn anything"
	if len(parentProf.SpawnableProfiles) == 0 && len(deleg.Profiles) == 0 {
		return true
	}
	return false
}

func (o *Orchestrator) startSpawn(ctx context.Context, req SpawnRequest, parentProf, childProf *config.Profile) (int, error) {
	// Populate handoff and speed from delegation profile if available.
	var handoff bool
	var speed string
	if req.Delegation != nil {
		if dp := req.Delegation.FindProfile(req.ChildProfile); dp != nil {
			handoff = dp.Handoff
			speed = dp.Speed
		}
	}
	if speed == "" {
		speed = childProf.Speed
	}

	// Create spawn record.
	rec := &store.SpawnRecord{
		ParentTurnID:  req.ParentTurnID,
		ParentProfile: req.ParentProfile,
		ChildProfile:  req.ChildProfile,
		Task:          req.Task,
		ReadOnly:      req.ReadOnly,
		Status:        "running",
		Handoff:       handoff,
		Speed:         speed,
	}

	var wtPath string
	if !req.ReadOnly {
		branchName := worktree.BranchName(req.ParentTurnID, req.ChildProfile)
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
	parentPlanID := req.PlanID
	if parentPlanID == "" {
		if parentTurn, err := o.store.GetTurn(req.ParentTurnID); err == nil && parentTurn != nil {
			parentPlanID = parentTurn.PlanID
		}
	}
	childPrompt, _ := promptpkg.Build(promptpkg.BuildOpts{
		Store:        o.store,
		Project:      projCfg,
		Profile:      childProf,
		GlobalCfg:    o.globalCfg,
		PlanID:       parentPlanID,
		Task:         req.Task,
		ReadOnly:     req.ReadOnly,
		ParentTurnID: req.ParentTurnID,
	})

	workDir := o.repoRoot
	if wtPath != "" {
		workDir = wtPath
	}

	// Build agent args (similar to startAgent in TUI).
	var agentArgs []string
	agentEnv := map[string]string{
		"ADAF_TURN_ID":     fmt.Sprintf("%d", rec.ID),
		"ADAF_PROFILE":     childProf.Name,
		"ADAF_PARENT_TURN": fmt.Sprintf("%d", req.ParentTurnID),
	}
	if parentPlanID != "" {
		agentEnv["ADAF_PLAN_ID"] = parentPlanID
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
		agentArgs = append(agentArgs, "--dangerously-bypass-approvals-and-sandbox")
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	case "gemini":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "-y")
	}

	// Look up custom command path.
	agentsCfg, _ := agent.LoadAgentsConfig()
	var customCmd string
	if agentsCfg != nil {
		if arec, ok := agentsCfg.Agents[childProf.Agent]; ok && arec.Path != "" {
			customCmd = arec.Path
		}
	}
	if customCmd == "" {
		switch childProf.Agent {
		case "claude", "codex", "vibe", "opencode", "gemini", "generic":
		default:
			customCmd = childProf.Agent
		}
	}

	// Set up event buffer for parent inspection.
	eventBuf := newEventRingBuffer(1000)
	streamCh := make(chan stream.RawEvent, 256)

	agentCfg := agent.Config{
		Name:      childProf.Agent,
		Command:   customCmd,
		Args:      agentArgs,
		Env:       agentEnv,
		WorkDir:   workDir,
		Prompt:    childPrompt,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		EventSink: streamCh,
	}

	childCtx, childCancel := context.WithCancel(ctx)
	done := make(chan struct{})
	interruptCh := make(chan string, 1)

	as := &activeSpawn{
		record:      rec,
		cancel:      childCancel,
		done:        done,
		eventBuffer: eventBuf,
		interruptCh: interruptCh,
	}

	o.mu.Lock()
	o.spawns[rec.ID] = as
	o.mu.Unlock()

	// Drain stream events into the ring buffer in the background.
	eventDone := make(chan struct{})
	go func() {
		defer close(eventDone)
		for ev := range streamCh {
			eventBuf.Add(ev)
		}
	}()

	// Watch for interrupt signals written by `adaf spawn-message --interrupt`.
	interruptDone := make(chan struct{})
	go func() {
		defer close(interruptDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-childCtx.Done():
				return
			case <-ticker.C:
				msg := o.store.CheckInterrupt(rec.ID)
				if msg == "" {
					continue
				}
				_ = o.store.ClearInterrupt(rec.ID)
				select {
				case interruptCh <- msg:
				default:
				}
				childCancel()
			}
		}
	}()

	// Run the child agent in a goroutine.
	go func() {
		defer close(done)
		defer func() {
			close(streamCh)
			<-eventDone
		}()
		defer func() {
			childCancel()
			<-interruptDone
		}()
		defer o.onSpawnComplete(ctx, rec, req.ParentProfile)

		l := &loop.Loop{
			Store:  o.store,
			Agent:  agentInstance,
			Config: agentCfg,
			PlanID: parentPlanID,
			OnStart: func(turnID int) {
				rec.ChildTurnID = turnID
				o.store.UpdateSpawn(rec)
			},
			PromptFunc: func(turnID int, supervisorNotes []store.SupervisorNote) string {
				msgs, _ := o.store.UnreadMessages(rec.ID, "parent_to_child")
				for _, m := range msgs {
					o.store.MarkMessageRead(m.SpawnID, m.ID)
				}
				newPrompt, _ := promptpkg.Build(promptpkg.BuildOpts{
					Store:           o.store,
					Project:         projCfg,
					Profile:         childProf,
					GlobalCfg:       o.globalCfg,
					PlanID:          parentPlanID,
					Task:            req.Task,
					ReadOnly:        req.ReadOnly,
					ParentTurnID:    req.ParentTurnID,
					SupervisorNotes: supervisorNotes,
					Messages:        msgs,
				})
				return newPrompt
			},
			OnWait: func(turnID int) []loop.WaitResult {
				// Wait for this child's own spawns to complete.
				results := o.Wait(rec.ChildTurnID)
				var wr []loop.WaitResult
				for _, r := range results {
					childRec, _ := o.store.GetSpawn(r.SpawnID)
					profile := ""
					if childRec != nil {
						profile = childRec.ChildProfile
					}
					wr = append(wr, loop.WaitResult{
						SpawnID:  r.SpawnID,
						Profile:  profile,
						Status:   r.Status,
						ExitCode: r.ExitCode,
						Result:   r.Result,
					})
				}
				return wr
			},
			InterruptCh: interruptCh,
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
	_ = o.store.ClearInterrupt(rec.ID)

	o.mu.Lock()
	delete(o.spawns, rec.ID)
	o.decrementRunningLocked(parentProfile)
	o.decrementInstancesLocked(rec.ChildProfile)

	// Check queue for next pending spawn that can now run.
	// A queued spawn becomes eligible when both the delegation MaxParallel
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

		// Check parent concurrency limit from delegation config.
		deleg := pending.req.Delegation
		maxPar := 4
		if deleg != nil {
			maxPar = deleg.EffectiveMaxParallel()
		}
		if o.running[pending.req.ParentProfile] >= maxPar {
			continue
		}

		// Check child instance limit (delegation profile overrides child profile).
		maxInst := childProf.MaxInstances
		if deleg != nil {
			if dp := deleg.FindProfile(pending.req.ChildProfile); dp != nil && dp.MaxInstances > 0 {
				maxInst = dp.MaxInstances
			}
		}
		if maxInst > 0 && o.instances[pending.req.ChildProfile] >= maxInst {
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

// Wait blocks until all spawns for the given parent turn are done.
func (o *Orchestrator) Wait(parentTurnID int) []SpawnResult {
	// Snapshot non-terminal spawns for this parent and wait until they complete.
	records, _ := o.store.SpawnsByParent(parentTurnID)
	pending := make(map[int]struct{})
	for _, r := range records {
		if !isTerminalSpawnStatus(r.Status) {
			pending[r.ID] = struct{}{}
		}
	}
	if len(pending) > 0 {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for len(pending) > 0 {
			<-ticker.C
			for id := range pending {
				rec, err := o.store.GetSpawn(id)
				if err != nil || isTerminalSpawnStatus(rec.Status) {
					delete(pending, id)
				}
			}
		}
	}

	// Return results from store.
	records, _ = o.store.SpawnsByParent(parentTurnID)
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

// InterruptSpawn sends an interrupt message to a running spawn's loop.
func (o *Orchestrator) InterruptSpawn(spawnID int, message string) error {
	o.mu.Lock()
	as, ok := o.spawns[spawnID]
	o.mu.Unlock()

	if !ok {
		return fmt.Errorf("spawn %d not found or already completed", spawnID)
	}

	// Send interrupt to the child's loop via its interrupt channel.
	select {
	case as.interruptCh <- message:
	default:
		// Channel already has a pending interrupt.
	}

	// Cancel the child's current turn.
	as.cancel()
	return nil
}

// InspectSpawn returns recent stream events from a running spawn's event buffer.
func (o *Orchestrator) InspectSpawn(spawnID int) ([]stream.RawEvent, error) {
	o.mu.Lock()
	as, ok := o.spawns[spawnID]
	o.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("spawn %d not found or already completed", spawnID)
	}

	return as.eventBuffer.Snapshot(), nil
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

// Status returns spawn records for a parent turn.
func (o *Orchestrator) Status(parentTurnID int) []store.SpawnRecord {
	records, _ := o.store.SpawnsByParent(parentTurnID)
	return records
}

// CleanupAll cleans up all active worktrees.
func (o *Orchestrator) CleanupAll(ctx context.Context) error {
	return o.worktrees.CleanupAll(ctx)
}

// ReparentSpawn updates a spawn's parent turn ID, used for handoff across loop steps.
func (o *Orchestrator) ReparentSpawn(spawnID, newParentTurnID int) error {
	rec, err := o.store.GetSpawn(spawnID)
	if err != nil {
		return fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}
	rec.ParentTurnID = newParentTurnID
	rec.HandedOffToTurn = newParentTurnID
	if err := o.store.UpdateSpawn(rec); err != nil {
		return err
	}

	o.mu.Lock()
	if as, ok := o.spawns[spawnID]; ok && as.record != nil {
		as.record.ParentTurnID = newParentTurnID
		as.record.HandedOffToTurn = newParentTurnID
	}
	o.mu.Unlock()
	return nil
}

// ActiveSpawnsForParent returns IDs of currently running spawns for a parent turn.
func (o *Orchestrator) ActiveSpawnsForParent(parentTurnID int) []int {
	o.mu.Lock()
	defer o.mu.Unlock()
	var ids []int
	for _, as := range o.spawns {
		if as.record.ParentTurnID == parentTurnID {
			ids = append(ids, as.record.ID)
		}
	}
	return ids
}

func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "merged", "rejected":
		return true
	default:
		return false
	}
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
