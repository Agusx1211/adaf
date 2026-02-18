package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

type stubAgent struct {
	runs []agent.Config
}

type errStubAgent struct {
	err error
}

type waitResumeStubAgent struct {
	store *store.Store
	runs  []agent.Config
}

type waitImmediateStopStubAgent struct {
	store       *store.Store
	runs        []agent.Config
	firstTurnID int
}

type waitSeenTrackingStubAgent struct {
	store     *store.Store
	runs      []agent.Config
	turnOrder []int
	turnRuns  map[int]int
}

type waitResumeSessionCarryStubAgent struct {
	store *store.Store
	runs  []agent.Config
}

type implicitSpawnWaitStubAgent struct {
	store   *store.Store
	runs    []agent.Config
	spawnID int
}

type diaryWritingStubAgent struct {
	store *store.Store
}

func (a *stubAgent) Name() string { return "stub" }

func (a *stubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	cloned := cfg
	cloned.Env = make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		cloned.Env[k] = v
	}
	a.runs = append(a.runs, cloned)
	return &agent.Result{ExitCode: 0, Duration: time.Millisecond}, nil
}

func (a *errStubAgent) Name() string { return "stub" }

func (a *errStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	return nil, a.err
}

func (a *waitResumeStubAgent) Name() string { return "stub" }

func (a *waitResumeStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	cloned := cfg
	cloned.Env = make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		cloned.Env[k] = v
	}
	a.runs = append(a.runs, cloned)

	if len(a.runs) == 1 {
		if err := a.store.SignalWait(cfg.TurnID); err != nil {
			return nil, err
		}
	}
	return &agent.Result{
		ExitCode:       0,
		Duration:       time.Millisecond,
		AgentSessionID: "sess-123",
	}, nil
}

func (a *waitImmediateStopStubAgent) Name() string { return "stub" }

func (a *waitImmediateStopStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	cloned := cfg
	cloned.Env = make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		cloned.Env[k] = v
	}
	a.runs = append(a.runs, cloned)

	if len(a.runs) == 1 {
		a.firstTurnID = cfg.TurnID
		if err := a.store.SignalWait(cfg.TurnID); err != nil {
			return nil, err
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &agent.Result{
		ExitCode:       0,
		Duration:       time.Millisecond,
		AgentSessionID: "sess-456",
	}, nil
}

func (a *waitSeenTrackingStubAgent) Name() string { return "stub" }

func (a *waitSeenTrackingStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	cloned := cfg
	cloned.Env = make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		cloned.Env[k] = v
	}
	a.runs = append(a.runs, cloned)

	if a.turnRuns == nil {
		a.turnRuns = make(map[int]int)
	}
	if _, ok := a.turnRuns[cfg.TurnID]; !ok {
		a.turnOrder = append(a.turnOrder, cfg.TurnID)
	}
	a.turnRuns[cfg.TurnID]++
	runInTurn := a.turnRuns[cfg.TurnID]

	shouldWait := false
	if len(a.turnOrder) > 0 && cfg.TurnID == a.turnOrder[0] && runInTurn <= 2 {
		shouldWait = true
	}
	if len(a.turnOrder) > 1 && cfg.TurnID == a.turnOrder[1] && runInTurn <= 1 {
		shouldWait = true
	}
	if shouldWait {
		if err := a.store.SignalWait(cfg.TurnID); err != nil {
			return nil, err
		}
	}

	return &agent.Result{
		ExitCode:       0,
		Duration:       time.Millisecond,
		AgentSessionID: fmt.Sprintf("sess-%d-%d", cfg.TurnID, runInTurn),
	}, nil
}

func (a *waitResumeSessionCarryStubAgent) Name() string { return "stub" }

func (a *waitResumeSessionCarryStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	cloned := cfg
	cloned.Env = make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		cloned.Env[k] = v
	}
	a.runs = append(a.runs, cloned)

	switch len(a.runs) {
	case 1:
		if err := a.store.SignalWait(cfg.TurnID); err != nil {
			return nil, err
		}
		return &agent.Result{
			ExitCode:       0,
			Duration:       time.Millisecond,
			AgentSessionID: "sess-keep",
		}, nil
	case 2:
		if err := a.store.SignalWait(cfg.TurnID); err != nil {
			return nil, err
		}
		<-ctx.Done()
		return nil, ctx.Err()
	default:
		return &agent.Result{
			ExitCode:       0,
			Duration:       time.Millisecond,
			AgentSessionID: "sess-final",
		}, nil
	}
}

func (a *implicitSpawnWaitStubAgent) Name() string { return "stub" }

func (a *implicitSpawnWaitStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	cloned := cfg
	cloned.Env = make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		cloned.Env[k] = v
	}
	a.runs = append(a.runs, cloned)

	if len(a.runs) == 1 {
		rec := &store.SpawnRecord{
			ParentTurnID:  cfg.TurnID,
			ParentProfile: "parent",
			ChildProfile:  "child",
			Task:          "test wait inference",
			Status:        store.SpawnStatusRunning,
		}
		if err := a.store.CreateSpawn(rec); err != nil {
			return nil, err
		}
		a.spawnID = rec.ID
	}

	return &agent.Result{
		ExitCode:       0,
		Duration:       time.Millisecond,
		AgentSessionID: "sess-implicit",
	}, nil
}

func (a *diaryWritingStubAgent) Name() string { return "stub" }

func (a *diaryWritingStubAgent) Run(ctx context.Context, cfg agent.Config, recorder *recording.Recorder) (*agent.Result, error) {
	turn, err := a.store.GetTurn(cfg.TurnID)
	if err != nil {
		return nil, err
	}
	turn.WhatWasBuilt = "agent-built"
	turn.KeyDecisions = "agent-decisions"
	turn.CurrentState = "agent-state"
	turn.KnownIssues = "agent-issues"
	turn.NextSteps = "agent-next"
	if err := a.store.UpdateTurn(turn); err != nil {
		return nil, err
	}
	return &agent.Result{ExitCode: 0, Duration: time.Millisecond}, nil
}

func TestLoopPromptFuncReceivesTurnID(t *testing.T) {
	tests := []struct {
		name     string
		maxTurns int
	}{
		{
			name:     "single turn",
			maxTurns: 1,
		},
		{
			name:     "multi turn",
			maxTurns: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			s, err := store.New(dir)
			if err != nil {
				t.Fatalf("store.New() error = %v", err)
			}
			if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
				t.Fatalf("store.Init() error = %v", err)
			}

			a := &stubAgent{}
			var calls []int

			l := &Loop{
				Store: s,
				Agent: a,
				Config: agent.Config{
					Prompt:   "base",
					MaxTurns: tt.maxTurns,
				},
				PromptFunc: func(turnID int) string {
					calls = append(calls, turnID)
					return fmt.Sprintf("turn=%d", turnID)
				},
			}

			if err := l.Run(context.Background()); err != nil {
				t.Fatalf("Loop.Run() error = %v", err)
			}

			if len(a.runs) != tt.maxTurns {
				t.Fatalf("agent runs = %d, want %d", len(a.runs), tt.maxTurns)
			}
			if len(calls) != tt.maxTurns {
				t.Fatalf("prompt calls = %d, want %d", len(calls), tt.maxTurns)
			}

			for i := 0; i < tt.maxTurns; i++ {
				wantTurnID := i + 1
				if calls[i] != wantTurnID {
					t.Fatalf("call[%d] turn id = %d, want %d", i, calls[i], wantTurnID)
				}

				if got := a.runs[i].Env["ADAF_AGENT"]; got != "1" {
					t.Fatalf("run[%d] ADAF_AGENT = %q, want %q", i, got, "1")
				}
				if got := a.runs[i].Env["ADAF_TURN_ID"]; got != fmt.Sprintf("%d", wantTurnID) {
					t.Fatalf("run[%d] ADAF_TURN_ID = %q, want %d", i, got, wantTurnID)
				}
				wantPrompt := fmt.Sprintf("turn=%d", wantTurnID)
				if got := a.runs[i].Prompt; got != wantPrompt {
					t.Fatalf("run[%d] prompt = %q, want %q", i, got, wantPrompt)
				}
			}
		})
	}
}

func TestLoopRunReturnsContextCanceledAndMarksCancelled(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	l := &Loop{
		Store: s,
		Agent: &errStubAgent{err: fmt.Errorf("wrapped: %w", context.Canceled)},
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
	}

	err = l.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Loop.Run() error = %v, want context canceled", err)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns() error = %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns count = %d, want 1", len(turns))
	}
	if turns[0].BuildState != "cancelled" {
		t.Fatalf("build state = %q, want %q", turns[0].BuildState, "cancelled")
	}
}

func TestLoopRunPreservesAgentAuthoredTurnDiaryFields(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	l := &Loop{
		Store: s,
		Agent: &diaryWritingStubAgent{store: s},
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
	}

	if err := l.Run(context.Background()); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns() error = %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns count = %d, want 1", len(turns))
	}
	if turns[0].WhatWasBuilt != "agent-built" {
		t.Fatalf("WhatWasBuilt = %q, want %q", turns[0].WhatWasBuilt, "agent-built")
	}
	if turns[0].KeyDecisions != "agent-decisions" {
		t.Fatalf("KeyDecisions = %q, want %q", turns[0].KeyDecisions, "agent-decisions")
	}
	if turns[0].CurrentState != "agent-state" {
		t.Fatalf("CurrentState = %q, want %q", turns[0].CurrentState, "agent-state")
	}
	if turns[0].KnownIssues != "agent-issues" {
		t.Fatalf("KnownIssues = %q, want %q", turns[0].KnownIssues, "agent-issues")
	}
	if turns[0].NextSteps != "agent-next" {
		t.Fatalf("NextSteps = %q, want %q", turns[0].NextSteps, "agent-next")
	}
}

func TestLoopRunFinalizesTurnAndRejectsFutureUpdates(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	l := &Loop{
		Store: s,
		Agent: &stubAgent{},
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
	}

	if err := l.Run(context.Background()); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns() error = %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns count = %d, want 1", len(turns))
	}
	if turns[0].FinalizedAt.IsZero() {
		t.Fatal("FinalizedAt is zero, want non-zero after turn end")
	}

	turns[0].WhatWasBuilt = "late-write"
	err = s.UpdateTurn(&turns[0])
	if !errors.Is(err, store.ErrTurnFrozen) {
		t.Fatalf("UpdateTurn() error = %v, want ErrTurnFrozen", err)
	}
}

func TestLoopRunReturnsErrorWhenRecordingFlushFails(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	// Block SaveRecording/MkdirAll by replacing the expected turn record path with a file.
	blockPath := filepath.Join(s.Root(), "local", "records", "1")
	if err := os.WriteFile(blockPath, []byte("block"), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", blockPath, err)
	}

	l := &Loop{
		Store: s,
		Agent: &stubAgent{},
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
	}

	err = l.Run(context.Background())
	if err == nil {
		t.Fatalf("Loop.Run() error = nil, want flush error")
	}
	if !strings.Contains(err.Error(), "flushing recording for turn 1") {
		t.Fatalf("Loop.Run() error = %v, want flush context", err)
	}
}

func TestLoopInitialResumeSessionIDStartsInResumeMode(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	a := &stubAgent{}
	var promptIsResume bool

	l := &Loop{
		Store: s,
		Agent: a,
		Config: agent.Config{
			Prompt:   "base prompt",
			MaxTurns: 1,
		},
		InitialResumeSessionID: "sess-prev",
		OnPrompt: func(turnID int, turnHexID, prompt string, isResume bool) {
			promptIsResume = isResume
		},
	}

	if err := l.Run(context.Background()); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	if len(a.runs) != 1 {
		t.Fatalf("agent runs = %d, want 1", len(a.runs))
	}
	if got := a.runs[0].ResumeSessionID; got != "sess-prev" {
		t.Fatalf("resume session id = %q, want %q", got, "sess-prev")
	}
	if got := a.runs[0].Prompt; !containsAll(got, "Continue from where you left off.") {
		t.Fatalf("resume prompt missing continuation lead: %q", got)
	}
	if !promptIsResume {
		t.Fatal("OnPrompt isResume = false, want true")
	}
}

func TestLoopWaitForSpawnsResumesSameTurn(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	a := &waitResumeStubAgent{store: s}
	var waited []int

	l := &Loop{
		Store: s,
		Agent: a,
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
		OnWait: func(_ context.Context, turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
			waited = append(waited, turnID)
			return []WaitResult{
				{SpawnID: 7, Profile: "scout", Status: "completed", Summary: "done"},
			}, false
		},
	}

	if err := l.Run(context.Background()); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	if len(a.runs) != 2 {
		t.Fatalf("agent runs = %d, want 2 (wait + resume)", len(a.runs))
	}
	if got, want := a.runs[0].TurnID, a.runs[1].TurnID; got != want {
		t.Fatalf("turn IDs differ across wait resume: first=%d second=%d", got, want)
	}
	if got := a.runs[1].ResumeSessionID; got != "sess-123" {
		t.Fatalf("resume session id = %q, want %q", got, "sess-123")
	}
	if got := a.runs[1].Env["ADAF_TURN_ID"]; got != a.runs[0].Env["ADAF_TURN_ID"] {
		t.Fatalf("ADAF_TURN_ID changed across wait resume: first=%q second=%q", a.runs[0].Env["ADAF_TURN_ID"], got)
	}
	if got := a.runs[1].Prompt; got == "" || got == "base" {
		t.Fatalf("resume prompt = %q, want continuation prompt", got)
	}
	if got := a.runs[1].Prompt; got != "" && !containsAll(got, "Spawn #7") {
		t.Fatalf("resume prompt missing wait results: %q", got)
	}
	if got := a.runs[1].Prompt; strings.Contains(got, "Continue from where you left off.") {
		t.Fatalf("wait resume prompt should not include continuation lead: %q", got)
	}

	if len(waited) != 1 {
		t.Fatalf("OnWait calls = %d, want 1", len(waited))
	}
	if waited[0] != a.runs[0].TurnID {
		t.Fatalf("OnWait turn ID = %d, want %d", waited[0], a.runs[0].TurnID)
	}

	turns, err := s.ListTurns()
	if err != nil {
		t.Fatalf("ListTurns() error = %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("turns count = %d, want 1", len(turns))
	}
	if turns[0].BuildState != "success" {
		t.Fatalf("final build state = %q, want %q", turns[0].BuildState, "success")
	}
}

func TestLoopWaitForSpawnsStopsTurnImmediately(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	a := &waitImmediateStopStubAgent{store: s}
	var waited []int

	l := &Loop{
		Store: s,
		Agent: a,
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
		OnWait: func(_ context.Context, turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
			waited = append(waited, turnID)
			return []WaitResult{{SpawnID: 11, Profile: "scout", Status: "completed", Summary: "done"}}, false
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.Run(ctx); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	if len(a.runs) != 2 {
		t.Fatalf("agent runs = %d, want 2 (wait-cancel + resume)", len(a.runs))
	}
	if a.runs[0].TurnID != a.runs[1].TurnID {
		t.Fatalf("turn IDs differ across wait resume: first=%d second=%d", a.runs[0].TurnID, a.runs[1].TurnID)
	}
	if len(waited) != 1 || waited[0] != a.firstTurnID {
		t.Fatalf("OnWait turn IDs = %v, want [%d]", waited, a.firstTurnID)
	}
	if s.IsWaiting(a.firstTurnID) {
		t.Fatalf("wait signal for turn %d was not cleared", a.firstTurnID)
	}
}

func TestLoopOnWaitSeenSpawnIDsAccumulateAndResetByTurn(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	a := &waitSeenTrackingStubAgent{store: s}
	var waited []int
	var seenCounts []int
	firstTurnID := 0

	l := &Loop{
		Store: s,
		Agent: a,
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 2,
		},
		OnWait: func(_ context.Context, turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
			waited = append(waited, turnID)
			seenCounts = append(seenCounts, len(alreadySeen))

			if firstTurnID == 0 {
				firstTurnID = turnID
				if len(alreadySeen) != 0 {
					t.Fatalf("first wait alreadySeen size = %d, want 0", len(alreadySeen))
				}
				return []WaitResult{{SpawnID: 101, Profile: "scout", Status: "completed", Summary: "first"}}, true
			}
			if turnID == firstTurnID {
				if len(alreadySeen) != 1 {
					t.Fatalf("second wait alreadySeen size = %d, want 1", len(alreadySeen))
				}
				if _, ok := alreadySeen[101]; !ok {
					t.Fatalf("second wait alreadySeen missing spawn 101")
				}
				return []WaitResult{{SpawnID: 102, Profile: "scout", Status: "completed", Summary: "second"}}, false
			}
			if len(alreadySeen) != 0 {
				t.Fatalf("new turn alreadySeen size = %d, want 0", len(alreadySeen))
			}
			return []WaitResult{{SpawnID: 201, Profile: "scout", Status: "completed", Summary: "third"}}, false
		},
	}

	if err := l.Run(context.Background()); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	if len(waited) != 3 {
		t.Fatalf("OnWait calls = %d, want 3", len(waited))
	}
	if waited[0] != waited[1] {
		t.Fatalf("first two OnWait turn IDs differ: %v", waited)
	}
	if waited[2] == waited[0] {
		t.Fatalf("third OnWait turn ID did not reset: %v", waited)
	}
	if len(seenCounts) != 3 || seenCounts[0] != 0 || seenCounts[1] != 1 || seenCounts[2] != 0 {
		t.Fatalf("seen counts = %v, want [0 1 0]", seenCounts)
	}
}

func TestLoopWaitResumePreservesSessionIDWhenCanceledRunHasNoResult(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	a := &waitResumeSessionCarryStubAgent{store: s}
	waitCalls := 0
	l := &Loop{
		Store: s,
		Agent: a,
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
		OnWait: func(_ context.Context, turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
			waitCalls++
			switch waitCalls {
			case 1:
				return []WaitResult{{SpawnID: 1, Profile: "scout", Status: "completed", Summary: "first"}}, true
			case 2:
				return []WaitResult{{SpawnID: 2, Profile: "scout", Status: "completed", Summary: "second"}}, false
			default:
				t.Fatalf("unexpected OnWait call #%d", waitCalls)
				return nil, false
			}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := l.Run(ctx); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	if len(a.runs) != 3 {
		t.Fatalf("agent runs = %d, want 3", len(a.runs))
	}
	if got := a.runs[1].ResumeSessionID; got != "sess-keep" {
		t.Fatalf("second run resume session id = %q, want %q", got, "sess-keep")
	}
	if got := a.runs[2].ResumeSessionID; got != "sess-keep" {
		t.Fatalf("third run resume session id = %q, want %q", got, "sess-keep")
	}
	if got := a.runs[2].Prompt; !containsAll(got, "Spawn #2") {
		t.Fatalf("third run prompt missing wait result markers: %q", got)
	}
	if got := a.runs[2].Prompt; strings.Contains(got, "Continue from where you left off.") {
		t.Fatalf("third run wait-resume prompt should not include continuation lead: %q", got)
	}
}

func TestLoopInfersWaitForSpawnsWhenTurnEndsWithRunningSpawn(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	a := &implicitSpawnWaitStubAgent{store: s}
	waitCalls := 0
	l := &Loop{
		Store: s,
		Agent: a,
		Config: agent.Config{
			Prompt:   "base",
			MaxTurns: 1,
		},
		OnWait: func(_ context.Context, turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
			waitCalls++
			if waitCalls > 1 {
				t.Fatalf("OnWait called %d times, want 1", waitCalls)
			}
			if len(alreadySeen) != 0 {
				t.Fatalf("alreadySeen size = %d, want 0", len(alreadySeen))
			}
			rec, err := s.GetSpawn(a.spawnID)
			if err != nil {
				t.Fatalf("GetSpawn(%d) error = %v", a.spawnID, err)
			}
			rec.Status = store.SpawnStatusCompleted
			rec.Summary = "child completed after implicit wait"
			if err := s.UpdateSpawn(rec); err != nil {
				t.Fatalf("UpdateSpawn(%d) error = %v", rec.ID, err)
			}
			return []WaitResult{{
				SpawnID:  rec.ID,
				Profile:  rec.ChildProfile,
				Status:   rec.Status,
				Summary:  rec.Summary,
				ExitCode: rec.ExitCode,
				ReadOnly: rec.ReadOnly,
				Branch:   rec.Branch,
			}}, false
		},
	}

	if err := l.Run(context.Background()); err != nil {
		t.Fatalf("Loop.Run() error = %v", err)
	}

	if waitCalls != 1 {
		t.Fatalf("OnWait calls = %d, want 1", waitCalls)
	}
	if len(a.runs) != 2 {
		t.Fatalf("agent runs = %d, want 2 (implicit wait + resume)", len(a.runs))
	}
	if a.runs[0].TurnID != a.runs[1].TurnID {
		t.Fatalf("turn IDs differ across implicit wait resume: first=%d second=%d", a.runs[0].TurnID, a.runs[1].TurnID)
	}
	if got := a.runs[1].ResumeSessionID; got != "sess-implicit" {
		t.Fatalf("resume session id = %q, want %q", got, "sess-implicit")
	}
	if got := a.runs[1].Prompt; !containsAll(got, "Spawn #", "child completed after implicit wait") {
		t.Fatalf("resume prompt missing implicit wait result: %q", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
