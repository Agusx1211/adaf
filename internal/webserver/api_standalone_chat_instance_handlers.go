package webserver

import (
	"encoding/json"
	"net/http"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

// handleListChatInstances returns all chat instances.
func handleListChatInstances(s *store.Store, w http.ResponseWriter, r *http.Request) {
	instances, err := s.ListChatInstances()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list chat instances")
		return
	}
	if instances == nil {
		instances = []store.StandaloneChatInstance{}
	}
	writeJSON(w, http.StatusOK, instances)
}

// handleCreateChatInstance creates a new chat instance with a profile+team combination.
func handleCreateChatInstance(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
		Team    string `json:"team"`
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

	if cfg.FindProfile(req.Profile) == nil {
		writeError(w, http.StatusBadRequest, "profile not found: "+req.Profile)
		return
	}
	if req.Team != "" {
		if cfg.FindTeam(req.Team) == nil {
			writeError(w, http.StatusBadRequest, "team not found: "+req.Team)
			return
		}
		cfg.RecordRecentCombination(req.Profile, req.Team)
		_ = config.Save(cfg)
	}

	inst, err := s.CreateChatInstance(req.Profile, req.Team)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create chat instance")
		return
	}
	writeJSON(w, http.StatusCreated, inst)
}

// handleGetChatInstanceMessages returns messages for a chat instance.
func handleGetChatInstanceMessages(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	inst, err := s.GetChatInstance(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chat instance")
		return
	}
	if inst == nil {
		writeError(w, http.StatusNotFound, "chat instance not found")
		return
	}

	messages, err := s.ListChatInstanceMessages(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}
	if messages == nil {
		messages = []store.StandaloneChatMessage{}
	}
	writeJSON(w, http.StatusOK, messages)
}

// handleSendChatInstanceMessage sends a user message to a chat instance and starts an agent session.
func handleSendChatInstanceMessage(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	inst, err := s.GetChatInstance(id)
	if err != nil || inst == nil {
		writeError(w, http.StatusNotFound, "chat instance not found")
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	prof := cfg.FindProfile(inst.Profile)
	if prof == nil {
		writeError(w, http.StatusBadRequest, "profile not found: "+inst.Profile)
		return
	}
	var delegation *config.DelegationConfig
	if inst.Team != "" {
		if team := cfg.FindTeam(inst.Team); team != nil {
			delegation = team.Delegation
		}
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
			Profile: inst.Profile,
			Role:    "user",
			Content: req.Message,
		}
		if err := s.CreateChatInstanceMessage(id, userMsg); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save message")
			return
		}
	}

	// Check if we can resume a previous agent session for this chat instance.
	var resumeSessionID string
	if inst.LastSessionID > 0 {
		resumeSessionID = session.ReadAgentSessionID(inst.LastSessionID)
	}

	fullPrompt := req.Message

	step := config.LoopStep{
		Profile:        prof.Name,
		Turns:          1,
		Instructions:   fullPrompt,
		StandaloneChat: true,
	}
	if delegation != nil {
		step.Delegation = delegation
	}

	loopDef := config.LoopDef{
		Name:  "standalone-chat",
		Steps: []config.LoopStep{step},
	}

	allProfiles := []config.Profile{*prof}
	if delegation != nil {
		for _, dp := range delegation.Profiles {
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

	// Store this session ID on the instance so follow-ups can resume.
	_ = s.UpdateChatInstanceLastSession(id, sessionID)

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

// handleSaveChatInstanceResponse saves an assistant response to a chat instance.
func handleSaveChatInstanceResponse(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	inst, err := s.GetChatInstance(id)
	if err != nil || inst == nil {
		writeError(w, http.StatusNotFound, "chat instance not found")
		return
	}

	var req struct {
		Content string          `json:"content"`
		Events  json.RawMessage `json:"events,omitempty"`
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
		Profile: inst.Profile,
		Role:    "assistant",
		Content: req.Content,
		Events:  req.Events,
	}
	if err := s.CreateChatInstanceMessage(id, msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save response")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": msg.ID})
}

// handleDeleteChatInstance removes a chat instance and all its messages.
func handleDeleteChatInstance(s *store.Store, w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.DeleteChatInstance(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete chat instance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
