package webserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/store"
)

var (
	issueStatuses = map[string]struct{}{
		"open":        {},
		"in_progress": {},
		"resolved":    {},
		"wontfix":     {},
	}
	issuePriorities = map[string]struct{}{
		"critical": {},
		"high":     {},
		"medium":   {},
		"low":      {},
	}
	planStatuses = map[string]struct{}{
		"active":    {},
		"done":      {},
		"cancelled": {},
		"frozen":    {},
	}

	wikiSlugInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)
	wikiSlugMultiDash    = regexp.MustCompile(`-+`)
)

type issueWriteRequest struct {
	PlanID      string   `json:"plan_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
	DependsOn   []int    `json:"depends_on"`
}

type planWriteRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type wikiWriteRequest struct {
	ID        string `json:"id"`
	PlanID    string `json:"plan_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	UpdatedBy string `json:"updated_by"`
}

type wikiUpdateRequest struct {
	PlanID    *string `json:"plan_id"`
	Title     *string `json:"title"`
	Content   *string `json:"content"`
	UpdatedBy *string `json:"updated_by"`
}

type turnWriteRequest struct {
	Objective    string `json:"objective"`
	WhatWasBuilt string `json:"what_was_built"`
	KeyDecisions string `json:"key_decisions"`
	Challenges   string `json:"challenges"`
	CurrentState string `json:"current_state"`
	KnownIssues  string `json:"known_issues"`
	NextSteps    string `json:"next_steps"`
	BuildState   string `json:"build_state"`
	DurationSecs int    `json:"duration_secs"`
	CommitHash   string `json:"commit_hash"`
	AgentModel   string `json:"agent_model"`
	ProfileName  string `json:"profile_name"`
	PlanID       string `json:"plan_id"`
	Agent        string `json:"agent"`
	LoopRunHexID string `json:"loop_run_hex_id"`
	StepHexID    string `json:"step_hex_id"`
}

func handleCreateIssueP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req issueWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	status := normalizeLower(req.Status)
	if status == "" {
		status = "open"
	}
	if !isAllowedValue(status, issueStatuses) {
		writeError(w, http.StatusBadRequest, "invalid issue status")
		return
	}

	priority := normalizeLower(req.Priority)
	if priority == "" {
		priority = "medium"
	}
	if !isAllowedValue(priority, issuePriorities) {
		writeError(w, http.StatusBadRequest, "invalid issue priority")
		return
	}
	dependsOn, err := s.ValidateIssueDependencies(0, req.DependsOn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue dependencies")
		return
	}

	now := time.Now().UTC()
	issue := store.Issue{
		PlanID:      strings.TrimSpace(req.PlanID),
		Title:       title,
		Description: req.Description,
		Status:      status,
		Priority:    priority,
		Labels:      req.Labels,
		DependsOn:   dependsOn,
		Created:     now,
		Updated:     now,
	}
	if err := s.CreateIssue(&issue); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create issue")
		return
	}

	writeJSON(w, http.StatusCreated, issue)
}

func handleUpdateIssueP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	issue, err := s.GetIssue(id)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load issue")
		return
	}

	var req issueWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if title := strings.TrimSpace(req.Title); title != "" {
		issue.Title = title
	}
	if req.Description != "" {
		issue.Description = req.Description
	}
	if planID := strings.TrimSpace(req.PlanID); planID != "" {
		issue.PlanID = planID
	}
	if req.Labels != nil {
		issue.Labels = req.Labels
	}
	if req.DependsOn != nil {
		dependsOn, depErr := s.ValidateIssueDependencies(issue.ID, req.DependsOn)
		if depErr != nil {
			writeError(w, http.StatusBadRequest, "invalid issue dependencies")
			return
		}
		issue.DependsOn = dependsOn
	}
	if status := normalizeLower(req.Status); status != "" {
		if !isAllowedValue(status, issueStatuses) {
			writeError(w, http.StatusBadRequest, "invalid issue status")
			return
		}
		issue.Status = status
	}
	if priority := normalizeLower(req.Priority); priority != "" {
		if !isAllowedValue(priority, issuePriorities) {
			writeError(w, http.StatusBadRequest, "invalid issue priority")
			return
		}
		issue.Priority = priority
	}

	issue.Updated = time.Now().UTC()
	if err := s.UpdateIssue(issue); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update issue")
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func handleDeleteIssueP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	if err := s.DeleteIssue(id); err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete issue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleCreatePlanP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req planWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id := strings.TrimSpace(req.ID)
	title := strings.TrimSpace(req.Title)
	if id == "" || title == "" {
		writeError(w, http.StatusBadRequest, "id and title are required")
		return
	}

	now := time.Now().UTC()
	plan := store.Plan{
		ID:          id,
		Title:       title,
		Description: req.Description,
		Status:      "active",
		Created:     now,
		Updated:     now,
	}
	if err := s.CreatePlan(&plan); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			writeError(w, http.StatusBadRequest, "plan already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create plan")
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

func handleUpdatePlanP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.PathValue("id"))
	if planID == "" {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}

	plan, err := s.GetPlan(planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load plan")
		return
	}
	if plan == nil {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}

	var req planWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if title := strings.TrimSpace(req.Title); title != "" {
		plan.Title = title
	}
	if req.Description != "" {
		plan.Description = req.Description
	}
	if status := normalizeLower(req.Status); status != "" {
		if !isAllowedValue(status, planStatuses) {
			writeError(w, http.StatusBadRequest, "invalid plan status")
			return
		}
		plan.Status = status
	}

	plan.Updated = time.Now().UTC()
	if err := s.UpdatePlan(plan); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func handleActivatePlanP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.PathValue("id"))
	if planID == "" {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}

	if err := s.SetActivePlan(planID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to activate plan")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleDeletePlanP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.PathValue("id"))
	if planID == "" {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}

	plan, err := s.GetPlan(planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load plan")
		return
	}
	if plan == nil {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}

	if err := s.DeletePlan(planID); err != nil {
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "active") || strings.Contains(errText, "done/cancelled") || strings.Contains(errText, "status") {
			writeError(w, http.StatusBadRequest, "plan cannot be deleted")
			return
		}
		if isNotFoundErr(err) || strings.Contains(errText, "not found") {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete plan")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleWikiP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.URL.Query().Get("plan"))

	var (
		wiki []store.WikiEntry
		err  error
	)
	if planID == "" {
		wiki, err = s.ListWiki()
	} else {
		wiki, err = s.ListWikiForPlan(planID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list wiki")
		return
	}
	if wiki == nil {
		wiki = []store.WikiEntry{}
	}

	writeJSON(w, http.StatusOK, wiki)
}

func handleWikiSearchP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = parsedLimit
	}

	planID := strings.TrimSpace(r.URL.Query().Get("plan"))
	sharedRaw := normalizeLower(r.URL.Query().Get("shared"))
	sharedOnly := sharedRaw == "1" || sharedRaw == "true" || sharedRaw == "yes"

	results, err := s.SearchWiki(query, limit*4)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search wiki")
		return
	}

	filtered := make([]store.WikiEntry, 0, len(results))
	for _, entry := range results {
		if sharedOnly && entry.PlanID != "" {
			continue
		}
		if !sharedOnly && planID != "" && entry.PlanID != "" && entry.PlanID != planID {
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	writeJSON(w, http.StatusOK, filtered)
}

func handleWikiByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	wikiID := strings.TrimSpace(r.PathValue("id"))
	if wikiID == "" {
		writeError(w, http.StatusNotFound, "wiki entry not found")
		return
	}

	entry, err := s.GetWikiEntry(wikiID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "wiki entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load wiki entry")
		return
	}

	writeJSON(w, http.StatusOK, entry)
}

func handleCreateWikiP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req wikiWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	wikiID := strings.TrimSpace(req.ID)
	if wikiID == "" {
		wikiID = slugifyWikiID(title)
	}
	if wikiID == "" {
		writeError(w, http.StatusBadRequest, "unable to derive wiki id")
		return
	}

	now := time.Now().UTC()
	actor := strings.TrimSpace(req.UpdatedBy)
	if actor == "" {
		actor = "web-ui"
	}
	entry := store.WikiEntry{
		ID:        wikiID,
		PlanID:    strings.TrimSpace(req.PlanID),
		Title:     title,
		Content:   req.Content,
		Created:   now,
		Updated:   now,
		CreatedBy: actor,
		UpdatedBy: actor,
	}
	if err := s.CreateWikiEntry(&entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create wiki entry")
		return
	}

	writeJSON(w, http.StatusCreated, entry)
}

func handleUpdateWikiP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	wikiID := strings.TrimSpace(r.PathValue("id"))
	if wikiID == "" {
		writeError(w, http.StatusNotFound, "wiki entry not found")
		return
	}

	entry, err := s.GetWikiEntry(wikiID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "wiki entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load wiki entry")
		return
	}

	var req wikiUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PlanID != nil {
		entry.PlanID = strings.TrimSpace(*req.PlanID)
	}
	if req.Title != nil {
		if title := strings.TrimSpace(*req.Title); title != "" {
			entry.Title = title
		}
	}
	if req.Content != nil {
		entry.Content = *req.Content
	}
	actor := ""
	if req.UpdatedBy != nil {
		actor = strings.TrimSpace(*req.UpdatedBy)
	}
	if actor == "" {
		actor = "web-ui"
	}
	entry.UpdatedBy = actor

	entry.Updated = time.Now().UTC()
	if err := s.UpdateWikiEntry(entry); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update wiki entry")
		return
	}

	writeJSON(w, http.StatusOK, entry)
}

func handleDeleteWikiP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	wikiID := strings.TrimSpace(r.PathValue("id"))
	if wikiID == "" {
		writeError(w, http.StatusNotFound, "wiki entry not found")
		return
	}

	if err := s.DeleteWikiEntry(wikiID); err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "wiki entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete wiki entry")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func handleUpdateTurnP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	turnID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "turn not found")
		return
	}

	turn, err := s.GetTurn(turnID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "turn not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load turn")
		return
	}

	var req turnWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	turn.Objective = req.Objective
	turn.WhatWasBuilt = req.WhatWasBuilt
	turn.KeyDecisions = req.KeyDecisions
	turn.Challenges = req.Challenges
	turn.CurrentState = req.CurrentState
	turn.KnownIssues = req.KnownIssues
	turn.NextSteps = req.NextSteps
	turn.BuildState = req.BuildState
	turn.CommitHash = req.CommitHash
	turn.AgentModel = req.AgentModel
	turn.ProfileName = req.ProfileName
	turn.PlanID = strings.TrimSpace(req.PlanID)
	turn.Agent = strings.TrimSpace(req.Agent)
	turn.LoopRunHexID = req.LoopRunHexID
	turn.StepHexID = req.StepHexID
	turn.DurationSecs = req.DurationSecs

	if err := s.UpdateTurn(turn); err != nil {
		if errors.Is(err, store.ErrTurnFrozen) {
			writeError(w, http.StatusConflict, "turn is frozen")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update turn")
		return
	}

	writeJSON(w, http.StatusOK, turn)
}

func normalizeLower(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func isAllowedValue(value string, allowed map[string]struct{}) bool {
	_, ok := allowed[value]
	return ok
}

func slugifyWikiID(title string) string {
	slug := normalizeLower(title)
	slug = strings.Join(strings.Fields(slug), "-")
	slug = wikiSlugInvalidChars.ReplaceAllString(slug, "")
	slug = wikiSlugMultiDash.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}
