package store

import "time"

// LoopRun tracks the state of a running loop.
type LoopRun struct {
	ID              int           `json:"id"`
	LoopName        string        `json:"loop_name"`
	PlanID          string        `json:"plan_id,omitempty"`
	Steps           []LoopRunStep `json:"steps"`      // snapshot of loop definition
	Status          string        `json:"status"`     // "running", "stopped", "cancelled"
	Cycle           int           `json:"cycle"`      // current cycle (0-indexed)
	StepIndex       int           `json:"step_index"` // current step in cycle
	StartedAt       time.Time     `json:"started_at"`
	StoppedAt       time.Time     `json:"stopped_at,omitempty"`
	TurnIDs         []int         `json:"session_ids"`                // all turn IDs created (JSON key kept for compat)
	StepLastSeenMsg map[int]int   `json:"step_last_seen_msg"`         // step_index -> last seen msg index
	PendingHandoffs []HandoffInfo `json:"pending_handoffs,omitempty"` // spawns handed off to next step
}

// HandoffInfo describes a spawn handed off from a previous loop step.
type HandoffInfo struct {
	SpawnID int    `json:"spawn_id"`
	Profile string `json:"child_profile"`
	Task    string `json:"task"`
	Status  string `json:"status"`
	Speed   string `json:"speed,omitempty"`
	Branch  string `json:"branch,omitempty"`
}

// LoopRunStep is a snapshot of a loop step definition stored with the run.
type LoopRunStep struct {
	Profile      string `json:"profile"`
	Turns        int    `json:"turns,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	CanStop      bool   `json:"can_stop,omitempty"`
	CanMessage   bool   `json:"can_message,omitempty"`
	CanPushover  bool   `json:"can_pushover,omitempty"`
}

// LoopMessage is a message posted by a loop step for subsequent steps.
type LoopMessage struct {
	ID        int       `json:"id"`
	RunID     int       `json:"run_id"`
	StepIndex int       `json:"step_index"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
