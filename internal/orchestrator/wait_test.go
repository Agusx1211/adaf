package orchestrator

import (
	"context"
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
		spawnID:      1,
		parentTurnID: 42,
		done:         done,
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
		spawnID:      1,
		parentTurnID: 7,
		done:         done,
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
		spawnID:      1,
		parentTurnID: 42,
		done:         targetDone,
	}
	o.spawns[2] = &activeSpawn{
		spawnID:      2,
		parentTurnID: 99,
		done:         otherDone,
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

func TestWaitAnyUnblocksOnSignalAndReturnsCompletion(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	rec := &store.SpawnRecord{
		ParentTurnID:  17,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test",
		Status:        "running",
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := &Orchestrator{
		store:   s,
		spawns:  map[int]*activeSpawn{},
		waitAny: map[int]chan struct{}{},
		waiters: map[int]int{},
	}

	type waitResult struct {
		results []SpawnResult
		more    bool
	}
	done := make(chan waitResult, 1)
	go func() {
		results, more := o.WaitAny(t.Context(), rec.ParentTurnID, map[int]struct{}{})
		done <- waitResult{results: results, more: more}
	}()

	time.Sleep(40 * time.Millisecond)
	updated, _ := s.GetSpawn(rec.ID)
	updated.Status = "completed"
	updated.ExitCode = 0
	updated.Result = "ok"
	if err := s.UpdateSpawn(updated); err != nil {
		t.Fatalf("UpdateSpawn: %v", err)
	}
	o.signalWaitAny(rec.ParentTurnID)

	select {
	case got := <-done:
		if len(got.results) != 1 {
			t.Fatalf("WaitAny results len = %d, want 1", len(got.results))
		}
		if got.results[0].SpawnID != rec.ID {
			t.Fatalf("WaitAny spawn id = %d, want %d", got.results[0].SpawnID, rec.ID)
		}
		if got.results[0].Status != "completed" {
			t.Fatalf("WaitAny status = %q, want completed", got.results[0].Status)
		}
		if got.more {
			t.Fatalf("WaitAny more = true, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("WaitAny did not return after completion signal")
	}

	o.mu.Lock()
	_, chExists := o.waitAny[rec.ParentTurnID]
	_, waiterExists := o.waiters[rec.ParentTurnID]
	o.mu.Unlock()
	if chExists || waiterExists {
		t.Fatalf("WaitAny state for parent turn %d should be cleaned up (ch=%v waiter=%v)", rec.ParentTurnID, chExists, waiterExists)
	}
}

func TestWaitAnyReturnsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	rec := &store.SpawnRecord{
		ParentTurnID:  18,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test",
		Status:        "running",
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := &Orchestrator{
		store:   s,
		spawns:  map[int]*activeSpawn{},
		waitAny: map[int]chan struct{}{},
		waiters: map[int]int{},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Millisecond)
	defer cancel()

	start := time.Now()
	results, more := o.WaitAny(ctx, rec.ParentTurnID, map[int]struct{}{})
	elapsed := time.Since(start)

	if len(results) != 0 {
		t.Fatalf("WaitAny returned %d results, want 0 on cancellation", len(results))
	}
	if more {
		t.Fatalf("WaitAny more = true, want false on cancellation")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("WaitAny cancellation took too long: %v", elapsed)
	}

	o.mu.Lock()
	_, chExists := o.waitAny[rec.ParentTurnID]
	_, waiterExists := o.waiters[rec.ParentTurnID]
	o.mu.Unlock()
	if chExists || waiterExists {
		t.Fatalf("WaitAny state for parent turn %d should be cleaned up after cancellation (ch=%v waiter=%v)", rec.ParentTurnID, chExists, waiterExists)
	}
}
