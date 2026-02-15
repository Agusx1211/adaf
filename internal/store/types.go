package store

import "time"

type ProjectConfig struct {
	Name         string            `json:"name"`
	RepoPath     string            `json:"repo_path"`
	Created      time.Time         `json:"created"`
	AgentConfig  map[string]string `json:"agent_config"` // agent name -> path/config
	Metadata     map[string]any    `json:"metadata"`
	ActivePlanID string            `json:"active_plan_id,omitempty"`
}

type PlanPhase struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // "not_started", "in_progress", "complete", "blocked"
	Priority    int      `json:"priority"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type Plan struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Status       string      `json:"status"` // "active", "done", "cancelled", "frozen"
	Phases       []PlanPhase `json:"phases"`
	CriticalPath []string    `json:"critical_path,omitempty"`
	Created      time.Time   `json:"created"`
	Updated      time.Time   `json:"updated"`
}

type Issue struct {
	ID          int       `json:"id"`
	PlanID      string    `json:"plan_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`   // "open", "in_progress", "resolved", "wontfix"
	Priority    string    `json:"priority"` // "critical", "high", "medium", "low"
	Labels      []string  `json:"labels,omitempty"`
	TurnID      int       `json:"session_id,omitempty"` // which turn created it
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

type Doc struct {
	ID      string    `json:"id"`
	PlanID  string    `json:"plan_id,omitempty"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// Turn records what an agent accomplished in a single invocation (one turn of a loop).
type Turn struct {
	ID           int       `json:"id"`
	HexID        string    `json:"hex_id,omitempty"`
	LoopRunHexID string    `json:"loop_run_hex_id,omitempty"`
	StepHexID    string    `json:"step_hex_id,omitempty"`
	PlanID       string    `json:"plan_id,omitempty"`
	Date         time.Time `json:"date"`
	Agent        string    `json:"agent"` // "claude", "codex", "vibe", etc.
	AgentModel   string    `json:"agent_model,omitempty"`
	ProfileName  string    `json:"profile_name,omitempty"`
	CommitHash   string    `json:"commit_hash,omitempty"`
	Objective    string    `json:"objective"`
	WhatWasBuilt string    `json:"what_was_built"`
	KeyDecisions string    `json:"key_decisions,omitempty"`
	Challenges   string    `json:"challenges,omitempty"`
	CurrentState string    `json:"current_state"`
	KnownIssues  string    `json:"known_issues,omitempty"`
	NextSteps    string    `json:"next_steps"`
	BuildState   string    `json:"build_state"`
	DurationSecs int       `json:"duration_secs,omitempty"`
}

// TurnRecording captures the raw I/O of a single agent turn.
type TurnRecording struct {
	TurnID    int              `json:"session_id"` // keep JSON key for backward compat
	Agent     string           `json:"agent"`
	StartTime time.Time        `json:"start_time"`
	EndTime   time.Time        `json:"end_time,omitempty"`
	ExitCode  int              `json:"exit_code"`
	Events    []RecordingEvent `json:"events"`
}

type RecordingEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "stdin", "stdout", "stderr", "meta"
	Data      string    `json:"data"`
}

type PMChatMessage struct {
	ID        int       `json:"id"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	SessionID int       `json:"session_id,omitempty"` // linked PM session if any
}

type StandaloneChatMessage struct {
	ID        int       `json:"id"`
	Profile   string    `json:"profile"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	SessionID int       `json:"session_id,omitempty"`
}

// StandaloneChatInstance is an independent conversation thread backed by a
// standalone profile.  Multiple instances can share the same profile.
type StandaloneChatInstance struct {
	ID            string    `json:"id"`
	Profile       string    `json:"profile"`                    // standalone profile name
	Title         string    `json:"title"`                      // auto-set from first user message
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastSessionID int       `json:"last_session_id,omitempty"` // most recent daemon session ID
}
