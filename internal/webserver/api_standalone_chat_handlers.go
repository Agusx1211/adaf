package webserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

func handleGetStandaloneChatP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	if profile == "" {
		writeError(w, http.StatusBadRequest, "profile is required")
		return
	}

	messages, err := s.ListStandaloneChatMessages(profile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chat messages")
		return
	}
	if messages == nil {
		messages = []store.StandaloneChatMessage{}
	}
	writeJSON(w, http.StatusOK, messages)
}

func handleClearStandaloneChatP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	if profile == "" {
		writeError(w, http.StatusBadRequest, "profile is required")
		return
	}

	if err := s.ClearStandaloneChatMessages(profile); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear chat")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleSendStandaloneChatMessageP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	profileParam := r.PathValue("profile")
	if profileParam == "" {
		writeError(w, http.StatusBadRequest, "profile is required")
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Message is optional for standalone — empty means "begin working"

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	sp := cfg.FindStandaloneProfile(profileParam)
	if sp == nil {
		writeError(w, http.StatusBadRequest, "standalone profile not found: "+profileParam)
		return
	}

	prof := cfg.FindProfile(sp.Profile)
	if prof == nil {
		writeError(w, http.StatusBadRequest, "referenced profile not found: "+sp.Profile)
		return
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}

	projDir := projectDir(s)

	// Save user message if provided
	var userMsg *store.StandaloneChatMessage
	if req.Message != "" {
		userMsg = &store.StandaloneChatMessage{
			Role:    "user",
			Content: req.Message,
		}
		if err := s.CreateStandaloneChatMessage(profileParam, userMsg); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save message")
			return
		}
	}

	// Check if we can resume a previous agent session for this profile chat.
	var resumeSessionID string
	if lastSID := s.ReadStandaloneChatLastSession(profileParam); lastSID > 0 {
		resumeSessionID = session.ReadAgentSessionID(lastSID)
	}

	// When resuming, just pass the user message directly — the agent already
	// has the full context from its previous session.
	var fullPrompt string
	if resumeSessionID != "" {
		fullPrompt = req.Message
	} else {
		fullPrompt = buildStandaloneChatPrompt(sp, req.Message)
	}

	step := config.LoopStep{
		Profile:        prof.Name,
		Turns:          1,
		Instructions:   fullPrompt,
		StandaloneChat: true,
	}
	if sp.Delegation != nil {
		step.Delegation = sp.Delegation
	}

	loopDef := config.LoopDef{
		Name:  "standalone-chat",
		Steps: []config.LoopStep{step},
	}

	// Collect all profiles (main + delegation profiles)
	allProfiles := []config.Profile{*prof}
	if sp.Delegation != nil {
		for _, dp := range sp.Delegation.Profiles {
			if p := cfg.FindProfile(dp.Name); p != nil {
				allProfiles = append(allProfiles, *p)
			}
		}
	}

	dcfg := session.DaemonConfig{
		ProjectDir:      projDir,
		ProjectName:     projCfg.Name,
		WorkDir:         projDir,
		ProfileName:     prof.Name,
		AgentName:       prof.Agent,
		Loop:            loopDef,
		Profiles:        allProfiles,
		MaxCycles:       1,
		ResumeSessionID: resumeSessionID,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	// Store this session ID so follow-ups can resume.
	_ = s.WriteStandaloneChatLastSession(profileParam, sessionID)

	if err := session.StartDaemon(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start daemon")
		return
	}

	resp := map[string]any{
		"ok":         true,
		"session_id": sessionID,
	}
	if userMsg != nil {
		resp["message_id"] = userMsg.ID
	}

	writeJSON(w, http.StatusCreated, resp)
}

func buildStandaloneChatPrompt(sp *config.StandaloneProfile, userMessage string) string {
	var b strings.Builder

	if sp.Instructions != "" {
		b.WriteString("## Standalone Profile Instructions\n\n")
		b.WriteString(sp.Instructions)
		b.WriteString("\n\n")
	}

	if userMessage != "" {
		b.WriteString(userMessage)
		b.WriteString("\n")
	} else {
		b.WriteString("Begin working on the project using the context and instructions above.\n")
	}

	return b.String()
}

func handleSaveStandaloneChatResponseP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	profile := r.PathValue("profile")
	if profile == "" {
		writeError(w, http.StatusBadRequest, "profile is required")
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

	msg := &store.StandaloneChatMessage{
		Role:    "assistant",
		Content: req.Content,
	}
	if err := s.CreateStandaloneChatMessage(profile, msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save response")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": msg.ID})
}
