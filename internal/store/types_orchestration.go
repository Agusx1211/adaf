package store

import "time"

// SpawnRecord tracks a sub-agent spawned by a parent agent.
type SpawnRecord struct {
	ID              int       `json:"id"`
	ParentTurnID    int       `json:"parent_session_id"`          // JSON key kept for compat
	ChildTurnID     int       `json:"child_session_id,omitempty"` // JSON key kept for compat
	ParentProfile   string    `json:"parent_profile"`
	ChildProfile    string    `json:"child_profile"`
	ChildRole       string    `json:"child_role,omitempty"`
	Task            string    `json:"task"`
	Branch          string    `json:"branch,omitempty"`
	WorktreePath    string    `json:"worktree_path,omitempty"`
	ReadOnly        bool      `json:"read_only,omitempty"`
	Status          string    `json:"status"` // "queued","running","awaiting_input","completed","failed","merged","rejected"
	Result          string    `json:"result,omitempty"`
	ExitCode        int       `json:"exit_code,omitempty"`
	Summary         string    `json:"summary,omitempty"` // child's final output for parent consumption
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at,omitzero"`
	MergeCommit     string    `json:"merge_commit,omitempty"`
	Handoff         bool      `json:"handoff,omitempty"`       // can be handed off to next loop step
	Speed           string    `json:"speed,omitempty"`         // speed rating from delegation profile
	HandedOffToTurn int       `json:"handed_off_to,omitempty"` // turn that inherited this spawn
}

// SpawnMessage is a message exchanged between parent and child agents.
type SpawnMessage struct {
	ID        int       `json:"id"`
	SpawnID   int       `json:"spawn_id"`
	Direction string    `json:"direction"` // "child_to_parent" or "parent_to_child"
	Type      string    `json:"type"`      // "ask", "reply", "message", "notify"
	Content   string    `json:"content"`
	ReplyToID int       `json:"reply_to_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ReadAt    time.Time `json:"read_at,omitempty"`
	Interrupt bool      `json:"interrupt,omitempty"` // interrupt child's current turn
}

// SupervisorNote is a message from a supervisor to a running turn.
type SupervisorNote struct {
	ID        int       `json:"id"`
	TurnID    int       `json:"session_id"` // JSON key kept for compat
	Author    string    `json:"author"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}
