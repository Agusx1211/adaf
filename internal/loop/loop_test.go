package loop

import (
	"context"
	"errors"
	"fmt"
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

func TestLoopPromptFuncReceivesSupervisorNotesByTurn(t *testing.T) {
	tests := []struct {
		name           string
		maxTurns       int
		notes          []store.SupervisorNote
		wantNoteCounts []int
	}{
		{
			name:     "single turn note delivery",
			maxTurns: 1,
			notes: []store.SupervisorNote{
				{TurnID: 1, Author: "sup", Note: "first"},
			},
			wantNoteCounts: []int{1},
		},
		{
			name:     "multi turn receives only matching turn notes",
			maxTurns: 2,
			notes: []store.SupervisorNote{
				{TurnID: 1, Author: "sup", Note: "for one"},
				{TurnID: 2, Author: "sup", Note: "for two"},
				{TurnID: 99, Author: "sup", Note: "other"},
			},
			wantNoteCounts: []int{1, 1},
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

			for _, n := range tt.notes {
				note := n
				if err := s.CreateNote(&note); err != nil {
					t.Fatalf("CreateNote() error = %v", err)
				}
			}

			a := &stubAgent{}
			type promptCall struct {
				turnID int
				notes  []store.SupervisorNote
			}
			var calls []promptCall

			l := &Loop{
				Store: s,
				Agent: a,
				Config: agent.Config{
					Prompt:   "base",
					MaxTurns: tt.maxTurns,
				},
				PromptFunc: func(turnID int, supervisorNotes []store.SupervisorNote) string {
					notes := make([]store.SupervisorNote, len(supervisorNotes))
					copy(notes, supervisorNotes)
					calls = append(calls, promptCall{turnID: turnID, notes: notes})
					return fmt.Sprintf("turn=%d notes=%d", turnID, len(supervisorNotes))
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
				if calls[i].turnID != wantTurnID {
					t.Fatalf("call[%d] turn id = %d, want %d", i, calls[i].turnID, wantTurnID)
				}
				if len(calls[i].notes) != tt.wantNoteCounts[i] {
					t.Fatalf("call[%d] notes = %d, want %d", i, len(calls[i].notes), tt.wantNoteCounts[i])
				}
				for _, note := range calls[i].notes {
					if note.TurnID != wantTurnID {
						t.Fatalf("call[%d] note turn id = %d, want %d", i, note.TurnID, wantTurnID)
					}
				}

				if got := a.runs[i].Env["ADAF_AGENT"]; got != "1" {
					t.Fatalf("run[%d] ADAF_AGENT = %q, want %q", i, got, "1")
				}
				if got := a.runs[i].Env["ADAF_TURN_ID"]; got != fmt.Sprintf("%d", wantTurnID) {
					t.Fatalf("run[%d] ADAF_TURN_ID = %q, want %d", i, got, wantTurnID)
				}
				wantPrompt := fmt.Sprintf("turn=%d notes=%d", wantTurnID, tt.wantNoteCounts[i])
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
		OnWait: func(turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
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
	if got := a.runs[1].Prompt; got != "" && !containsAll(got, "Continue from where you left off.", "Spawn #7") {
		t.Fatalf("resume prompt missing continuation markers: %q", got)
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
		OnWait: func(turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
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
		OnWait: func(turnID int, alreadySeen map[int]struct{}) ([]WaitResult, bool) {
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

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
