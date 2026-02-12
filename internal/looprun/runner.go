// Package looprun implements the loop execution engine.
package looprun

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/loop"
	"github.com/agusx1211/adaf/internal/orchestrator"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

// RunConfig holds everything needed to launch a loop run.
type RunConfig struct {
	Store     *store.Store
	GlobalCfg *config.GlobalConfig
	LoopDef   *config.LoopDef
	Project   *store.ProjectConfig
	AgentsCfg *agent.AgentsConfig

	// WorkDir is the working directory for agent processes.
	WorkDir string
}

// StartLoopRun launches the loop in a goroutine and returns a cancel function.
// Events are sent to eventCh. The caller must drain eventCh.
func StartLoopRun(cfg RunConfig, eventCh chan any) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := Run(ctx, cfg, eventCh)
		reason := "stopped"
		if err != nil {
			if ctx.Err() != nil {
				reason = "cancelled"
			} else {
				reason = "error"
			}
		}
		eventCh <- runtui.LoopDoneMsg{Reason: reason, Err: err}
		close(eventCh)
	}()
	return cancel
}

// Run is the blocking loop execution implementation.
func Run(ctx context.Context, cfg RunConfig, eventCh chan any) error {
	loopDef := cfg.LoopDef

	// Create the loop run record.
	steps := make([]store.LoopRunStep, len(loopDef.Steps))
	for i, s := range loopDef.Steps {
		steps[i] = store.LoopRunStep{
			Profile:      s.Profile,
			Turns:        s.Turns,
			Instructions: s.Instructions,
			CanStop:      s.CanStop,
			CanMessage:   s.CanMessage,
			CanPushover:  s.CanPushover,
		}
	}

	run := &store.LoopRun{
		LoopName:        loopDef.Name,
		Steps:           steps,
		Status:          "running",
		StepLastSeenMsg: make(map[int]int),
	}

	if err := cfg.Store.CreateLoopRun(run); err != nil {
		return fmt.Errorf("creating loop run: %w", err)
	}

	defer func() {
		run.Status = "stopped"
		run.StoppedAt = time.Now().UTC()
		cfg.Store.UpdateLoopRun(run)
		_ = stats.UpdateLoopStats(cfg.Store, loopDef.Name, run)
	}()

	// Run cycles indefinitely until stopped or cancelled.
	for cycle := 0; ; cycle++ {
		run.Cycle = cycle
		cfg.Store.UpdateLoopRun(run)

		for stepIdx, stepDef := range loopDef.Steps {
			select {
			case <-ctx.Done():
				run.Status = "cancelled"
				return ctx.Err()
			default:
			}

			run.StepIndex = stepIdx
			cfg.Store.UpdateLoopRun(run)

			// Resolve profile.
			prof := cfg.GlobalCfg.FindProfile(stepDef.Profile)
			if prof == nil {
				return fmt.Errorf("profile %q not found for step %d", stepDef.Profile, stepIdx)
			}

			// Resolve agent.
			agentInstance, ok := agent.Get(prof.Agent)
			if !ok {
				return fmt.Errorf("agent %q not found for profile %q", prof.Agent, prof.Name)
			}

			turns := stepDef.Turns
			if turns <= 0 {
				turns = 1
			}

			// Emit step start event.
			eventCh <- runtui.LoopStepStartMsg{
				RunID:     run.ID,
				Cycle:     cycle,
				StepIndex: stepIdx,
				Profile:   prof.Name,
				Turns:     turns,
			}

			// Build agent config.
			agentCfg := buildAgentConfig(cfg, prof, run.ID, stepIdx)

			// Gather unseen messages for this step.
			unseenMsgs := gatherUnseenMessages(cfg.Store, run, stepIdx)

			// Build prompt with loop context.
			loopCtx := &promptpkg.LoopPromptContext{
				LoopName:     loopDef.Name,
				Cycle:        cycle,
				StepIndex:    stepIdx,
				TotalSteps:   len(loopDef.Steps),
				Instructions: stepDef.Instructions,
				CanStop:      stepDef.CanStop,
				CanMessage:   stepDef.CanMessage,
				CanPushover:  stepDef.CanPushover,
				Messages:     unseenMsgs,
				RunID:        run.ID,
			}

			// Pass any pending handoffs from previous step and clear them.
			handoffs := run.PendingHandoffs
			run.PendingHandoffs = nil
			cfg.Store.UpdateLoopRun(run)

			promptOpts := promptpkg.BuildOpts{
				Store:       cfg.Store,
				Project:     cfg.Project,
				Profile:     prof,
				GlobalCfg:   cfg.GlobalCfg,
				LoopContext: loopCtx,
				Delegation:  stepDef.Delegation,
				Handoffs:    handoffs,
			}

			prompt, err := promptpkg.Build(promptOpts)
			if err != nil {
				return fmt.Errorf("building prompt for step %d: %w", stepIdx, err)
			}
			agentCfg.Prompt = prompt
			agentCfg.MaxTurns = turns

			// Run the agent for this step using the existing loop infrastructure.
			streamCh := make(chan stream.RawEvent, 64)

			// Bridge stream events to the TUI event channel.
			bridgeDone := make(chan struct{})
			go func() {
				for ev := range streamCh {
					if ev.Err != nil {
						continue
					}
					if ev.Text != "" {
						eventCh <- runtui.AgentRawOutputMsg{Data: ev.Text, SessionID: ev.SessionID}
						continue
					}
					eventCh <- runtui.AgentEventMsg{Event: ev.Parsed, Raw: ev.Raw}
				}
				close(bridgeDone)
			}()

			agentCfg.EventSink = streamCh
			agentCfg.Stdout = io.Discard
			agentCfg.Stderr = io.Discard

			var pollCancel context.CancelFunc
			var pollDone chan struct{}
			basePrompt := agentCfg.Prompt
			stepSessionStart := len(run.SessionIDs)
			handoffsReparented := false

			l := &loop.Loop{
				Store:  cfg.Store,
				Agent:  agentInstance,
				Config: agentCfg,
				PromptFunc: func(sessionID int, supervisorNotes []store.SupervisorNote) string {
					opts := promptOpts
					opts.SupervisorNotes = supervisorNotes
					built, err := promptpkg.Build(opts)
					if err != nil {
						return basePrompt
					}
					return built
				},
				ProfileName: prof.Name,
				OnStart: func(sessionID int) {
					if !handoffsReparented && len(handoffs) > 0 {
						reparentHandoffs(cfg.Store, handoffs, sessionID)
						handoffsReparented = true
					}

					run.SessionIDs = append(run.SessionIDs, sessionID)
					cfg.Store.UpdateLoopRun(run)
					eventCh <- runtui.AgentStartedMsg{SessionID: sessionID}

					pollCtx, cancel := context.WithCancel(ctx)
					pollCancel = cancel
					pollDone = make(chan struct{})
					go func() {
						defer close(pollDone)
						pollSpawnStatus(pollCtx, cfg.Store, sessionID, eventCh)
					}()
				},
				OnEnd: func(sessionID int, result *agent.Result) {
					if pollCancel != nil {
						pollCancel()
						pollCancel = nil
					}
					if pollDone != nil {
						<-pollDone
						pollDone = nil
					}
					eventCh <- runtui.AgentFinishedMsg{
						SessionID: sessionID,
						Result:    result,
					}
				},
				OnWait: func(sessionID int) []loop.WaitResult {
					return waitForSessionSpawns(cfg.Store, sessionID)
				},
			}

			loopErr := l.Run(ctx)
			if pollCancel != nil {
				pollCancel()
			}
			if pollDone != nil {
				<-pollDone
			}
			close(streamCh)
			<-bridgeDone

			// Update watermark: step has seen all current messages.
			allMsgs, _ := cfg.Store.ListLoopMessages(run.ID)
			if len(allMsgs) > 0 {
				run.StepLastSeenMsg[stepIdx] = allMsgs[len(allMsgs)-1].ID
			}
			cfg.Store.UpdateLoopRun(run)

			// Emit step end event.
			eventCh <- runtui.LoopStepEndMsg{
				RunID:     run.ID,
				Cycle:     cycle,
				StepIndex: stepIdx,
				Profile:   prof.Name,
			}

			if loopErr != nil {
				if ctx.Err() != nil {
					run.Status = "cancelled"
					return ctx.Err()
				}
				return fmt.Errorf("step %d (%s) failed: %w", stepIdx, prof.Name, loopErr)
			}

			// Collect handoffs: running spawns marked as handoff from this step's sessions.
			var stepSessionIDs []int
			if stepSessionStart < len(run.SessionIDs) {
				stepSessionIDs = append(stepSessionIDs, run.SessionIDs[stepSessionStart:]...)
			}
			run.PendingHandoffs = collectHandoffs(cfg.Store, stepSessionIDs)
			cfg.Store.UpdateLoopRun(run)

			// Check stop signal after steps with can_stop.
			if stepDef.CanStop && cfg.Store.IsLoopStopped(run.ID) {
				return nil
			}
		}
	}
}

func pollSpawnStatus(ctx context.Context, s *store.Store, parentSessionID int, eventCh chan any) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastSnapshot := ""
	emitIfChanged := func() {
		records, err := s.SpawnsByParent(parentSessionID)
		if err != nil {
			return
		}
		sort.Slice(records, func(i, j int) bool {
			return records[i].ID < records[j].ID
		})

		spawns := make([]runtui.SpawnInfo, 0, len(records))
		for _, rec := range records {
			info := runtui.SpawnInfo{
				ID:      rec.ID,
				Profile: rec.ChildProfile,
				Status:  rec.Status,
			}
			if rec.Status == "awaiting_input" {
				if ask, err := s.PendingAsk(rec.ID); err == nil && ask != nil {
					info.Question = ask.Content
				}
			}
			spawns = append(spawns, info)
		}

		snapshot := spawnSnapshotFingerprint(spawns)
		if snapshot == lastSnapshot {
			return
		}
		lastSnapshot = snapshot
		eventCh <- runtui.SpawnStatusMsg{Spawns: spawns}
	}

	emitIfChanged()
	for {
		select {
		case <-ctx.Done():
			emitIfChanged()
			return
		case <-ticker.C:
			emitIfChanged()
		}
	}
}

func spawnSnapshotFingerprint(spawns []runtui.SpawnInfo) string {
	if len(spawns) == 0 {
		return ""
	}
	var b strings.Builder
	for _, sp := range spawns {
		b.WriteString(fmt.Sprintf("%d|%s|%s|%s;", sp.ID, sp.Profile, sp.Status, sp.Question))
	}
	return b.String()
}

// buildAgentConfig creates an agent.Config for a profile step.
func buildAgentConfig(cfg RunConfig, prof *config.Profile, runID, stepIndex int) agent.Config {
	var agentArgs []string
	agentEnv := make(map[string]string)

	// Set loop environment variables.
	agentEnv["ADAF_LOOP_RUN_ID"] = fmt.Sprintf("%d", runID)
	agentEnv["ADAF_LOOP_STEP_INDEX"] = fmt.Sprintf("%d", stepIndex)

	modelOverride := prof.Model
	reasoningLevel := prof.ReasoningLevel

	// Look up custom command from agents config.
	var customCmd string
	if cfg.AgentsCfg != nil {
		if rec, ok := cfg.AgentsCfg.Agents[prof.Agent]; ok && rec.Path != "" {
			customCmd = rec.Path
		}
	}

	switch prof.Agent {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentEnv["CLAUDE_CODE_EFFORT_LEVEL"] = reasoningLevel
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentArgs = append(agentArgs, "-c", `model_reasoning_effort="`+reasoningLevel+`"`)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	case "gemini":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "-y")
	case "vibe":
		// Vibe has no --model flag. It uses pydantic-settings with
		// env_prefix="VIBE_", so any config field can be overridden via
		// environment variables. VIBE_ACTIVE_MODEL sets the active model
		// alias while preserving the full config (providers, models, etc.)
		// from ~/.vibe/config.toml.
		if modelOverride != "" {
			agentEnv["VIBE_ACTIVE_MODEL"] = modelOverride
		}
	}

	if customCmd == "" {
		switch prof.Agent {
		case "claude", "codex", "vibe", "opencode", "gemini", "generic":
		default:
			customCmd = prof.Agent
		}
	}

	return agent.Config{
		Name:    prof.Agent,
		Command: customCmd,
		Args:    agentArgs,
		Env:     agentEnv,
		WorkDir: cfg.WorkDir,
	}
}

// collectHandoffs finds running spawns marked as handoff from any of the given session IDs.
func collectHandoffs(s *store.Store, sessionIDs []int) []store.HandoffInfo {
	var handoffs []store.HandoffInfo
	seen := make(map[int]struct{})
	for _, sid := range sessionIDs {
		records, err := s.SpawnsByParent(sid)
		if err != nil {
			continue
		}
		for _, rec := range records {
			if _, ok := seen[rec.ID]; ok {
				continue
			}
			if rec.Handoff && (rec.Status == "running" || rec.Status == "queued") {
				seen[rec.ID] = struct{}{}
				handoffs = append(handoffs, store.HandoffInfo{
					SpawnID: rec.ID,
					Profile: rec.ChildProfile,
					Task:    rec.Task,
					Status:  rec.Status,
					Speed:   rec.Speed,
					Branch:  rec.Branch,
				})
			}
		}
	}
	return handoffs
}

func reparentHandoffs(s *store.Store, handoffs []store.HandoffInfo, newParentSessionID int) {
	if len(handoffs) == 0 {
		return
	}

	// Prefer orchestrator API when available so in-memory active spawn records
	// stay in sync with persisted store data.
	if o := orchestrator.Get(); o != nil {
		for _, h := range handoffs {
			_ = o.ReparentSpawn(h.SpawnID, newParentSessionID)
		}
		return
	}

	for _, h := range handoffs {
		rec, err := s.GetSpawn(h.SpawnID)
		if err != nil {
			continue
		}
		rec.ParentSessionID = newParentSessionID
		rec.HandedOffTo = newParentSessionID
		_ = s.UpdateSpawn(rec)
	}
}

func waitForSessionSpawns(s *store.Store, parentSessionID int) []loop.WaitResult {
	records, err := s.SpawnsByParent(parentSessionID)
	if err != nil || len(records) == 0 {
		return nil
	}

	pending := make(map[int]struct{})
	for _, rec := range records {
		if !isTerminalSpawnStatus(rec.Status) {
			pending[rec.ID] = struct{}{}
		}
	}
	if len(pending) == 0 {
		return nil
	}

	waitedIDs := make([]int, 0, len(pending))
	for id := range pending {
		waitedIDs = append(waitedIDs, id)
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for len(pending) > 0 {
		<-ticker.C
		for id := range pending {
			rec, err := s.GetSpawn(id)
			if err != nil || isTerminalSpawnStatus(rec.Status) {
				delete(pending, id)
			}
		}
	}

	sort.Ints(waitedIDs)
	results := make([]loop.WaitResult, 0, len(waitedIDs))
	for _, id := range waitedIDs {
		rec, err := s.GetSpawn(id)
		if err != nil {
			continue
		}
		results = append(results, loop.WaitResult{
			SpawnID:  rec.ID,
			Profile:  rec.ChildProfile,
			Status:   rec.Status,
			ExitCode: rec.ExitCode,
			Result:   rec.Result,
		})
	}
	return results
}

func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "merged", "rejected":
		return true
	default:
		return false
	}
}

// gatherUnseenMessages returns messages that this step hasn't seen yet,
// excluding messages posted by this step itself.
func gatherUnseenMessages(s *store.Store, run *store.LoopRun, stepIndex int) []store.LoopMessage {
	allMsgs, err := s.ListLoopMessages(run.ID)
	if err != nil || len(allMsgs) == 0 {
		return nil
	}

	lastSeen := run.StepLastSeenMsg[stepIndex] // 0 = hasn't seen any (IDs start at 1)

	var unseen []store.LoopMessage
	for _, msg := range allMsgs {
		if msg.ID > lastSeen && msg.StepIndex != stepIndex {
			unseen = append(unseen, msg)
		}
	}
	return unseen
}
