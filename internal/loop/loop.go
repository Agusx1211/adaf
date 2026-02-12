package loop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/hexid"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

// WaitCallback is called when the loop detects a wait signal.
// It should block until spawns complete and return results for the next prompt.
type WaitCallback func(turnID int) []WaitResult

// WaitResult describes the outcome of a spawn that was waited on.
type WaitResult struct {
	SpawnID  int
	Profile  string
	Status   string
	ExitCode int
	Result   string
}

// Loop is the main agent loop controller. It runs an agent one or more times,
// creating a new turn recording for each iteration, and persists results
// to the store.
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

	// OnEnd is called after each iteration completes (successfully or not).
	// The turnID, turnHexID, and the result (which may be nil on error) are passed.
	OnEnd func(turnID int, turnHexID string, result *agent.Result)

	// PromptFunc, if set, is called before each turn to dynamically refresh the
	// prompt (e.g. to inject supervisor notes). If nil, Config.Prompt is used.
	PromptFunc func(turnID int, supervisorNotes []store.SupervisorNote) string

	// OnWait is called when the agent signals a wait-for-spawns.
	// It should block until spawns complete and return results.
	// If nil, wait signals are ignored.
	OnWait WaitCallback

	// InterruptCh, if set, receives signals when the agent's turn should be
	// interrupted (e.g. parent sends an interrupt message).
	InterruptCh <-chan string

	// OnTurnContext is called at the start of each turn with the turn-scoped
	// cancel function. This allows external code (e.g. guardrail monitors) to
	// cancel only the current turn without stopping the entire loop.
	OnTurnContext func(cancel context.CancelFunc)

	// lastWaitResults holds results from a wait cycle, injected into the next prompt.
	lastWaitResults []WaitResult

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

	for {
		// Check if we've hit the turn limit.
		if maxTurns > 0 && turn >= maxTurns {
			return nil
		}

		// Check for context cancellation before starting a new turn.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Allocate a new turn ID by creating a turn entry.
		objective := summarizeObjectiveForLog(l.Config.Prompt)
		if objective == "" {
			objective = "Agent run"
		}
		turnHexID := hexid.New()
		turnLog := &store.Turn{
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
		turnID := turnLog.ID

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
			projectDir := strings.TrimSpace(filepath.Dir(l.Store.Root()))
			if projectDir != "" {
				cfg.Env["ADAF_PROJECT_DIR"] = projectDir
			}
		}
		var supervisorNotes []store.SupervisorNote
		if l.PromptFunc != nil && l.Store != nil {
			notes, err := l.Store.NotesByTurn(turnID)
			if err != nil {
				fmt.Printf("warning: failed to load supervisor notes for turn %d: %v\n", turnID, err)
			} else {
				supervisorNotes = notes
			}
		}
		if l.PromptFunc != nil {
			cfg.Prompt = l.PromptFunc(turnID, supervisorNotes)
		}

		// Inject interrupt message from a previous guardrail violation.
		if l.lastInterruptMsg != "" {
			cfg.Prompt += "\n## Guardrail Interrupt\n\n" + l.lastInterruptMsg + "\n\n"
			l.lastInterruptMsg = ""
		}

		// Inject wait results from a previous wait-for-spawns cycle.
		if len(l.lastWaitResults) > 0 {
			cfg.Prompt += "\n## Spawn Wait Results\n\nThe spawns you waited for have completed:\n\n"
			for _, wr := range l.lastWaitResults {
				cfg.Prompt += fmt.Sprintf("- Spawn #%d (profile=%s): status=%s, exit_code=%d",
					wr.SpawnID, wr.Profile, wr.Status, wr.ExitCode)
				if wr.Result != "" {
					cfg.Prompt += fmt.Sprintf(" — %s", wr.Result)
				}
				cfg.Prompt += "\n"
			}
			cfg.Prompt += "\nReview their diffs with `adaf spawn-diff --spawn-id N` and merge or reject as needed.\n\n"
			l.lastWaitResults = nil // Clear after injecting.
		}

		// Notify listener.
		if l.OnStart != nil {
			l.OnStart(turnID, turnHexID)
		}

		// Create a recorder for this turn.
		rec := recording.New(turnID, l.Store)
		rec.RecordMeta("agent", l.Agent.Name())
		rec.RecordMeta("turn", fmt.Sprintf("%d", turn+1))
		rec.RecordMeta("start_time", time.Now().UTC().Format(time.RFC3339))
		rec.RecordMeta("turn_hex_id", turnHexID)
		if l.LoopRunHexID != "" {
			rec.RecordMeta("loop_run_hex_id", l.LoopRunHexID)
		}
		if l.StepHexID != "" {
			rec.RecordMeta("step_hex_id", l.StepHexID)
		}

		// Create a turn-scoped context so guardrails can cancel just the
		// current turn without stopping the entire loop.
		turnCtx, turnCancel := context.WithCancel(ctx)
		if l.OnTurnContext != nil {
			l.OnTurnContext(turnCancel)
		}

		// Run the agent.
		result, runErr := l.Agent.Run(turnCtx, cfg, rec)
		turnCancel() // ensure turn context is always cleaned up

		// Record completion metadata.
		rec.RecordMeta("end_time", time.Now().UTC().Format(time.RFC3339))
		if result != nil {
			rec.RecordMeta("exit_code", fmt.Sprintf("%d", result.ExitCode))
			rec.RecordMeta("duration", result.Duration.String())
		}

		// Flush the recording to the store.
		if flushErr := rec.Flush(); flushErr != nil {
			// Log flush error but don't fail the loop.
			fmt.Printf("warning: failed to flush recording for turn %d: %v\n", turnID, flushErr)
		}

		// Update profile stats from the completed turn.
		if l.ProfileName != "" {
			_ = stats.UpdateProfileStats(l.Store, l.ProfileName, turnID)
		}

		// Update the turn with results.
		if result != nil {
			turnLog.DurationSecs = int(result.Duration.Seconds())
			if result.ExitCode == 0 {
				turnLog.BuildState = "success"
			} else {
				turnLog.BuildState = fmt.Sprintf("exit_code_%d", result.ExitCode)
			}
			turnLog.CurrentState = fmt.Sprintf("Turn %d completed", turn+1)
		} else {
			if errors.Is(runErr, context.Canceled) {
				turnLog.BuildState = "cancelled"
			} else {
				turnLog.BuildState = "error"
			}
			if runErr != nil {
				turnLog.KnownIssues = runErr.Error()
			}
		}

		// Best-effort update of the turn. We re-read the ID since
		// CreateTurn already assigned it.
		_ = l.Store.UpdateTurn(turnLog)

		// Notify listener.
		if l.OnEnd != nil {
			l.OnEnd(turnID, turnHexID, result)
		}

		// If the agent run failed with a hard error (not just non-zero exit),
		// stop the loop — unless it was an interrupt.
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) {
				if msg := l.drainInterrupt(); msg != "" {
					// Interrupted (e.g. guardrail violation or parent signal) —
					// continue to next turn with the interrupt message injected.
					l.lastInterruptMsg = msg
					turn++
					continue
				}
				// If the parent context is still alive and we have turn-scoped
				// cancel support (OnTurnContext), this was a turn-only cancel
				// (e.g. guardrail) that raced with drainInterrupt.
				if l.OnTurnContext != nil && ctx.Err() == nil {
					turn++
					continue
				}
				// Preserve cancellation semantics so callers can classify graceful stop.
				return context.Canceled
			}
			return fmt.Errorf("agent run failed (turn %d): %w", turnID, runErr)
		}

		// Check for wait-for-spawns signal.
		if l.Store != nil && l.Store.IsWaiting(turnID) {
			if err := l.Store.ClearWait(turnID); err != nil {
				fmt.Printf("warning: failed to clear wait signal for turn %d: %v\n", turnID, err)
			}
			if l.OnWait != nil {
				l.lastWaitResults = l.OnWait(turnID)
			}
			// Don't increment turn count — the wait turn doesn't count toward the limit.
			// Loop continues to next iteration with wait results in the prompt.
			continue
		}

		turn++
	}
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
