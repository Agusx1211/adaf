// Package orchestrator manages hierarchical agent spawning, delegation, and merging.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/events"
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
	ChildRole     string
	PlanID        string
	Task          string
	IssueIDs      []int
	ReadOnly      bool
	Wait          bool                     // if true, Spawn blocks until child completes
	Delegation    *config.DelegationConfig // parent delegation config (required for strict spawning)

	// Resolved child execution settings populated during Spawn validation.
	ChildDelegation   *config.DelegationConfig
	ChildMaxInstances int
	ChildSpeed        string
	ChildHandoff      bool
}

// SpawnResult is the outcome of a completed spawn.
type SpawnResult struct {
	SpawnID  int
	Status   string
	ExitCode int
	Result   string
	Summary  string // child's final output
	ReadOnly bool   // whether this was a read-only scout
	Branch   string // worktree branch (empty for read-only)
}

type activeSpawn struct {
	spawnID       int
	parentProfile string
	childProfile  string

	metaMu       sync.RWMutex
	parentTurnID int
	recordMu     sync.Mutex

	cancel      context.CancelFunc
	done        chan struct{}
	eventBuffer *eventRingBuffer // circular buffer of recent events
	interruptCh chan string      // signals the child loop about an interrupt
}

func (as *activeSpawn) ParentTurnID() int {
	if as == nil {
		return 0
	}
	as.metaMu.RLock()
	defer as.metaMu.RUnlock()
	return as.parentTurnID
}

func (as *activeSpawn) SetParentTurnID(turnID int) {
	if as == nil {
		return
	}
	as.metaMu.Lock()
	as.parentTurnID = turnID
	as.metaMu.Unlock()
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

// Orchestrator manages sub-agent lifecycle.
type Orchestrator struct {
	store     *store.Store
	globalCfg *config.GlobalConfig
	worktrees *worktree.Manager
	repoRoot  string

	mu        sync.Mutex
	running   map[string]int // parent profile -> count of running spawns
	instances map[string]int // child profile -> count of running instances
	spawns    map[int]*activeSpawn
	waitAny   map[int]chan struct{} // parent turn -> completion notification channel
	waiters   map[int]int           // parent turn -> active WaitAny waiter count
	spawnWG   sync.WaitGroup        // tracks running spawn goroutines
	eventCh   chan any              // optional event channel for real-time sub-agent events
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
		waitAny:   make(map[int]chan struct{}),
		waiters:   make(map[int]int),
	}
}

// SetEventCh sets the event channel so sub-agent lifecycle events
// (prompts, finishes, raw output) are forwarded to the runtime event stream.
func (o *Orchestrator) SetEventCh(ch chan any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.eventCh = ch
}

func (o *Orchestrator) emitEvent(eventType string, msg any) bool {
	o.mu.Lock()
	ch := o.eventCh
	o.mu.Unlock()
	if ch == nil {
		return false
	}
	sent, closed := safeOfferAny(ch, msg)
	if sent {
		return true
	}
	if closed {
		o.mu.Lock()
		if o.eventCh == ch {
			o.eventCh = nil
		}
		o.mu.Unlock()
		debug.LogKV("orch", "dropping event: channel closed", "type", eventType)
		return false
	}
	debug.LogKV("orch", "dropping event due to backpressure", "type", eventType)
	return false
}

func safeOfferAny(ch chan any, msg any) (sent bool, closed bool) {
	defer func() {
		if recover() != nil {
			sent = false
			closed = true
		}
	}()
	select {
	case ch <- msg:
		return true, false
	default:
		return false, false
	}
}

func (o *Orchestrator) acquireWaitAny(parentTurnID int) chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.waitAny == nil {
		o.waitAny = make(map[int]chan struct{})
	}
	if o.waiters == nil {
		o.waiters = make(map[int]int)
	}
	ch := o.waitAny[parentTurnID]
	if ch == nil {
		ch = make(chan struct{}, 1)
		o.waitAny[parentTurnID] = ch
	}
	o.waiters[parentTurnID]++
	return ch
}

func (o *Orchestrator) releaseWaitAny(parentTurnID int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.waiters == nil {
		return
	}
	next := o.waiters[parentTurnID] - 1
	if next > 0 {
		o.waiters[parentTurnID] = next
		return
	}
	delete(o.waiters, parentTurnID)
	delete(o.waitAny, parentTurnID)
}

func (o *Orchestrator) signalWaitAny(parentTurnID int) {
	if parentTurnID <= 0 {
		return
	}
	o.mu.Lock()
	ch := o.waitAny[parentTurnID]
	o.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (o *Orchestrator) withSpawnRecordLock(spawnID int, fn func(*store.SpawnRecord) error) error {
	var as *activeSpawn
	o.mu.Lock()
	as = o.spawns[spawnID]
	o.mu.Unlock()
	if as != nil {
		as.recordMu.Lock()
		defer as.recordMu.Unlock()
	}
	rec, err := o.store.GetSpawn(spawnID)
	if err != nil {
		return err
	}
	if err := fn(rec); err != nil {
		return err
	}
	return o.store.UpdateSpawn(rec)
}

const promptEventLimitBytes = 256 * 1024

func truncatePrompt(prompt string) (string, bool, int) {
	origLen := len(prompt)
	if origLen <= promptEventLimitBytes {
		return prompt, false, origLen
	}
	if promptEventLimitBytes <= 0 {
		return "", true, origLen
	}
	cut := promptEventLimitBytes
	if cut > len(prompt) {
		cut = len(prompt)
	}
	suffix := fmt.Sprintf("\n\n[Prompt truncated to %d bytes; original=%d bytes]\n", cut, origLen)
	return prompt[:cut] + suffix, true, origLen
}

// Spawn starts a sub-agent.
func (o *Orchestrator) Spawn(ctx context.Context, req SpawnRequest) (int, error) {
	req.ChildRole = strings.ToLower(strings.TrimSpace(req.ChildRole))
	debug.LogKV("orch", "Spawn() called",
		"parent_turn", req.ParentTurnID,
		"parent_profile", req.ParentProfile,
		"child_profile", req.ChildProfile,
		"child_role", req.ChildRole,
		"read_only", req.ReadOnly,
		"wait", req.Wait,
		"task_len", len(req.Task),
	)

	// Validate parent profile exists.
	if o.globalCfg.FindProfile(req.ParentProfile) == nil {
		return 0, fmt.Errorf("parent profile %q not found", req.ParentProfile)
	}

	if strings.TrimSpace(req.Task) == "" {
		return 0, fmt.Errorf("task is required for spawning a sub-agent")
	}

	deleg := req.Delegation
	if deleg == nil {
		return 0, fmt.Errorf("spawning requires explicit delegation rules in the current loop/agent context")
	}

	// Validate child profile exists and resolve delegation option.
	childProf := o.globalCfg.FindProfile(req.ChildProfile)
	if childProf == nil {
		return 0, fmt.Errorf("child profile %q not found", req.ChildProfile)
	}

	resolved, resolvedRole, err := deleg.ResolveProfile(req.ChildProfile, req.ChildRole)
	if err != nil {
		return 0, err
	}
	req.ChildRole = resolvedRole
	if !config.ValidRole(req.ChildRole, o.globalCfg) {
		return 0, fmt.Errorf("child role %q is not defined in the roles catalog", req.ChildRole)
	}
	req.ChildHandoff = resolved.Handoff
	req.ChildSpeed = resolved.Speed
	if resolved.MaxInstances > 0 {
		req.ChildMaxInstances = resolved.MaxInstances
	}
	if resolved.Delegation != nil {
		req.ChildDelegation = resolved.Delegation.Clone()
	} else {
		// Nil child rules means explicit no-spawn for this child.
		req.ChildDelegation = &config.DelegationConfig{}
	}

	o.mu.Lock()

	// Check child profile instance limit.
	maxInst := childProf.MaxInstances
	if req.ChildMaxInstances > 0 {
		maxInst = req.ChildMaxInstances
	}
	if maxInst > 0 {
		currentInstances := o.instances[req.ChildProfile]
		if currentInstances >= maxInst {
			o.mu.Unlock()
			return 0, fmt.Errorf(
				"spawn limit reached: child profile %q has %d running instance(s) (max %d)",
				req.ChildProfile, currentInstances, maxInst,
			)
		}
	}

	// Check parent concurrency limit from delegation config.
	maxPar := deleg.EffectiveMaxParallel()
	currentRunning := o.running[req.ParentProfile]
	if currentRunning >= maxPar {
		o.mu.Unlock()
		return 0, fmt.Errorf(
			"spawn limit reached: parent profile %q has %d running sub-agent(s) (max %d)",
			req.ParentProfile, currentRunning, maxPar,
		)
	}

	o.running[req.ParentProfile]++
	o.instances[req.ChildProfile]++
	runningCount := o.running[req.ParentProfile]
	instanceCount := o.instances[req.ChildProfile]
	o.mu.Unlock()

	debug.LogKV("orch", "spawn starting immediately",
		"parent_profile", req.ParentProfile,
		"child_profile", req.ChildProfile,
		"running", runningCount,
		"instances", instanceCount,
	)
	return o.startSpawn(ctx, req, childProf)
}

func (o *Orchestrator) startSpawn(ctx context.Context, req SpawnRequest, childProf *config.Profile) (int, error) {
	debug.LogKV("orch", "startSpawn()",
		"parent_profile", req.ParentProfile,
		"child_profile", req.ChildProfile,
		"child_role", req.ChildRole,
		"child_agent", childProf.Agent,
		"read_only", req.ReadOnly,
	)
	handoff := req.ChildHandoff
	speed := req.ChildSpeed
	if speed == "" {
		speed = childProf.Speed
	}

	// Create spawn record.
	rec := &store.SpawnRecord{
		ParentTurnID:  req.ParentTurnID,
		ParentProfile: req.ParentProfile,
		ChildProfile:  req.ChildProfile,
		ChildRole:     req.ChildRole,
		Task:          req.Task,
		IssueIDs:      req.IssueIDs,
		ReadOnly:      req.ReadOnly,
		Status:        "running",
		Handoff:       handoff,
		Speed:         speed,
	}

	var wtPath string
	if !req.ReadOnly {
		branchName, createdPath, err := o.createWritableWorktree(ctx, req.ParentTurnID, req.ChildProfile)
		if err != nil {
			o.releaseSpawnSlot(req.ParentProfile, req.ChildProfile)
			return 0, fmt.Errorf("creating worktree: %w", err)
		}
		wtPath = createdPath
		rec.Branch = branchName
		rec.WorktreePath = wtPath
	} else {
		// Read-only spawns get an isolated worktree (detached HEAD) so
		// concurrent agents don't contend for lock files in the same directory.
		// Note: these worktrees are at HEAD and won't see uncommitted changes.
		if p, err := o.createReadOnlyWorktree(ctx, req.ParentTurnID, req.ChildProfile); err == nil {
			wtPath = p
			rec.WorktreePath = wtPath
		} else {
			debug.LogKV("orch", "read-only worktree create failed; falling back to repo root",
				"parent_turn", req.ParentTurnID,
				"child_profile", req.ChildProfile,
				"error", err,
			)
		}
		// If creation fails, fall back to repoRoot.
	}

	if err := o.store.CreateSpawn(rec); err != nil {
		if wtPath != "" {
			if rmErr := o.worktrees.RemoveWithBranch(ctx, wtPath, rec.Branch); rmErr != nil {
				debug.LogKV("orch", "worktree cleanup failed after spawn record error",
					"worktree", wtPath, "branch", rec.Branch, "error", rmErr)
			}
		}
		o.releaseSpawnSlot(req.ParentProfile, req.ChildProfile)
		debug.LogKV("orch", "spawn record creation failed", "error", err)
		return 0, fmt.Errorf("creating spawn record: %w", err)
	}
	debug.LogKV("orch", "spawn record created",
		"spawn_id", rec.ID,
		"branch", rec.Branch,
		"worktree", rec.WorktreePath,
		"handoff", handoff,
		"speed", speed,
	)

	// Resolve agent.
	agentInstance, ok := agent.Get(childProf.Agent)
	if !ok {
		rec.Status = "failed"
		rec.Result = "agent not found: " + childProf.Agent
		o.store.UpdateSpawn(rec)
		if wtPath != "" {
			if rmErr := o.worktrees.RemoveWithBranch(ctx, wtPath, rec.Branch); rmErr != nil {
				debug.LogKV("orch", "worktree cleanup failed after agent lookup error",
					"worktree", wtPath, "branch", rec.Branch, "error", rmErr)
			}
		}
		o.releaseSpawnSlot(req.ParentProfile, req.ChildProfile)
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
		Role:         req.ChildRole,
		GlobalCfg:    o.globalCfg,
		PlanID:       parentPlanID,
		Task:         req.Task,
		ReadOnly:     req.ReadOnly,
		ParentTurnID: req.ParentTurnID,
		Delegation:   req.ChildDelegation,
	})

	workDir := o.repoRoot
	if wtPath != "" {
		workDir = wtPath
	}

	agentEnv := map[string]string{
		"ADAF_TURN_ID":     fmt.Sprintf("%d", rec.ID),
		"ADAF_PROFILE":     childProf.Name,
		"ADAF_PARENT_TURN": fmt.Sprintf("%d", req.ParentTurnID),
	}
	if req.ChildRole != "" {
		agentEnv["ADAF_ROLE"] = req.ChildRole
	}
	if parentPlanID != "" {
		agentEnv["ADAF_PLAN_ID"] = parentPlanID
	}
	if req.ChildDelegation != nil {
		if delegationJSON, err := json.Marshal(req.ChildDelegation); err == nil {
			agentEnv["ADAF_DELEGATION_JSON"] = string(delegationJSON)
		} else {
			debug.LogKV("orch", "failed to encode child delegation for env",
				"spawn_parent_profile", req.ParentProfile,
				"spawn_child_profile", req.ChildProfile,
				"error", err,
			)
		}
	}

	// Look up custom command path.
	agentsCfg, _ := agent.LoadAgentsConfig()
	launch := agent.BuildLaunchSpec(childProf, agentsCfg, "")
	for k, v := range launch.Env {
		agentEnv[k] = v
	}

	// Set up event buffer for parent inspection.
	eventBuf := newEventRingBuffer(1000)
	streamCh := make(chan stream.RawEvent, 256)

	agentCfg := agent.Config{
		Name:      childProf.Agent,
		Command:   launch.Command,
		Args:      append([]string(nil), launch.Args...),
		Env:       agentEnv,
		WorkDir:   workDir,
		Prompt:    childPrompt,
		MaxTurns:  1,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		EventSink: streamCh,
	}

	childCtx, childCancel := context.WithCancel(ctx)
	done := make(chan struct{})
	interruptCh := make(chan string, 8)

	as := &activeSpawn{
		spawnID:       rec.ID,
		parentProfile: req.ParentProfile,
		childProfile:  req.ChildProfile,
		parentTurnID:  req.ParentTurnID,
		cancel:        childCancel,
		done:          done,
		eventBuffer:   eventBuf,
		interruptCh:   interruptCh,
	}

	o.mu.Lock()
	o.spawns[rec.ID] = as
	o.mu.Unlock()

	// Bridge stream events into the ring buffer and forward them to eventCh for live sessions.
	eventDone := make(chan struct{})
	go func() {
		defer close(eventDone)
		for ev := range streamCh {
			eventBuf.Add(ev)

			if ev.Err != nil {
				continue
			}
			if ev.Text != "" {
				o.emitEvent("agent_raw_output", events.AgentRawOutputMsg{Data: ev.Text, SessionID: -rec.ID})
				continue
			}
			o.emitEvent("agent_event", events.AgentEventMsg{Event: ev.Parsed, Raw: ev.Raw})
		}
	}()

	// Watch for interrupt signals written by `adaf spawn-message --interrupt`.
	interruptDone := make(chan struct{})
	go func() {
		defer close(interruptDone)
		defer o.store.ReleaseInterruptSignal(rec.ID)
		signalCh := o.store.InterruptSignalChan(rec.ID)
		pollTicker := time.NewTicker(2 * time.Second)
		defer pollTicker.Stop()
		for {
			select {
			case <-childCtx.Done():
				return
			case msg := <-signalCh:
				if strings.TrimSpace(msg) == "" {
					continue
				}
				_ = o.store.ClearInterrupt(rec.ID)
				select {
				case interruptCh <- msg:
				default:
					debug.LogKV("orch", "interrupt dropped: channel full",
						"spawn_id", rec.ID,
					)
				}
				childCancel()
			case <-pollTicker.C:
				// Fallback for external writers that bypass this process-local channel.
				msg := strings.TrimSpace(o.store.CheckInterrupt(rec.ID))
				if msg == "" {
					continue
				}
				_ = o.store.ClearInterrupt(rec.ID)
				select {
				case interruptCh <- msg:
				default:
					debug.LogKV("orch", "file interrupt dropped: channel full",
						"spawn_id", rec.ID,
					)
				}
				childCancel()
			}
		}
	}()

	// Run the child agent in a goroutine.
	o.spawnWG.Add(1)
	go func() {
		defer o.spawnWG.Done()
		defer close(done)
		defer o.onSpawnComplete(rec.ID, req.ParentProfile, req.ChildProfile)

		debug.LogKV("orch", "spawn goroutine started",
			"spawn_id", rec.ID,
			"child_profile", req.ChildProfile,
			"workdir", workDir,
		)

		l := &loop.Loop{
			Store:       o.store,
			Agent:       agentInstance,
			Config:      agentCfg,
			PlanID:      parentPlanID,
			ProfileName: req.ChildProfile,
			OnStart: func(turnID int, turnHexID string) {
				if err := o.withSpawnRecordLock(rec.ID, func(stored *store.SpawnRecord) error {
					stored.ChildTurnID = turnID
					return nil
				}); err != nil {
					debug.LogKV("orch", "failed to persist child turn id",
						"spawn_id", rec.ID,
						"turn_id", turnID,
						"error", err,
					)
				}
			},
			OnPrompt: func(turnID int, turnHexID, prompt string, isResume bool) {
				trimmed, truncated, origLen := truncatePrompt(prompt)
				o.emitEvent("agent_prompt", events.AgentPromptMsg{
					SessionID:      -rec.ID,
					TurnHexID:      turnHexID,
					Prompt:         trimmed,
					IsResume:       isResume,
					Truncated:      truncated,
					OriginalLength: origLen,
				})
			},
			OnEnd: func(turnID int, turnHexID string, result *agent.Result) {
				o.emitEvent("agent_finished", events.AgentFinishedMsg{
					SessionID: -rec.ID,
					TurnHexID: turnHexID,
					Result:    result,
				})
			},
			PromptFunc: func(turnID int) string {
				newPrompt, _ := promptpkg.Build(promptpkg.BuildOpts{
					Store:        o.store,
					Project:      projCfg,
					Profile:      childProf,
					Role:         req.ChildRole,
					GlobalCfg:    o.globalCfg,
					Task:         req.Task,
					ReadOnly:     req.ReadOnly,
					ParentTurnID: req.ParentTurnID,
					Delegation:   req.ChildDelegation,
				})
				return newPrompt
			},
			OnWait: func(ctx context.Context, turnID int, alreadySeen map[int]struct{}) ([]loop.WaitResult, bool) {
				// Wait for at least one of this child's own spawns to complete.
				results, morePending := o.WaitAny(ctx, turnID, alreadySeen)
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
						Summary:  r.Summary,
						ReadOnly: r.ReadOnly,
						Branch:   r.Branch,
					})
				}
				return wr, morePending
			},
			InterruptCh: interruptCh,
		}

		err := l.Run(childCtx)
		debug.LogKV("orch", "spawn loop finished",
			"spawn_id", rec.ID,
			"child_profile", req.ChildProfile,
			"error", err,
		)
		completedAt := time.Now().UTC()

		// Capture child's final report for parent consumption.
		// Prefer the last assistant message when available (for models that
		// stream JSON transcript lines), otherwise fall back to raw output.
		summary := ""
		if l.LastResult != nil {
			report, reportErr := extractSpawnReport(l.LastResult.Output)
			if reportErr != nil {
				debug.LogKV("orch", "spawn report extraction failed",
					"spawn_id", rec.ID,
					"child_profile", rec.ChildProfile,
					"error", reportErr,
					"output_len", len(l.LastResult.Output),
				)
				summary = missingSpawnReportMessage(rec.ID, reportErr)
			} else {
				summary = report
			}
		} else {
			summary = missingSpawnReportMessage(rec.ID, errors.New("child returned no result payload"))
		}

		status, exitCode, result := classifySpawnCompletion(err, l.LastResult)
		recSnapshot, snapErr := o.store.GetSpawn(rec.ID)
		if snapErr != nil || recSnapshot == nil {
			recSnapshot = rec
		}
		autoCommitNote, autoCommitErr := o.autoCommitSpawnWork(recSnapshot)
		if status == "canceled" {
			cancelNote := canceledSpawnMessage(autoCommitNote != "")
			result = appendSpawnResult(result, cancelNote)
			summary = appendSpawnSummary(summary, cancelNote)
		}
		if autoCommitErr != nil {
			result = appendSpawnResult(result, fmt.Sprintf("auto-commit fallback failed: %v", autoCommitErr))
		} else if autoCommitNote != "" {
			result = appendSpawnResult(result, autoCommitNote)
		}
		// Clean up read-only worktrees immediately — there's nothing to merge.
		if recSnapshot.ReadOnly && recSnapshot.WorktreePath != "" {
			cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if rmErr := o.worktrees.Remove(cleanCtx, recSnapshot.WorktreePath, false); rmErr != nil {
				debug.LogKV("orch", "read-only worktree cleanup failed",
					"spawn_id", rec.ID, "worktree", recSnapshot.WorktreePath, "error", rmErr)
			}
			cleanCancel()
		}
		if err := o.withSpawnRecordLock(rec.ID, func(stored *store.SpawnRecord) error {
			stored.CompletedAt = completedAt
			stored.Summary = summary
			stored.Status = status
			stored.ExitCode = exitCode
			stored.Result = result
			return nil
		}); err != nil {
			debug.LogKV("orch", "failed to persist spawn completion",
				"spawn_id", rec.ID,
				"error", err,
			)
		}

		parentTurnID := req.ParentTurnID
		if finalRec, err := o.store.GetSpawn(rec.ID); err == nil && finalRec != nil && finalRec.ParentTurnID > 0 {
			parentTurnID = finalRec.ParentTurnID
		}
		o.signalWaitAny(parentTurnID)

		childCancel()
		close(streamCh)
		o.waitForAuxiliary("event bridge", rec.ID, eventDone)
		o.waitForAuxiliary("interrupt watcher", rec.ID, interruptDone)
	}()

	if req.Wait {
		<-done
	}

	return rec.ID, nil
}

const worktreeCreateRetries = 4

func (o *Orchestrator) createWritableWorktree(ctx context.Context, parentTurnID int, childProfile string) (branchName, wtPath string, _ error) {
	var lastErr error
	for attempt := 1; attempt <= worktreeCreateRetries; attempt++ {
		branchName = worktree.BranchName(parentTurnID, childProfile)
		wtPath, lastErr = o.worktrees.Create(ctx, branchName)
		if lastErr == nil {
			return branchName, wtPath, nil
		}
		if !isAlreadyExistsErr(lastErr) {
			return "", "", lastErr
		}
		debug.LogKV("orch", "writable worktree name collision; retrying",
			"attempt", attempt,
			"branch", branchName,
			"error", lastErr,
		)
	}
	return "", "", lastErr
}

func (o *Orchestrator) createReadOnlyWorktree(ctx context.Context, parentTurnID int, childProfile string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= worktreeCreateRetries; attempt++ {
		name := "ro-" + worktree.BranchName(parentTurnID, childProfile)
		wtPath, err := o.worktrees.CreateDetached(ctx, name)
		if err == nil {
			return wtPath, nil
		}
		lastErr = err
		if !isAlreadyExistsErr(err) {
			return "", err
		}
		debug.LogKV("orch", "read-only worktree name collision; retrying",
			"attempt", attempt,
			"name", name,
			"error", err,
		)
	}
	return "", lastErr
}

func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

func (o *Orchestrator) autoCommitSpawnWork(rec *store.SpawnRecord) (string, error) {
	if rec == nil || rec.WorktreePath == "" || rec.Branch == "" || rec.ReadOnly {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msg := fmt.Sprintf("adaf: auto-commit spawn #%d (%s)", rec.ID, rec.ChildProfile)
	hash, committed, err := o.worktrees.AutoCommitIfDirty(ctx, rec.WorktreePath, msg)
	if err != nil {
		return "", err
	}
	if !committed {
		return "", nil
	}

	return fmt.Sprintf("auto-commit: child left uncommitted changes; adaf created commit %s because the child did not commit.", shortHash(hash)), nil
}

func appendSpawnResult(base, extra string) string {
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	return base + " | " + extra
}

func appendSpawnSummary(base, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	base = strings.TrimSpace(base)
	if base == "" {
		return extra
	}
	return base + "\n\n" + extra
}

func classifySpawnCompletion(runErr error, lastResult *agent.Result) (status string, exitCode int, result string) {
	status = "completed"
	exitCode = 0
	result = ""

	if lastResult != nil {
		exitCode = lastResult.ExitCode
	}

	switch {
	case errors.Is(runErr, context.Canceled):
		status = "canceled"
		if lastResult == nil {
			// Keep canceled runs distinguishable even when no child result was captured.
			exitCode = -1
		}
	case runErr != nil:
		status = "failed"
		if lastResult == nil && exitCode == 0 {
			exitCode = 1
		}
		result = runErr.Error()
	}

	return status, exitCode, result
}

func canceledSpawnMessage(autoCommitted bool) string {
	if autoCommitted {
		return "Spawn was canceled before completion. Partial work was auto-committed."
	}
	return "Spawn was canceled before completion."
}

// extractSpawnReport returns the child agent's last assistant message as-is.
// If the output is plain text (not JSON event lines), it falls back to
// returning the trimmed output so stream-agent summaries remain usable.
// If neither an assistant message nor plain text can be extracted, it returns
// an error.
func extractSpawnReport(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", errors.New("child output is empty")
	}

	lines := strings.Split(trimmed, "\n")
	lastAssistant := ""
	jsonLineCount := 0
	jsonParsedCount := 0
	seenNonJSON := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") {
			seenNonJSON = true
			continue
		}
		jsonLineCount++

		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		jsonParsedCount++
		if msg := assistantMessageFromJSON(raw); msg != "" {
			lastAssistant = msg
		}
	}

	if strings.TrimSpace(lastAssistant) == "" {
		if seenNonJSON || jsonLineCount == 0 || jsonParsedCount == 0 {
			return trimmed, nil
		}
		return "", errors.New("no assistant message found in child output")
	}
	return lastAssistant, nil
}

func assistantMessageFromJSON(raw map[string]any) string {
	// Common transcript form: {"role":"assistant","content":"..."}
	if role, _ := raw["role"].(string); role == "assistant" {
		if msg := assistantContentValue(raw["content"]); msg != "" {
			return msg
		}
	}

	// Claude-style event projection:
	// {"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
	if typ, _ := raw["type"].(string); typ == "assistant" {
		if msg, ok := raw["message"].(map[string]any); ok {
			if msgText := assistantContentValue(msg["content"]); msgText != "" {
				return msgText
			}
		}
	}

	// Codex-style raw events:
	// {"type":"item.completed","item":{"type":"agent_message","text":"..."}}
	if typ, _ := raw["type"].(string); typ == "item.completed" {
		if item, ok := raw["item"].(map[string]any); ok {
			if itemType, _ := item["type"].(string); itemType == "agent_message" {
				if text, _ := item["text"].(string); strings.TrimSpace(text) != "" {
					return text
				}
			}
		}
	}

	return ""
}

func assistantContentValue(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, elem := range c {
			obj, ok := elem.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := obj["type"].(string)
			if blockType != "text" {
				continue
			}
			text, _ := obj["text"].(string)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func missingSpawnReportMessage(spawnID int, reason error) string {
	return fmt.Sprintf(
		"Report unavailable: automatic extraction failed (%v). Fetch it manually with `adaf spawn-inspect --spawn-id %d --last 200` or `adaf spawn-watch --spawn-id %d`.",
		reason, spawnID, spawnID,
	)
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

const auxiliaryGoroutineWaitTimeout = 3 * time.Second

func (o *Orchestrator) waitForAuxiliary(name string, spawnID int, done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(auxiliaryGoroutineWaitTimeout):
		debug.LogKV("orch", "auxiliary goroutine timed out during cleanup",
			"spawn_id", spawnID,
			"name", name,
			"timeout", auxiliaryGoroutineWaitTimeout,
		)
	}
}

func (o *Orchestrator) onSpawnComplete(spawnID int, parentProfile, childProfile string) {
	status := ""
	exitCode := 0
	if rec, err := o.store.GetSpawn(spawnID); err == nil && rec != nil {
		status = rec.Status
		exitCode = rec.ExitCode
		if childProfile == "" {
			childProfile = rec.ChildProfile
		}
	}
	debug.LogKV("orch", "onSpawnComplete",
		"spawn_id", spawnID,
		"child_profile", childProfile,
		"status", status,
		"exit_code", exitCode,
	)
	_ = o.store.ClearInterrupt(spawnID)

	o.mu.Lock()
	delete(o.spawns, spawnID)
	o.decrementRunningLocked(parentProfile)
	o.decrementInstancesLocked(childProfile)
	o.mu.Unlock()
}

func (o *Orchestrator) releaseSpawnSlot(parentProfile, childProfile string) {
	o.mu.Lock()
	o.decrementRunningLocked(parentProfile)
	o.decrementInstancesLocked(childProfile)
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
	debug.LogKV("orch", "Wait() called", "parent_turn", parentTurnID)
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
			Summary:  r.Summary,
			ReadOnly: r.ReadOnly,
			Branch:   r.Branch,
		})
	}
	return results
}

// WaitAny blocks until at least one unseen non-terminal spawn for the given
// parent turn reaches a terminal state, then returns newly completed results
// only. alreadySeen contains spawn IDs returned in prior wait cycles for this
// turn. The bool return indicates whether more spawns are still running.
func (o *Orchestrator) WaitAny(ctx context.Context, parentTurnID int, alreadySeen map[int]struct{}) ([]SpawnResult, bool) {
	debug.LogKV("orch", "WaitAny() called",
		"parent_turn", parentTurnID,
		"already_seen", len(alreadySeen),
	)
	waitAnyCh := o.acquireWaitAny(parentTurnID)
	defer o.releaseWaitAny(parentTurnID)
	records, _ := o.store.SpawnsByParent(parentTurnID)
	pending := make(map[int]struct{})
	var completed []int
	seenCompleted := 0
	for _, r := range records {
		if isTerminalSpawnStatus(r.Status) {
			if _, seen := alreadySeen[r.ID]; seen {
				seenCompleted++
				continue
			}
			completed = append(completed, r.ID)
		} else {
			pending[r.ID] = struct{}{}
		}
	}
	debug.LogKV("orch", "WaitAny initial state",
		"parent_turn", parentTurnID,
		"total_spawns", len(records),
		"already_seen", len(alreadySeen),
		"seen_completed", seenCompleted,
		"newly_completed", len(completed),
		"pending", len(pending),
	)
	if len(pending) == 0 && len(completed) == 0 {
		return nil, false
	}

	// Wait until at least one pending spawn reaches a terminal state that
	// has not already been delivered, or all spawns are terminal.
	if len(completed) == 0 && len(pending) > 0 {
		debug.LogKV("orch", "WaitAny waiting for completion signal",
			"parent_turn", parentTurnID,
			"already_seen", len(alreadySeen),
			"pending", len(pending),
		)
		waitStart := time.Now()

		for len(completed) == 0 && len(pending) > 0 {
			select {
			case <-ctx.Done():
				debug.LogKV("orch", "WaitAny cancelled while waiting",
					"parent_turn", parentTurnID,
					"wait_duration", time.Since(waitStart),
					"pending", len(pending),
				)
				return nil, false
			case <-waitAnyCh:
			}
			for id := range pending {
				rec, err := o.store.GetSpawn(id)
				if err != nil {
					delete(pending, id)
					continue
				}
				if isTerminalSpawnStatus(rec.Status) {
					delete(pending, id)
					if _, seen := alreadySeen[id]; seen {
						continue
					}
					completed = append(completed, id)
				}
			}
		}
		debug.LogKV("orch", "WaitAny signal wait complete",
			"parent_turn", parentTurnID,
			"wait_duration", time.Since(waitStart),
			"already_seen", len(alreadySeen),
			"newly_completed", len(completed),
			"still_pending", len(pending),
		)
	}

	results := make([]SpawnResult, 0, len(completed))
	for _, id := range completed {
		rec, err := o.store.GetSpawn(id)
		if err != nil {
			continue
		}
		results = append(results, SpawnResult{
			SpawnID:  rec.ID,
			Status:   rec.Status,
			ExitCode: rec.ExitCode,
			Result:   rec.Result,
			Summary:  rec.Summary,
			ReadOnly: rec.ReadOnly,
			Branch:   rec.Branch,
		})
	}
	debug.LogKV("orch", "WaitAny returning",
		"parent_turn", parentTurnID,
		"already_seen", len(alreadySeen),
		"results", len(results),
		"more_pending", len(pending) > 0,
	)
	return results, len(pending) > 0
}

// WaitOne blocks until a specific spawn completes.
func (o *Orchestrator) WaitOne(spawnID int) SpawnResult {
	o.mu.Lock()
	as, ok := o.spawns[spawnID]
	o.mu.Unlock()

	if ok {
		<-as.done
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		rec, err := o.store.GetSpawn(spawnID)
		if err != nil {
			return SpawnResult{SpawnID: spawnID, Status: "unknown"}
		}
		if isTerminalSpawnStatus(rec.Status) {
			return SpawnResult{
				SpawnID:  rec.ID,
				Status:   rec.Status,
				ExitCode: rec.ExitCode,
				Result:   rec.Result,
				Summary:  rec.Summary,
				ReadOnly: rec.ReadOnly,
				Branch:   rec.Branch,
			}
		}
		<-ticker.C
	}
}

// WaitForRunningSpawns waits up to timeout for running spawn goroutines to
// finish their cleanup (including auto-commit and onSpawnComplete).
// If parentTurnIDs is non-empty, only spawns belonging to those parent turns
// are waited on. It returns true when all targeted spawns completed before the timeout.
func (o *Orchestrator) WaitForRunningSpawns(parentTurnIDs []int, timeout time.Duration) bool {
	type waitTarget struct {
		done <-chan struct{}
	}

	parentFilter := make(map[int]struct{}, len(parentTurnIDs))
	for _, turnID := range parentTurnIDs {
		parentFilter[turnID] = struct{}{}
	}

	o.mu.Lock()
	targets := make([]waitTarget, 0, len(o.spawns))
	for _, as := range o.spawns {
		if len(parentFilter) > 0 {
			if as == nil {
				continue
			}
			if _, ok := parentFilter[as.ParentTurnID()]; !ok {
				continue
			}
		}
		targets = append(targets, waitTarget{done: as.done})
	}
	o.mu.Unlock()
	if len(targets) == 0 {
		return true
	}

	debug.LogKV("orch", "WaitForRunningSpawns() called",
		"running_spawns", len(targets),
		"parent_turns", fmt.Sprintf("%v", parentTurnIDs),
		"timeout", timeout,
	)

	waitCtx := context.Background()
	cancel := func() {}
	if timeout <= 0 {
		waitCtx, cancel = context.WithCancel(waitCtx)
	} else {
		waitCtx, cancel = context.WithTimeout(waitCtx, timeout)
	}
	defer cancel()

	for i, t := range targets {
		select {
		case <-t.done:
		case <-waitCtx.Done():
			remaining := len(targets) - i
			if remaining < 0 {
				remaining = 0
			}
			if timeout <= 0 {
				debug.LogKV("orch", "WaitForRunningSpawns() cancelled", "remaining_spawns", remaining)
			} else {
				debug.LogKV("orch", "WaitForRunningSpawns() timed out",
					"remaining_spawns", remaining,
					"timeout", timeout,
				)
			}
			return false
		}
	}

	debug.LogKV("orch", "WaitForRunningSpawns() completed", "running_spawns", len(targets))
	return true
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
		debug.LogKV("orch", "InterruptSpawn dropped: channel full",
			"spawn_id", spawnID,
		)
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
	debug.LogKV("orch", "Merge() called", "spawn_id", spawnID, "squash", squash)
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
		if rmErr := o.worktrees.RemoveWithBranch(ctx, rec.WorktreePath, rec.Branch); rmErr != nil {
			debug.LogKV("orch", "worktree cleanup failed after merge",
				"spawn_id", spawnID, "worktree", rec.WorktreePath, "error", rmErr)
		}
	}

	rec.Status = "merged"
	rec.MergeCommit = hash
	o.store.UpdateSpawn(rec)

	return hash, nil
}

// Reject rejects a spawn's work and cleans up.
func (o *Orchestrator) Reject(ctx context.Context, spawnID int) error {
	debug.LogKV("orch", "Reject() called", "spawn_id", spawnID)
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
		if rmErr := o.worktrees.RemoveWithBranch(ctx, rec.WorktreePath, rec.Branch); rmErr != nil {
			debug.LogKV("orch", "worktree cleanup failed after reject",
				"spawn_id", spawnID, "worktree", rec.WorktreePath, "error", rmErr)
		}
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

// staleWorktreeMaxAge is the TTL after which an untracked worktree is considered stale.
// Tracked spawn worktrees are preserved for review/merge/reject flows.
const staleWorktreeMaxAge = 24 * time.Hour

// cleanupStaleWorktrees removes worktrees from merged/rejected spawns and
// old untracked worktrees. Best-effort, errors are logged.
func (o *Orchestrator) cleanupStaleWorktrees() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build sets of tracked paths:
	// - deadPaths: cleanup eligible on startup
	// - keepPaths: must be preserved for diff/merge/reject flows
	deadPaths := make(map[string]bool)
	keepPaths := make(map[string]bool)
	spawns, _ := o.store.ListSpawns()
	for _, rec := range spawns {
		if rec.WorktreePath == "" {
			continue
		}
		switch rec.Status {
		case "merged", "rejected":
			deadPaths[rec.WorktreePath] = true
		default:
			keepPaths[rec.WorktreePath] = true
		}
	}

	// For untracked worktrees, apply age-based cleanup.
	if staleWorktreeMaxAge > 0 {
		if active, err := o.worktrees.ListActive(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: stale worktree cleanup list: %v\n", err)
		} else {
			now := time.Now()
			for _, wt := range active {
				if deadPaths[wt.Path] || keepPaths[wt.Path] {
					continue
				}
				info, err := os.Stat(wt.Path)
				if err != nil {
					continue
				}
				if now.Sub(info.ModTime()) > staleWorktreeMaxAge {
					deadPaths[wt.Path] = true
				}
			}
		}
	}

	removed, err := o.worktrees.CleanupStale(ctx, 0, deadPaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: stale worktree cleanup: %v\n", err)
	}
	if removed > 0 {
		fmt.Fprintf(os.Stderr, "cleaned up %d stale worktree(s)\n", removed)
	}
}

// ReparentSpawn updates a spawn's parent turn ID, used for handoff across loop steps.
func (o *Orchestrator) ReparentSpawn(spawnID, newParentTurnID int) error {
	if err := o.withSpawnRecordLock(spawnID, func(rec *store.SpawnRecord) error {
		rec.ParentTurnID = newParentTurnID
		rec.HandedOffToTurn = newParentTurnID
		return nil
	}); err != nil {
		return fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}

	o.mu.Lock()
	as, ok := o.spawns[spawnID]
	o.mu.Unlock()
	if ok && as != nil {
		as.SetParentTurnID(newParentTurnID)
	}
	return nil
}

// ActiveSpawnsForParent returns IDs of currently running spawns for a parent turn.
func (o *Orchestrator) ActiveSpawnsForParent(parentTurnID int) []int {
	o.mu.Lock()
	defer o.mu.Unlock()
	var ids []int
	for _, as := range o.spawns {
		if as == nil {
			continue
		}
		if as.ParentTurnID() == parentTurnID {
			ids = append(ids, as.spawnID)
		}
	}
	return ids
}
func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
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

// Init initializes the global orchestrator singleton and cleans up stale
// worktrees left behind by previous sessions (crashed, killed, etc.).
func Init(s *store.Store, globalCfg *config.GlobalConfig, repoRoot string) *Orchestrator {
	debug.LogKV("orch", "Init() called", "repo_root", repoRoot)
	globalOrchMu.Lock()
	defer globalOrchMu.Unlock()
	globalOrch = New(s, globalCfg, repoRoot)
	globalOrch.cleanupStaleWorktrees()
	return globalOrch
}

// Get returns the global orchestrator singleton, or nil if not initialized.
func Get() *Orchestrator {
	globalOrchMu.Lock()
	defer globalOrchMu.Unlock()
	return globalOrch
}
