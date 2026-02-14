package runtui

import "github.com/agusx1211/adaf/internal/events"

// Re-export event types for backwards compatibility within the TUI layer.
type AgentEventMsg = events.AgentEventMsg
type AgentRawOutputMsg = events.AgentRawOutputMsg
type AgentStartedMsg = events.AgentStartedMsg
type AgentPromptMsg = events.AgentPromptMsg
type SessionSnapshotMsg = events.SessionSnapshotMsg
type SessionLoopSnapshot = events.SessionLoopSnapshot
type SessionTurnSnapshot = events.SessionTurnSnapshot
type SessionLiveMsg = events.SessionLiveMsg
type AgentFinishedMsg = events.AgentFinishedMsg
type AgentLoopDoneMsg = events.AgentLoopDoneMsg
type SpawnStatusMsg = events.SpawnStatusMsg
type SpawnInfo = events.SpawnInfo
type LoopStepStartMsg = events.LoopStepStartMsg
type LoopStepEndMsg = events.LoopStepEndMsg
type LoopDoneMsg = events.LoopDoneMsg
type GuardrailViolationMsg = events.GuardrailViolationMsg

// BackToSelectorMsg signals that the user wants to return to the agent selector.
type BackToSelectorMsg struct{}

// DetachMsg signals that the user wants to detach from the session without
// stopping the agent. The session continues running in the background.
type DetachMsg struct {
	SessionID int
}

// tickMsg is sent every second to update the elapsed time display.
type tickMsg struct{}
