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
	Data string
}

// AgentStartedMsg signals that a new agent session has begun.
type AgentStartedMsg struct {
	SessionID int
}

// AgentFinishedMsg signals that a single agent session completed.
type AgentFinishedMsg struct {
	SessionID int
	Result    *agent.Result
	Err       error
}

// AgentLoopDoneMsg signals the entire loop has completed.
type AgentLoopDoneMsg struct {
	Err error
}

// BackToSelectorMsg signals that the user wants to return to the agent selector.
type BackToSelectorMsg struct{}

// SpawnStatusMsg carries hierarchical spawn status for the left panel.
type SpawnStatusMsg struct {
	Spawns []SpawnInfo
}

// SpawnInfo describes a spawn entry for the hierarchy view.
type SpawnInfo struct {
	ID      int
	Profile string
	Status  string // "queued", "running", "completed", "failed", "merged", "rejected"
}

// tickMsg is sent every second to update the elapsed time display.
type tickMsg struct{}
