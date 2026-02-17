package store

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// AutoCommit stages the given files (relative to the store root) and creates a commit.
// It is best-effort: errors are logged but do not fail the caller.
// Commits are recorded in the project's global store git repository.
func (s *Store) AutoCommit(files []string, message string) {
	repoRoot := strings.TrimSpace(s.root)
	if repoRoot == "" {
		return
	}

	// Check if we're in a git repo
	if err := gitExec(repoRoot, "rev-parse", "--git-dir"); err != nil {
		return // not a git repo, skip silently
	}

	// Stage the specific files
	for _, f := range files {
		relPath := strings.TrimSpace(f)
		if relPath == "" {
			continue
		}
		if err := gitExec(repoRoot, "add", "-A", "--", relPath); err != nil {
			return // can't stage, skip
		}
	}

	// Check if there are staged changes
	if err := gitExec(repoRoot, "diff", "--cached", "--quiet"); err == nil {
		return // nothing staged, skip
	}

	// Commit with a stable fallback identity so auto-commit works even when
	// user-level git identity is not configured.
	_ = gitExec(repoRoot,
		"-c", "user.name=ADAF",
		"-c", "user.email=adaf@local",
		"commit", "-m", message,
	)
}

// ensureStoreGitRepo initializes a git repository in the store root if needed.
func (s *Store) ensureStoreGitRepo() error {
	root := strings.TrimSpace(s.root)
	if root == "" {
		return nil
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	if err := gitExec(root, "rev-parse", "--git-dir"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	if err := gitExec(root, "init"); err != nil {
		return err
	}
	return nil
}

func gitExec(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return nil
}
