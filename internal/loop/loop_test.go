package loop

import (
	"context"
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

func TestLoopPromptFuncReceivesSupervisorNotesBySession(t *testing.T) {
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
				{SessionID: 1, Author: "sup", Note: "first"},
			},
			wantNoteCounts: []int{1},
		},
		{
			name:     "multi turn receives only matching session notes",
			maxTurns: 2,
			notes: []store.SupervisorNote{
				{SessionID: 1, Author: "sup", Note: "for one"},
				{SessionID: 2, Author: "sup", Note: "for two"},
				{SessionID: 99, Author: "sup", Note: "other"},
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
				sessionID int
				notes     []store.SupervisorNote
			}
			var calls []promptCall

			l := &Loop{
				Store: s,
				Agent: a,
				Config: agent.Config{
					Prompt:   "base",
					MaxTurns: tt.maxTurns,
				},
				PromptFunc: func(sessionID int, supervisorNotes []store.SupervisorNote) string {
					notes := make([]store.SupervisorNote, len(supervisorNotes))
					copy(notes, supervisorNotes)
					calls = append(calls, promptCall{sessionID: sessionID, notes: notes})
					return fmt.Sprintf("session=%d notes=%d", sessionID, len(supervisorNotes))
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
				wantSessionID := i + 1
				if calls[i].sessionID != wantSessionID {
					t.Fatalf("call[%d] session id = %d, want %d", i, calls[i].sessionID, wantSessionID)
				}
				if len(calls[i].notes) != tt.wantNoteCounts[i] {
					t.Fatalf("call[%d] notes = %d, want %d", i, len(calls[i].notes), tt.wantNoteCounts[i])
				}
				for _, note := range calls[i].notes {
					if note.SessionID != wantSessionID {
						t.Fatalf("call[%d] note session id = %d, want %d", i, note.SessionID, wantSessionID)
					}
				}

				if got := a.runs[i].Env["ADAF_AGENT"]; got != "1" {
					t.Fatalf("run[%d] ADAF_AGENT = %q, want %q", i, got, "1")
				}
				if got := a.runs[i].Env["ADAF_SESSION_ID"]; got != fmt.Sprintf("%d", wantSessionID) {
					t.Fatalf("run[%d] ADAF_SESSION_ID = %q, want %d", i, got, wantSessionID)
				}
				wantPrompt := fmt.Sprintf("session=%d notes=%d", wantSessionID, tt.wantNoteCounts[i])
				if got := a.runs[i].Prompt; got != wantPrompt {
					t.Fatalf("run[%d] prompt = %q, want %q", i, got, wantPrompt)
				}
			}
		})
	}
}
