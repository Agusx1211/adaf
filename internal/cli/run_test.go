package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunTurnConfig(t *testing.T) {
	tests := []struct {
		name         string
		maxTurns     int
		wantTurns    int
		wantMaxCycle int
	}{
		{name: "unlimited", maxTurns: 0, wantTurns: 1, wantMaxCycle: 0},
		{name: "single turn", maxTurns: 1, wantTurns: 1, wantMaxCycle: 1},
		{name: "multi turn", maxTurns: 3, wantTurns: 3, wantMaxCycle: 1},
		{name: "negative treated as unlimited", maxTurns: -4, wantTurns: 1, wantMaxCycle: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTurns, gotCycles := runTurnConfig(tt.maxTurns)
			if gotTurns != tt.wantTurns {
				t.Fatalf("runTurnConfig(%d) turns = %d, want %d", tt.maxTurns, gotTurns, tt.wantTurns)
			}
			if gotCycles != tt.wantMaxCycle {
				t.Fatalf("runTurnConfig(%d) maxCycles = %d, want %d", tt.maxTurns, gotCycles, tt.wantMaxCycle)
			}
		})
	}
}

func TestBuildRunLoopDefinition(t *testing.T) {
	loopDef, maxCycles := buildRunLoopDefinition("codex", "run:codex", "fix bug", 4)
	if maxCycles != 1 {
		t.Fatalf("maxCycles = %d, want 1", maxCycles)
	}
	if loopDef.Name != "run:codex" {
		t.Fatalf("loop name = %q, want %q", loopDef.Name, "run:codex")
	}
	if len(loopDef.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(loopDef.Steps))
	}
	step := loopDef.Steps[0]
	if step.Profile != "run:codex" {
		t.Fatalf("step profile = %q, want %q", step.Profile, "run:codex")
	}
	if step.Turns != 4 {
		t.Fatalf("step turns = %d, want 4", step.Turns)
	}
	if step.Instructions != "fix bug" {
		t.Fatalf("instructions = %q, want %q", step.Instructions, "fix bug")
	}
}

func TestRunAgentRejectsAgentContext(t *testing.T) {
	t.Setenv("ADAF_AGENT", "1")
	err := runAgent(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("runAgent() error = nil, want agent-context rejection")
	}
	if !strings.Contains(err.Error(), "not available inside an agent context") {
		t.Fatalf("runAgent() error = %q, want context rejection message", err.Error())
	}
}

func TestLoopStartRejectsAgentContext(t *testing.T) {
	t.Setenv("ADAF_AGENT", "1")
	err := loopStart(&cobra.Command{}, []string{"dev-loop"})
	if err == nil {
		t.Fatal("loopStart() error = nil, want agent-context rejection")
	}
	if !strings.Contains(err.Error(), "not available inside an agent context") {
		t.Fatalf("loopStart() error = %q, want context rejection message", err.Error())
	}
}

func TestLoopStartForegroundFlagExists(t *testing.T) {
	flag := loopStartCmd.Flags().Lookup("foreground")
	if flag == nil {
		t.Fatal("loop start --foreground flag missing")
	}
}
