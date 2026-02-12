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
	if meta.Status != "cancelled" {
		t.Fatalf("status = %q, want %q", meta.Status, "cancelled")
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
