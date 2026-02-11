package store

import "time"

// LoopRun tracks the state of a running loop.
type LoopRun struct {
	ID              int            `json:"id"`
	LoopName        string         `json:"loop_name"`
	Steps           []LoopRunStep  `json:"steps"`                      // snapshot of loop definition
	Status          string         `json:"status"`                     // "running", "stopped", "cancelled"
	Cycle           int            `json:"cycle"`                      // current cycle (0-indexed)
	StepIndex       int            `json:"step_index"`                 // current step in cycle
	StartedAt       time.Time      `json:"started_at"`
	StoppedAt       time.Time      `json:"stopped_at,omitempty"`
	SessionIDs      []int          `json:"session_ids"`                // all session IDs created
	StepLastSeenMsg map[int]int    `json:"step_last_seen_msg"`         // step_index -> last seen msg index
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
