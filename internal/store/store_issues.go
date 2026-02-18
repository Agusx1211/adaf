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

const (
	IssueStatusOpen     = "open"
	IssueStatusOngoing  = "ongoing"
	IssueStatusInReview = "in_review"
	IssueStatusClosed   = "closed"
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
	now := time.Now().UTC()
	normalizeIssueForCreate(issue, now)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%d.json", issue.ID)
	path := filepath.Join(s.root, "issues", filename)

	var existing Issue
	if err := s.readJSON(path, &existing); err != nil {
		return err
	}

	now := time.Now().UTC()
	normalizeIssueForUpdate(issue, &existing, now)

	changes := issueChangeHistory(existing, *issue)
	if len(changes) > 0 {
		nextHistoryID := nextIssueHistoryID(issue.History)
		actor := resolveIssueActor(issue.UpdatedBy, issue.CreatedBy)
		for _, ch := range changes {
			ch.ID = nextHistoryID
			ch.By = actor
			ch.At = now
			issue.History = append(issue.History, ch)
			nextHistoryID++
		}
	}

	if err := s.writeJSON(path, issue); err != nil {
		return err
	}

	// Auto-commit the updated issue
	s.AutoCommit([]string{"issues/" + filename}, fmt.Sprintf("adaf: update issue #%d", issue.ID))
	return nil
}

func (s *Store) AddIssueComment(issueID int, body, by string) (*Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("%d.json", issueID)
	path := filepath.Join(s.root, "issues", filename)

	var issue Issue
	if err := s.readJSON(path, &issue); err != nil {
		return nil, err
	}

	text := strings.TrimSpace(body)
	if text == "" {
		return nil, fmt.Errorf("comment body is required")
	}

	now := time.Now().UTC()
	actor := resolveIssueActor(by, issue.UpdatedBy, issue.CreatedBy)
	issue.Comments = normalizeIssueComments(issue.Comments, issue.Created)

	comment := IssueComment{
		ID:      nextIssueCommentID(issue.Comments),
		Body:    text,
		By:      actor,
		Created: now,
		Updated: now,
	}
	issue.Comments = append(issue.Comments, comment)
	issue.Updated = now
	issue.UpdatedBy = actor
	if issue.CreatedBy == "" {
		issue.CreatedBy = actor
	}
	issue.History = normalizeIssueHistory(issue.History, issue.Created, actor)

	nextHistoryID := nextIssueHistoryID(issue.History)
	issue.History = append(issue.History, IssueHistory{
		ID:        nextHistoryID,
		Type:      "commented",
		CommentID: comment.ID,
		Message:   previewIssueText(comment.Body, 120),
		By:        actor,
		At:        now,
	})

	if err := s.writeJSON(path, &issue); err != nil {
		return nil, err
	}

	s.AutoCommit([]string{"issues/" + filename}, fmt.Sprintf("adaf: comment issue #%d", issueID))
	return &issue, nil
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
	switch NormalizeIssueStatus(status) {
	case IssueStatusOpen, IssueStatusOngoing, IssueStatusInReview:
		return true
	default:
		return false
	}
}

func IsTerminalIssueStatus(status string) bool {
	switch NormalizeIssueStatus(status) {
	case IssueStatusClosed:
		return true
	default:
		return false
	}
}

func IsValidIssueStatus(status string) bool {
	switch NormalizeIssueStatus(status) {
	case IssueStatusOpen, IssueStatusOngoing, IssueStatusInReview, IssueStatusClosed:
		return true
	default:
		return false
	}
}

func NormalizeIssueStatus(status string) string {
	s := strings.TrimSpace(strings.ToLower(status))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "_")
	switch s {
	case "open":
		return IssueStatusOpen
	case "ongoing", "in_progress", "wip", "working":
		return IssueStatusOngoing
	case "in_review", "review", "reviewing":
		return IssueStatusInReview
	case "closed", "done", "complete", "completed", "resolved", "fixed":
		return IssueStatusClosed
	default:
		return s
	}
}

func IsValidIssuePriority(priority string) bool {
	switch NormalizeIssuePriority(priority) {
	case "critical", "high", "medium", "low":
		return true
	default:
		return false
	}
}

func NormalizeIssuePriority(priority string) string {
	p := strings.TrimSpace(strings.ToLower(priority))
	if p == "" {
		return "medium"
	}
	switch p {
	case "critical", "high", "medium", "low":
		return p
	default:
		return p
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

func normalizeIssueForCreate(issue *Issue, now time.Time) {
	if issue == nil {
		return
	}

	issue.Status = NormalizeIssueStatus(issue.Status)
	if issue.Status == "" {
		issue.Status = IssueStatusOpen
	}
	issue.Priority = NormalizeIssuePriority(issue.Priority)
	issue.Labels = normalizeIssueLabels(issue.Labels)
	issue.DependsOn = NormalizeIssueDependencyIDs(issue.DependsOn)
	issue.PlanID = strings.TrimSpace(issue.PlanID)
	issue.Title = strings.TrimSpace(issue.Title)
	issue.Description = strings.TrimSpace(issue.Description)
	issue.Created = now
	issue.Updated = now

	actor := resolveIssueActor(issue.CreatedBy, issue.UpdatedBy)
	if actor != "" {
		issue.CreatedBy = actor
		issue.UpdatedBy = actor
	}

	issue.Comments = normalizeIssueComments(issue.Comments, issue.Created)
	issue.History = normalizeIssueHistory(issue.History, issue.Created, actor)
}

func normalizeIssueForUpdate(issue *Issue, existing *Issue, now time.Time) {
	if issue == nil {
		return
	}
	if existing == nil {
		normalizeIssueForCreate(issue, now)
		return
	}

	issue.ID = existing.ID
	issue.Status = NormalizeIssueStatus(issue.Status)
	if issue.Status == "" {
		issue.Status = NormalizeIssueStatus(existing.Status)
	}
	if issue.Status == "" {
		issue.Status = IssueStatusOpen
	}
	issue.Priority = NormalizeIssuePriority(issue.Priority)
	issue.Labels = normalizeIssueLabels(issue.Labels)
	issue.DependsOn = NormalizeIssueDependencyIDs(issue.DependsOn)
	issue.PlanID = strings.TrimSpace(issue.PlanID)
	issue.Title = strings.TrimSpace(issue.Title)
	issue.Description = strings.TrimSpace(issue.Description)
	issue.Created = existing.Created
	issue.Updated = now

	actor := resolveIssueActor(issue.UpdatedBy, issue.CreatedBy, existing.UpdatedBy, existing.CreatedBy)
	issue.CreatedBy = resolveIssueActor(issue.CreatedBy, existing.CreatedBy, actor)
	issue.UpdatedBy = actor

	if issue.Comments == nil {
		issue.Comments = append([]IssueComment(nil), existing.Comments...)
	}
	issue.Comments = normalizeIssueComments(issue.Comments, issue.Created)

	baseHistory := existing.History
	if issue.History != nil && len(issue.History) > len(baseHistory) {
		baseHistory = issue.History
	}
	issue.History = normalizeIssueHistory(baseHistory, issue.Created, issue.CreatedBy)
}

func normalizeIssueLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, label)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeIssueComments(comments []IssueComment, fallback time.Time) []IssueComment {
	if len(comments) == 0 {
		return nil
	}

	nextID := 1
	out := make([]IssueComment, 0, len(comments))
	for _, comment := range comments {
		comment.Body = strings.TrimSpace(comment.Body)
		if comment.Body == "" {
			continue
		}

		if comment.ID <= 0 {
			comment.ID = nextID
		}
		if comment.ID >= nextID {
			nextID = comment.ID + 1
		}
		if comment.Created.IsZero() {
			comment.Created = fallback
		}
		if comment.Created.IsZero() {
			comment.Created = time.Now().UTC()
		}
		comment.Created = comment.Created.UTC()
		if comment.Updated.IsZero() {
			comment.Updated = comment.Created
		}
		comment.Updated = comment.Updated.UTC()
		comment.By = strings.TrimSpace(comment.By)
		out = append(out, comment)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func normalizeIssueHistory(history []IssueHistory, createdAt time.Time, actor string) []IssueHistory {
	out := make([]IssueHistory, 0, len(history)+1)
	nextID := 1

	for _, item := range history {
		item.Type = strings.TrimSpace(item.Type)
		if item.Type == "" {
			continue
		}
		if item.ID <= 0 {
			item.ID = nextID
		}
		if item.ID >= nextID {
			nextID = item.ID + 1
		}
		if item.At.IsZero() {
			item.At = createdAt
		}
		if item.At.IsZero() {
			item.At = time.Now().UTC()
		}
		item.At = item.At.UTC()
		item.By = resolveIssueActor(item.By, actor)
		item.Field = strings.TrimSpace(item.Field)
		item.From = strings.TrimSpace(item.From)
		item.To = strings.TrimSpace(item.To)
		item.Message = strings.TrimSpace(item.Message)
		out = append(out, item)
	}

	if len(out) == 0 {
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		out = []IssueHistory{{
			ID:   1,
			Type: "created",
			By:   actor,
			At:   createdAt.UTC(),
		}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func issueChangeHistory(before, after Issue) []IssueHistory {
	changes := make([]IssueHistory, 0, 8)

	if strings.TrimSpace(before.Status) != strings.TrimSpace(after.Status) {
		changes = append(changes, IssueHistory{
			Type:  "status_changed",
			Field: "status",
			From:  before.Status,
			To:    after.Status,
		})
	}
	if strings.TrimSpace(before.PlanID) != strings.TrimSpace(after.PlanID) {
		changes = append(changes, IssueHistory{
			Type:  "moved",
			Field: "plan_id",
			From:  issuePlanScopeValue(before.PlanID),
			To:    issuePlanScopeValue(after.PlanID),
		})
	}
	if strings.TrimSpace(before.Title) != strings.TrimSpace(after.Title) {
		changes = append(changes, IssueHistory{
			Type:  "updated",
			Field: "title",
			From:  previewIssueText(before.Title, 120),
			To:    previewIssueText(after.Title, 120),
		})
	}
	if strings.TrimSpace(before.Description) != strings.TrimSpace(after.Description) {
		changes = append(changes, IssueHistory{
			Type:  "updated",
			Field: "description",
			From:  previewIssueText(before.Description, 120),
			To:    previewIssueText(after.Description, 120),
		})
	}
	if strings.TrimSpace(before.Priority) != strings.TrimSpace(after.Priority) {
		changes = append(changes, IssueHistory{
			Type:  "updated",
			Field: "priority",
			From:  before.Priority,
			To:    after.Priority,
		})
	}

	beforeLabels := normalizeIssueLabels(before.Labels)
	afterLabels := normalizeIssueLabels(after.Labels)
	if !equalStringSlice(beforeLabels, afterLabels) {
		changes = append(changes, IssueHistory{
			Type:  "updated",
			Field: "labels",
			From:  formatIssueLabels(beforeLabels),
			To:    formatIssueLabels(afterLabels),
		})
	}

	beforeDeps := NormalizeIssueDependencyIDs(before.DependsOn)
	afterDeps := NormalizeIssueDependencyIDs(after.DependsOn)
	if !equalIntSlice(beforeDeps, afterDeps) {
		changes = append(changes, IssueHistory{
			Type:  "updated",
			Field: "depends_on",
			From:  formatIssueDependsOn(beforeDeps),
			To:    formatIssueDependsOn(afterDeps),
		})
	}

	return changes
}

func resolveIssueActor(candidates ...string) string {
	for _, raw := range candidates {
		value := strings.TrimSpace(raw)
		if value != "" {
			return value
		}
	}
	return ""
}

func nextIssueCommentID(comments []IssueComment) int {
	maxID := 0
	for _, comment := range comments {
		if comment.ID > maxID {
			maxID = comment.ID
		}
	}
	return maxID + 1
}

func nextIssueHistoryID(history []IssueHistory) int {
	maxID := 0
	for _, item := range history {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID + 1
}

func issuePlanScopeValue(planID string) string {
	if strings.TrimSpace(planID) == "" {
		return "shared"
	}
	return strings.TrimSpace(planID)
}

func formatIssueLabels(labels []string) string {
	if len(labels) == 0 {
		return "-"
	}
	return strings.Join(labels, ", ")
}

func formatIssueDependsOn(dependsOn []int) string {
	if len(dependsOn) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(dependsOn))
	for _, id := range dependsOn {
		if id <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("#%d", id))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func previewIssueText(raw string, max int) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "-"
	}
	if max <= 0 {
		max = 120
	}
	if max < 4 {
		max = 4
	}
	if len(value) <= max {
		return value
	}
	return value[:max-3] + "..."
}

func equalStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalIntSlice(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
