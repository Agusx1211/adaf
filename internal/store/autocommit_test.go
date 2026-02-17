package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGitOutput runs a git command and returns its output
func runGitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func TestAutoCommit(t *testing.T) {
	t.Run("creates commit in git repo", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init store:", err)
		}

		// Create a test file
		testFile := filepath.Join(s.Root(), "test.json")
		testContent := []byte(`{"test": "data"}`)
		if err := os.WriteFile(testFile, testContent, 0644); err != nil {
			t.Fatal("failed to write test file:", err)
		}

		// Call AutoCommit
		s.AutoCommit([]string{"test.json"}, "test commit message")

		// Check that a commit was created
		output, err := runGitOutput(s.Root(), "log", "--oneline", "-1")
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
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store without init/marker -> no git repo to commit into.
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Call AutoCommit (should not fail)
		s.AutoCommit([]string{"test.json"}, "test commit message")

		// Should not have created any git files in the project dir.
		if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
			t.Error("git directory was created when it shouldn't be")
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init store:", err)
		}

		// Create a test file
		testFile := filepath.Join(s.Root(), "test.json")
		testContent := []byte(`{"test": "data"}`)
		if err := os.WriteFile(testFile, testContent, 0644); err != nil {
			t.Fatal("failed to write test file:", err)
		}

		// Call AutoCommit twice
		beforeLog, err := runGitOutput(s.Root(), "log", "--oneline")
		if err != nil {
			t.Fatal("failed to get pre-commit git log:", err, string(beforeLog))
		}
		beforeCommits := countLines(string(beforeLog))

		s.AutoCommit([]string{"test.json"}, "first commit message")
		s.AutoCommit([]string{"test.json"}, "second commit message")

		// Check that only one commit was created
		output, err := runGitOutput(s.Root(), "log", "--oneline")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		// Count commits (exactly one new commit should be created)
		commits := countLines(string(output))
		if commits != beforeCommits+1 {
			t.Errorf("expected %d commits, got %d", beforeCommits+1, commits)
		}
	})

	t.Run("handles file deletion", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init store:", err)
		}

		// Create a test file
		testFile := filepath.Join(s.Root(), "test.json")
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
		output, err := runGitOutput(s.Root(), "log", "--oneline", "-1")
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
