package store

import "strings"

const (
	SpawnStatusRunning       = "running"
	SpawnStatusAwaitingInput = "awaiting_input"
	SpawnStatusCompleted     = "completed"
	SpawnStatusFailed        = "failed"
	SpawnStatusCanceled      = "canceled"
	SpawnStatusCancelled     = "cancelled"
	SpawnStatusMerged        = "merged"
	SpawnStatusRejected      = "rejected"
)

// IsTerminalSpawnStatus reports whether status represents a completed spawn
// lifecycle state.
func IsTerminalSpawnStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case SpawnStatusCompleted, SpawnStatusFailed, SpawnStatusCanceled, SpawnStatusCancelled, SpawnStatusMerged, SpawnStatusRejected:
		return true
	default:
		return false
	}
}
