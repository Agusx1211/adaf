package webserver

import (
	"encoding/json"
	"net/http"
	"regexp"
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
	planPhaseStatuses = map[string]struct{}{
		"not_started": {},
		"in_progress": {},
		"complete":    {},
		"blocked":     {},
	}

	docSlugInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)
	docSlugMultiDash    = regexp.MustCompile(`-+`)
)

type issueWriteRequest struct {
	PlanID      string   `json:"plan_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
}

type planWriteRequest struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Phases      []store.PlanPhase `json:"phases"`
}

type planPhaseWriteRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    *int     `json:"priority"`
	DependsOn   []string `json:"depends_on"`
}

type docWriteRequest struct {
	ID      string `json:"id"`
	PlanID  string `json:"plan_id"`
	Title   string `json:"title"`
	Content string `json:"content"`
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

func (srv *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	handleCreateIssueP(srv.store, w, r)
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

	now := time.Now().UTC()
	issue := store.Issue{
		PlanID:      strings.TrimSpace(req.PlanID),
		Title:       title,
		Description: req.Description,
		Status:      status,
		Priority:    priority,
		Labels:      req.Labels,
		Created:     now,
		Updated:     now,
	}
	if err := s.CreateIssue(&issue); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create issue")
		return
	}

	writeJSON(w, http.StatusCreated, issue)
}

func (srv *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	handleUpdateIssueP(srv.store, w, r)
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

func (srv *Server) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	handleDeleteIssueP(srv.store, w, r)
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

func (srv *Server) handleCreatePlan(w http.ResponseWriter, r *http.Request) {
	handleCreatePlanP(srv.store, w, r)
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
		Phases:      req.Phases,
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

func (srv *Server) handleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	handleUpdatePlanP(srv.store, w, r)
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
	if req.Phases != nil {
		plan.Phases = req.Phases
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

func (srv *Server) handleUpdatePlanPhase(w http.ResponseWriter, r *http.Request) {
	handleUpdatePlanPhaseP(srv.store, w, r)
}

func handleUpdatePlanPhaseP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.PathValue("id"))
	phaseID := strings.TrimSpace(r.PathValue("phaseId"))
	if planID == "" || phaseID == "" {
		writeError(w, http.StatusNotFound, "plan phase not found")
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

	phaseIndex := -1
	for i := range plan.Phases {
		if plan.Phases[i].ID == phaseID {
			phaseIndex = i
			break
		}
	}
	if phaseIndex < 0 {
		writeError(w, http.StatusNotFound, "phase not found")
		return
	}

	var req planPhaseWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	phase := &plan.Phases[phaseIndex]
	if title := strings.TrimSpace(req.Title); title != "" {
		phase.Title = title
	}
	if req.Description != "" {
		phase.Description = req.Description
	}
	if status := normalizeLower(req.Status); status != "" {
		if !isAllowedValue(status, planPhaseStatuses) {
			writeError(w, http.StatusBadRequest, "invalid phase status")
			return
		}
		phase.Status = status
	}
	if req.Priority != nil {
		phase.Priority = *req.Priority
	}
	if req.DependsOn != nil {
		phase.DependsOn = req.DependsOn
	}

	plan.Updated = time.Now().UTC()
	if err := s.UpdatePlan(plan); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (srv *Server) handleActivatePlan(w http.ResponseWriter, r *http.Request) {
	handleActivatePlanP(srv.store, w, r)
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

func (srv *Server) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	handleDeletePlanP(srv.store, w, r)
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

func (srv *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	handleDocsP(srv.store, w, r)
}

func handleDocsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.URL.Query().Get("plan"))

	var (
		docs []store.Doc
		err  error
	)
	if planID == "" {
		docs, err = s.ListDocs()
	} else {
		docs, err = s.ListDocsForPlan(planID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list docs")
		return
	}
	if docs == nil {
		docs = []store.Doc{}
	}

	writeJSON(w, http.StatusOK, docs)
}

func (srv *Server) handleDocByID(w http.ResponseWriter, r *http.Request) {
	handleDocByIDP(srv.store, w, r)
}

func handleDocByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	docID := strings.TrimSpace(r.PathValue("id"))
	if docID == "" {
		writeError(w, http.StatusNotFound, "doc not found")
		return
	}

	doc, err := s.GetDoc(docID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "doc not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load doc")
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

func (srv *Server) handleCreateDoc(w http.ResponseWriter, r *http.Request) {
	handleCreateDocP(srv.store, w, r)
}

func handleCreateDocP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req docWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	docID := strings.TrimSpace(req.ID)
	if docID == "" {
		docID = slugifyDocID(title)
	}
	if docID == "" {
		writeError(w, http.StatusBadRequest, "unable to derive document id")
		return
	}

	now := time.Now().UTC()
	doc := store.Doc{
		ID:      docID,
		PlanID:  strings.TrimSpace(req.PlanID),
		Title:   title,
		Content: req.Content,
		Created: now,
		Updated: now,
	}
	if err := s.CreateDoc(&doc); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create doc")
		return
	}

	writeJSON(w, http.StatusCreated, doc)
}

func (srv *Server) handleUpdateDoc(w http.ResponseWriter, r *http.Request) {
	handleUpdateDocP(srv.store, w, r)
}

func handleUpdateDocP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	docID := strings.TrimSpace(r.PathValue("id"))
	if docID == "" {
		writeError(w, http.StatusNotFound, "doc not found")
		return
	}

	doc, err := s.GetDoc(docID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "doc not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load doc")
		return
	}

	var req docWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if planID := strings.TrimSpace(req.PlanID); planID != "" {
		doc.PlanID = planID
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		doc.Title = title
	}
	if req.Content != "" {
		doc.Content = req.Content
	}

	doc.Updated = time.Now().UTC()
	if err := s.UpdateDoc(doc); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update doc")
		return
	}

	writeJSON(w, http.StatusOK, doc)
}

func (srv *Server) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	handleDeleteDocP(srv.store, w, r)
}

func handleDeleteDocP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	docID := strings.TrimSpace(r.PathValue("id"))
	if docID == "" {
		writeError(w, http.StatusNotFound, "doc not found")
		return
	}

	if err := s.DeleteDoc(docID); err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "doc not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete doc")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (srv *Server) handleUpdateTurn(w http.ResponseWriter, r *http.Request) {
	handleUpdateTurnP(srv.store, w, r)
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

func slugifyDocID(title string) string {
	slug := normalizeLower(title)
	slug = strings.Join(strings.Fields(slug), "-")
	slug = docSlugInvalidChars.ReplaceAllString(slug, "")
	slug = docSlugMultiDash.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}
