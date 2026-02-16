package store

import "testing"

func TestIsTerminalSpawnStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "running", status: SpawnStatusRunning, want: false},
		{name: "awaiting_input", status: SpawnStatusAwaitingInput, want: false},
		{name: "completed", status: SpawnStatusCompleted, want: true},
		{name: "failed", status: SpawnStatusFailed, want: true},
		{name: "canceled", status: SpawnStatusCanceled, want: true},
		{name: "cancelled", status: SpawnStatusCancelled, want: true},
		{name: "merged", status: SpawnStatusMerged, want: true},
		{name: "rejected", status: SpawnStatusRejected, want: true},
		{name: "mixed_case_and_space", status: "  Completed  ", want: true},
		{name: "unknown", status: "paused", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminalSpawnStatus(tt.status); got != tt.want {
				t.Fatalf("IsTerminalSpawnStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
