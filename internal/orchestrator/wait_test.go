package orchestrator

import (
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/store"
)

func TestWaitOnePollsStoreAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	rec := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test",
		Status:        "running",
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := &Orchestrator{
		store:  s,
		spawns: map[int]*activeSpawn{},
	}

	go func() {
		time.Sleep(60 * time.Millisecond)
		updated, _ := s.GetSpawn(rec.ID)
		updated.Status = "completed"
		updated.ExitCode = 0
		updated.Result = "done"
		_ = s.UpdateSpawn(updated)
	}()

	start := time.Now()
	result := o.WaitOne(rec.ID)
	elapsed := time.Since(start)

	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", result.ExitCode)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("waited %v, want to block until completion", elapsed)
	}
}

func TestWaitOneReturnsUnknownForMissingSpawn(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	o := &Orchestrator{
		store:  s,
		spawns: map[int]*activeSpawn{},
	}

	result := o.WaitOne(99999)
	if result.Status != "unknown" {
		t.Fatalf("status = %q, want unknown", result.Status)
	}
}
