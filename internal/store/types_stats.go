package store

import "time"

// ProfileStats holds aggregated statistics for a single profile.
type ProfileStats struct {
	ProfileName    string         `json:"profile_name"`
	TotalRuns      int            `json:"total_runs"`
	TotalTurns     int            `json:"total_turns"`
	TotalDuration  int            `json:"total_duration_secs"`
	TotalCostUSD   float64        `json:"total_cost_usd"`
	TotalInputTok  int            `json:"total_input_tokens"`
	TotalOutputTok int            `json:"total_output_tokens"`
	ToolCalls      map[string]int `json:"tool_calls"`     // tool_name -> count
	SpawnsCreated  int            `json:"spawns_created"` // times this profile spawned sub-agents
	SpawnedBy      map[string]int `json:"spawned_by"`     // parent_profile -> count
	TurnIDs        []int          `json:"session_ids"`    // all session IDs for this profile
	SuccessCount   int            `json:"success_count"`
	FailureCount   int            `json:"failure_count"`
	LastRunAt      time.Time      `json:"last_run_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// LoopStats holds aggregated statistics for a loop definition.
type LoopStats struct {
	LoopName      string         `json:"loop_name"`
	TotalCycles   int            `json:"total_cycles"`
	TotalRuns     int            `json:"total_runs"` // number of loop run instances
	TotalCostUSD  float64        `json:"total_cost_usd"`
	TotalDuration int            `json:"total_duration_secs"`
	StepStats     map[string]int `json:"step_stats"` // profile_name -> total runs in this loop
	TurnIDs       []int          `json:"session_ids"`
	LastRunAt     time.Time      `json:"last_run_at,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
