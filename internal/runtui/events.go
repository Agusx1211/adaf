package runtui

import (
	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/stream"
)

// AgentEventMsg wraps a parsed Claude stream event for the TUI.
type AgentEventMsg struct {
	Event stream.ClaudeEvent
	Raw   []byte
}

// AgentRawOutputMsg carries raw text for non-Claude agents.
type AgentRawOutputMsg struct {
	Data      string
	SessionID int
}

// AgentStartedMsg signals that a new agent session has begun.
type AgentStartedMsg struct {
	SessionID  int
	TurnHexID  string
	StepHexID  string
	RunHexID   string
}

// AgentFinishedMsg signals that a single agent session completed.
type AgentFinishedMsg struct {
	SessionID int
	TurnHexID string
	Result    *agent.Result
	Err       error
}

// AgentLoopDoneMsg signals the entire loop has completed.
type AgentLoopDoneMsg struct {
	Err error
}

// BackToSelectorMsg signals that the user wants to return to the agent selector.
type BackToSelectorMsg struct{}

// DetachMsg signals that the user wants to detach from the session without
// stopping the agent. The session continues running in the background.
type DetachMsg struct {
	SessionID int
}

// SpawnStatusMsg carries hierarchical spawn status for the left panel.
type SpawnStatusMsg struct {
	Spawns []SpawnInfo
}

// SpawnInfo describes a spawn entry for the hierarchy view.
type SpawnInfo struct {
	ID       int
	Profile  string
	Status   string // "queued", "running", "awaiting_input", "completed", "failed", "merged", "rejected"
	Question string // pending question when status is "awaiting_input"
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

// GuardrailViolationMsg signals that a guardrail violation was detected.
type GuardrailViolationMsg struct {
	Tool string
	Role string
}

// tickMsg is sent every second to update the elapsed time display.
type tickMsg struct{}
