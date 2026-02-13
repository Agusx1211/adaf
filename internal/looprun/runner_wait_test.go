package looprun

import (
	"context"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/store"
)

func TestWaitForAnySessionSpawnsReturnsOnlyUnseenCompletions(t *testing.T) {
	s := newLooprunTestStore(t)
	parentTurnID := 77

	doneOne := createLooprunSpawn(t, s, parentTurnID, "completed")
	doneTwo := createLooprunSpawn(t, s, parentTurnID, "completed")
	running := createLooprunSpawn(t, s, parentTurnID, "running")

	first, morePending := waitForAnySessionSpawns(context.Background(), s, parentTurnID, nil)
	if !morePending {
		t.Fatalf("first morePending = false, want true")
	}
	if len(first) != 2 {
		t.Fatalf("first results = %d, want 2", len(first))
	}
	if first[0].SpawnID != doneOne.ID || first[1].SpawnID != doneTwo.ID {
		t.Fatalf("first spawn IDs = [%d %d], want [%d %d]", first[0].SpawnID, first[1].SpawnID, doneOne.ID, doneTwo.ID)
	}

	alreadySeen := map[int]struct{}{
		doneOne.ID: {},
		doneTwo.ID: {},
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		rec, err := s.GetSpawn(running.ID)
		if err != nil {
			return
		}
		rec.Status = "completed"
		rec.Summary = "runner done"
		_ = s.UpdateSpawn(rec)
	}()

	second, morePending := waitForAnySessionSpawns(context.Background(), s, parentTurnID, alreadySeen)
	if morePending {
		t.Fatalf("second morePending = true, want false")
	}
	if len(second) != 1 {
		t.Fatalf("second results = %d, want 1", len(second))
	}
	if second[0].SpawnID != running.ID {
		t.Fatalf("second spawn ID = %d, want %d", second[0].SpawnID, running.ID)
	}

	alreadySeen[running.ID] = struct{}{}
	third, morePending := waitForAnySessionSpawns(context.Background(), s, parentTurnID, alreadySeen)
	if morePending {
		t.Fatalf("third morePending = true, want false")
	}
	if len(third) != 0 {
		t.Fatalf("third results = %d, want 0", len(third))
	}
}

func newLooprunTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	return s
}

func createLooprunSpawn(t *testing.T, s *store.Store, parentTurnID int, status string) *store.SpawnRecord {
	t.Helper()
	rec := &store.SpawnRecord{
		ParentTurnID:  parentTurnID,
		ParentProfile: "parent",
		ChildProfile:  "child",
		Task:          "test",
		Status:        status,
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn() error = %v", err)
	}
	return rec
}
