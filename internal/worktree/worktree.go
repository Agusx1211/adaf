// Package worktree manages git worktrees for isolated sub-agent execution.
package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const worktreeDir = ".adaf-worktrees"

// WorktreeInfo describes an active git worktree.
type WorktreeInfo struct {
	Path   string
	Branch string
}

// Manager handles creation, merging, and cleanup of git worktrees.
type Manager struct {
	repoRoot string
}

// NewManager creates a Manager rooted at the given git repository root.
func NewManager(repoRoot string) *Manager {
	return &Manager{repoRoot: repoRoot}
}

// sanitizeBranch replaces characters not safe for directory names.
var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func sanitize(s string) string {
	return unsafeChars.ReplaceAllString(s, "_")
}

// BranchName builds a conventional branch name for a spawn.
func BranchName(parentSession int, childProfile string) string {
	ts := time.Now().UTC().Format("20060102T150405")
	return fmt.Sprintf("adaf/%d/%s/%s", parentSession, sanitize(childProfile), ts)
}

// Create creates a new worktree for the given branch.
// It returns the worktree path on disk.
func (m *Manager) Create(ctx context.Context, branchName string) (string, error) {
	base := filepath.Join(m.repoRoot, worktreeDir)
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("creating worktree dir: %w", err)
	}

	wtPath := filepath.Join(base, sanitize(branchName))

	// Get HEAD commit.
	head, err := m.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("rev-parse HEAD: %w", err)
	}
	head = strings.TrimSpace(head)

	// Create branch at HEAD.
	if _, err := m.git(ctx, "branch", branchName, head); err != nil {
		return "", fmt.Errorf("creating branch %s: %w", branchName, err)
	}

	// Create worktree.
	if _, err := m.git(ctx, "worktree", "add", wtPath, branchName); err != nil {
		// Rollback branch on failure.
		m.git(ctx, "branch", "-D", branchName)
		return "", fmt.Errorf("worktree add: %w", err)
	}

	// Symlink .adaf/ from the main repo into the worktree so sub-agents share the store.
	adafSrc := filepath.Join(m.repoRoot, ".adaf")
	adafDst := filepath.Join(wtPath, ".adaf")
	if _, err := os.Stat(adafSrc); err == nil {
		os.Remove(adafDst) // remove if exists
		if err := os.Symlink(adafSrc, adafDst); err != nil {
			// Non-fatal: sub-agent will still work, just without shared store.
			fmt.Fprintf(os.Stderr, "warning: failed to symlink .adaf into worktree: %v\n", err)
		}
	}

	return wtPath, nil
}

// Remove removes a worktree and optionally deletes its branch.
func (m *Manager) Remove(ctx context.Context, wtPath string, deleteBranch bool) error {
	// Remove symlinked .adaf first to avoid git complaints.
	adafLink := filepath.Join(wtPath, ".adaf")
	if info, err := os.Lstat(adafLink); err == nil && info.Mode()&os.ModeSymlink != 0 {
		os.Remove(adafLink)
	}

	if _, err := m.git(ctx, "worktree", "remove", "--force", wtPath); err != nil {
		// Fallback: manual cleanup.
		os.RemoveAll(wtPath)
		m.git(ctx, "worktree", "prune")
	}

	if deleteBranch {
		// Determine branch from worktree path is not reliable, caller should pass branch.
		// This is a best-effort operation.
	}
	return nil
}

// RemoveWithBranch removes a worktree and its branch.
func (m *Manager) RemoveWithBranch(ctx context.Context, wtPath, branchName string) error {
	if err := m.Remove(ctx, wtPath, false); err != nil {
		return err
	}
	if branchName != "" {
		m.git(ctx, "branch", "-D", branchName)
	}
	return nil
}

// Merge merges the given branch into the current branch with a merge commit.
func (m *Manager) Merge(ctx context.Context, branchName, message string) (string, error) {
	if message == "" {
		message = "Merge " + branchName
	}
	if _, err := m.git(ctx, "merge", "--no-ff", "-m", message, branchName); err != nil {
		return "", fmt.Errorf("merge %s: %w", branchName, err)
	}
	hash, err := m.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(hash), nil
}

// MergeSquash squash-merges the given branch into the current branch.
func (m *Manager) MergeSquash(ctx context.Context, branchName, message string) (string, error) {
	if _, err := m.git(ctx, "merge", "--squash", branchName); err != nil {
		return "", fmt.Errorf("squash-merge %s: %w", branchName, err)
	}
	if message == "" {
		message = "Squash merge " + branchName
	}
	if _, err := m.git(ctx, "commit", "-m", message); err != nil {
		return "", fmt.Errorf("commit squash: %w", err)
	}
	hash, err := m.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(hash), nil
}

// Diff returns the diff between the current branch and the given branch.
func (m *Manager) Diff(ctx context.Context, branchName string) (string, error) {
	out, err := m.git(ctx, "diff", "HEAD..."+branchName)
	if err != nil {
		return "", err
	}
	return out, nil
}

// ListActive returns all active worktrees under .adaf-worktrees/.
func (m *Manager) ListActive(ctx context.Context) ([]WorktreeInfo, error) {
	out, err := m.git(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	base := filepath.Join(m.repoRoot, worktreeDir)
	var result []WorktreeInfo
	var current WorktreeInfo
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" && strings.HasPrefix(current.Path, base) {
				result = append(result, current)
			}
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	if current.Path != "" && strings.HasPrefix(current.Path, base) {
		result = append(result, current)
	}
	return result, nil
}

// CleanupAll removes all adaf-managed worktrees. Used for crash recovery.
func (m *Manager) CleanupAll(ctx context.Context) error {
	active, err := m.ListActive(ctx)
	if err != nil {
		return err
	}
	for _, wt := range active {
		m.RemoveWithBranch(ctx, wt.Path, wt.Branch)
	}
	// Clean up the worktree directory itself.
	base := filepath.Join(m.repoRoot, worktreeDir)
	os.RemoveAll(base)
	m.git(ctx, "worktree", "prune")
	return nil
}

// git runs a git command in the repo root and returns combined output.
func (m *Manager) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return string(out), nil
}
