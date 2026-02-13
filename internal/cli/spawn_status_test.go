package cli

import "testing"

func TestIsTerminalSpawnStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "completed", status: "completed", want: true},
		{name: "failed", status: "failed", want: true},
		{name: "canceled", status: "canceled", want: true},
		{name: "cancelled", status: "cancelled", want: true},
		{name: "merged", status: "merged", want: true},
		{name: "rejected", status: "rejected", want: true},
		{name: "running", status: "running", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTerminalSpawnStatus(tt.status); got != tt.want {
				t.Fatalf("isTerminalSpawnStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestTreeTerminalStatusUsesSpawnTerminalStatus(t *testing.T) {
	for _, status := range []string{"completed", "failed", "canceled", "cancelled", "merged", "rejected"} {
		if !isTerminalStatus(status) {
			t.Fatalf("isTerminalStatus(%q) = false, want true", status)
		}
	}
	if isTerminalStatus("running") {
		t.Fatalf("isTerminalStatus(%q) = true, want false", "running")
	}
}
