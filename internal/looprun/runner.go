// Package looprun implements the loop execution engine.
package looprun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sync"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/guardrail"
	"github.com/agusx1211/adaf/internal/hexid"
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
	PlanID    string
	// SessionID is the owning daemon session ID (0 when not daemon-backed).
	SessionID int

	// WorkDir is the working directory for agent processes.
	WorkDir string

	// MaxCycles limits loop cycles. 0 means unlimited.
	MaxCycles int
}

// Run is the blocking loop execution implementation.
func Run(ctx context.Context, cfg RunConfig, eventCh chan any) error {
	loopDef := cfg.LoopDef

	// Create the loop run record.
	steps := make([]store.LoopRunStep, len(loopDef.Steps))
	for i, s := range loopDef.Steps {
		steps[i] = store.LoopRunStep{
			Profile:      s.Profile,
			Role:         s.Role,
			Turns:        s.Turns,
			Instructions: s.Instructions,
			CanStop:      s.CanStop,
			CanMessage:   s.CanMessage,
			CanPushover:  s.CanPushover,
			Guardrails:   s.Guardrails,
		}
	}

	run := &store.LoopRun{
		LoopName:        loopDef.Name,
		PlanID:          cfg.PlanID,
		Steps:           steps,
		Status:          "running",
		StepLastSeenMsg: make(map[int]int),
		HexID:           hexid.New(),
		StepHexIDs:      make(map[string]string),
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

	// Run cycles until stopped/cancelled (or MaxCycles if configured).
	for cycle := 0; ; cycle++ {
		if cfg.MaxCycles > 0 && cycle >= cfg.MaxCycles {
			return nil
		}

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

			// Generate step hex ID.
			stepHexID := hexid.New()
			stepKey := fmt.Sprintf("%d:%d", cycle, stepIdx)
			run.StepHexIDs[stepKey] = stepHexID
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
				RunID:      run.ID,
				RunHexID:   run.HexID,
				StepHexID:  stepHexID,
				Cycle:      cycle,
				StepIndex:  stepIdx,
				Profile:    prof.Name,
				Turns:      turns,
				TotalSteps: len(loopDef.Steps),
			}

			// Build agent config.
			agentCfg := buildAgentConfig(cfg, prof, run.ID, stepIdx, run.HexID, stepHexID)

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
				Role:        stepDef.Role,
				GlobalCfg:   cfg.GlobalCfg,
				PlanID:      cfg.PlanID,
				LoopContext: loopCtx,
				Delegation:  stepDef.Delegation,
				Handoffs:    handoffs,
				Guardrails:  stepDef.Guardrails,
			}

			prompt, err := promptpkg.Build(promptOpts)
			if err != nil {
				return fmt.Errorf("building prompt for step %d: %w", stepIdx, err)
			}
			agentCfg.Prompt = prompt
			agentCfg.MaxTurns = turns

			// Run the agent for this step using the existing loop infrastructure.
			streamCh := make(chan stream.RawEvent, 64)

			// Set up guardrail monitor for this step.
			effectiveRole := config.EffectiveStepRole(stepDef.Role, prof)
			monitor := guardrail.NewMonitor(effectiveRole, stepDef.Guardrails)
			interruptCh := make(chan string, 1)

			// Track the current turn's cancel function (mutex-protected).
			var turnCancelMu sync.Mutex
			var currentTurnCancel context.CancelFunc

			// Bridge stream events to the TUI event channel.
			bridgeDone := make(chan struct{})
			go func() {
				for ev := range streamCh {
					if ev.Err != nil {
						continue
					}
					if ev.Text != "" {
						eventCh <- runtui.AgentRawOutputMsg{Data: ev.Text, SessionID: ev.TurnID}
						continue
					}
					eventCh <- runtui.AgentEventMsg{Event: ev.Parsed, Raw: ev.Raw}

					// Guardrail check on parsed events.
					if monitor != nil {
						if toolName := monitor.CheckEvent(ev.Parsed); toolName != "" {
							eventCh <- runtui.GuardrailViolationMsg{
								Tool: toolName,
								Role: effectiveRole,
							}
							msg := guardrail.WarningMessage(effectiveRole, toolName, monitor.Violations())
							select {
							case interruptCh <- msg:
							default:
							}
							turnCancelMu.Lock()
							if currentTurnCancel != nil {
								currentTurnCancel()
							}
							turnCancelMu.Unlock()
						}
					}
				}
				close(bridgeDone)
			}()

			agentCfg.EventSink = streamCh
			agentCfg.Stdout = io.Discard
			agentCfg.Stderr = io.Discard

			var pollCancel context.CancelFunc
			var pollDone chan struct{}
			basePrompt := agentCfg.Prompt
			stepTurnStart := len(run.TurnIDs)
			handoffsReparented := false

			l := &loop.Loop{
				Store:        cfg.Store,
				Agent:        agentInstance,
				Config:       agentCfg,
				PlanID:       cfg.PlanID,
				LoopRunHexID: run.HexID,
				StepHexID:    stepHexID,
				PromptFunc: func(turnID int, supervisorNotes []store.SupervisorNote) string {
					opts := promptOpts
					opts.SupervisorNotes = supervisorNotes
					built, err := promptpkg.Build(opts)
					if err != nil {
						return basePrompt
					}
					return built
				},
				ProfileName: prof.Name,
				InterruptCh: interruptCh,
				OnTurnContext: func(cancel context.CancelFunc) {
					turnCancelMu.Lock()
					currentTurnCancel = cancel
					turnCancelMu.Unlock()
				},
				OnStart: func(turnID int, turnHexID string) {
					if !handoffsReparented && len(handoffs) > 0 {
						reparentHandoffs(cfg.Store, handoffs, turnID)
						handoffsReparented = true
					}

					run.TurnIDs = append(run.TurnIDs, turnID)
					cfg.Store.UpdateLoopRun(run)
					eventCh <- runtui.AgentStartedMsg{
						SessionID: turnID,
						TurnHexID: turnHexID,
						StepHexID: stepHexID,
						RunHexID:  run.HexID,
					}

					pollCtx, cancel := context.WithCancel(ctx)
					pollCancel = cancel
					pollDone = make(chan struct{})
					go func() {
						defer close(pollDone)
						pollSpawnStatus(pollCtx, cfg.Store, turnID, eventCh)
					}()
				},
				OnEnd: func(turnID int, turnHexID string, result *agent.Result) {
					if pollCancel != nil {
						pollCancel()
						pollCancel = nil
					}
					if pollDone != nil {
						<-pollDone
						pollDone = nil
					}
					waitingForSpawns := cfg.Store != nil && cfg.Store.IsWaiting(turnID)
					eventCh <- runtui.AgentFinishedMsg{
						SessionID:     turnID,
						TurnHexID:     turnHexID,
						WaitForSpawns: waitingForSpawns,
						Result:        result,
					}
				},
				OnWait: func(turnID int) []loop.WaitResult {
					return waitForSessionSpawns(cfg.Store, turnID)
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
				RunID:      run.ID,
				RunHexID:   run.HexID,
				StepHexID:  stepHexID,
				Cycle:      cycle,
				StepIndex:  stepIdx,
				Profile:    prof.Name,
				TotalSteps: len(loopDef.Steps),
			}

			if loopErr != nil {
				if ctx.Err() != nil {
					run.Status = "cancelled"
					return ctx.Err()
				}
				return fmt.Errorf("step %d (%s) failed: %w", stepIdx, prof.Name, loopErr)
			}

			// Collect handoffs: running spawns marked as handoff from this step's turns.
			var stepTurnIDs []int
			if stepTurnStart < len(run.TurnIDs) {
				stepTurnIDs = append(stepTurnIDs, run.TurnIDs[stepTurnStart:]...)
			}
			run.PendingHandoffs = collectHandoffs(cfg.Store, stepTurnIDs)
			cfg.Store.UpdateLoopRun(run)

			// Check stop signal after steps with can_stop.
			if stepDef.CanStop && cfg.Store.IsLoopStopped(run.ID) {
				return nil
			}
		}
	}
}

func pollSpawnStatus(ctx context.Context, s *store.Store, parentTurnID int, eventCh chan any) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastSnapshot := ""
	spawnOffsets := make(map[int]int64)

	poll := func(forceStatusEmit bool) {
		records, err := s.SpawnsByParent(parentTurnID)
		if err != nil {
			return
		}
		sort.Slice(records, func(i, j int) bool {
			return records[i].ID < records[j].ID
		})

		emitSpawnOutput(records, s, spawnOffsets, eventCh)

		spawns := make([]runtui.SpawnInfo, 0, len(records))
		for _, rec := range records {
			info := runtui.SpawnInfo{
				ID:           rec.ID,
				ParentTurnID: rec.ParentTurnID,
				Profile:      rec.ChildProfile,
				Status:       rec.Status,
			}
			if rec.Status == "awaiting_input" {
				if ask, err := s.PendingAsk(rec.ID); err == nil && ask != nil {
					info.Question = ask.Content
				}
			}
			spawns = append(spawns, info)
		}

		active := make(map[int]struct{}, len(records))
		for _, rec := range records {
			active[rec.ID] = struct{}{}
		}
		for id := range spawnOffsets {
			if _, ok := active[id]; !ok {
				delete(spawnOffsets, id)
			}
		}

		snapshot := spawnSnapshotFingerprint(spawns)
		if !forceStatusEmit && snapshot == lastSnapshot {
			return
		}
		lastSnapshot = snapshot
		eventCh <- runtui.SpawnStatusMsg{Spawns: spawns}
	}

	poll(true)
	for {
		select {
		case <-ctx.Done():
			poll(true)
			return
		case <-ticker.C:
			poll(false)
		}
	}
}

// emitSpawnOutput tails child spawn recording events and forwards readable
// output chunks into spawn-scoped raw output events for the loop TUI.
func emitSpawnOutput(records []store.SpawnRecord, s *store.Store, offsets map[int]int64, eventCh chan any) {
	for _, rec := range records {
		if rec.ID <= 0 || rec.ChildTurnID <= 0 {
			continue
		}

		eventsPath := filepath.Join(s.Root(), "records", fmt.Sprintf("%d", rec.ChildTurnID), "events.jsonl")
		f, err := os.Open(eventsPath)
		if err != nil {
			continue
		}

		prevOffset := offsets[rec.ID]
		if info, statErr := f.Stat(); statErr == nil && prevOffset > info.Size() {
			prevOffset = 0
		}
		if prevOffset > 0 {
			if _, err := f.Seek(prevOffset, io.SeekStart); err != nil {
				_ = f.Close()
				continue
			}
		}

		chunk, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil || len(chunk) == 0 {
			offsets[rec.ID] = prevOffset
			continue
		}
		offsets[rec.ID] = prevOffset + int64(len(chunk))

		for _, line := range strings.Split(string(chunk), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			var ev store.RecordingEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}

			var data string
			switch ev.Type {
			case "stdout":
				data = ev.Data
			case "stderr":
				data = "[stderr] " + ev.Data
			case "claude_stream":
				data = ev.Data
			default:
				continue
			}
			if strings.TrimSpace(data) == "" {
				continue
			}

			eventCh <- runtui.AgentRawOutputMsg{
				Data:      data,
				SessionID: -rec.ID, // Negative SessionID maps to spawn scope in runtui.
			}
		}
	}
}

func spawnSnapshotFingerprint(spawns []runtui.SpawnInfo) string {
	if len(spawns) == 0 {
		return ""
	}
	var b strings.Builder
	for _, sp := range spawns {
		b.WriteString(fmt.Sprintf("%d|%d|%s|%s|%s;", sp.ID, sp.ParentTurnID, sp.Profile, sp.Status, sp.Question))
	}
	return b.String()
}

// buildAgentConfig creates an agent.Config for a profile step.
func buildAgentConfig(cfg RunConfig, prof *config.Profile, runID, stepIndex int, runHexID, stepHexID string) agent.Config {
	launch := agent.BuildLaunchSpec(prof, cfg.AgentsCfg, "")
	agentArgs := append([]string(nil), launch.Args...)
	agentEnv := make(map[string]string)

	// Set loop environment variables.
	agentEnv["ADAF_LOOP_RUN_ID"] = fmt.Sprintf("%d", runID)
	agentEnv["ADAF_LOOP_STEP_INDEX"] = fmt.Sprintf("%d", stepIndex)
	if cfg.SessionID > 0 {
		agentEnv["ADAF_SESSION_ID"] = fmt.Sprintf("%d", cfg.SessionID)
	}
	if runHexID != "" {
		agentEnv["ADAF_LOOP_RUN_HEX_ID"] = runHexID
	}
	if stepHexID != "" {
		agentEnv["ADAF_LOOP_STEP_HEX_ID"] = stepHexID
	}

	for k, v := range launch.Env {
		agentEnv[k] = v
	}

	return agent.Config{
		Name:    prof.Agent,
		Command: launch.Command,
		Args:    agentArgs,
		Env:     agentEnv,
		WorkDir: cfg.WorkDir,
	}
}

// collectHandoffs finds running spawns marked as handoff from any of the given turn IDs.
func collectHandoffs(s *store.Store, turnIDs []int) []store.HandoffInfo {
	var handoffs []store.HandoffInfo
	seen := make(map[int]struct{})
	for _, sid := range turnIDs {
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

func reparentHandoffs(s *store.Store, handoffs []store.HandoffInfo, newParentTurnID int) {
	if len(handoffs) == 0 {
		return
	}

	// Prefer orchestrator API when available so in-memory active spawn records
	// stay in sync with persisted store data.
	if o := orchestrator.Get(); o != nil {
		for _, h := range handoffs {
			_ = o.ReparentSpawn(h.SpawnID, newParentTurnID)
		}
		return
	}

	for _, h := range handoffs {
		rec, err := s.GetSpawn(h.SpawnID)
		if err != nil {
			continue
		}
		rec.ParentTurnID = newParentTurnID
		rec.HandedOffToTurn = newParentTurnID
		_ = s.UpdateSpawn(rec)
	}
}

func waitForSessionSpawns(s *store.Store, parentTurnID int) []loop.WaitResult {
	records, err := s.SpawnsByParent(parentTurnID)
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
