package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/worktree"
)

func createSpawnWithCommittedWorktree(t *testing.T, status string) (context.Context, string, *store.Store, *Orchestrator, *store.SpawnRecord) {
	t.Helper()

	ctx := context.Background()
	repo := initGitRepo(t)
	runGit(t, repo, "config", "user.name", "Test")
	runGit(t, repo, "config", "user.email", "test@example.com")

	mgr := worktree.NewManager(repo)
	branch := worktree.BranchName(1, "worker")
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create(%q): %v", branch, err)
	}

	if err := os.WriteFile(filepath.Join(wtPath, "spawn-work.txt"), []byte("spawn changes\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runGitWithConfig(t, wtPath, []string{"user.name=Test", "user.email=test@example.com"}, "add", "spawn-work.txt")
	runGitWithConfig(t, wtPath, []string{"user.name=Test", "user.email=test@example.com"}, "commit", "-m", "spawn work")

	s := newTestStore(t, repo)
	rec := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test task",
		Status:        status,
		Branch:        branch,
		WorktreePath:  wtPath,
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := New(s, &config.GlobalConfig{}, repo)
	return ctx, repo, s, o, rec
}

func TestMerge_Success(t *testing.T) {
	ctx, repo, s, o, rec := createSpawnWithCommittedWorktree(t, "completed")
	if !branchExists(repo, rec.Branch) {
		t.Fatalf("branch %q must exist before merge", rec.Branch)
	}

	hash, err := o.Merge(ctx, rec.ID, false)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if strings.TrimSpace(hash) == "" {
		t.Fatalf("merge hash is empty")
	}
	runGit(t, repo, "rev-parse", "--verify", hash+"^{commit}")

	got, err := s.GetSpawn(rec.ID)
	if err != nil {
		t.Fatalf("GetSpawn(%d): %v", rec.ID, err)
	}
	if got.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Status)
	}
	if got.MergeCommit != hash {
		t.Fatalf("MergeCommit = %q, want %q", got.MergeCommit, hash)
	}

	parents := strings.Fields(strings.TrimSpace(gitOutput(t, repo, "show", "-s", "--format=%P", hash)))
	if len(parents) != 2 {
		t.Fatalf("merge commit parents = %d, want 2 for non-squash", len(parents))
	}

	if _, err := os.Stat(rec.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree %q should be removed, stat err=%v", rec.WorktreePath, err)
	}
	if branchExists(repo, rec.Branch) {
		t.Fatalf("branch %q should be removed after merge", rec.Branch)
	}
}

func TestMerge_Squash(t *testing.T) {
	ctx, repo, s, o, rec := createSpawnWithCommittedWorktree(t, "completed")

	hash, err := o.Merge(ctx, rec.ID, true)
	if err != nil {
		t.Fatalf("Merge(squash): %v", err)
	}
	if strings.TrimSpace(hash) == "" {
		t.Fatalf("merge hash is empty")
	}

	got, err := s.GetSpawn(rec.ID)
	if err != nil {
		t.Fatalf("GetSpawn(%d): %v", rec.ID, err)
	}
	if got.Status != "merged" {
		t.Fatalf("status = %q, want merged", got.Status)
	}
	if got.MergeCommit != hash {
		t.Fatalf("MergeCommit = %q, want %q", got.MergeCommit, hash)
	}

	parents := strings.Fields(strings.TrimSpace(gitOutput(t, repo, "show", "-s", "--format=%P", hash)))
	if len(parents) != 1 {
		t.Fatalf("squash commit parents = %d, want 1", len(parents))
	}

	if _, err := os.Stat(rec.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree %q should be removed, stat err=%v", rec.WorktreePath, err)
	}
	if branchExists(repo, rec.Branch) {
		t.Fatalf("branch %q should be removed after merge", rec.Branch)
	}
}

func TestMerge_SpawnNotFound(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	o := New(s, &config.GlobalConfig{}, repo)

	_, err := o.Merge(context.Background(), 99999, false)
	if err == nil {
		t.Fatalf("Merge error = nil, want not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Merge error = %q, want not found hint", err)
	}
}

func TestMerge_NotCompleted(t *testing.T) {
	ctx, _, _, o, rec := createSpawnWithCommittedWorktree(t, "running")

	hash, err := o.Merge(ctx, rec.ID, false)
	if err == nil {
		t.Fatalf("Merge error = nil, want status error")
	}
	if hash != "" {
		t.Fatalf("hash = %q, want empty", hash)
	}
	if !strings.Contains(err.Error(), "not completed") {
		t.Fatalf("Merge error = %q, want not completed hint", err)
	}
}

func TestMerge_NoBranch(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	rec := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test task",
		Status:        "completed",
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := New(s, &config.GlobalConfig{}, repo)
	_, err := o.Merge(context.Background(), rec.ID, false)
	if err == nil {
		t.Fatalf("Merge error = nil, want no branch error")
	}
	if !strings.Contains(err.Error(), "no branch") {
		t.Fatalf("Merge error = %q, want no branch hint", err)
	}
}

func TestReject_CompletedSpawn(t *testing.T) {
	ctx, repo, s, o, rec := createSpawnWithCommittedWorktree(t, "completed")

	if err := o.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, err := s.GetSpawn(rec.ID)
	if err != nil {
		t.Fatalf("GetSpawn(%d): %v", rec.ID, err)
	}
	if got.Status != "rejected" {
		t.Fatalf("status = %q, want rejected", got.Status)
	}
	if _, err := os.Stat(rec.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree %q should be removed, stat err=%v", rec.WorktreePath, err)
	}
	if branchExists(repo, rec.Branch) {
		t.Fatalf("branch %q should be removed after reject", rec.Branch)
	}
}

func TestReject_RunningSpawn(t *testing.T) {
	ctx, repo, s, o, rec := createSpawnWithCommittedWorktree(t, "running")

	cancelCalled := false
	o.mu.Lock()
	o.spawns[rec.ID] = &activeSpawn{
		spawnID: rec.ID,
		cancel: func() {
			cancelCalled = true
		},
		done: make(chan struct{}),
	}
	o.mu.Unlock()

	if err := o.Reject(ctx, rec.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if !cancelCalled {
		t.Fatalf("cancel not called for running spawn")
	}

	got, err := s.GetSpawn(rec.ID)
	if err != nil {
		t.Fatalf("GetSpawn(%d): %v", rec.ID, err)
	}
	if got.Status != "rejected" {
		t.Fatalf("status = %q, want rejected", got.Status)
	}
	if _, err := os.Stat(rec.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree %q should be removed, stat err=%v", rec.WorktreePath, err)
	}
	if branchExists(repo, rec.Branch) {
		t.Fatalf("branch %q should be removed after reject", rec.Branch)
	}
}

func TestReject_SpawnNotFound(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	o := New(s, &config.GlobalConfig{}, repo)

	err := o.Reject(context.Background(), 99999)
	if err == nil {
		t.Fatalf("Reject error = nil, want not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Reject error = %q, want not found hint", err)
	}
}

func TestCancel_RunningSpawn(t *testing.T) {
	cancelCalled := false
	o := &Orchestrator{
		spawns: map[int]*activeSpawn{
			17: {
				spawnID: 17,
				cancel: func() {
					cancelCalled = true
				},
				done: make(chan struct{}),
			},
		},
	}

	if err := o.Cancel(17); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !cancelCalled {
		t.Fatalf("cancel not called")
	}
}

func TestCancel_NotFound(t *testing.T) {
	o := &Orchestrator{spawns: map[int]*activeSpawn{}}

	err := o.Cancel(99999)
	if err == nil {
		t.Fatalf("Cancel error = nil, want not found")
	}
	if !strings.Contains(err.Error(), "not found or already completed") {
		t.Fatalf("Cancel error = %q, want not found/already completed hint", err)
	}
}

func TestCancel_AlreadyCompleted(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	rec := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "done task",
		Status:        "completed",
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := New(s, &config.GlobalConfig{}, repo)
	err := o.Cancel(rec.ID)
	if err == nil {
		t.Fatalf("Cancel error = nil, want already completed error")
	}
	if !strings.Contains(err.Error(), "not found or already completed") {
		t.Fatalf("Cancel error = %q, want not found/already completed hint", err)
	}
}

func TestSpawn_RejectsWhenProfileMaxInstancesReached(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "worker", Agent: "codex", MaxInstances: 1},
		},
	}
	o := New(s, cfg, repo)

	running := &store.SpawnRecord{
		ParentTurnID:  7,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "running task",
		Status:        "running",
	}
	if err := s.CreateSpawn(running); err != nil {
		t.Fatalf("CreateSpawn(running): %v", err)
	}

	o.mu.Lock()
	o.instances["worker"] = 1
	o.running["parent"] = 1
	o.spawns[running.ID] = &activeSpawn{
		spawnID: running.ID,
		cancel:  func() {},
		done:    make(chan struct{}),
	}
	o.mu.Unlock()

	spawnID, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  8,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "do work",
		Delegation: &config.DelegationConfig{
			MaxParallel: 2,
			Profiles: []config.DelegationProfile{
				{Name: "worker"},
			},
		},
	})
	if err == nil {
		t.Fatalf("Spawn() error = nil, want max instances error")
	}
	if !strings.Contains(err.Error(), "child profile") || !strings.Contains(err.Error(), "max 1") {
		t.Fatalf("Spawn() error = %q, want child max instances hint", err)
	}
	if spawnID != 0 {
		t.Fatalf("spawnID = %d, want 0", spawnID)
	}

	spawns, listErr := s.ListSpawns()
	if listErr != nil {
		t.Fatalf("ListSpawns: %v", listErr)
	}
	if len(spawns) != 1 {
		t.Fatalf("expected only the pre-existing running spawn, got %d", len(spawns))
	}
}
