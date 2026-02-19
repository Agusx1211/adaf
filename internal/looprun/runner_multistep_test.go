package looprun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	loopctrl "github.com/agusx1211/adaf/internal/loop"
	"github.com/agusx1211/adaf/internal/store"
)

func TestRun_MultipleStepsExecuteInOrder(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	// Create a dummy script that just exits 0.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "ok.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loopDef := &config.LoopDef{
		Name: "multi-step-test",
		Steps: []config.LoopStep{
			{Profile: "step1-profile", Turns: 1, Instructions: "step 1", ManualPrompt: "manual step 1"},
			{Profile: "step2-profile", Turns: 1, Instructions: "step 2"},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "step1-profile", Agent: "generic"},
			{Name: "step2-profile", Agent: "generic"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: scriptPath},
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
		t.Fatalf("Run() error = %v", err)
	}

	// Verify both steps executed by checking turn records.
	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}

	// We expect 2 turns, one for each step.
	if len(turns) != 2 {
		t.Fatalf("len(turns) = %d, want 2", len(turns))
	}

	// Check that they are from different profiles and in order.
	if turns[0].ProfileName != "step1-profile" {
		t.Errorf("turns[0].ProfileName = %q, want %q", turns[0].ProfileName, "step1-profile")
	}
	if turns[1].ProfileName != "step2-profile" {
		t.Errorf("turns[1].ProfileName = %q, want %q", turns[1].ProfileName, "step2-profile")
	}

	runs, err := s.ListLoopRuns()
	if err != nil {
		t.Fatalf("ListLoopRuns: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one loop run record")
	}
	if len(runs[0].Steps) == 0 {
		t.Fatal("expected loop run step snapshots")
	}
	if runs[0].Steps[0].ManualPrompt != "manual step 1" {
		t.Fatalf("run step manual_prompt = %q, want %q", runs[0].Steps[0].ManualPrompt, "manual step 1")
	}
}

func TestRun_MultipleCyclesExecuteAllSteps(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	// Create a dummy script that just exits 0.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "ok.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loopDef := &config.LoopDef{
		Name: "multi-cycle-test",
		Steps: []config.LoopStep{
			{Profile: "p1", Turns: 1},
			{Profile: "p2", Turns: 1},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
			{Name: "p2", Agent: "generic"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: scriptPath},
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
		MaxCycles: 2,
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// We expect 2 cycles * 2 steps = 4 turns.
	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}

	if len(turns) != 4 {
		t.Fatalf("len(turns) = %d, want 4", len(turns))
	}
}

func TestWaitForAnySessionSpawns_PollDetectsCompletion(t *testing.T) {
	s := newLooprunTestStore(t)
	parentTurnID := 100

	rec := createLooprunSpawn(t, s, parentTurnID, "running")

	go func() {
		time.Sleep(300 * time.Millisecond)
		rec.Status = "completed"
		_ = s.UpdateSpawn(rec)
	}()

	results, morePending := waitForAnySessionSpawns(t.Context(), s, parentTurnID, nil)
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].SpawnID != rec.ID {
		t.Fatalf("spawn ID = %d, want %d", results[0].SpawnID, rec.ID)
	}
	if morePending {
		t.Fatal("morePending = true, want false")
	}
}

func TestWaitForAnySessionSpawns_ContextCancellation(t *testing.T) {
	s := newLooprunTestStore(t)
	parentTurnID := 200

	_ = createLooprunSpawn(t, s, parentTurnID, "running")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	results, morePending := waitForAnySessionSpawns(ctx, s, parentTurnID, nil)
	if len(results) != 0 {
		t.Fatalf("results = %d, want 0", len(results))
	}
	if morePending {
		t.Fatal("morePending = true, want false")
	}
}

func TestWaitForAnySessionSpawns_MultiplePendingPartialCompletion(t *testing.T) {
	s := newLooprunTestStore(t)
	parentTurnID := 300

	doneOne := createLooprunSpawn(t, s, parentTurnID, "completed")
	runningOne := createLooprunSpawn(t, s, parentTurnID, "running")
	runningTwo := createLooprunSpawn(t, s, parentTurnID, "running")

	// First call: returns already completed.
	first, morePending := waitForAnySessionSpawns(t.Context(), s, parentTurnID, nil)
	if len(first) != 1 {
		t.Fatalf("first results = %d, want 1", len(first))
	}
	if first[0].SpawnID != doneOne.ID {
		t.Fatalf("first spawn ID = %d, want %d", first[0].SpawnID, doneOne.ID)
	}
	if !morePending {
		t.Fatal("first morePending = false, want true")
	}

	alreadySeen := map[int]struct{}{doneOne.ID: {}}

	// Second call: wait for runningOne to complete.
	go func() {
		time.Sleep(300 * time.Millisecond)
		r1, _ := s.GetSpawn(runningOne.ID)
		r1.Status = "completed"
		_ = s.UpdateSpawn(r1)
	}()

	second, morePending := waitForAnySessionSpawns(t.Context(), s, parentTurnID, alreadySeen)
	if len(second) != 1 {
		t.Fatalf("second results = %d, want 1", len(second))
	}
	if second[0].SpawnID != runningOne.ID {
		t.Fatalf("second spawn ID = %d, want %d", second[0].SpawnID, runningOne.ID)
	}
	if !morePending {
		t.Fatal("second morePending = false, want true (runningTwo is still pending)")
	}
	_ = runningTwo
}

func TestCollectHandoffs_ReparentsAcrossSteps(t *testing.T) {
	s := newLooprunTestStore(t)

	// Step 1 Turn.
	t1Rec := &store.Turn{
		ProfileName: "p1",
	}
	if err := s.CreateTurn(t1Rec); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	// Create running handoff from step 1.
	h1 := &store.SpawnRecord{
		ParentTurnID: t1Rec.ID,
		ChildProfile: "worker",
		Status:       "running",
		Handoff:      true,
	}
	if err := s.CreateSpawn(h1); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	// Step 2 Turn.
	t2Rec := &store.Turn{
		ProfileName: "p2",
	}
	if err := s.CreateTurn(t2Rec); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	// Collect handoffs from step 1.
	handoffs := collectHandoffs(s, []int{t1Rec.ID})
	if len(handoffs) != 1 {
		t.Fatalf("handoffs = %d, want 1", len(handoffs))
	}

	// Reparent to step 2.
	reparentHandoffs(s, handoffs, t2Rec.ID)

	// Verify h1 now has t2Rec.ID as parent.
	got, err := s.GetSpawn(h1.ID)
	if err != nil {
		t.Fatalf("GetSpawn: %v", err)
	}
	if got.ParentTurnID != t2Rec.ID {
		t.Errorf("got.ParentTurnID = %d, want %d", got.ParentTurnID, t2Rec.ID)
	}
}

func TestRun_CanStopSignalExitsLoop(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	// Dummy script that takes some time.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "ok.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 0.1\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loopDef := &config.LoopDef{
		Name: "stop-test",
		Steps: []config.LoopStep{
			{Profile: "p1", Position: config.PositionSupervisor, Turns: 1},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: scriptPath},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// In a goroutine, wait for the run to be created, then set stop flag.
	go func() {
		var runID int
		for i := 0; i < 100; i++ {
			runs, _ := s.ListLoopRuns()
			if len(runs) > 0 {
				runID = runs[0].ID
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if runID == 0 {
			return
		}
		// Signal stop during the first or second cycle.
		time.Sleep(50 * time.Millisecond)
		_ = s.SignalLoopStop(runID)
	}()

	err = Run(ctx, RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: agentsCfg,
		WorkDir:   proj.RepoPath,
		MaxCycles: 5, // Should stop before reaching 5.
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify we didn't run too many cycles.
	runs, _ := s.ListLoopRuns()
	if len(runs) == 0 {
		t.Fatal("no loop run found")
	}
	if runs[0].Cycle >= 4 {
		t.Errorf("runs[0].Cycle = %d, want < 4", runs[0].Cycle)
	}
}

func TestRun_WindDownSignalStopsAfterCurrentTurn(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	// Keep the turn alive long enough to set wind-down while it is running.
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "slow-turn.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 0.2\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loopDef := &config.LoopDef{
		Name: "wind-down-test",
		Steps: []config.LoopStep{
			{Profile: "p1", Position: config.PositionLead, Turns: 3},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: scriptPath},
		},
	}

	// Wait until the first turn exists, then request wind-down.
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			runs, _ := s.ListLoopRuns()
			turns, _ := s.ListTurns()
			if len(runs) > 0 && len(turns) > 0 {
				_ = s.SignalLoopWindDown(runs[0].ID)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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
		t.Fatalf("Run() error = %v", err)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("len(turns) = %d, want 1 (finish current turn, skip next)", len(turns))
	}
}

func TestRun_CallSupervisorFastForwardSkipsIntermediateSteps(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "slow.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 2\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loopDef := &config.LoopDef{
		Name: "call-supervisor-skip",
		Steps: []config.LoopStep{
			{Profile: "mgr", Position: config.PositionManager, Team: "workers", Turns: 1},
			{Profile: "lead-1", Position: config.PositionLead, Turns: 1},
			{Profile: "lead-2", Position: config.PositionLead, Turns: 1},
			{Profile: "sup", Position: config.PositionSupervisor, Turns: 1},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "mgr", Agent: "generic"},
			{Name: "lead-1", Agent: "generic"},
			{Name: "lead-2", Agent: "generic"},
			{Name: "sup", Agent: "generic"},
		},
		Teams: []config.Team{
			{
				Name: "workers",
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{Name: "lead-1", Position: config.PositionWorker, Role: config.RoleDeveloper},
					},
				},
			},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: scriptPath},
		},
	}

	go func() {
		var runID int
		var turnID int
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			runs, _ := s.ListLoopRuns()
			if len(runs) > 0 {
				runID = runs[0].ID
			}
			turns, _ := s.ListTurns()
			if len(turns) > 0 {
				turnID = turns[0].ID
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if runID == 0 || turnID == 0 {
			return
		}
		_ = s.SignalLoopCallSupervisor(runID, 0, 3, "need supervisor")
		_ = s.SignalInterrupt(turnID, loopctrl.InterruptMessageCallSupervisor)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		t.Fatalf("Run() error = %v", err)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("len(turns) = %d, want 2", len(turns))
	}
	if turns[0].ProfileName != "mgr" {
		t.Fatalf("turns[0].ProfileName = %q, want %q", turns[0].ProfileName, "mgr")
	}
	if turns[1].ProfileName != "sup" {
		t.Fatalf("turns[1].ProfileName = %q, want %q", turns[1].ProfileName, "sup")
	}
}

func TestRun_CallSupervisorFastForwardWrapsToNextCycleSupervisor(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "slow.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 2\nexit 0\n"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loopDef := &config.LoopDef{
		Name: "call-supervisor-wrap",
		Steps: []config.LoopStep{
			{Profile: "sup", Position: config.PositionSupervisor, Turns: 1},
			{Profile: "lead", Position: config.PositionLead, Turns: 1},
			{Profile: "mgr", Position: config.PositionManager, Team: "workers", Turns: 1},
			{Profile: "tail", Position: config.PositionLead, Turns: 1},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "sup", Agent: "generic"},
			{Name: "lead", Agent: "generic"},
			{Name: "mgr", Agent: "generic"},
			{Name: "tail", Agent: "generic"},
		},
		Teams: []config.Team{
			{
				Name: "workers",
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{Name: "lead", Position: config.PositionWorker, Role: config.RoleDeveloper},
					},
				},
			},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: scriptPath},
		},
	}

	go func() {
		var runID int
		var managerTurnID int
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			runs, _ := s.ListLoopRuns()
			if len(runs) > 0 {
				runID = runs[0].ID
			}
			turns, _ := s.ListTurns()
			for _, turn := range turns {
				if turn.ProfileName == "mgr" {
					managerTurnID = turn.ID
					break
				}
			}
			if runID > 0 && managerTurnID > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if runID == 0 || managerTurnID == 0 {
			return
		}
		_ = s.SignalLoopCallSupervisor(runID, 2, 0, "wrap to supervisor")
		_ = s.SignalInterrupt(managerTurnID, loopctrl.InterruptMessageCallSupervisor)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
	defer cancel()

	err = Run(ctx, RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: agentsCfg,
		WorkDir:   proj.RepoPath,
		MaxCycles: 2,
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns: %v", err)
	}
	if len(turns) != 7 {
		t.Fatalf("len(turns) = %d, want 7", len(turns))
	}

	if turns[0].ProfileName != "sup" || turns[1].ProfileName != "lead" || turns[2].ProfileName != "mgr" {
		t.Fatalf("unexpected pre-jump order: [%s %s %s]", turns[0].ProfileName, turns[1].ProfileName, turns[2].ProfileName)
	}
	if turns[3].ProfileName != "sup" {
		t.Fatalf("turns[3].ProfileName = %q, want %q (next-cycle supervisor after manager)", turns[3].ProfileName, "sup")
	}
}
