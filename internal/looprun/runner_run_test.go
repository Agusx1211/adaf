package looprun

import (
	"context"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
)

func TestRun_ProfileNotFound(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{}, // no profiles
	}
	loopDef := &config.LoopDef{
		Name: "test-loop",
		Steps: []config.LoopStep{
			{Profile: "missing-profile", Turns: 1, Instructions: "test"},
		},
	}

	err = Run(context.Background(), RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: &agent.AgentsConfig{Agents: map[string]agent.AgentRecord{}},
		WorkDir:   proj.RepoPath,
		MaxCycles: 1,
	}, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want profile not found error")
	}
	if got := err.Error(); got == "" {
		t.Fatal("error message is empty")
	}
}

func TestRun_AgentNotFound(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "nonexistent-agent-xyz"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "test-loop",
		Steps: []config.LoopStep{
			{Profile: "p1", Turns: 1},
		},
	}

	err = Run(context.Background(), RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: &agent.AgentsConfig{Agents: map[string]agent.AgentRecord{}},
		WorkDir:   proj.RepoPath,
		MaxCycles: 1,
	}, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want agent not found error")
	}
}

func TestRun_ContextCancellationReturnsError(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "cancel-test",
		Steps: []config.LoopStep{
			{Profile: "p1", Turns: 1, Instructions: "test"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/cat"},
		},
	}

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = Run(ctx, RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: agentsCfg,
		WorkDir:   proj.RepoPath,
		MaxCycles: 1,
	}, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want context cancellation error")
	}
	if err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRun_MaxCyclesLimitsExecution(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "cycle-test",
		Steps: []config.LoopStep{
			{Profile: "p1", Turns: 1, Instructions: "echo hello"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/cat"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = Run(ctx, RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: agentsCfg,
		WorkDir:   proj.RepoPath,
		MaxCycles: 1,
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
}

func TestRun_NilEventChannelDoesNotPanic(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "nil-ch-test",
		Steps: []config.LoopStep{
			{Profile: "p1", Turns: 1, Instructions: "test"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/cat"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Nil event channel should not cause a panic.
	err = Run(ctx, RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: agentsCfg,
		WorkDir:   proj.RepoPath,
		MaxCycles: 1,
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
}
