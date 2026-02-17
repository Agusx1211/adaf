package store

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// runGitOutput runs a git command and returns its output
func runGitOutputIntegration(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func TestAutoCommitIntegration(t *testing.T) {
	t.Run("issue creation triggers auto-commit", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Initialize project
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init project:", err)
		}

		// Create an issue
		issue := &Issue{
			Title:       "Test Issue",
			Description: "This is a test issue",
			Status:      "open",
			Priority:    "medium",
		}

		if err := s.CreateIssue(issue); err != nil {
			t.Fatal("failed to create issue:", err)
		}

		// Check that a commit was created
		output, err := runGitOutputIntegration(s.Root(), "log", "--oneline", "-1")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		if len(output) == 0 {
			t.Fatal("no commit was created")
		}

		// Check commit message contains issue ID and title
		expectedMsg := "adaf: create issue #" + fmt.Sprintf("%d", issue.ID)
		if !strings.Contains(string(output), expectedMsg) {
			t.Errorf("commit message = %q, want to contain %q", string(output), expectedMsg)
		}
		if !strings.Contains(string(output), issue.Title) {
			t.Errorf("commit message = %q, want to contain %q", string(output), issue.Title)
		}
	})

	t.Run("plan creation triggers auto-commit", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Initialize project
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init project:", err)
		}

		// Create a plan
		plan := &Plan{
			ID:          "test-plan",
			Title:       "Test Plan",
			Description: "This is a test plan",
			Status:      "active",
		}

		if err := s.CreatePlan(plan); err != nil {
			t.Fatal("failed to create plan:", err)
		}

		// Check that a commit was created
		output, err := runGitOutputIntegration(s.Root(), "log", "--oneline", "-1")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		if len(output) == 0 {
			t.Fatal("no commit was created")
		}

		// Check commit message
		expectedMsg := "adaf: create plan test-plan"
		if !strings.Contains(string(output), expectedMsg) {
			t.Errorf("commit message = %q, want to contain %q", string(output), expectedMsg)
		}
	})

	t.Run("wiki creation triggers auto-commit", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()

		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}

		// Initialize project
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init project:", err)
		}

		// Create a wiki entry
		entry := &WikiEntry{
			ID:      "test-wiki",
			Title:   "Test Wiki",
			Content: "This is test content",
		}
		entry.CreatedBy = "test-agent"
		entry.UpdatedBy = "test-agent"

		if err := s.CreateWikiEntry(entry); err != nil {
			t.Fatal("failed to create wiki entry:", err)
		}

		// Check that a commit was created
		output, err := runGitOutputIntegration(s.Root(), "log", "--oneline", "-1")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}

		if len(output) == 0 {
			t.Fatal("no commit was created")
		}

		// Check commit message
		expectedMsg := "adaf: create wiki test-wiki"
		if !strings.Contains(string(output), expectedMsg) {
			t.Errorf("commit message = %q, want to contain %q", string(output), expectedMsg)
		}
	})
}
