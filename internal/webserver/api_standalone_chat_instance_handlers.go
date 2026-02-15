package webserver

import (
	"encoding/json"
	"net/http"
	"strings"

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

// handleCreateChatInstance creates a new chat instance for a standalone profile.
func handleCreateChatInstance(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
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
	if sp := cfg.FindStandaloneProfile(req.Profile); sp == nil {
		writeError(w, http.StatusBadRequest, "standalone profile not found: "+req.Profile)
		return
	}

	inst, err := s.CreateChatInstance(req.Profile)
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

	sp := cfg.FindStandaloneProfile(inst.Profile)
	if sp == nil {
		writeError(w, http.StatusBadRequest, "standalone profile not found: "+inst.Profile)
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

	// When resuming, just pass the user message directly — the agent already
	// has the full context from its previous session.
	var fullPrompt string
	if resumeSessionID != "" {
		fullPrompt = req.Message
	} else {
		fullPrompt = buildStandaloneChatInstancePrompt(sp, req.Message)
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
		Profile: inst.Profile,
		Role:    "assistant",
		Content: req.Content,
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

// buildStandaloneChatInstancePrompt is analogous to buildStandaloneChatPrompt
// but works with messages from a chat instance.
func buildStandaloneChatInstancePrompt(sp *config.StandaloneProfile, userMessage string) string {
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
