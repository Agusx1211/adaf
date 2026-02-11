package store

import "time"

// SpawnRecord tracks a sub-agent spawned by a parent agent.
type SpawnRecord struct {
	ID               int       `json:"id"`
	ParentSessionID  int       `json:"parent_session_id"`
	ChildSessionID   int       `json:"child_session_id,omitempty"`
	ParentProfile    string    `json:"parent_profile"`
	ChildProfile     string    `json:"child_profile"`
	Task             string    `json:"task"`
	Branch           string    `json:"branch,omitempty"`
	WorktreePath     string    `json:"worktree_path,omitempty"`
	ReadOnly         bool      `json:"read_only,omitempty"`
	Status           string    `json:"status"` // "queued","running","completed","failed","merged","rejected"
	Result           string    `json:"result,omitempty"`
	ExitCode         int       `json:"exit_code,omitempty"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at,omitzero"`
	MergeCommit      string    `json:"merge_commit,omitempty"`
}

// SupervisorNote is a message from a supervisor to a running session.
type SupervisorNote struct {
	ID        int       `json:"id"`
	SessionID int       `json:"session_id"`
	Author    string    `json:"author"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}
