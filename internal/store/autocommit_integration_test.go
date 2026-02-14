package store

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// runGit runs a git command in the specified directory
func runGitIntegration(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return nil
}

// runGitOutput runs a git command and returns its output
func runGitOutputIntegration(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func TestAutoCommitIntegration(t *testing.T) {
	t.Run("issue creation triggers auto-commit", func(t *testing.T) {
		dir := t.TempDir()
		
		// Initialize a git repo
		if err := runGitIntegration(dir, "init"); err != nil {
			t.Fatal("failed to init git repo:", err)
		}
		if err := runGitIntegration(dir, "config", "user.email", "test@example.com"); err != nil {
			t.Fatal("failed to set git email:", err)
		}
		if err := runGitIntegration(dir, "config", "user.name", "Test User"); err != nil {
			t.Fatal("failed to set git name:", err)
		}
		
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
		output, err := runGitOutputIntegration(dir, "log", "--oneline", "-1")
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
		dir := t.TempDir()
		
		// Initialize a git repo
		if err := runGitIntegration(dir, "init"); err != nil {
			t.Fatal("failed to init git repo:", err)
		}
		if err := runGitIntegration(dir, "config", "user.email", "test@example.com"); err != nil {
			t.Fatal("failed to set git email:", err)
		}
		if err := runGitIntegration(dir, "config", "user.name", "Test User"); err != nil {
			t.Fatal("failed to set git name:", err)
		}
		
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
		output, err := runGitOutputIntegration(dir, "log", "--oneline", "-1")
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
	
	t.Run("doc creation triggers auto-commit", func(t *testing.T) {
		dir := t.TempDir()
		
		// Initialize a git repo
		if err := runGitIntegration(dir, "init"); err != nil {
			t.Fatal("failed to init git repo:", err)
		}
		if err := runGitIntegration(dir, "config", "user.email", "test@example.com"); err != nil {
			t.Fatal("failed to set git email:", err)
		}
		if err := runGitIntegration(dir, "config", "user.name", "Test User"); err != nil {
			t.Fatal("failed to set git name:", err)
		}
		
		// Create store
		s, err := New(dir)
		if err != nil {
			t.Fatal("failed to create store:", err)
		}
		
		// Initialize project
		if err := s.Init(ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
			t.Fatal("failed to init project:", err)
		}
		
		// Create a doc
		doc := &Doc{
			ID:      "test-doc",
			Title:   "Test Document",
			Content: "This is test content",
		}
		
		if err := s.CreateDoc(doc); err != nil {
			t.Fatal("failed to create doc:", err)
		}
		
		// Check that a commit was created
		output, err := runGitOutputIntegration(dir, "log", "--oneline", "-1")
		if err != nil {
			t.Fatal("failed to get git log:", err, string(output))
		}
		
		if len(output) == 0 {
			t.Fatal("no commit was created")
		}
		
		// Check commit message
		expectedMsg := "adaf: create doc test-doc"
		if !strings.Contains(string(output), expectedMsg) {
			t.Errorf("commit message = %q, want to contain %q", string(output), expectedMsg)
		}
	})
}