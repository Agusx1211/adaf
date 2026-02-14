package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

func (srv *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	project, err := srv.store.LoadProject()
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

func (srv *Server) handlePlans(w http.ResponseWriter, r *http.Request) {
	plans, err := srv.store.ListPlans()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}
	if plans == nil {
		plans = []store.Plan{}
	}
	writeJSON(w, http.StatusOK, plans)
}

func (srv *Server) handlePlanByID(w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.PathValue("id"))
	if planID == "" {
		writeError(w, http.StatusNotFound, "plan not found")
		return
	}

	plan, err := srv.store.GetPlan(planID)
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

func (srv *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.URL.Query().Get("plan"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	var (
		issues []store.Issue
		err    error
	)

	if planID != "" {
		issues, err = srv.store.ListIssuesForPlan(planID)
	} else {
		issues, err = srv.store.ListIssues()
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

func (srv *Server) handleIssueByID(w http.ResponseWriter, r *http.Request) {
	issueID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	issue, err := srv.store.GetIssue(issueID)
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

func (srv *Server) handleTurns(w http.ResponseWriter, r *http.Request) {
	turns, err := srv.store.ListTurns()
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

func (srv *Server) handleTurnByID(w http.ResponseWriter, r *http.Request) {
	turnID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "turn not found")
		return
	}

	turn, err := srv.store.GetTurn(turnID)
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

func (srv *Server) handleSpawns(w http.ResponseWriter, r *http.Request) {
	spawns, err := srv.store.ListSpawns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list spawns")
		return
	}
	if spawns == nil {
		spawns = []store.SpawnRecord{}
	}
	writeJSON(w, http.StatusOK, spawns)
}

func (srv *Server) handleSpawnByID(w http.ResponseWriter, r *http.Request) {
	spawnID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "spawn not found")
		return
	}

	rec, err := srv.store.GetSpawn(spawnID)
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

func (srv *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := session.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	if sessions == nil {
		sessions = []session.SessionMeta{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (srv *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
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

	for i := range sessions {
		if sessions[i].ID == sessionID {
			writeJSON(w, http.StatusOK, sessions[i])
			return
		}
	}

	writeError(w, http.StatusNotFound, "session not found")
}

func (srv *Server) handleLoopStats(w http.ResponseWriter, r *http.Request) {
	stats, err := srv.store.ListLoopStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list loop stats")
		return
	}
	if stats == nil {
		stats = []store.LoopStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func (srv *Server) handleProfileStats(w http.ResponseWriter, r *http.Request) {
	stats, err := srv.store.ListProfileStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list profile stats")
		return
	}
	if stats == nil {
		stats = []store.ProfileStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}
