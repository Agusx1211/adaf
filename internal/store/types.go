package store

import "time"

type ProjectConfig struct {
	Name        string            `json:"name"`
	RepoPath    string            `json:"repo_path"`
	Created     time.Time         `json:"created"`
	AgentConfig map[string]string `json:"agent_config"` // agent name -> path/config
	Metadata    map[string]string `json:"metadata"`
}

type PlanPhase struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"` // "not_started", "in_progress", "complete", "blocked"
	Priority    int    `json:"priority"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type Plan struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Phases      []PlanPhase `json:"phases"`
	CriticalPath []string   `json:"critical_path,omitempty"`
	Updated     time.Time   `json:"updated"`
}

type Issue struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "open", "in_progress", "resolved", "wontfix"
	Priority    string    `json:"priority"` // "critical", "high", "medium", "low"
	Labels      []string  `json:"labels,omitempty"`
	SessionID   int       `json:"session_id,omitempty"` // which session created it
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

type Doc struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type SessionLog struct {
	ID             int       `json:"id"`
	Date           time.Time `json:"date"`
	Agent          string    `json:"agent"` // "claude", "codex", "vibe", etc.
	AgentModel     string    `json:"agent_model,omitempty"`
	CommitHash     string    `json:"commit_hash,omitempty"`
	Objective      string    `json:"objective"`
	WhatWasBuilt   string    `json:"what_was_built"`
	KeyDecisions   string    `json:"key_decisions,omitempty"`
	Challenges     string    `json:"challenges,omitempty"`
	CurrentState   string    `json:"current_state"`
	KnownIssues    string    `json:"known_issues,omitempty"`
	NextSteps      string    `json:"next_steps"`
	BuildState     string    `json:"build_state"`
	DurationSecs   int       `json:"duration_secs,omitempty"`
}

type Decision struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Context      string    `json:"context"`
	Decision     string    `json:"decision"`
	Alternatives string    `json:"alternatives,omitempty"`
	Rationale    string    `json:"rationale"`
	SessionID    int       `json:"session_id,omitempty"`
	Date         time.Time `json:"date"`
}

type SessionRecording struct {
	SessionID  int              `json:"session_id"`
	Agent      string           `json:"agent"`
	StartTime  time.Time        `json:"start_time"`
	EndTime    time.Time        `json:"end_time,omitempty"`
	ExitCode   int              `json:"exit_code"`
	Events     []RecordingEvent `json:"events"`
}

type RecordingEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "stdin", "stdout", "stderr", "meta"
	Data      string    `json:"data"`
}
