package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyRuntimeViewVisibility(t *testing.T) {
	root := &cobra.Command{Use: "adaf", Short: "base short", Long: "base long"}
	run := &cobra.Command{Use: "run"}
	spawn := &cobra.Command{Use: "spawn"}
	loop := &cobra.Command{Use: "loop"}
	loopStart := &cobra.Command{Use: "start <name>"}
	loopStop := &cobra.Command{Use: "stop"}
	internal := &cobra.Command{Use: "_internal", Hidden: true}

	loop.AddCommand(loopStart, loopStop)
	root.AddCommand(run, spawn, loop, internal)

	applyRuntimeView(root, cliRuntimeViewUser)

	if run.Hidden {
		t.Fatal("run hidden in user view, want visible")
	}
	if !spawn.Hidden {
		t.Fatal("spawn visible in user view, want hidden")
	}
	if loopStart.Hidden {
		t.Fatal("loop start hidden in user view, want visible")
	}
	if !loopStop.Hidden {
		t.Fatal("loop stop visible in user view, want hidden")
	}
	if !internal.Hidden {
		t.Fatal("internal command visible in user view, want hidden")
	}

	applyRuntimeView(root, cliRuntimeViewAgent)

	if !run.Hidden {
		t.Fatal("run visible in agent view, want hidden")
	}
	if spawn.Hidden {
		t.Fatal("spawn hidden in agent view, want visible")
	}
	if !loopStart.Hidden {
		t.Fatal("loop start visible in agent view, want hidden")
	}
	if loopStop.Hidden {
		t.Fatal("loop stop hidden in agent view, want visible")
	}
	if !internal.Hidden {
		t.Fatal("internal command visible in agent view, want hidden")
	}
}

func TestApplyRuntimeViewRestoresRootText(t *testing.T) {
	root := &cobra.Command{
		Use:   "adaf",
		Short: "Autonomous Developer Agent Flow",
		Long:  "original long text",
	}

	applyRuntimeView(root, cliRuntimeViewAgent)
	if root.Short != "ADAF agent command interface" {
		t.Fatalf("root short in agent view = %q, want %q", root.Short, "ADAF agent command interface")
	}
	if root.Long == "original long text" {
		t.Fatal("root long in agent view did not change")
	}

	applyRuntimeView(root, cliRuntimeViewUser)
	if root.Short != "Autonomous Developer Agent Flow" {
		t.Fatalf("root short in user view = %q, want %q", root.Short, "Autonomous Developer Agent Flow")
	}
	if root.Long != "original long text" {
		t.Fatalf("root long in user view = %q, want %q", root.Long, "original long text")
	}
}

func TestEnforceCommandAccessForView(t *testing.T) {
	root := &cobra.Command{Use: "adaf"}
	run := &cobra.Command{Use: "run"}
	spawn := &cobra.Command{Use: "spawn"}
	loop := &cobra.Command{Use: "loop"}
	loopStart := &cobra.Command{Use: "start"}
	loopStop := &cobra.Command{Use: "stop"}
	config := &cobra.Command{Use: "config"}
	configAgents := &cobra.Command{Use: "agents"}
	configAgentsDetect := &cobra.Command{Use: "detect"}
	status := &cobra.Command{Use: "status"}

	loop.AddCommand(loopStart, loopStop)
	config.AddCommand(configAgents)
	configAgents.AddCommand(configAgentsDetect)
	root.AddCommand(run, spawn, loop, config, status)

	tests := []struct {
		name      string
		cmd       *cobra.Command
		view      cliRuntimeView
		wantError bool
		wantText  string
	}{
		{
			name:      "agent blocks user-only command",
			cmd:       run,
			view:      cliRuntimeViewAgent,
			wantError: true,
			wantText:  "not available inside an agent context",
		},
		{
			name:      "agent allows agent-only command",
			cmd:       spawn,
			view:      cliRuntimeViewAgent,
			wantError: false,
		},
		{
			name:      "user blocks agent-only command",
			cmd:       spawn,
			view:      cliRuntimeViewUser,
			wantError: true,
			wantText:  "only available inside an agent context",
		},
		{
			name:      "user allows user-only command",
			cmd:       run,
			view:      cliRuntimeViewUser,
			wantError: false,
		},
		{
			name:      "agent blocks nested user-only command",
			cmd:       loopStart,
			view:      cliRuntimeViewAgent,
			wantError: true,
			wantText:  "not available inside an agent context",
		},
		{
			name:      "user blocks nested agent-only command",
			cmd:       loopStop,
			view:      cliRuntimeViewUser,
			wantError: true,
			wantText:  "only available inside an agent context",
		},
		{
			name:      "both command always allowed",
			cmd:       status,
			view:      cliRuntimeViewAgent,
			wantError: false,
		},
		{
			name:      "agent blocks nested command under user-only parent",
			cmd:       configAgentsDetect,
			view:      cliRuntimeViewAgent,
			wantError: true,
			wantText:  "not available inside an agent context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enforceCommandAccessForView(tt.cmd, tt.view)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantText)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
