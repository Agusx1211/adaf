package events

import (
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/stream"
)

// AgentEventMsg wraps a parsed Claude stream event.
type AgentEventMsg struct {
	Event   stream.ClaudeEvent
	Raw     []byte
	SpawnID int // positive = sub-agent spawn, zero = parent session
	TurnID  int // parent turn ID for session-scoped events
}

// AgentRawOutputMsg carries raw text for non-Claude agents.
type AgentRawOutputMsg struct {
	Data      string
	SessionID int
}

// AgentStartedMsg signals that a new agent session has begun.
type AgentStartedMsg struct {
	SessionID int
	TurnHexID string
	StepHexID string
	RunHexID  string
}

// AgentPromptMsg carries the exact prompt that is about to be sent to a turn.
type AgentPromptMsg struct {
	SessionID      int
	TurnHexID      string
	Prompt         string
	IsResume       bool
	Truncated      bool
	OriginalLength int
}

// SessionSnapshotMsg carries daemon snapshot state sent on reconnect.
type SessionSnapshotMsg struct {
	Loop    SessionLoopSnapshot
	Session *SessionTurnSnapshot
	Spawns  []SpawnInfo
}

// SessionLiveMsg signals that snapshot replay is complete and the stream is live.
type SessionLiveMsg struct{}

// SessionLoopSnapshot captures current loop progress on reconnect.
type SessionLoopSnapshot struct {
	RunID      int
	RunHexID   string
	StepHexID  string
	Cycle      int
	StepIndex  int
	Profile    string
	TotalSteps int
}

// SessionTurnSnapshot captures current turn status on reconnect.
type SessionTurnSnapshot struct {
	SessionID    int
	TurnHexID    string
	Agent        string
	Profile      string
	Model        string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	NumTurns     int
	Status       string
	Action       string
	StartedAt    time.Time
	EndedAt      time.Time
}

// AgentFinishedMsg signals that a single agent session completed.
type AgentFinishedMsg struct {
	SessionID     int
	TurnHexID     string
	WaitForSpawns bool
	Result        *agent.Result
	Err           error
}

// AgentLoopDoneMsg signals the entire loop has completed.
type AgentLoopDoneMsg struct {
	Err error
}

// SpawnStatusMsg carries hierarchical spawn status for the left panel.
type SpawnStatusMsg struct {
	Spawns []SpawnInfo
}

// SpawnInfo describes a spawn entry for the hierarchy view.
type SpawnInfo struct {
	ID            int
	ParentTurnID  int
	ParentSpawnID int
	ChildTurnID   int
	Profile       string
	Position      string
	Role          string
	Status        string // "running", "awaiting_input", "completed", "failed", "canceled", "merged", "rejected"
	Question      string // pending question when status is "awaiting_input"
	Summary       string // parent-facing final summary (or crash note on failure)
	Result        string // raw completion/crash result text
}

// LoopStepStartMsg signals that a loop step has started.
type LoopStepStartMsg struct {
	RunID      int
	RunHexID   string
	StepHexID  string
	Cycle      int
	StepIndex  int
	Profile    string
	Turns      int
	TotalSteps int
}

// LoopStepEndMsg signals that a loop step has ended.
type LoopStepEndMsg struct {
	RunID      int
	RunHexID   string
	StepHexID  string
	Cycle      int
	StepIndex  int
	Profile    string
	TotalSteps int
}

// LoopDoneMsg signals that the entire loop has finished.
type LoopDoneMsg struct {
	RunID    int
	RunHexID string
	Reason   string // "stopped", "cancelled", "error"
	Err      error
}
