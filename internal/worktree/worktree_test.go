package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoCommitIfDirty_CommitsChanges(t *testing.T) {
	repo := initGitRepo(t)
	mgr := NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T000000"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, wtPath, branch)

	target := filepath.Join(wtPath, "main.txt")
	if err := os.WriteFile(target, []byte("updated\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	hash, committed, err := mgr.AutoCommitIfDirty(ctx, wtPath, "test auto-commit")
	if err != nil {
		t.Fatalf("AutoCommitIfDirty: %v", err)
	}
	if !committed {
		t.Fatalf("committed = false, want true")
	}
	if hash == "" {
		t.Fatalf("hash is empty")
	}

	head := strings.TrimSpace(gitOutput(t, repo, "rev-parse", branch))
	if head != hash {
		t.Fatalf("branch head = %s, want %s", head, hash)
	}

	status := strings.TrimSpace(gitOutput(t, wtPath, "status", "--porcelain"))
	if status != "" {
		t.Fatalf("worktree should be clean after auto-commit, status=%q", status)
	}
}

func TestAutoCommitIfDirty_NoChanges(t *testing.T) {
	repo := initGitRepo(t)
	mgr := NewManager(repo)
	ctx := context.Background()

	branch := "adaf/test/worker/20260212T000001"
	wtPath, err := mgr.Create(ctx, branch)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer mgr.RemoveWithBranch(ctx, wtPath, branch)

	hash, committed, err := mgr.AutoCommitIfDirty(ctx, wtPath, "test auto-commit")
	if err != nil {
		t.Fatalf("AutoCommitIfDirty: %v", err)
	}
	if committed {
		t.Fatalf("committed = true, want false")
	}
	if hash != "" {
		t.Fatalf("hash = %q, want empty", hash)
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
