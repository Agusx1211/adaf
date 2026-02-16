package webserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

func (srv *Server) handleStartAskSession(w http.ResponseWriter, r *http.Request) {
	handleStartAskSessionP(srv.store, w, r)
}

func handleStartAskSessionP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string   `json:"profile"`
		Prompt  string   `json:"prompt"`
		PlanID  string   `json:"plan_id"`
		Skills  []string `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Profile == "" {
		writeError(w, http.StatusBadRequest, "profile is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	prof := cfg.FindProfile(req.Profile)
	if prof == nil {
		writeError(w, http.StatusBadRequest, "profile not found")
		return
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}

	planID := req.PlanID
	if planID == "" {
		planID = projCfg.ActivePlanID
	}

	projDir := projectDir(s)

	// Build prompt: if user provided one, wrap it with project context;
	// otherwise use project context alone (standalone mode).
	instructions := strings.TrimSpace(req.Prompt)
	if instructions != "" {
		built, err := promptpkg.Build(promptpkg.BuildOpts{
			Store:   s,
			Project: projCfg,
			PlanID:  planID,
		})
		if err == nil {
			instructions = built + "\n\n# Task\n\n" + instructions + "\n"
		}
	} else {
		built, err := promptpkg.Build(promptpkg.BuildOpts{
			Store:   s,
			Project: projCfg,
			PlanID:  planID,
		})
		if err == nil {
			instructions = built
		} else {
			instructions = "Work on the project using the available context."
		}
	}

	loopDef := config.LoopDef{
		Name: "ask",
		Steps: []config.LoopStep{{
			Profile:      prof.Name,
			Turns:        1,
			Instructions: instructions,
			Skills:       req.Skills,
		}},
	}

	dcfg := session.DaemonConfig{
		ProjectDir:  projDir,
		ProjectName: projCfg.Name,
		WorkDir:     projDir,
		PlanID:      planID,
		ProfileName: prof.Name,
		AgentName:   prof.Agent,
		Loop:        loopDef,
		Profiles:    []config.Profile{*prof},
		MaxCycles:   1,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	if err := session.StartDaemon(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start daemon")
		return
	}

	meta, _ := session.LoadMeta(sessionID)
	writeJSON(w, http.StatusCreated, meta)
}

func (srv *Server) handleStartLoopSession(w http.ResponseWriter, r *http.Request) {
	handleStartLoopSessionP(srv.store, w, r)
}

func handleStartLoopSessionP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Loop          string `json:"loop"`
		PlanID        string `json:"plan_id"`
		InitialPrompt string `json:"initial_prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Loop == "" {
		writeError(w, http.StatusBadRequest, "loop is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	loopDef := cfg.FindLoop(req.Loop)
	if loopDef == nil {
		writeError(w, http.StatusBadRequest, "loop not found")
		return
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}

	planID := req.PlanID
	if planID == "" {
		planID = projCfg.ActivePlanID
	}

	projDir := projectDir(s)
	profiles := collectLoopProfiles(cfg, loopDef)

	// We need a main profile for the daemon config, pick the first step's profile
	var mainProf *config.Profile
	if len(loopDef.Steps) > 0 {
		mainProf = cfg.FindProfile(loopDef.Steps[0].Profile)
	}
	if mainProf == nil {
		writeError(w, http.StatusBadRequest, "invalid loop: no valid profile in first step")
		return
	}

	dcfg := session.DaemonConfig{
		ProjectDir:    projDir,
		ProjectName:   projCfg.Name,
		WorkDir:       projDir,
		PlanID:        planID,
		ProfileName:   mainProf.Name,
		AgentName:     mainProf.Agent,
		Loop:          *loopDef,
		Profiles:      profiles,
		InitialPrompt: strings.TrimSpace(req.InitialPrompt),
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	if err := session.StartDaemon(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start daemon")
		return
	}

	meta, _ := session.LoadMeta(sessionID)
	writeJSON(w, http.StatusCreated, meta)
}

func collectLoopProfiles(cfg *config.GlobalConfig, loopDef *config.LoopDef) []config.Profile {
	seen := map[string]bool{}
	var profiles []config.Profile
	for _, step := range loopDef.Steps {
		if !seen[step.Profile] {
			if p := cfg.FindProfile(step.Profile); p != nil {
				profiles = append(profiles, *p)
				seen[step.Profile] = true
			}
		}
		if step.Team != "" {
			if t := cfg.FindTeam(step.Team); t != nil && t.Delegation != nil {
				for _, dp := range t.Delegation.Profiles {
					if !seen[dp.Name] {
						if p := cfg.FindProfile(dp.Name); p != nil {
							profiles = append(profiles, *p)
							seen[dp.Name] = true
						}
					}
				}
			}
		}
	}
	return profiles
}

func (srv *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
	handleStopSessionP(srv.store, w, r)
}

func handleStopSessionP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	sessionID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	meta, err := session.LoadMeta(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// If project-scoped, verify the session belongs to this project
	if projectID := r.PathValue("projectID"); projectID != "" {
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

	if !session.IsActiveStatus(meta.Status) {
		writeError(w, http.StatusBadRequest, "session is not running")
		return
	}

	// Check if process is alive
	if err := syscall.Kill(meta.PID, 0); err != nil {
		// Process is already dead
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session_id": sessionID})
		return
	}

	if err := syscall.Kill(meta.PID, syscall.SIGTERM); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stop session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session_id": sessionID})
}

func (srv *Server) handleSessionMessage(w http.ResponseWriter, r *http.Request) {
	handleSessionMessageP(srv.store, w, r)
}

func handleSessionMessageP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	sessionID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Check if session exists (though we mainly need the active loop run)
	meta, err := session.LoadMeta(sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// If project-scoped, verify the session belongs to this project
	if projectID := r.PathValue("projectID"); projectID != "" {
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

	run, err := s.ActiveLoopRun()
	if err != nil {
		writeError(w, http.StatusNotFound, "no active loop run found")
		return
	}

	msg := &store.LoopMessage{
		RunID:     run.ID,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}
	if err := s.CreateLoopMessage(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleLoopRuns(w http.ResponseWriter, r *http.Request) {
	handleLoopRunsP(srv.store, w, r)
}

func handleLoopRunsP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	runs, err := s.ListLoopRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list loop runs")
		return
	}
	if runs == nil {
		runs = []store.LoopRun{}
	}
	backfillDaemonSessionIDs(runs)
	writeJSON(w, http.StatusOK, runs)
}

// backfillDaemonSessionIDs fills in DaemonSessionID for loop runs created
// before the field was added, by matching against daemon sessions via
// loop_name and start timestamp (within 2 seconds).
func backfillDaemonSessionIDs(runs []store.LoopRun) {
	// Collect runs that need backfill.
	needsBackfill := false
	for i := range runs {
		if runs[i].DaemonSessionID == 0 {
			needsBackfill = true
			break
		}
	}
	if !needsBackfill {
		return
	}

	sessions, err := session.ListSessions()
	if err != nil || len(sessions) == 0 {
		return
	}

	// Build a lookup: claimed daemon session IDs (already assigned).
	claimed := make(map[int]struct{})
	for _, run := range runs {
		if run.DaemonSessionID > 0 {
			claimed[run.DaemonSessionID] = struct{}{}
		}
	}

	for i := range runs {
		if runs[i].DaemonSessionID > 0 {
			continue
		}
		runStart := runs[i].StartedAt
		runLoop := runs[i].LoopName
		var bestSession *session.SessionMeta
		var bestDiff time.Duration
		for j := range sessions {
			sess := &sessions[j]
			if _, ok := claimed[sess.ID]; ok {
				continue
			}
			if sess.LoopName != runLoop {
				continue
			}
			diff := runStart.Sub(sess.StartedAt)
			if diff < 0 {
				diff = -diff
			}
			if diff <= 2*time.Second && (bestSession == nil || diff < bestDiff) {
				bestSession = sess
				bestDiff = diff
			}
		}
		if bestSession != nil {
			runs[i].DaemonSessionID = bestSession.ID
			claimed[bestSession.ID] = struct{}{}
		}
	}
}

func (srv *Server) handleLoopRunByID(w http.ResponseWriter, r *http.Request) {
	handleLoopRunByIDP(srv.store, w, r)
}

func handleLoopRunByIDP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	run, err := s.GetLoopRun(runID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "loop run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load loop run")
		return
	}

	if run.DaemonSessionID == 0 {
		runs := []store.LoopRun{*run}
		backfillDaemonSessionIDs(runs)
		*run = runs[0]
	}

	writeJSON(w, http.StatusOK, run)
}

func (srv *Server) handleLoopMessages(w http.ResponseWriter, r *http.Request) {
	handleLoopMessagesP(srv.store, w, r)
}

func handleLoopMessagesP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	msgs, err := s.ListLoopMessages(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	if msgs == nil {
		msgs = []store.LoopMessage{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (srv *Server) handleStopLoopRun(w http.ResponseWriter, r *http.Request) {
	handleStopLoopRunP(srv.store, w, r)
}

func handleStopLoopRunP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	if err := s.SignalLoopStop(runID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to signal stop")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleLoopRunMessage(w http.ResponseWriter, r *http.Request) {
	handleLoopRunMessageP(srv.store, w, r)
}

func handleLoopRunMessageP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	msg := &store.LoopMessage{
		RunID:     runID,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}
	if err := s.CreateLoopMessage(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
