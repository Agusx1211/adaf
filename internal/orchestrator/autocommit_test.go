package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/worktree"
)

func TestAutoCommitSpawnWork_AddsCommitAndNotice(t *testing.T) {
	repo := initGitRepo(t)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T010000"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, wtPath, branch)

	if err := os.WriteFile(filepath.Join(wtPath, "main.txt"), []byte("updated\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	o := &Orchestrator{worktrees: mgr}
	rec := &store.SpawnRecord{
		ID:           42,
		ChildProfile: "worker",
		Branch:       branch,
		WorktreePath: wtPath,
	}

	note, err := o.autoCommitSpawnWork(rec)
	if err != nil {
		t.Fatalf("autoCommitSpawnWork: %v", err)
	}
	if !strings.Contains(note, "auto-commit: child left uncommitted changes") {
		t.Fatalf("note = %q, want auto-commit notice", note)
	}

	mainHash := strings.TrimSpace(gitOutput(t, repo, "rev-parse", "main"))
	branchHash := strings.TrimSpace(gitOutput(t, repo, "rev-parse", branch))
	if mainHash == branchHash {
		t.Fatalf("branch hash = main hash (%s), want a new commit on child branch", mainHash)
	}
}

func TestAutoCommitSpawnWork_NoOpWhenClean(t *testing.T) {
	repo := initGitRepo(t)
	mgr := worktree.NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T010001"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, wtPath, branch)

	o := &Orchestrator{worktrees: mgr}
	rec := &store.SpawnRecord{
		ID:           43,
		ChildProfile: "worker",
		Branch:       branch,
		WorktreePath: wtPath,
	}

	note, err := o.autoCommitSpawnWork(rec)
	if err != nil {
		t.Fatalf("autoCommitSpawnWork: %v", err)
	}
	if note != "" {
		t.Fatalf("note = %q, want empty", note)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	runGit(t, repo, "init")
	runGit(t, repo, "checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(repo, "main.txt"), []byte("initial\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runGit(t, repo, "add", "main.txt")
	runGitWithConfig(t, repo, []string{"user.name=Test", "user.email=test@example.com"}, "commit", "-m", "initial commit")
	return repo
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = gitOutput(t, dir, args...)
}

func runGitWithConfig(t *testing.T, dir string, config []string, args ...string) {
	t.Helper()
	fullArgs := make([]string, 0, len(config)*2+len(args))
	for _, kv := range config {
		fullArgs = append(fullArgs, "-c", kv)
	}
	fullArgs = append(fullArgs, args...)
	runGit(t, dir, fullArgs...)
}
