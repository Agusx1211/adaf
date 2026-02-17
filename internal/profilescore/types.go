package profilescore

import "time"

const (
	MinScore = 0.0
	MaxScore = 10.0
)

// FeedbackRecord captures one parent review of a completed spawn.
type FeedbackRecord struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
	ProjectID      string    `json:"project_id,omitempty"`
	ProjectName    string    `json:"project_name,omitempty"`
	SpawnID        int       `json:"spawn_id"`
	ParentTurnID   int       `json:"parent_turn_id,omitempty"`
	ChildTurnID    int       `json:"child_turn_id,omitempty"`
	ParentProfile  string    `json:"parent_profile,omitempty"`
	ParentRole     string    `json:"parent_role,omitempty"`
	ParentPosition string    `json:"parent_position,omitempty"`
	ChildProfile   string    `json:"child_profile"`
	ChildRole      string    `json:"child_role,omitempty"`
	ChildPosition  string    `json:"child_position,omitempty"`
	ChildStatus    string    `json:"child_status,omitempty"`
	ExitCode       int       `json:"exit_code,omitempty"`
	Task           string    `json:"task,omitempty"`
	DurationSecs   int       `json:"duration_secs,omitempty"`
	Difficulty     float64   `json:"difficulty"`
	Quality        float64   `json:"quality"`
	Notes          string    `json:"notes,omitempty"`
}

type dataset struct {
	Version  int              `json:"version"`
	Updated  time.Time        `json:"updated_at,omitempty"`
	Feedback []FeedbackRecord `json:"feedback"`
}

// ProfileCatalogEntry is optional static profile metadata for summaries.
type ProfileCatalogEntry struct {
	Name string `json:"name"`
	Cost string `json:"cost,omitempty"`
}

// BreakdownStats summarizes feedback grouped by one dimension.
type BreakdownStats struct {
	Name            string  `json:"name"`
	Count           int     `json:"count"`
	AvgQuality      float64 `json:"avg_quality"`
	AvgDifficulty   float64 `json:"avg_difficulty"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
	Score           float64 `json:"score"`
	SpeedScore      float64 `json:"speed_score"`
}

// TrendPoint is one daily aggregate used by trend charts.
type TrendPoint struct {
	Date            string  `json:"date"`
	Count           int     `json:"count"`
	AvgQuality      float64 `json:"avg_quality"`
	AvgDifficulty   float64 `json:"avg_difficulty"`
	AvgDurationSecs float64 `json:"avg_duration_secs"`
}

// ProfileSummary is the aggregated performance view for one profile.
type ProfileSummary struct {
	Profile          string           `json:"profile"`
	Cost             string           `json:"cost,omitempty"`
	TotalFeedback    int              `json:"total_feedback"`
	AvgQuality       float64          `json:"avg_quality"`
	AvgDifficulty    float64          `json:"avg_difficulty"`
	AvgDurationSecs  float64          `json:"avg_duration_secs"`
	Score            float64          `json:"score"`
	SpeedScore       float64          `json:"speed_score"`
	RoleBreakdown    []BreakdownStats `json:"role_breakdown"`
	ParentBreakdown  []BreakdownStats `json:"parent_breakdown"`
	Trend            []TrendPoint     `json:"trend"`
	Signals          []string         `json:"signals,omitempty"`
	RecentFeedback   []FeedbackRecord `json:"recent_feedback,omitempty"`
	HasEnoughSamples bool             `json:"has_enough_samples"`
}

// Dashboard is the top-level API payload for profile performance.
type Dashboard struct {
	GeneratedAt   time.Time        `json:"generated_at"`
	TotalFeedback int              `json:"total_feedback"`
	Profiles      []ProfileSummary `json:"profiles"`
}
