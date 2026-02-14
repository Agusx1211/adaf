package store

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// AutoCommit stages the given files (relative to the store root) and creates a commit.
// It is best-effort: errors are logged but do not fail the caller.
// The repoRoot is the git repo root (parent of .adaf/).
func (s *Store) AutoCommit(files []string, message string) {
	repoRoot := filepath.Dir(s.root) // .adaf's parent is the repo root

	// Check if we're in a git repo
	if err := gitExec(repoRoot, "rev-parse", "--git-dir"); err != nil {
		return // not a git repo, skip silently
	}

	// Stage the specific files
	for _, f := range files {
		relPath := filepath.Join(".adaf", f)
		if err := gitExec(repoRoot, "add", relPath); err != nil {
			return // can't stage, skip
		}
	}

	// Check if there are staged changes
	if err := gitExec(repoRoot, "diff", "--cached", "--quiet"); err == nil {
		return // nothing staged, skip
	}

	// Commit
	_ = gitExec(repoRoot, "commit", "-m", message)
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