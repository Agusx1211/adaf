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
		OnWait: func(turnID int) ([]WaitResult, bool) {
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

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
