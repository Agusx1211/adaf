package stats

import (
	"time"

	"github.com/agusx1211/adaf/internal/store"
)

// UpdateProfileStats extracts metrics from a completed turn and
// merges them into the profile's aggregate stats.
func UpdateProfileStats(st *store.Store, profileName string, turnID int) error {
	if profileName == "" {
		return nil
	}

	metrics, err := ExtractFromRecording(st, turnID)
	if err != nil {
		// Recording may not exist yet or be unreadable; non-fatal.
		return nil
	}

	stats, err := st.GetProfileStats(profileName)
	if err != nil {
		return err
	}

	stats.TotalRuns++
	stats.TotalTurns += metrics.NumTurns
	stats.TotalDuration += metrics.DurationSecs
	stats.TotalCostUSD += metrics.TotalCostUSD
	stats.TotalInputTok += metrics.InputTokens
	stats.TotalOutputTok += metrics.OutputTokens

	for tool, count := range metrics.ToolCalls {
		stats.ToolCalls[tool] += count
	}

	if metrics.Success {
		stats.SuccessCount++
	} else {
		stats.FailureCount++
	}

	stats.TurnIDs = append(stats.TurnIDs, turnID)
	stats.LastRunAt = time.Now().UTC()
	stats.UpdatedAt = time.Now().UTC()

	// Update spawn tracking.
	updateSpawnStats(st, stats, profileName)

	return st.SaveProfileStats(stats)
}

// updateSpawnStats queries spawn records and updates spawn-related stats.
func updateSpawnStats(st *store.Store, stats *store.ProfileStats, profileName string) {
	spawns, err := st.ListSpawns()
	if err != nil {
		return
	}

	spawnsCreated := 0
	spawnedBy := make(map[string]int)

	for _, sp := range spawns {
		if sp.ParentProfile == profileName {
			spawnsCreated++
		}
		if sp.ChildProfile == profileName && sp.ParentProfile != "" {
			spawnedBy[sp.ParentProfile]++
		}
	}

	stats.SpawnsCreated = spawnsCreated
	stats.SpawnedBy = spawnedBy
}

// UpdateLoopStats updates loop-level stats after a loop run completes.
func UpdateLoopStats(st *store.Store, loopName string, run *store.LoopRun) error {
	if loopName == "" {
		return nil
	}

	stats, err := st.GetLoopStats(loopName)
	if err != nil {
		return err
	}

	stats.TotalRuns++
	stats.TotalCycles += run.Cycle + 1 // Cycle is 0-indexed

	// Aggregate cost and duration from all turns in this run.
	for _, sid := range run.TurnIDs {
		metrics, err := ExtractFromRecording(st, sid)
		if err != nil {
			continue
		}
		stats.TotalCostUSD += metrics.TotalCostUSD
		stats.TotalDuration += metrics.DurationSecs
	}

	// Count runs per step profile.
	for _, step := range run.Steps {
		stats.StepStats[step.Profile]++
	}

	stats.TurnIDs = append(stats.TurnIDs, run.TurnIDs...)
	stats.LastRunAt = time.Now().UTC()
	stats.UpdatedAt = time.Now().UTC()

	return st.SaveLoopStats(stats)
}
