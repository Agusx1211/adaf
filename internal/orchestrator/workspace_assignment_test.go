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

func TestResolveWorkspaceBaseRef_RejectsMergedSpawn(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	o := New(s, &config.GlobalConfig{}, repo)

	source := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "done",
		Status:        "merged",
		Branch:        "adaf/test/source/merged",
	}
	if err := s.CreateSpawn(source); err != nil {
		t.Fatalf("CreateSpawn(source): %v", err)
	}

	_, err := o.resolveWorkspaceBaseRef(SpawnRequest{WorkspaceFromSpawnID: source.ID})
	if err == nil {
		t.Fatalf("resolveWorkspaceBaseRef error = nil, want merged status error")
	}
	if !strings.Contains(err.Error(), "only completed/failed/canceled") {
		t.Fatalf("resolveWorkspaceBaseRef error = %q, want status validation hint", err)
	}
}

func TestCreateWritableWorktree_UsesWorkspaceBaseRef(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	o := New(s, &config.GlobalConfig{}, repo)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	sourceBranch := "adaf/test/source/20260217T140000"
	sourceWtPath, err := mgr.Create(ctx, sourceBranch)
	if err != nil {
		t.Fatalf("Create(source): %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, sourceWtPath, sourceBranch)

	sourceOnly := filepath.Join(sourceWtPath, "source-only.txt")
	if err := os.WriteFile(sourceOnly, []byte("source state\n"), 0644); err != nil {
		t.Fatalf("WriteFile(source-only): %v", err)
	}
	runGit(t, sourceWtPath, "add", "source-only.txt")
	runGitWithConfig(t, sourceWtPath, []string{"user.name=Test", "user.email=test@example.com"}, "commit", "-m", "source state")

	branch, wtPath, err := o.createWritableWorktree(ctx, 77, "worker", sourceBranch)
	if err != nil {
		t.Fatalf("createWritableWorktree(from source): %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, wtPath, branch)

	if branch == sourceBranch {
		t.Fatalf("new branch = source branch %q, want a distinct branch", branch)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "source-only.txt")); err != nil {
		t.Fatalf("spawned worktree should include source branch file: %v", err)
	}
}

func TestSpawn_WorkspaceFromSpawn_PersistsSourceID(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "worker", Agent: "missing-agent"},
		},
	}
	o := New(s, cfg, repo)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	sourceBranch := "adaf/test/source/20260217T140001"
	sourceWtPath, err := mgr.Create(ctx, sourceBranch)
	if err != nil {
		t.Fatalf("Create(source): %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, sourceWtPath, sourceBranch)

	source := &store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "initial implementation",
		Status:        "completed",
		Branch:        sourceBranch,
		WorktreePath:  sourceWtPath,
	}
	if err := s.CreateSpawn(source); err != nil {
		t.Fatalf("CreateSpawn(source): %v", err)
	}

	spawnID, err := o.Spawn(ctx, SpawnRequest{
		ParentTurnID:         2,
		ParentProfile:        "parent",
		ChildProfile:         "worker",
		Task:                 "qa pass",
		WorkspaceFromSpawnID: source.ID,
		Delegation:           &config.DelegationConfig{Profiles: []config.DelegationProfile{{Name: "worker"}}},
	})
	if err == nil {
		t.Fatalf("Spawn() error = nil, want missing-agent failure")
	}
	if !strings.Contains(err.Error(), "agent") {
		t.Fatalf("Spawn() error = %q, want missing-agent hint", err)
	}
	if spawnID == 0 {
		t.Fatalf("spawnID = 0, want non-zero")
	}

	rec, getErr := s.GetSpawn(spawnID)
	if getErr != nil {
		t.Fatalf("GetSpawn(%d): %v", spawnID, getErr)
	}
	if rec.WorkspaceFromSpawnID != source.ID {
		t.Fatalf("WorkspaceFromSpawnID = %d, want %d", rec.WorkspaceFromSpawnID, source.ID)
	}
	if rec.Branch == "" {
		t.Fatalf("spawn branch is empty, want populated branch")
	}
	if rec.Branch == source.Branch {
		t.Fatalf("spawn branch reused source branch %q; want an isolated branch", rec.Branch)
	}

	if !branchExists(repo, source.Branch) {
		t.Fatalf("source branch %q should still exist", source.Branch)
	}
}
