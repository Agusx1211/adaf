package loop

import (
	"context"
	"fmt"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

// Loop is the main agent loop controller. It runs an agent one or more times,
// creating a new session recording for each iteration, and persists results
// to the store.
type Loop struct {
	Store  *store.Store
	Agent  agent.Agent
	Config agent.Config

	// ProfileName is the name of the profile that launched this loop.
	ProfileName string

	// OnStart is called at the beginning of each iteration, before the agent runs.
	// The sessionID of the upcoming run is passed as an argument.
	OnStart func(sessionID int)

	// OnEnd is called after each iteration completes (successfully or not).
	// The sessionID and the result (which may be nil on error) are passed.
	OnEnd func(sessionID int, result *agent.Result)

	// PromptFunc, if set, is called before each turn to dynamically refresh the
	// prompt (e.g. to inject supervisor notes). If nil, Config.Prompt is used.
	PromptFunc func(sessionID int) string
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

		// Allocate a new session ID by creating a session log entry.
		sessionLog := &store.SessionLog{
			Agent:       l.Agent.Name(),
			ProfileName: l.ProfileName,
			Objective:   l.Config.Prompt,
		}
		if err := l.Store.CreateLog(sessionLog); err != nil {
			return fmt.Errorf("creating session log: %w", err)
		}
		sessionID := sessionLog.ID

		// Update config with the current session ID.
		cfg := l.Config
		cfg.SessionID = sessionID

		// Set ADAF_AGENT=1 so the agent process knows it's running under adaf
		// and session management commands are blocked.
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		cfg.Env["ADAF_AGENT"] = "1"
		if l.PromptFunc != nil {
			cfg.Prompt = l.PromptFunc(sessionID)
		}

		// Notify listener.
		if l.OnStart != nil {
			l.OnStart(sessionID)
		}

		// Create a recorder for this session.
		rec := recording.New(sessionID, l.Store)
		rec.RecordMeta("agent", l.Agent.Name())
		rec.RecordMeta("turn", fmt.Sprintf("%d", turn+1))
		rec.RecordMeta("start_time", time.Now().UTC().Format(time.RFC3339))

		// Run the agent.
		result, runErr := l.Agent.Run(ctx, cfg, rec)

		// Record completion metadata.
		rec.RecordMeta("end_time", time.Now().UTC().Format(time.RFC3339))
		if result != nil {
			rec.RecordMeta("exit_code", fmt.Sprintf("%d", result.ExitCode))
			rec.RecordMeta("duration", result.Duration.String())
		}

		// Flush the recording to the store.
		if flushErr := rec.Flush(); flushErr != nil {
			// Log flush error but don't fail the loop.
			fmt.Printf("warning: failed to flush recording for session %d: %v\n", sessionID, flushErr)
		}

		// Update profile stats from the completed session.
		if l.ProfileName != "" {
			_ = stats.UpdateProfileStats(l.Store, l.ProfileName, sessionID)
		}

		// Update the session log with results.
		if result != nil {
			sessionLog.DurationSecs = int(result.Duration.Seconds())
			if result.ExitCode == 0 {
				sessionLog.BuildState = "success"
			} else {
				sessionLog.BuildState = fmt.Sprintf("exit_code_%d", result.ExitCode)
			}
			sessionLog.CurrentState = fmt.Sprintf("Turn %d completed", turn+1)
		} else {
			sessionLog.BuildState = "error"
			if runErr != nil {
				sessionLog.KnownIssues = runErr.Error()
			}
		}

		// Best-effort update of the session log. We re-read the ID since
		// CreateLog already assigned it.
		_ = l.Store.UpdateLog(sessionLog)

		// Notify listener.
		if l.OnEnd != nil {
			l.OnEnd(sessionID, result)
		}

		// If the agent run failed with a hard error (not just non-zero exit),
		// stop the loop.
		if runErr != nil {
			return fmt.Errorf("agent run failed (session %d): %w", sessionID, runErr)
		}

		turn++
	}
}
