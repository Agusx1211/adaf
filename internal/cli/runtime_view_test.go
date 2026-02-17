package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
)

func TestApplyRuntimeViewVisibility(t *testing.T) {
	root := &cobra.Command{Use: "adaf", Short: "base short", Long: "base long"}
	run := &cobra.Command{Use: "run"}
	spawn := &cobra.Command{Use: "spawn"}
	loop := &cobra.Command{Use: "loop"}
	loopStart := &cobra.Command{Use: "start <name>"}
	loopStop := &cobra.Command{Use: "stop"}
	loopCallSupervisor := &cobra.Command{Use: "call-supervisor <text>"}
	internal := &cobra.Command{Use: "_internal", Hidden: true}

	loop.AddCommand(loopStart, loopStop, loopCallSupervisor)
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
	if !loopCallSupervisor.Hidden {
		t.Fatal("loop call-supervisor visible in user view, want hidden")
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
	if loopCallSupervisor.Hidden {
		t.Fatal("loop call-supervisor hidden in agent view, want visible")
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

func TestEnforceCommandAccessForView_SpawnedSubAgentBlocksTurnCommands(t *testing.T) {
	root := &cobra.Command{Use: "adaf"}
	turn := &cobra.Command{Use: "turn"}
	finish := &cobra.Command{Use: "finish"}
	turn.AddCommand(finish)
	root.AddCommand(turn)

	t.Setenv("ADAF_PARENT_TURN", "42")

	err := enforceCommandAccessForView(turn, cliRuntimeViewAgent)
	if err == nil {
		t.Fatal("expected spawned sub-agent turn command to be blocked")
	}
	if !strings.Contains(err.Error(), "spawned sub-agents cannot manage turns") {
		t.Fatalf("error = %q, want spawned sub-agent turn block message", err.Error())
	}

	err = enforceCommandAccessForView(finish, cliRuntimeViewAgent)
	if err == nil {
		t.Fatal("expected spawned sub-agent turn finish command to be blocked")
	}
	if !strings.Contains(err.Error(), "spawned sub-agents cannot manage turns") {
		t.Fatalf("error = %q, want spawned sub-agent turn block message", err.Error())
	}
}

func TestApplyRuntimeView_HidesTurnForSpawnedSubAgent(t *testing.T) {
	root := &cobra.Command{Use: "adaf"}
	turn := &cobra.Command{Use: "turn"}
	finish := &cobra.Command{Use: "finish"}
	spawn := &cobra.Command{Use: "spawn"}
	turn.AddCommand(finish)
	root.AddCommand(turn, spawn)

	t.Setenv("ADAF_PARENT_TURN", "42")
	applyRuntimeView(root, cliRuntimeViewAgent)

	if !turn.Hidden {
		t.Fatal("turn should be hidden for spawned sub-agent agent view")
	}
	if !finish.Hidden {
		t.Fatal("turn finish should be hidden for spawned sub-agent agent view")
	}
	if spawn.Hidden {
		t.Fatal("spawn should remain visible for spawned sub-agent agent view")
	}
}

func TestEnforceCommandAccessForView_LoopRoleCommands(t *testing.T) {
	root := &cobra.Command{Use: "adaf"}
	loop := &cobra.Command{Use: "loop"}
	loopStop := &cobra.Command{Use: "stop"}
	loopMessage := &cobra.Command{Use: "message"}
	loopCallSupervisor := &cobra.Command{Use: "call-supervisor"}
	loop.AddCommand(loopStop, loopMessage, loopCallSupervisor)
	root.AddCommand(loop)

	projectDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ADAF_PROJECT_DIR", projectDir)
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "runtime-view", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	runWithSupervisor := &store.LoopRun{
		LoopName: "with-supervisor",
		Steps: []store.LoopRunStep{
			{Profile: "manager", Position: "manager"},
			{Profile: "lead", Position: "lead"},
			{Profile: "supervisor", Position: "supervisor"},
		},
		StepLastSeenMsg: map[int]int{},
	}
	if err := s.CreateLoopRun(runWithSupervisor); err != nil {
		t.Fatalf("CreateLoopRun(with supervisor) error = %v", err)
	}
	runWithoutSupervisor := &store.LoopRun{
		LoopName: "without-supervisor",
		Steps: []store.LoopRunStep{
			{Profile: "manager", Position: "manager"},
			{Profile: "lead", Position: "lead"},
		},
		StepLastSeenMsg: map[int]int{},
	}
	if err := s.CreateLoopRun(runWithoutSupervisor); err != nil {
		t.Fatalf("CreateLoopRun(without supervisor) error = %v", err)
	}

	t.Run("manager cannot stop", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithSupervisor.ID))
		t.Setenv("ADAF_POSITION", "manager")
		err := enforceCommandAccessForView(loopStop, cliRuntimeViewAgent)
		if err == nil || !strings.Contains(err.Error(), "supervisor-only") {
			t.Fatalf("error = %v, want supervisor-only", err)
		}
	})

	t.Run("manager can call supervisor", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithSupervisor.ID))
		t.Setenv("ADAF_POSITION", "manager")
		if err := enforceCommandAccessForView(loopCallSupervisor, cliRuntimeViewAgent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("supervisor can stop", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithSupervisor.ID))
		t.Setenv("ADAF_POSITION", "supervisor")
		if err := enforceCommandAccessForView(loopStop, cliRuntimeViewAgent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("supervisor cannot call supervisor", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithSupervisor.ID))
		t.Setenv("ADAF_POSITION", "supervisor")
		err := enforceCommandAccessForView(loopCallSupervisor, cliRuntimeViewAgent)
		if err == nil || !strings.Contains(err.Error(), "manager-only") {
			t.Fatalf("error = %v, want manager-only", err)
		}
	})

	t.Run("manager gets hint on message", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithSupervisor.ID))
		t.Setenv("ADAF_POSITION", "manager")
		err := enforceCommandAccessForView(loopMessage, cliRuntimeViewAgent)
		if err == nil || !strings.Contains(err.Error(), "call-supervisor") {
			t.Fatalf("error = %v, want call-supervisor hint", err)
		}
	})

	t.Run("manager call-supervisor unavailable when no supervisor step exists", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithoutSupervisor.ID))
		t.Setenv("ADAF_POSITION", "manager")
		err := enforceCommandAccessForView(loopCallSupervisor, cliRuntimeViewAgent)
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("error = %v, want unavailable", err)
		}
	})

	t.Run("manager message omits call-supervisor hint when loop has no supervisor", func(t *testing.T) {
		t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", runWithoutSupervisor.ID))
		t.Setenv("ADAF_POSITION", "manager")
		err := enforceCommandAccessForView(loopMessage, cliRuntimeViewAgent)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if strings.Contains(err.Error(), "call-supervisor") {
			t.Fatalf("error = %q, want no call-supervisor hint", err.Error())
		}
	})
}

func TestApplyRuntimeView_HidesCallSupervisorWhenLoopHasNoSupervisor(t *testing.T) {
	root := &cobra.Command{Use: "adaf"}
	loop := &cobra.Command{Use: "loop"}
	loopCallSupervisor := &cobra.Command{Use: "call-supervisor"}
	loop.AddCommand(loopCallSupervisor)
	root.AddCommand(loop)

	projectDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ADAF_PROJECT_DIR", projectDir)
	t.Setenv("ADAF_POSITION", "manager")

	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "runtime-view-hidden", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	run := &store.LoopRun{
		LoopName: "without-supervisor",
		Steps: []store.LoopRunStep{
			{Profile: "manager", Position: "manager"},
			{Profile: "lead", Position: "lead"},
		},
		StepLastSeenMsg: map[int]int{},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun() error = %v", err)
	}
	t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", run.ID))

	applyRuntimeView(root, cliRuntimeViewAgent)
	if !loopCallSupervisor.Hidden {
		t.Fatal("loop call-supervisor should be hidden when loop has no supervisor step")
	}
}
