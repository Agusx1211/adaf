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

func IsOpenIssueStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "open", "in_progress":
		return true
	default:
		return false
	}
}

func IsTerminalIssueStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "resolved", "wontfix":
		return true
	default:
		return false
	}
}

func NormalizeIssueDependencyIDs(dependsOn []int) []int {
	if len(dependsOn) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(dependsOn))
	out := make([]int, 0, len(dependsOn))
	for _, id := range dependsOn {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Ints(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Store) ValidateIssueDependencies(issueID int, dependsOn []int) ([]int, error) {
	normalized := NormalizeIssueDependencyIDs(dependsOn)
	for _, depID := range normalized {
		if depID <= 0 {
			return nil, fmt.Errorf("invalid dependency issue ID %d", depID)
		}
		if issueID > 0 && depID == issueID {
			return nil, fmt.Errorf("issue #%d cannot depend on itself", issueID)
		}
	}

	issues, err := s.ListIssues()
	if err != nil {
		return nil, err
	}

	byID := make(map[int]Issue, len(issues))
	for _, iss := range issues {
		byID[iss.ID] = iss
	}
	for _, depID := range normalized {
		if _, ok := byID[depID]; !ok {
			return nil, fmt.Errorf("dependency issue #%d not found", depID)
		}
	}

	if issueID <= 0 || len(normalized) == 0 {
		return normalized, nil
	}

	graph := make(map[int][]int, len(byID)+1)
	for _, iss := range issues {
		if iss.ID == issueID {
			graph[issueID] = normalized
			continue
		}
		graph[iss.ID] = NormalizeIssueDependencyIDs(iss.DependsOn)
	}
	if _, ok := graph[issueID]; !ok {
		graph[issueID] = normalized
	}

	visiting := make(map[int]bool, len(graph))
	visited := make(map[int]bool, len(graph))
	var visit func(int) bool
	visit = func(id int) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}

		visiting[id] = true
		for _, depID := range graph[id] {
			if visit(depID) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	if visit(issueID) {
		return nil, fmt.Errorf("dependency cycle detected for issue #%d", issueID)
	}

	return normalized, nil
}
