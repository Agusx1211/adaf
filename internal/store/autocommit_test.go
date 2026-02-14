package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit runs a git command in the specified directory
func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return nil
}

// runGitOutput runs a git command and returns its output
func runGitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func TestAutoCommit(t *testing.T) {
	t.Run("creates commit in git repo", func(t *testing.T) {
		dir := t.TempDir()

		// Initialize a git repo
		if err := runGit(dir, "init"); err != nil {
			t.Fatal("failed to init git repo:", err)
		}
		if err := runGit(dir, "config", "user.email", "test@example.com"); err != nil {
			t.Fatal("failed to set git email:", err)
		}
		if err := runGit(dir, "config", "user.name", "Test User"); err != nil {
			t.Fatal("failed to set git name:", err)
		}

		// Create .adaf directory
		adafDir := filepath.Join(dir, ".adaf")
		if err := os.MkdirAll(adafDir, 0755); err != nil {
			t.Fatal("failed to create .adaf dir:", err)
		}

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Create a test file
		testFile := filepath.Join(adafDir, "test.json")
		testContent := []byte(`{"test": "data"}`)
		if err := os.WriteFile(testFile, testContent, 0644); err != nil {
			t.Fatal("failed to write test file:", err)
		}

		// Call AutoCommit
		s.AutoCommit([]string{"test.json"}, "test commit message")

		// Check that a commit was created
		output, err := runGitOutput(dir, "log", "--oneline", "-1")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		if len(output) == 0 {
			t.Fatal("no commit was created")
		}

		// Check commit message
		expectedMsg := "test commit message"
		if !strings.Contains(string(output), expectedMsg) {
			t.Errorf("commit message = %q, want to contain %q", string(output), expectedMsg)
		}
	})

	t.Run("is silent when not in git repo", func(t *testing.T) {
		dir := t.TempDir()

		// Create .adaf directory
		adafDir := filepath.Join(dir, ".adaf")
		if err := os.MkdirAll(adafDir, 0755); err != nil {
			t.Fatal("failed to create .adaf dir:", err)
		}

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Call AutoCommit (should not fail)
		s.AutoCommit([]string{"test.json"}, "test commit message")

		// Should not have created any git files
		if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
			t.Error("git directory was created when it shouldn't be")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		dir := t.TempDir()

		// Initialize a git repo
		if err := runGit(dir, "init"); err != nil {
			t.Fatal("failed to init git repo:", err)
		}
		if err := runGit(dir, "config", "user.email", "test@example.com"); err != nil {
			t.Fatal("failed to set git email:", err)
		}
		if err := runGit(dir, "config", "user.name", "Test User"); err != nil {
			t.Fatal("failed to set git name:", err)
		}

		// Create .adaf directory
		adafDir := filepath.Join(dir, ".adaf")
		if err := os.MkdirAll(adafDir, 0755); err != nil {
			t.Fatal("failed to create .adaf dir:", err)
		}

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Create a test file
		testFile := filepath.Join(adafDir, "test.json")
		testContent := []byte(`{"test": "data"}`)
		if err := os.WriteFile(testFile, testContent, 0644); err != nil {
			t.Fatal("failed to write test file:", err)
		}

		// Call AutoCommit twice
		s.AutoCommit([]string{"test.json"}, "first commit message")
		s.AutoCommit([]string{"test.json"}, "second commit message")

		// Check that only one commit was created
		output, err := runGitOutput(dir, "log", "--oneline")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		// Count commits (should be 1, not 2)
		commits := countLines(string(output))
		if commits != 1 {
			t.Errorf("expected 1 commit, got %d", commits)
		}
	})

	t.Run("handles file deletion", func(t *testing.T) {
		dir := t.TempDir()

		// Initialize a git repo
		if err := runGit(dir, "init"); err != nil {
			t.Fatal("failed to init git repo:", err)
		}
		if err := runGit(dir, "config", "user.email", "test@example.com"); err != nil {
			t.Fatal("failed to set git email:", err)
		}
		if err := runGit(dir, "config", "user.name", "Test User"); err != nil {
			t.Fatal("failed to set git name:", err)
		}

		// Create .adaf directory
		adafDir := filepath.Join(dir, ".adaf")
		if err := os.MkdirAll(adafDir, 0755); err != nil {
			t.Fatal("failed to create .adaf dir:", err)
		}

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Create a test file
		testFile := filepath.Join(adafDir, "test.json")
		testContent := []byte(`{"test": "data"}`)
		if err := os.WriteFile(testFile, testContent, 0644); err != nil {
			t.Fatal("failed to write test file:", err)
		}

		// Call AutoCommit to add the file
		s.AutoCommit([]string{"test.json"}, "add test file")

		// Delete the file
		if err := os.Remove(testFile); err != nil {
			t.Fatal("failed to remove test file:", err)
		}

		// Call AutoCommit to stage the deletion
		s.AutoCommit([]string{"test.json"}, "remove test file")

		// Check that a commit was created for the deletion
		output, err := runGitOutput(dir, "log", "--oneline", "-1")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		if len(output) == 0 {
			t.Fatal("no commit was created for deletion")
		}

		// Check commit message
		expectedMsg := "remove test file"
		if !strings.Contains(string(output), expectedMsg) {
			t.Errorf("commit message = %q, want to contain %q", string(output), expectedMsg)
		}
	})
}

// Helper functions

func countLines(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}
