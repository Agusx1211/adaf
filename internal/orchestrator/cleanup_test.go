package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/worktree"
)

func TestCleanupStaleWorktrees_PreservesCompletedSpawnWorktree(t *testing.T) {
	repo := initGitRepo(t)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T020000"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, wtPath, branch)

	s := newTestStore(t, repo)
	rec := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test",
		Status:        "completed",
		Branch:        branch,
		WorktreePath:  wtPath,
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := New(s, nil, repo)
	o.cleanupStaleWorktrees()

	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("completed spawn worktree should exist after cleanup: %v", err)
	}
	if !branchExists(repo, branch) {
		t.Fatalf("completed spawn branch %q should exist after cleanup", branch)
	}
}

func TestCleanupStaleWorktrees_RemovesMergedSpawnWorktree(t *testing.T) {
	repo := initGitRepo(t)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T020001"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	s := newTestStore(t, repo)
	rec := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "test",
		Status:        "merged",
		Branch:        branch,
		WorktreePath:  wtPath,
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	o := New(s, nil, repo)
	o.cleanupStaleWorktrees()

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("merged spawn worktree should be removed, stat err=%v", err)
	}
	if branchExists(repo, branch) {
		t.Fatalf("merged spawn branch %q should be removed", branch)
	}
}

func TestCleanupStaleWorktrees_RemovesOldUntrackedWorktree(t *testing.T) {
	repo := initGitRepo(t)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T020002"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make this untracked worktree old enough to be considered stale.
	old := time.Now().Add(-(staleWorktreeMaxAge + time.Hour))
	if err := os.Chtimes(wtPath, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	s := newTestStore(t, repo)
	o := New(s, nil, repo)
	o.cleanupStaleWorktrees()

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("old untracked worktree should be removed, stat err=%v", err)
	}
	if branchExists(repo, branch) {
		t.Fatalf("old untracked branch %q should be removed", branch)
	}
}

func newTestStore(t *testing.T, repoRoot string) *store.Store {
	t.Helper()

	root := filepath.Join(t.TempDir(), ".adaf")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: repoRoot}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	return s
}

func branchExists(repoRoot, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}
