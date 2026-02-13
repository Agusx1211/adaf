package session

import (
	"strings"
	"sync"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestAbortSessionStartupMarksMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionID, err := CreateSession(DaemonConfig{
		ProjectDir:  t.TempDir(),
		ProjectName: "proj",
		WorkDir:     t.TempDir(),
		ProfileName: "p1",
		AgentName:   "generic",
		Loop: config.LoopDef{
			Name: "test",
			Steps: []config.LoopStep{
				{Profile: "p1", Turns: 1},
			},
		},
		Profiles: []config.Profile{{Name: "p1", Agent: "generic"}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	AbortSessionStartup(sessionID, "connect failed")

	meta, err := LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Status != StatusCancelled {
		t.Fatalf("status = %q, want %q", meta.Status, StatusCancelled)
	}
	if !strings.Contains(meta.Error, "connect failed") {
		t.Fatalf("error = %q, want to contain %q", meta.Error, "connect failed")
	}
	if meta.EndedAt.IsZero() {
		t.Fatal("EndedAt is zero, want non-zero")
	}
}

func TestCreateSessionConcurrentUniqueIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	const n = 24
	ids := make(chan int, n)
	errs := make(chan error, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id, err := CreateSession(DaemonConfig{
				ProjectDir:  projectDir,
				ProjectName: "proj",
				WorkDir:     projectDir,
				ProfileName: "p1",
				AgentName:   "generic",
				Loop: config.LoopDef{
					Name: "test",
					Steps: []config.LoopStep{
						{Profile: "p1", Turns: 1},
					},
				},
				Profiles: []config.Profile{{Name: "p1", Agent: "generic"}},
			})
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Fatalf("CreateSession concurrent error: %v", err)
	}

	seen := make(map[int]struct{}, n)
	for id := range ids {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate session id detected: %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("created sessions = %d, want %d", len(seen), n)
	}
}

func TestIsAgentContext(t *testing.T) {
	t.Run("false when no marker env vars are set", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "")
		t.Setenv("ADAF_AGENT", "")
		if IsAgentContext() {
			t.Fatal("IsAgentContext() = true, want false")
		}
	})

	t.Run("true when ADAF_TURN_ID is set", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "123")
		t.Setenv("ADAF_SESSION_ID", "")
		t.Setenv("ADAF_AGENT", "")
		if !IsAgentContext() {
			t.Fatal("IsAgentContext() = false, want true")
		}
	})

	t.Run("true when ADAF_SESSION_ID is set", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "55")
		t.Setenv("ADAF_AGENT", "")
		if !IsAgentContext() {
			t.Fatal("IsAgentContext() = false, want true")
		}
	})

	t.Run("true when ADAF_AGENT is 1", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "")
		t.Setenv("ADAF_AGENT", "1")
		if !IsAgentContext() {
			t.Fatal("IsAgentContext() = false, want true")
		}
	})
}

func TestIsActiveStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "starting", status: StatusStarting, want: true},
		{name: "running", status: StatusRunning, want: true},
		{name: "done", status: StatusDone, want: false},
		{name: "cancelled", status: StatusCancelled, want: false},
		{name: "error", status: StatusError, want: false},
		{name: "dead", status: StatusDead, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsActiveStatus(tt.status)
			if got != tt.want {
				t.Fatalf("IsActiveStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
