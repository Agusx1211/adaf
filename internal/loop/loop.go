package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/hexid"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

// WaitCallback is called when the loop detects a wait signal.
// It should block until at least one unseen spawn completes (wait-for-any)
// and return results for newly completed spawns. alreadySeen contains
// spawn IDs returned in previous wait cycles for the same turn. The bool
// return indicates whether more spawns are still pending.
// The context must be respected: implementations should return promptly
// when ctx is cancelled.
type WaitCallback func(ctx context.Context, turnID int, alreadySeen map[int]struct{}) (results []WaitResult, morePending bool)

// WaitResult describes the outcome of a spawn that was waited on.
type WaitResult struct {
	SpawnID  int
	Profile  string
	Status   string
	ExitCode int
	Result   string
	Summary  string // child's final output
	ReadOnly bool   // whether this was a read-only scout
	Branch   string // worktree branch (empty for read-only)

	// Review marks a periodic running-spawn checkpoint (not a completion).
	Review bool

	// Health metrics populated for review checkpoints.
	Elapsed           time.Duration
	CompactionCount   int
	ReadCount         int
	WriteCount        int
	CommitCount       int
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
}

const InterruptMessageCallSupervisor = "__adaf_control:call_supervisor__"

var ErrStepEndedByControlSignal = errors.New("step ended by control signal")

// Loop is the main agent loop controller. It runs an agent one or more times,
// creating turn recordings in the store. Normal iterations create new turns;
// wait-for-spawns resumes continue on the same turn.
type Loop struct {
	Store  *store.Store
	Agent  agent.Agent
	Config agent.Config

	// ProfileName is the name of the profile that launched this loop.
	ProfileName string

	// PlanID tracks the plan context for this loop.
	PlanID string

	// LoopRunHexID is the hex ID of the parent loop run (set by looprun).
	LoopRunHexID string

	// StepHexID is the hex ID of the current loop step (set by looprun).
	StepHexID string

	// OnStart is called at the beginning of each iteration, before the agent runs.
	// The turnID and turnHexID of the upcoming run are passed as arguments.
	OnStart func(turnID int, turnHexID string)

	// OnPrompt is called after the prompt is finalized for the turn and before
	// the agent starts.
	OnPrompt func(turnID int, turnHexID, prompt string, isResume bool)

	// OnEnd is called after each iteration completes (successfully or not).
	// The turnID, turnHexID, and the result (which may be nil on error) are passed.
	OnEnd func(turnID int, turnHexID string, result *agent.Result)

	// PromptFunc, if set, is called before each turn to dynamically refresh the
	// prompt. If nil, Config.Prompt is used.
	PromptFunc func(turnID int) string

	// OnWait is called when the agent signals a wait-for-spawns.
	// It should block until spawns complete and return results.
	// If nil, wait signals are ignored.
	OnWait WaitCallback

	// InterruptCh, if set, receives signals when the agent's turn should be
	// interrupted (e.g. parent sends an interrupt message).
	InterruptCh <-chan string

	// OnTurnContext is called at the start of each turn with the turn-scoped
	// cancel function. This allows external code to cancel only the current
	// turn without stopping the entire loop.
	OnTurnContext func(cancel context.CancelFunc)

	// StopAfterTurn, if set, is called after each fully completed turn and
	// before the next turn starts. Returning true exits the loop gracefully.
	StopAfterTurn func(turnID int) bool

	// LastResult is populated after each Agent.Run() call so callers
	// (e.g. orchestrator) can inspect the child's output.
	LastResult *agent.Result

	// lastAgentSessionID holds the session/thread ID from the last agent
	// run, used to resume the session on the next turn (e.g. after wait-for-spawns).
	lastAgentSessionID string

	// InitialResumeSessionID seeds resume-mode for the first turn of this loop
	// instance. Unlike Config.ResumeSessionID (which passes through to agents
	// while keeping full prompt mode), this forces resume prompt mode.
	InitialResumeSessionID string

	// lastWaitResults holds results from a wait cycle, injected into the next prompt.
	lastWaitResults []WaitResult

	// moreSpawnsPending is true when the last wait returned partial results
	// and more spawns are still running.
	moreSpawnsPending bool

	// waitResumeTurnID/HexID identify the turn that should be resumed after
	// wait-for-spawns. This keeps the logical turn stable instead of creating
	// a new turn record for each wait cycle.
	waitResumeTurnID    int
	waitResumeTurnHexID string

	// seenSpawnIDs tracks spawn IDs already returned to the parent while
	// waiting on a specific turn. Reset when the turn changes.
	seenSpawnIDs    map[int]struct{}
	seenSpawnTurnID int

	// lastInterruptMsg holds the message from the last interrupt, injected
	// into the next turn's prompt.
	lastInterruptMsg string
}

// Run executes the agent loop. It will run the agent up to Config.MaxTurns times
// (or infinitely if MaxTurns is 0). The loop respects context cancellation for
// graceful shutdown (e.g. ctrl+c).
func (l *Loop) Run(ctx context.Context) error {
	maxTurns := l.Config.MaxTurns
	turn := 0

	debug.LogKV("loop", "loop starting",
		"agent", l.Agent.Name(),
		"profile", l.ProfileName,
		"plan_id", l.PlanID,
		"max_turns", maxTurns,
		"loop_run_hex", l.LoopRunHexID,
		"step_hex", l.StepHexID,
	)

	if l.lastAgentSessionID == "" {
		l.lastAgentSessionID = strings.TrimSpace(l.InitialResumeSessionID)
	}

	for {
		// Check if we've hit the turn limit.
		if maxTurns > 0 && turn >= maxTurns {
			debug.LogKV("loop", "max turns reached; exiting loop",
				"completed_turns", turn,
				"max_turns", maxTurns,
			)
			return nil
		}

		// Check for context cancellation before starting a new turn.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Allocate a turn ID. For wait-for-spawns resumes, reuse the same turn.
		var (
			turnID       int
			turnHexID    string
			turnLog      *store.Turn
			resumingTurn bool
		)
		if l.waitResumeTurnID > 0 {
			resumingTurn = true
			turnID = l.waitResumeTurnID
			debug.LogKV("loop", "resuming wait turn",
				"turn_id", turnID,
				"wait_resume_hex", l.waitResumeTurnHexID,
				"more_spawns_pending", l.moreSpawnsPending,
			)
			if l.Store != nil {
				existing, err := l.Store.GetTurn(turnID)
				if err != nil {
					return fmt.Errorf("loading wait-resume turn %d: %w", turnID, err)
				}
				turnLog = existing
				if strings.TrimSpace(turnLog.HexID) != "" {
					turnHexID = strings.TrimSpace(turnLog.HexID)
				}
			}
			if turnHexID == "" {
				turnHexID = strings.TrimSpace(l.waitResumeTurnHexID)
			}
			if turnHexID == "" {
				turnHexID = hexid.New()
			}
			// Consume the pending wait-resume marker now; if this iteration
			// is interrupted/error'd, the next iteration should allocate a new
			// turn unless another wait signal is raised.
			l.waitResumeTurnID = 0
			l.waitResumeTurnHexID = ""
		} else {
			objective := summarizeObjectiveForLog(l.Config.Prompt)
			if objective == "" {
				objective = "Agent run"
			}
			turnHexID = hexid.New()
			turnLog = &store.Turn{
				Agent:        l.Agent.Name(),
				ProfileName:  l.ProfileName,
				PlanID:       l.PlanID,
				Objective:    objective,
				HexID:        turnHexID,
				LoopRunHexID: l.LoopRunHexID,
				StepHexID:    l.StepHexID,
			}
			if err := l.Store.CreateTurn(turnLog); err != nil {
				return fmt.Errorf("creating turn: %w", err)
			}
			turnID = turnLog.ID
		}
		if turnLog == nil {
			turnLog = &store.Turn{ID: turnID}
		}
		if strings.TrimSpace(turnLog.HexID) == "" {
			turnLog.HexID = turnHexID
		}

		debug.LogKV("loop", "turn allocated",
			"turn_id", turnID,
			"turn_hex", turnHexID,
			"turn_num", turn+1,
			"agent", l.Agent.Name(),
			"profile", l.ProfileName,
		)
		if !resumingTurn && l.seenSpawnTurnID != 0 && l.seenSpawnTurnID != turnID {
			l.seenSpawnTurnID = 0
			l.seenSpawnIDs = nil
		}

		// Update config with the current turn ID.
		cfg := l.Config
		cfg.TurnID = turnID

		// Set ADAF_AGENT=1 so the agent process knows it's running under adaf
		// and session management commands are blocked.
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		cfg.Env["ADAF_AGENT"] = "1"
		cfg.Env["ADAF_TURN_ID"] = fmt.Sprintf("%d", turnID)
		cfg.Env["ADAF_TURN_HEX_ID"] = turnHexID
		if l.LoopRunHexID != "" {
			cfg.Env["ADAF_LOOP_RUN_HEX_ID"] = l.LoopRunHexID
		}
		if l.StepHexID != "" {
			cfg.Env["ADAF_LOOP_STEP_HEX_ID"] = l.StepHexID
		}
		if strings.TrimSpace(l.ProfileName) != "" {
			cfg.Env["ADAF_PROFILE"] = l.ProfileName
		}
		if strings.TrimSpace(l.PlanID) != "" {
			cfg.Env["ADAF_PLAN_ID"] = l.PlanID
		}
		if l.Store != nil {
			projectDir := strings.TrimSpace(l.Store.ProjectDir())
			if projectDir != "" {
				cfg.Env["ADAF_PROJECT_DIR"] = projectDir
			}
		}
		// Determine if we're resuming a previous agent session.
		isResume := l.lastAgentSessionID != ""

		debug.LogKV("loop", "turn mode", "turn_id", turnID, "is_resume", isResume)

		if isResume {
			// When resuming, the agent already has the full system prompt
			// and conversation context from the previous turn. Send only
			// new information (wait results and interrupt messages)
			// as a continuation message — NOT the full system prompt.
			cfg.ResumeSessionID = l.lastAgentSessionID
			l.lastAgentSessionID = "" // consume after use
			cfg.Prompt = buildResumePrompt(
				l.lastWaitResults,
				l.moreSpawnsPending,
				l.lastInterruptMsg,
				!resumingTurn,
			)
			l.lastWaitResults = nil
			l.moreSpawnsPending = false
			l.lastInterruptMsg = ""
		} else {
			if l.PromptFunc != nil {
				cfg.Prompt = l.PromptFunc(turnID)
			}

			// Inject interrupt message from a previous interruption.
			if l.lastInterruptMsg != "" {
				cfg.Prompt += "\n## Interrupt\n\n" + l.lastInterruptMsg + "\n\n"
				l.lastInterruptMsg = ""
			}

			// Inject wait results from a previous wait-for-spawns cycle.
			if len(l.lastWaitResults) > 0 {
				cfg.Prompt += "\n" + renderWaitResultsSection(l.lastWaitResults, l.moreSpawnsPending)
				l.lastWaitResults = nil
				l.moreSpawnsPending = false
			}
		}

		// Notify listener.
		if l.OnPrompt != nil {
			l.OnPrompt(turnID, turnHexID, cfg.Prompt, isResume)
		}
		if l.OnStart != nil {
			l.OnStart(turnID, turnHexID)
		}

		// Create a recorder for this turn.
		rec := recording.New(turnID, l.Store)
		rec.RecordMeta("agent", l.Agent.Name())
		rec.RecordMeta("turn", fmt.Sprintf("%d", turn+1))
		rec.RecordMeta("start_time", time.Now().UTC().Format(time.RFC3339))
		rec.RecordMeta("turn_hex_id", turnHexID)
		if isResume {
			rec.RecordMeta("resume_session_id", cfg.ResumeSessionID)
		}
		if l.LoopRunHexID != "" {
			rec.RecordMeta("loop_run_hex_id", l.LoopRunHexID)
		}
		if l.StepHexID != "" {
			rec.RecordMeta("step_hex_id", l.StepHexID)
		}

		// Create a turn-scoped context so external code can cancel just the
		// current turn without stopping the entire loop.
		var (
			turnCtx    context.Context
			turnCancel context.CancelFunc
		)
		turnCtx, turnCancel = context.WithCancel(ctx)
		debug.LogKV("loop", "turn context ready",
			"turn_id", turnID,
		)
		// Enforce wait-for-spawns as immediate control flow: as soon as a
		// wait signal exists for this turn, cancel the active agent turn.
		waitSignalSeen := make(chan struct{}, 1)
		waitWatcherDone := make(chan struct{})
		if l.Store != nil {
			go func(turnID int) {
				defer close(waitWatcherDone)
				defer l.Store.ReleaseWaitSignal(turnID)
				waitSignalCh := l.Store.WaitSignalChan(turnID)
				pollTicker := time.NewTicker(2 * time.Second)
				defer pollTicker.Stop()
				if l.Store.IsWaiting(turnID) {
					select {
					case waitSignalSeen <- struct{}{}:
					default:
					}
					debug.LogKV("loop", "wait-for-spawns: canceling active turn (expected)",
						"turn_id", turnID,
						"source", "startup_check",
					)
					turnCancel()
					return
				}
				for {
					select {
					case <-turnCtx.Done():
						return
					case <-waitSignalCh:
					case <-pollTicker.C:
						// Fallback for external signal writers in non-daemon mode.
					}
					if l.Store.IsWaiting(turnID) {
						select {
						case waitSignalSeen <- struct{}{}:
						default:
						}
						debug.LogKV("loop", "wait-for-spawns: canceling active turn (expected)",
							"turn_id", turnID,
							"source", "watcher",
						)
						turnCancel()
						return
					}
				}
			}(turnID)
		} else {
			close(waitWatcherDone)
		}
		if l.OnTurnContext != nil {
			l.OnTurnContext(turnCancel)
		}

		// Run the agent.
		debug.LogKV("loop", "agent.Run() starting",
			"turn_id", turnID,
			"agent", l.Agent.Name(),
			"workdir", cfg.WorkDir,
			"prompt_len", len(cfg.Prompt),
			"resume_session", cfg.ResumeSessionID,
		)
		agentStart := time.Now()
		result, runErr := l.Agent.Run(turnCtx, cfg, rec)
		turnCancel() // ensure turn context is always cleaned up
		<-waitWatcherDone
		waitTriggeredMidTurn := false
		select {
		case <-waitSignalSeen:
			waitTriggeredMidTurn = true
		default:
		}
		debug.LogKV("loop", "agent.Run() finished",
			"turn_id", turnID,
			"duration", time.Since(agentStart),
			"has_result", result != nil,
			"has_error", runErr != nil,
		)
		if waitTriggeredMidTurn {
			debug.LogKV("loop", "agent interrupted by wait-for-spawns signal (expected)",
				"turn_id", turnID,
				"exit_code", resultExitCode(result),
				"output_len", resultOutputLen(result),
				"stderr_len", resultErrorLen(result),
				"session_id", resultSessionID(result),
			)
		} else if result != nil {
			debug.LogKV("loop", "agent result",
				"turn_id", turnID,
				"exit_code", result.ExitCode,
				"duration", result.Duration,
				"output_len", len(result.Output),
				"stderr_len", len(result.Error),
				"session_id", result.AgentSessionID,
			)
		}
		if runErr != nil {
			if waitTriggeredMidTurn && errors.Is(runErr, context.Canceled) {
				debug.LogKV("loop", "agent canceled by wait-for-spawns signal (expected)",
					"turn_id", turnID,
					"error", runErr,
				)
			} else {
				debug.LogKV("loop", "agent error", "turn_id", turnID, "error", runErr)
			}
		}

		// Capture the result and session ID for potential resume.
		l.LastResult = result
		if result != nil && result.AgentSessionID != "" {
			l.lastAgentSessionID = result.AgentSessionID
		} else if cfg.ResumeSessionID != "" {
			// Preserve the resume target across canceled/error turns when the
			// agent did not emit a fresh session ID.
			l.lastAgentSessionID = cfg.ResumeSessionID
		}

		// Record completion metadata.
		rec.RecordMeta("end_time", time.Now().UTC().Format(time.RFC3339))
		if result != nil {
			rec.RecordMeta("exit_code", fmt.Sprintf("%d", result.ExitCode))
			rec.RecordMeta("duration", result.Duration.String())
		}

		// Flush the recording to the store.
		flushErr := rec.Flush()
		if flushErr != nil {
			debug.LogKV("loop", "recording flush failed", "turn_id", turnID, "error", flushErr)
		} else {
			debug.LogKV("loop", "recording flushed", "turn_id", turnID)
		}

		waitingForSpawns := l.Store != nil && l.Store.IsWaiting(turnID)
		if !waitingForSpawns && l.Store != nil {
			runningNonHandoff, runningHandoff, err := runningSpawnCountsForTurn(l.Store, turnID)
			if err != nil {
				debug.LogKV("loop", "failed to inspect running spawns after turn",
					"turn_id", turnID,
					"error", err,
				)
			} else if runningNonHandoff > 0 {
				waitingForSpawns = true
				debug.LogKV("loop", "inferring wait-for-spawns from running child spawns",
					"turn_id", turnID,
					"running_spawns", runningNonHandoff,
					"running_handoff_spawns", runningHandoff,
				)
			}
		}
		if waitTriggeredMidTurn {
			waitingForSpawns = true
		}
		debug.LogKV("loop", "post-run state",
			"turn_id", turnID,
			"waiting_for_spawns", waitingForSpawns,
			"wait_triggered_mid_turn", waitTriggeredMidTurn,
			"run_error", runErr != nil,
		)

		// Refresh the latest turn snapshot before final metadata write so we
		// never clobber handoff fields an agent updated during the turn.
		if latestTurn, err := l.Store.GetTurn(turnID); err != nil {
			debug.LogKV("loop", "turn refresh failed before finalize", "turn_id", turnID, "error", err)
		} else {
			turnLog = latestTurn
		}

		// Update system-owned metadata.
		if result != nil {
			durationSecs := int(result.Duration.Seconds())
			if resumingTurn && turnLog.DurationSecs > 0 {
				turnLog.DurationSecs += durationSecs
			} else {
				turnLog.DurationSecs = durationSecs
			}
		}
		if waitingForSpawns {
			turnLog.BuildState = "waiting_for_spawns"
		} else if runErr != nil {
			if errors.Is(runErr, context.Canceled) {
				turnLog.BuildState = "cancelled"
			} else {
				turnLog.BuildState = "error"
			}
		} else if result != nil {
			if result.ExitCode == 0 {
				turnLog.BuildState = "success"
			} else {
				turnLog.BuildState = fmt.Sprintf("exit_code_%d", result.ExitCode)
			}
		} else {
			turnLog.BuildState = "error"
		}
		if !waitingForSpawns && turnLog.FinalizedAt.IsZero() {
			turnLog.FinalizedAt = time.Now().UTC()
		}

		// Best-effort update of the turn. We re-read the ID since
		// CreateTurn already assigned it.
		if err := l.Store.UpdateTurn(turnLog); err != nil {
			debug.LogKV("loop", "turn update failed", "turn_id", turnID, "error", err)
		} else {
			debug.LogKV("loop", "turn updated",
				"turn_id", turnID,
				"build_state", turnLog.BuildState,
			)
		}

		// Notify listener.
		if l.OnEnd != nil {
			debug.LogKV("loop", "calling OnEnd callback", "turn_id", turnID)
			cbStart := time.Now()
			l.OnEnd(turnID, turnHexID, result)
			debug.LogKV("loop", "OnEnd callback returned",
				"turn_id", turnID,
				"duration", time.Since(cbStart),
			)
		}
		if flushErr != nil {
			flushRunErr := fmt.Errorf("flushing recording for turn %d: %w", turnID, flushErr)
			if runErr != nil {
				return errors.Join(runErr, flushRunErr)
			}
			return flushRunErr
		}

		// Check for wait-for-spawns signal first. This is turn control flow,
		// not a terminal error condition.
		if waitingForSpawns {
			debug.LogKV("loop", "wait-for-spawns signal detected", "turn_id", turnID)
			if l.Store != nil {
				if err := l.Store.ClearWait(turnID); err != nil {
					fmt.Printf("warning: failed to clear wait signal for turn %d: %v\n", turnID, err)
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if l.OnWait != nil {
				seenSpawnIDs := l.ensureSeenSpawnIDs(turnID)
				debug.LogKV("loop", "blocking on OnWait callback",
					"turn_id", turnID,
					"already_seen", len(seenSpawnIDs),
				)
				waitStart := time.Now()
				l.lastWaitResults, l.moreSpawnsPending = l.OnWait(ctx, turnID, seenSpawnIDs)
				for _, wr := range l.lastWaitResults {
					if store.IsTerminalSpawnStatus(wr.Status) {
						seenSpawnIDs[wr.SpawnID] = struct{}{}
					}
				}
				debug.LogKV("loop", "OnWait callback returned",
					"turn_id", turnID,
					"wait_duration", time.Since(waitStart),
					"results_count", len(l.lastWaitResults),
					"already_seen", len(seenSpawnIDs),
					"more_pending", l.moreSpawnsPending,
				)
			}
			l.waitResumeTurnID = turnID
			l.waitResumeTurnHexID = turnHexID
			debug.LogKV("loop", "wait-for-spawns resume prepared",
				"turn_id", turnID,
				"results", len(l.lastWaitResults),
				"more_pending", l.moreSpawnsPending,
			)
			// Don't increment turn count — the wait turn doesn't count toward the limit.
			// Loop continues to next iteration with wait results in the prompt.
			continue
		}

		// If the agent run failed with a hard error (not just non-zero exit),
		// stop the loop — unless it was an interrupt.
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) {
				if msg := l.drainInterrupt(); msg != "" {
					debug.LogKV("loop", "interrupt drained after cancel", "turn_id", turnID, "msg_len", len(msg))
					if isCallSupervisorControlInterrupt(msg) {
						debug.LogKV("loop", "ending step from control interrupt", "turn_id", turnID)
						return ErrStepEndedByControlSignal
					}
					// Interrupted (e.g. parent signal) — continue to next
					// turn with the interrupt message injected.
					// Don't increment turn count — interrupt turns don't count
					// toward MaxTurns (same as wait-for-spawns).
					l.lastInterruptMsg = msg
					continue
				}
				// If the parent context is still alive and we have turn-scoped
				// cancel support (OnTurnContext), this was a turn-only cancel
				// that raced with drainInterrupt.
				if l.OnTurnContext != nil && ctx.Err() == nil {
					debug.LogKV("loop", "turn canceled without interrupt payload; continuing",
						"turn_id", turnID,
					)
					continue
				}
				// Preserve cancellation semantics so callers can classify graceful stop.
				debug.LogKV("loop", "loop exiting with context canceled", "turn_id", turnID)
				return context.Canceled
			}
			return fmt.Errorf("agent run failed (turn %d): %w", turnID, runErr)
		}

		// Check for interrupt even when runErr is nil.
		// When agents are killed by SIGKILL (e.g. interrupt from parent),
		// cmd.Wait() returns *exec.ExitError which Agent.Run()
		// handles as a normal exit — returning (*Result, nil) rather than
		// (nil, context.Canceled). We must still drain the interrupt channel
		// so the message is injected into the next turn.
		if msg := l.drainInterrupt(); msg != "" {
			if isCallSupervisorControlInterrupt(msg) {
				debug.LogKV("loop", "ending step from control interrupt", "turn_id", turnID)
				return ErrStepEndedByControlSignal
			}
			l.lastInterruptMsg = msg
			debug.LogKV("loop", "interrupt drained after successful run",
				"turn_id", turnID,
				"msg_len", len(msg),
			)
			continue
		}
		// If the parent context was canceled but no interrupt message was
		// pending, exit the loop. This handles child contexts canceled by
		// the orchestrator.
		if ctx.Err() != nil {
			debug.LogKV("loop", "parent context canceled after turn",
				"turn_id", turnID,
				"error", ctx.Err(),
			)
			return ctx.Err()
		}

		// Update profile stats from a fully completed turn.
		if l.ProfileName != "" {
			if err := stats.UpdateProfileStats(l.Store, l.ProfileName, turnID); err != nil {
				debug.LogKV("loop", "profile stats update failed",
					"turn_id", turnID,
					"profile", l.ProfileName,
					"error", err,
				)
			} else {
				debug.LogKV("loop", "profile stats updated",
					"turn_id", turnID,
					"profile", l.ProfileName,
				)
			}
		}

		if l.StopAfterTurn != nil && l.StopAfterTurn(turnID) {
			debug.LogKV("loop", "loop exiting after completed turn",
				"turn_id", turnID,
				"reason", "stop_after_turn",
			)
			return nil
		}

		turn++
		debug.LogKV("loop", "turn counter incremented", "next_turn_index", turn)
	}
}

func resultExitCode(result *agent.Result) int {
	if result == nil {
		return -1
	}
	return result.ExitCode
}

func resultOutputLen(result *agent.Result) int {
	if result == nil {
		return 0
	}
	return len(result.Output)
}

func resultErrorLen(result *agent.Result) int {
	if result == nil {
		return 0
	}
	return len(result.Error)
}

func resultSessionID(result *agent.Result) string {
	if result == nil {
		return ""
	}
	return result.AgentSessionID
}

// drainInterrupt checks if there's a pending interrupt and drains the channel.
// Returns the interrupt message, or "" if no interrupt was pending.
func (l *Loop) drainInterrupt() string {
	if l.InterruptCh == nil {
		return ""
	}
	select {
	case msg := <-l.InterruptCh:
		if msg == "" {
			msg = "interrupted"
		}
		return msg
	default:
		return ""
	}
}

func isCallSupervisorControlInterrupt(msg string) bool {
	return strings.TrimSpace(msg) == InterruptMessageCallSupervisor
}

func (l *Loop) ensureSeenSpawnIDs(turnID int) map[int]struct{} {
	if l.seenSpawnTurnID != turnID || l.seenSpawnIDs == nil {
		l.seenSpawnTurnID = turnID
		l.seenSpawnIDs = make(map[int]struct{})
	}
	return l.seenSpawnIDs
}

// LastAgentSessionID returns the most recent agent session ID available for
// resume after the loop run.
func (l *Loop) LastAgentSessionID() string {
	return strings.TrimSpace(l.lastAgentSessionID)
}

// BuildResumePrompt returns the exact continuation prompt used by the loop
// runtime when resuming an existing agent session.
func BuildResumePrompt(waitResults []WaitResult, moreSpawnsPending bool, interruptMsg string, includeContinueLead bool) string {
	return buildResumePrompt(waitResults, moreSpawnsPending, interruptMsg, includeContinueLead)
}

// buildResumePrompt constructs a minimal continuation prompt for a resumed
// agent session. Unlike a fresh turn, the agent already has the full system
// prompt and conversation history — we only send new information.
func buildResumePrompt(waitResults []WaitResult, moreSpawnsPending bool, interruptMsg string, includeContinueLead bool) string {
	var b strings.Builder

	if includeContinueLead {
		b.WriteString("Continue from where you left off.\n\n")
	}

	if interruptMsg != "" {
		b.WriteString("## Interrupt\n\n")
		b.WriteString(interruptMsg)
		b.WriteString("\n\n")
	}

	if len(waitResults) > 0 {
		b.WriteString(renderWaitResultsSection(waitResults, moreSpawnsPending))
	}

	return b.String()
}

func renderWaitResultsSection(waitResults []WaitResult, moreSpawnsPending bool) string {
	if len(waitResults) == 0 {
		return ""
	}

	reviewCheckpoint := false
	for _, wr := range waitResults {
		if wr.Review || !store.IsTerminalSpawnStatus(wr.Status) {
			reviewCheckpoint = true
			break
		}
	}

	var b strings.Builder
	if reviewCheckpoint {
		b.WriteString("## Spawn Review Checkpoint\n\n")
		b.WriteString("These spawns are still running. Review health signals and decide whether to continue waiting or intervene.\n\n")
	} else {
		b.WriteString("## Spawn Wait Results\n\n")
		b.WriteString("The following spawns have completed:\n\n")
	}

	for _, wr := range waitResults {
		b.WriteString(formatWaitResult(wr))
	}

	if moreSpawnsPending {
		if reviewCheckpoint {
			b.WriteString("**Spawns are still running.** Call `adaf wait-for-spawns` to keep waiting, or intervene with `adaf spawn-message`, `adaf spawn-reject`, or `adaf spawn-status`.\n\n")
		} else {
			b.WriteString("**Other spawns are still running.** Call `adaf wait-for-spawns` again when you need more results.\n\n")
		}
	}

	return b.String()
}

// formatWaitResult formats a single WaitResult for injection into the prompt.
func formatWaitResult(wr WaitResult) string {
	var b strings.Builder

	// Header: ### Spawn #7 (profile=devstral2, read-only) — completed
	fmt.Fprintf(&b, "### Spawn #%d (profile=%s", wr.SpawnID, wr.Profile)
	if wr.ReadOnly {
		b.WriteString(", read-only")
	} else if wr.Branch != "" {
		fmt.Fprintf(&b, ", branch=%s", wr.Branch)
	}
	b.WriteString(") — ")
	b.WriteString(wr.Status)
	if wr.ExitCode != 0 {
		fmt.Fprintf(&b, " (exit_code=%d)", wr.ExitCode)
	}
	b.WriteString("\n\n")

	if wr.Review || !store.IsTerminalSpawnStatus(wr.Status) {
		if wr.Elapsed > 0 {
			fmt.Fprintf(&b, "- Elapsed: %s\n", wr.Elapsed.Round(time.Second))
		}
		fmt.Fprintf(&b, "- Compactions: %d\n", wr.CompactionCount)
		if wr.ReadCount > 0 || wr.WriteCount > 0 || wr.CommitCount > 0 {
			fmt.Fprintf(&b, "- Activity: %d writes / %d reads", wr.WriteCount, wr.ReadCount)
			if wr.CommitCount > 0 {
				fmt.Fprintf(&b, " (commits: %d)", wr.CommitCount)
			}
			b.WriteString("\n")
		}
		if wr.InputTokens > 0 || wr.CachedInputTokens > 0 || wr.OutputTokens > 0 {
			fmt.Fprintf(&b, "- Tokens: input=%d", wr.InputTokens)
			if wr.CachedInputTokens > 0 {
				fmt.Fprintf(&b, ", cached_input=%d", wr.CachedInputTokens)
			}
			if wr.OutputTokens > 0 {
				fmt.Fprintf(&b, ", output=%d", wr.OutputTokens)
			}
			b.WriteString("\n")
		}
		if wr.Summary != "" {
			b.WriteString("\n")
			b.WriteString(wr.Summary)
			b.WriteString("\n\n")
		} else {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Body: prefer Summary, fall back to Result.
	body := wr.Summary
	if body == "" {
		body = wr.Result
	}
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	} else {
		b.WriteString("(no output captured)\n\n")
	}

	return b.String()
}

func runningSpawnCountsForTurn(s *store.Store, turnID int) (runningNonHandoff int, runningHandoff int, err error) {
	if s == nil || turnID <= 0 {
		return 0, 0, nil
	}
	records, err := s.SpawnsByParent(turnID)
	if err != nil {
		return 0, 0, err
	}
	for _, rec := range records {
		if store.IsTerminalSpawnStatus(rec.Status) {
			continue
		}
		if rec.Handoff {
			runningHandoff++
		} else {
			runningNonHandoff++
		}
	}
	return runningNonHandoff, runningHandoff, nil
}

// summarizeObjectiveForLog extracts a compact objective summary from a full
// generated prompt so turn logs don't recursively store entire prior prompts.
func summarizeObjectiveForLog(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}

	const (
		objectiveHeader = "# Objective"
		rulesHeader     = "# Rules"
		contextHeader   = "# Context"
		maxLen          = 320
	)

	section := prompt
	if idx := strings.Index(section, objectiveHeader); idx >= 0 {
		section = section[idx+len(objectiveHeader):]
	}
	if idx := strings.Index(section, rulesHeader); idx >= 0 {
		section = section[:idx]
	}
	if idx := strings.Index(section, contextHeader); idx >= 0 {
		section = section[:idx]
	}

	section = strings.TrimSpace(section)
	if section == "" {
		section = prompt
	}
	section = strings.ReplaceAll(section, "\r", " ")
	section = strings.ReplaceAll(section, "\n", " ")
	section = strings.Join(strings.Fields(section), " ")

	if len(section) > maxLen {
		return section[:maxLen-3] + "..."
	}
	return section
}
