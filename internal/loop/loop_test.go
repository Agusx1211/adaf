package loop

import (
	"context"
	"errors"
	"fmt"
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
