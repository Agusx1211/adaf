package looprun

import (
	"context"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
)

func TestRunDoesNotBlockOnFullEventChannel(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject() error = %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "bp-loop",
		Steps: []config.LoopStep{
			{
				Profile:      "p1",
				Turns:        1,
				Instructions: "Say hello",
			},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {
				Name: "generic",
				Path: "/bin/cat",
			},
		},
	}

	eventCh := make(chan any, 1) // Intentionally unconsumed.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, RunConfig{
			Store:     s,
			GlobalCfg: globalCfg,
			LoopDef:   loopDef,
			Project:   proj,
			AgentsCfg: agentsCfg,
			WorkDir:   proj.RepoPath,
			MaxCycles: 1,
		}, eventCh)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() blocked on backpressured event channel")
	}
}
