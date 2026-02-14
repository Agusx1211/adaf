// store_issues.go contains issue management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) ListIssues() ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "issues")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var issues []Issue
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var issue Issue
		if err := s.readJSON(filepath.Join(dir, e.Name()), &issue); err != nil {
			continue
		}
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues, nil
}

func (s *Store) ListIssuesForPlan(planID string) ([]Issue, error) {
	if planID == "" {
		return s.ListSharedIssues()
	}
	issues, err := s.ListIssues()
	if err != nil {
		return nil, err
	}
	var filtered []Issue
	for _, issue := range issues {
		if issue.PlanID == "" || issue.PlanID == planID {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

func (s *Store) ListSharedIssues() ([]Issue, error) {
	issues, err := s.ListIssues()
	if err != nil {
		return nil, err
	}
	var filtered []Issue
	for _, issue := range issues {
		if issue.PlanID == "" {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

func (s *Store) CreateIssue(issue *Issue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	issue.ID = s.nextID(filepath.Join(s.root, "issues"))
	issue.Created = time.Now().UTC()
	issue.Updated = issue.Created
	if issue.Status == "" {
		issue.Status = "open"
	}
	filename := fmt.Sprintf("%d.json", issue.ID)
	if err := s.writeJSON(filepath.Join(s.root, "issues", filename), issue); err != nil {
		return err
	}
	
	// Auto-commit the created issue
	s.AutoCommit([]string{"issues/" + filename}, fmt.Sprintf("adaf: create issue #%d: %s", issue.ID, issue.Title))
	return nil
}

func (s *Store) GetIssue(id int) (*Issue, error) {
	var issue Issue
	if err := s.readJSON(filepath.Join(s.root, "issues", fmt.Sprintf("%d.json", id)), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (s *Store) UpdateIssue(issue *Issue) error {
	issue.Updated = time.Now().UTC()
	filename := fmt.Sprintf("%d.json", issue.ID)
	if err := s.writeJSON(filepath.Join(s.root, "issues", filename), issue); err != nil {
		return err
	}
	
	// Auto-commit the updated issue
	s.AutoCommit([]string{"issues/" + filename}, fmt.Sprintf("adaf: update issue #%d", issue.ID))
	return nil
}

func (s *Store) DeleteIssue(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%d.json", id)
	path := filepath.Join(s.root, "issues", filename)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("deleting issue %d: %w", id, err)
	}
	
	// Auto-commit the deletion
	s.AutoCommit([]string{"issues/" + filename}, fmt.Sprintf("adaf: delete issue #%d", id))
	return nil
}
