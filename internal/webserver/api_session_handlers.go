package webserver

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

func (srv *Server) handleStartAskSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
		Prompt  string `json:"prompt"`
		PlanID  string `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Profile == "" || req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "profile and prompt are required")
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

	projCfg, err := srv.store.LoadProject()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}

	planID := req.PlanID
	if planID == "" {
		planID = projCfg.ActivePlanID
	}

	projectDir := filepath.Dir(srv.store.Root())

	loopDef := config.LoopDef{
		Name: "ask",
		Steps: []config.LoopStep{{
			Profile:      prof.Name,
			Turns:        1,
			Instructions: req.Prompt,
		}},
	}

	dcfg := session.DaemonConfig{
		ProjectDir:  projectDir,
		ProjectName: projCfg.Name,
		WorkDir:     projectDir,
		PlanID:      planID,
		ProfileName: prof.Name,
		AgentName:   prof.Agent,
		Loop:        loopDef,
		Profiles:    []config.Profile{*prof},
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
	var req struct {
		Loop   string `json:"loop"`
		PlanID string `json:"plan_id"`
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

	projCfg, err := srv.store.LoadProject()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}

	planID := req.PlanID
	if planID == "" {
		planID = projCfg.ActivePlanID
	}

	projectDir := filepath.Dir(srv.store.Root())
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
		ProjectDir:  projectDir,
		ProjectName: projCfg.Name,
		WorkDir:     projectDir,
		PlanID:      planID,
		ProfileName: mainProf.Name,
		AgentName:   mainProf.Agent,
		Loop:        *loopDef,
		Profiles:    profiles,
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
		if step.Delegation != nil {
			for _, dp := range step.Delegation.Profiles {
				if !seen[dp.Name] {
					if p := cfg.FindProfile(dp.Name); p != nil {
						profiles = append(profiles, *p)
						seen[dp.Name] = true
					}
				}
			}
		}
	}
	return profiles
}

func (srv *Server) handleStopSession(w http.ResponseWriter, r *http.Request) {
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
	if _, err := session.LoadMeta(sessionID); err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	run, err := srv.store.ActiveLoopRun()
	if err != nil {
		writeError(w, http.StatusNotFound, "no active loop run found")
		return
	}

	msg := &store.LoopMessage{
		RunID:     run.ID,
		Content:   req.Content,
		CreatedAt: time.Now(),
	}
	if err := srv.store.CreateLoopMessage(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleLoopRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := srv.store.ListLoopRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list loop runs")
		return
	}
	if runs == nil {
		runs = []store.LoopRun{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (srv *Server) handleLoopRunByID(w http.ResponseWriter, r *http.Request) {
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	run, err := srv.store.GetLoopRun(runID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "loop run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load loop run")
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (srv *Server) handleLoopMessages(w http.ResponseWriter, r *http.Request) {
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	msgs, err := srv.store.ListLoopMessages(runID)
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
	runID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "loop run not found")
		return
	}

	if err := srv.store.SignalLoopStop(runID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to signal stop")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleLoopRunMessage(w http.ResponseWriter, r *http.Request) {
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
	if err := srv.store.CreateLoopMessage(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to send message")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
