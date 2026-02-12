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

func TestWaitForRunningSpawnsCompletesWithinTimeout(t *testing.T) {
	o := &Orchestrator{
		spawns: map[int]*activeSpawn{},
	}

	done := make(chan struct{})
	o.spawns[1] = &activeSpawn{
		record: &store.SpawnRecord{ID: 1, ParentTurnID: 42},
		done:   done,
	}

	o.spawnWG.Add(1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(done)
		o.spawnWG.Done()
	}()

	start := time.Now()
	ok := o.WaitForRunningSpawns([]int{42}, 500*time.Millisecond)
	elapsed := time.Since(start)

	if !ok {
		t.Fatalf("WaitForRunningSpawns returned false, want true")
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("waited %v, want to block until spawn completion", elapsed)
	}
}

func TestWaitForRunningSpawnsTimeout(t *testing.T) {
	o := &Orchestrator{
		spawns: map[int]*activeSpawn{},
	}

	done := make(chan struct{})
	o.spawns[1] = &activeSpawn{
		record: &store.SpawnRecord{ID: 1, ParentTurnID: 7},
		done:   done,
	}

	o.spawnWG.Add(1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		close(done)
		o.spawnWG.Done()
	}()

	start := time.Now()
	ok := o.WaitForRunningSpawns([]int{7}, 80*time.Millisecond)
	elapsed := time.Since(start)

	if ok {
		t.Fatalf("WaitForRunningSpawns returned true, want false on timeout")
	}
	if elapsed < 70*time.Millisecond {
		t.Fatalf("waited %v, want to respect timeout before returning", elapsed)
	}
}

func TestWaitForRunningSpawnsFiltersByParentTurn(t *testing.T) {
	o := &Orchestrator{
		spawns: map[int]*activeSpawn{},
	}

	targetDone := make(chan struct{})
	otherDone := make(chan struct{})
	o.spawns[1] = &activeSpawn{
		record: &store.SpawnRecord{ID: 1, ParentTurnID: 42},
		done:   targetDone,
	}
	o.spawns[2] = &activeSpawn{
		record: &store.SpawnRecord{ID: 2, ParentTurnID: 99},
		done:   otherDone,
	}

	o.spawnWG.Add(2)
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(targetDone)
		o.spawnWG.Done()
	}()
	go func() {
		time.Sleep(300 * time.Millisecond)
		close(otherDone)
		o.spawnWG.Done()
	}()

	start := time.Now()
	ok := o.WaitForRunningSpawns([]int{42}, 150*time.Millisecond)
	elapsed := time.Since(start)

	if !ok {
		t.Fatalf("WaitForRunningSpawns returned false, want true for filtered target")
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("waited %v, want to block until filtered target completes", elapsed)
	}
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("waited %v, should not wait for unrelated parent turn", elapsed)
	}
}
