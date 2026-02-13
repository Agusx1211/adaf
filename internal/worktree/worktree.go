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
	"sync/atomic"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
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

var branchNonce atomic.Uint64

// BranchName builds a conventional branch name for a spawn.
func BranchName(parentSession int, childProfile string) string {
	now := time.Now().UTC()
	ts := fmt.Sprintf("%s%09d", now.Format("20060102T150405"), now.Nanosecond())
	nonce := branchNonce.Add(1)
	return fmt.Sprintf("adaf/%d/%s/%s-%d", parentSession, sanitize(childProfile), ts, nonce)
}

// CreateDetached creates a worktree with a detached HEAD at the current commit.
// Used for read-only spawns that need an isolated working directory without a branch.
func (m *Manager) CreateDetached(ctx context.Context, name string) (string, error) {
	base := filepath.Join(m.repoRoot, worktreeDir)
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("creating worktree dir: %w", err)
	}

	wtPath := filepath.Join(base, sanitize(name))

	if _, err := m.git(ctx, "worktree", "add", "--detach", wtPath); err != nil {
		return "", fmt.Errorf("worktree add --detach: %w", err)
	}

	// Symlink .adaf/ so sub-agents share the store.
	m.symlinkAdaf(wtPath)

	return wtPath, nil
}

// Create creates a new worktree for the given branch.
// It returns the worktree path on disk.
func (m *Manager) Create(ctx context.Context, branchName string) (string, error) {
	debug.LogKV("worktree", "Create()", "branch", branchName, "repo_root", m.repoRoot)
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

	// Symlink .adaf/ so sub-agents share the store.
	m.symlinkAdaf(wtPath)

	debug.LogKV("worktree", "created", "branch", branchName, "path", wtPath, "head", strings.TrimSpace(head))
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
		if removeErr := os.RemoveAll(wtPath); removeErr != nil {
			m.git(ctx, "worktree", "prune")
			return fmt.Errorf("worktree remove failed (%w) and manual cleanup also failed: %v", err, removeErr)
		}
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
	debug.LogKV("worktree", "Merge()", "branch", branchName)
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

// AutoCommitIfDirty stages and commits all changes in a worktree when needed.
// It returns (commitHash, committed, error). If there are no changes, committed=false.
func (m *Manager) AutoCommitIfDirty(ctx context.Context, worktreePath, message string) (string, bool, error) {
	debug.LogKV("worktree", "AutoCommitIfDirty()", "path", worktreePath)
	if strings.TrimSpace(worktreePath) == "" {
		return "", false, fmt.Errorf("worktree path is empty")
	}

	status, err := m.git(ctx, "-C", worktreePath, "status", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("status in worktree %s: %w", worktreePath, err)
	}
	if strings.TrimSpace(status) == "" {
		return "", false, nil
	}

	if _, err := m.git(ctx, "-C", worktreePath, "add", "-A"); err != nil {
		return "", false, fmt.Errorf("staging changes in worktree %s: %w", worktreePath, err)
	}

	// Ignore user-level git identity settings and use a stable local identity for fallback commits.
	commitArgs := []string{
		"-C", worktreePath,
		"-c", "user.name=ADAF",
		"-c", "user.email=adaf@local",
		"commit", "-m", message,
	}
	if _, err := m.git(ctx, commitArgs...); err != nil {
		return "", false, fmt.Errorf("auto-commit in worktree %s: %w", worktreePath, err)
	}

	hash, err := m.git(ctx, "-C", worktreePath, "rev-parse", "HEAD")
	if err != nil {
		return "", false, fmt.Errorf("rev-parse HEAD in worktree %s: %w", worktreePath, err)
	}
	return strings.TrimSpace(hash), true, nil
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
		if err := m.RemoveWithBranch(ctx, wt.Path, wt.Branch); err != nil {
			debug.LogKV("worktree", "CleanupAll: remove failed", "path", wt.Path, "branch", wt.Branch, "error", err)
		}
	}
	// Clean up the worktree directory itself.
	base := filepath.Join(m.repoRoot, worktreeDir)
	os.RemoveAll(base)
	m.git(ctx, "worktree", "prune")
	return nil
}

// CleanupStale removes worktrees that are older than maxAge or whose paths
// match entries in the deadPaths set (e.g. worktrees belonging to completed/failed spawns).
// It is safe to call on every startup.
func (m *Manager) CleanupStale(ctx context.Context, maxAge time.Duration, deadPaths map[string]bool) (removed int, _ error) {
	active, err := m.ListActive(ctx)
	if err != nil {
		return 0, err
	}

	for _, wt := range active {
		shouldRemove := deadPaths[wt.Path]

		if !shouldRemove && maxAge > 0 {
			if info, err := os.Stat(wt.Path); err == nil {
				if time.Since(info.ModTime()) > maxAge {
					shouldRemove = true
				}
			}
		}

		if !shouldRemove {
			continue
		}

		if err := m.RemoveWithBranch(ctx, wt.Path, wt.Branch); err != nil {
			debug.LogKV("worktree", "CleanupStale: remove failed", "path", wt.Path, "branch", wt.Branch, "error", err)
		}
		removed++
	}

	if removed > 0 {
		m.git(ctx, "worktree", "prune")
	}
	return removed, nil
}

func (m *Manager) symlinkAdaf(wtPath string) {
	adafSrc := filepath.Join(m.repoRoot, ".adaf")
	adafDst := filepath.Join(wtPath, ".adaf")
	if _, err := os.Stat(adafSrc); err == nil {
		os.Remove(adafDst)
		if err := os.Symlink(adafSrc, adafDst); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to symlink .adaf into worktree: %v\n", err)
		}
	}
}

// git runs a git command in the repo root and returns combined output.
func (m *Manager) git(ctx context.Context, args ...string) (string, error) {
	debug.LogKV("worktree", "git exec", "cmd", "git "+strings.Join(args, " "), "dir", m.repoRoot)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		debug.LogKV("worktree", "git exec failed", "cmd", "git "+strings.Join(args, " "), "error", err, "output_len", len(out))
		return string(out), fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	debug.LogKV("worktree", "git exec ok", "cmd", "git "+strings.Join(args, " "), "output_len", len(out))
	return string(out), nil
}
