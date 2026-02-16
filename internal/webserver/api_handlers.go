package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		debug.LogKV("webserver", "failed to encode json response", "status", status, "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func parsePathID(raw string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func isNotFoundErr(err error) bool {
	return os.IsNotExist(err)
}

// --- Multi-project management endpoints ---

// handleListProjects returns all registered projects.
func (srv *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	entries := srv.registry.List()
	writeJSON(w, http.StatusOK, entries)
}

// globalDashboardResponse is the aggregate view across all projects.
type globalDashboardResponse struct {
	Projects []projectSummary `json:"projects"`
}

type projectSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	IsDefault      bool   `json:"is_default"`
	ActivePlanID   string `json:"active_plan_id,omitempty"`
	OpenIssueCount int    `json:"open_issue_count"`
	PlanCount      int    `json:"plan_count"`
	TurnCount      int    `json:"turn_count"`
}

// handleGlobalDashboard returns an aggregate view across all projects.
func (srv *Server) handleGlobalDashboard(w http.ResponseWriter, r *http.Request) {
	entries := srv.registry.List()
	summaries := make([]projectSummary, 0, len(entries))

	for _, entry := range entries {
		summary := projectSummary{
			ID:        entry.ID,
			Name:      entry.Name,
			Path:      entry.Path,
			IsDefault: entry.IsDefault,
		}

		if cfg, err := entry.store.LoadProject(); err == nil {
			summary.ActivePlanID = cfg.ActivePlanID
		}

		if plans, err := entry.store.ListPlans(); err == nil {
			summary.PlanCount = len(plans)
		}

		if issues, err := entry.store.ListIssues(); err == nil {
			for _, issue := range issues {
				if strings.EqualFold(issue.Status, "open") {
					summary.OpenIssueCount++
				}
			}
		}

		if turns, err := entry.store.ListTurns(); err == nil {
			summary.TurnCount = len(turns)
		}

		summaries = append(summaries, summary)
	}

	writeJSON(w, http.StatusOK, globalDashboardResponse{Projects: summaries})
}

// --- Project-scoped handlers (store-parameterized) ---

func handleProjectP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	project, err := s.LoadProject()
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func handlePlansP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	plans, err := s.ListPlans()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	if plans == nil {
		plans = []store.Plan{}
	}
	writeJSON(w, http.StatusOK, plans)
}

func handlePlanByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, plan)
}

func handleIssuesP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.URL.Query().Get("plan"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	var (
		issues []store.Issue
		err    error
	)

	if planID != "" {
		issues, err = s.ListIssuesForPlan(planID)
	} else {
		issues, err = s.ListIssues()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}

	if status != "" {
		filtered := make([]store.Issue, 0, len(issues))
		for i := range issues {
			if strings.EqualFold(issues[i].Status, status) {
				filtered = append(filtered, issues[i])
			}
		}
		issues = filtered
	}

	if issues == nil {
		issues = []store.Issue{}
	}
	writeJSON(w, http.StatusOK, issues)
}

func handleIssueByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	issueID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	issue, err := s.GetIssue(issueID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load issue")
		return
	}

	writeJSON(w, http.StatusOK, issue)
}

func handleTurnsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	turns, err := s.ListTurns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list turns")
		return
	}

	limitText := strings.TrimSpace(r.URL.Query().Get("limit"))
	if limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil || limit < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if limit > 0 && len(turns) > limit {
			turns = turns[len(turns)-limit:]
		}
	}

	if turns == nil {
		turns = []store.Turn{}
	}
	writeJSON(w, http.StatusOK, turns)
}

func handleTurnByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, turn)
}

func handleTurnRecordingEventsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "turn not found")
		return
	}

	// Try store recording first (keyed by store turn ID).
	eventsPath := filepath.Join(s.RecordsDirs()[0], fmt.Sprintf("%d", id), "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil && os.IsNotExist(err) {
		// Fall back to the session daemon events file (keyed by session ID).
		// Session IDs and store turn IDs use independent counters, so the
		// frontend may pass a session ID that has no matching store recording.
		sessionEventsPath := session.EventsPath(id)
		data, err = os.ReadFile(sessionEventsPath)
	}
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "no recording found for this turn")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read recording")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func handleSessionRecordingEventsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// For project-scoped routes, ensure the session belongs to this project.
	if projectID := r.PathValue("projectID"); projectID != "" {
		meta, err := session.LoadMeta(id)
		if err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}

		expectedProjectID := session.ProjectIDFromDir(projectDir(s))
		match := false
		if meta.ProjectID == expectedProjectID {
			match = true
		} else if meta.ProjectID == "" && meta.ProjectDir != "" {
			if session.ProjectIDFromDir(meta.ProjectDir) == expectedProjectID {
				match = true
			}
		}

		if !match {
			writeError(w, http.StatusNotFound, "session not found in this project")
			return
		}
	}

	data, err := os.ReadFile(session.EventsPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "no recording found for this session")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read recording")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// enrichedSpawn extends SpawnRecord with a computed ParentSpawnID for tree rendering.
type enrichedSpawn struct {
	store.SpawnRecord
	ParentSpawnID int `json:"parent_spawn_id,omitempty"`
	// ParentDaemonSessionID maps this spawn to the owning daemon session
	// used by websocket/session-scoped filtering in the UI.
	ParentDaemonSessionID int `json:"parent_daemon_session_id,omitempty"`
	ChildDaemonSessionID  int `json:"child_daemon_session_id,omitempty"`
}

func enrichSpawns(spawns []store.SpawnRecord, turnToDaemonSession map[int]int) []enrichedSpawn {
	// Build ChildTurnID -> SpawnID map so we can resolve ParentSpawnID.
	turnToSpawn := make(map[int]int, len(spawns))
	for _, rec := range spawns {
		if rec.ChildTurnID > 0 {
			turnToSpawn[rec.ChildTurnID] = rec.ID
		}
	}
	result := make([]enrichedSpawn, len(spawns))
	for i, rec := range spawns {
		parentSpawnID := 0
		if rec.ParentTurnID > 0 {
			parentSpawnID = turnToSpawn[rec.ParentTurnID]
		}
		result[i] = enrichedSpawn{
			SpawnRecord:           rec,
			ParentSpawnID:         parentSpawnID,
			ParentDaemonSessionID: turnToDaemonSession[rec.ParentTurnID],
			ChildDaemonSessionID:  turnToDaemonSession[rec.ChildTurnID],
		}
	}
	return result
}

func handleSpawnsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	spawns, err := s.ListSpawns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list spawns")
		return
	}
	if spawns == nil {
		spawns = []store.SpawnRecord{}
	}

	turnToDaemonSession := make(map[int]int)
	loopRuns, err := s.ListLoopRuns()
	if err == nil {
		backfillDaemonSessionIDs(loopRuns)
		for _, run := range loopRuns {
			if run.DaemonSessionID <= 0 {
				continue
			}
			for _, turnID := range run.TurnIDs {
				if turnID > 0 {
					turnToDaemonSession[turnID] = run.DaemonSessionID
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, enrichSpawns(spawns, turnToDaemonSession))
}

func handleSpawnByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	spawnID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "spawn not found")
		return
	}

	rec, err := s.GetSpawn(spawnID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "spawn not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load spawn")
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func handleSessionsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	sessions, err := session.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	if sessions == nil {
		sessions = []session.SessionMeta{}
	}

	// Filter by project if projectID is present in the URL (project-scoped route)
	if projectID := r.PathValue("projectID"); projectID != "" {
		expectedProjectID := session.ProjectIDFromDir(projectDir(s))
		filtered := make([]session.SessionMeta, 0)
		for _, sess := range sessions {
			// Match by ProjectID or derive it from ProjectDir for older sessions
			if sess.ProjectID == expectedProjectID {
				filtered = append(filtered, sess)
			} else if sess.ProjectID == "" && sess.ProjectDir != "" {
				if session.ProjectIDFromDir(sess.ProjectDir) == expectedProjectID {
					filtered = append(filtered, sess)
				}
			}
		}
		sessions = filtered
	}

	writeJSON(w, http.StatusOK, sessions)
}

func handleSessionByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	sessionID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	sessions, err := session.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	var found *session.SessionMeta
	for i := range sessions {
		if sessions[i].ID == sessionID {
			found = &sessions[i]
			break
		}
	}

	if found == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// If project-scoped, verify the session belongs to this project
	if projectID := r.PathValue("projectID"); projectID != "" {
		expectedProjectID := session.ProjectIDFromDir(projectDir(s))
		match := false
		if found.ProjectID == expectedProjectID {
			match = true
		} else if found.ProjectID == "" && found.ProjectDir != "" {
			if session.ProjectIDFromDir(found.ProjectDir) == expectedProjectID {
				match = true
			}
		}

		if !match {
			writeError(w, http.StatusNotFound, "session not found in this project")
			return
		}
	}

	writeJSON(w, http.StatusOK, *found)
}

func handleLoopStatsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	stats, err := s.ListLoopStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list loop stats")
		return
	}
	if stats == nil {
		stats = []store.LoopStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func handleProfileStatsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	stats, err := s.ListProfileStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list profile stats")
		return
	}
	if stats == nil {
		stats = []store.ProfileStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// projectDir derives the project root directory from a store.
func projectDir(s *store.Store) string {
	return filepath.Dir(s.Root())
}
