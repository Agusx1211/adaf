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

	history, err := s.ListStandaloneChatMessages(profileParam)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chat history")
		return
	}

	fullPrompt := buildStandaloneChatPrompt(sp, req.Message, history)

	step := config.LoopStep{
		Profile:      prof.Name,
		Turns:        1,
		Instructions: fullPrompt,
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
		ProjectDir:  projDir,
		ProjectName: projCfg.Name,
		WorkDir:     projDir,
		ProfileName: prof.Name,
		AgentName:   prof.Agent,
		Loop:        loopDef,
		Profiles:    allProfiles,
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

	resp := map[string]any{
		"ok":         true,
		"session_id": sessionID,
	}
	if userMsg != nil {
		resp["message_id"] = userMsg.ID
	}

	writeJSON(w, http.StatusCreated, resp)
}

func buildStandaloneChatPrompt(sp *config.StandaloneProfile, userMessage string, history []store.StandaloneChatMessage) string {
	// NOTE: Do NOT call promptpkg.Build here. The looprun runner already builds
	// the full project context (including role, rules, session logs, delegation
	// instructions) and prepends it to the step instructions. Including it here
	// would duplicate the context and create contradictions (e.g. "no spawning"
	// from a no-delegation Build vs delegation instructions from the looprun).

	var b strings.Builder
	b.WriteString("# OVERRIDE — Interactive Chat Mode\n\n")
	b.WriteString("**IMPORTANT: Ignore the autonomous agent instructions above.** You are NOT running autonomously. ")
	b.WriteString("You are in a LIVE INTERACTIVE CHAT with the user. A human IS in the loop — they are typing messages to you.\n\n")
	b.WriteString("## Rules for this chat session\n\n")
	b.WriteString("1. RESPOND DIRECTLY to the user's message. Be conversational.\n")
	b.WriteString("2. Do NOT start exploring the codebase or working on tasks unless the user asks you to.\n")
	b.WriteString("3. Do NOT give unsolicited project status reports.\n")
	b.WriteString("4. Keep responses concise unless the user asks for detail.\n")
	b.WriteString("5. When the user asks you to do work (write code, fix bugs, etc.), then use your tools to do it.\n\n")

	if sp.Instructions != "" {
		b.WriteString("## Standalone Profile Instructions\n\n")
		b.WriteString(sp.Instructions)
		b.WriteString("\n\n")
	}

	if len(history) > 0 {
		b.WriteString("## Conversation History\n\n")
		for _, msg := range history {
			if msg.Role == "user" {
				b.WriteString("User: ")
			} else {
				b.WriteString("Assistant: ")
			}
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		}
	}

	if userMessage != "" {
		b.WriteString("## Current User Message\n\n")
		b.WriteString(userMessage)
		b.WriteString("\n\nRespond directly to the message above.\n")
	} else {
		b.WriteString("## Task\n\n")
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
