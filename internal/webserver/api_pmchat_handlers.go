package webserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

func (srv *Server) handleGetPMChat(w http.ResponseWriter, r *http.Request) {
	handleGetPMChatP(srv.store, w, r)
}

func handleGetPMChatP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	messages, err := s.ListPMChatMessages()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chat messages")
		return
	}
	if messages == nil {
		messages = []store.PMChatMessage{}
	}
	writeJSON(w, http.StatusOK, messages)
}

func (srv *Server) handleClearPMChat(w http.ResponseWriter, r *http.Request) {
	handleClearPMChatP(srv.store, w, r)
}

func handleClearPMChatP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	if err := s.ClearPMChatMessages(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear chat")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleSendPMChatMessage(w http.ResponseWriter, r *http.Request) {
	handleSendPMChatMessageP(srv.store, w, r)
}

func handleSendPMChatMessageP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
		Profile string `json:"profile"`
		PlanID  string `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	profileName := req.Profile
	if profileName == "" {
		for _, p := range cfg.Profiles {
			if strings.HasPrefix(strings.ToLower(p.Name), "pm") {
				profileName = p.Name
				break
			}
		}
	}
	if profileName == "" && len(cfg.Profiles) > 0 {
		profileName = cfg.Profiles[0].Name
	}
	if profileName == "" {
		writeError(w, http.StatusBadRequest, "no profile available - please create a profile first")
		return
	}

	prof := cfg.FindProfile(profileName)
	if prof == nil {
		writeError(w, http.StatusBadRequest, "profile not found: "+profileName)
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

	userMsg := &store.PMChatMessage{
		Role:    "user",
		Content: req.Message,
	}
	if err := s.CreatePMChatMessage(userMsg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save message")
		return
	}

	history, err := s.ListPMChatMessages()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chat history")
		return
	}

	fullPrompt := buildPMChatPrompt(s, projCfg, planID, prof, cfg, req.Message, history)

	loopDef := config.LoopDef{
		Name: "pm-chat",
		Steps: []config.LoopStep{{
			Profile:      prof.Name,
			Turns:        1,
			Instructions: fullPrompt,
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

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":         true,
		"session_id": sessionID,
		"message_id": userMsg.ID,
	})
}

func buildPMChatPrompt(s *store.Store, projCfg *store.ProjectConfig, planID string, prof *config.Profile, globalCfg *config.GlobalConfig, userMessage string, history []store.PMChatMessage) string {
	basePrompt, err := promptpkg.Build(promptpkg.BuildOpts{
		Store:     s,
		Project:   projCfg,
		PlanID:    planID,
		Profile:   prof,
		Role:      config.RoleManager,
		GlobalCfg: globalCfg,
	})
	if err != nil {
		basePrompt = ""
	}

	var b strings.Builder
	b.WriteString("# PM Chat Session\n\n")
	b.WriteString("You are a PROJECT MANAGER chatting with the user about their project.\n\n")

	b.WriteString("## STRICT RULES — NEVER VIOLATE THESE\n\n")
	b.WriteString("1. You MUST NOT create, edit, write, or delete any files.\n")
	b.WriteString("2. You MUST NOT write any code or implementation.\n")
	b.WriteString("3. You MUST NOT use git commands that modify the repository (commit, push, checkout, reset, etc.).\n")
	b.WriteString("4. You MUST NOT install packages, run builds, or execute project code (no npm, pip, go build, make, etc.).\n")
	b.WriteString("5. If the user asks you to implement, code, build, or set up something, create a PLAN and ISSUES for it instead of doing it yourself.\n")
	b.WriteString("6. Your ONLY permitted shell commands are the `adaf` subcommands listed below, plus read-only inspection commands (`cat`, `ls`, `find`, `head`, `tail`).\n\n")

	b.WriteString("## Your role\n\n")
	b.WriteString("- Manage plans, issues, and documentation using the `adaf` CLI.\n")
	b.WriteString("- Answer questions about the project.\n")
	b.WriteString("- Help organize and prioritize work.\n")
	b.WriteString("- Create plans and issues that developers can follow.\n\n")

	b.WriteString("## Permitted commands\n\n")
	b.WriteString("ONLY these commands are allowed:\n")
	b.WriteString("- `adaf plan ...` — plan management (list, show, create, update)\n")
	b.WriteString("- `adaf issue ...` — issue management (list, show, create, update)\n")
	b.WriteString("- `adaf doc ...` — documentation management\n")
	b.WriteString("- `adaf status` — project overview\n")
	b.WriteString("- `adaf log` — historical context\n")
	b.WriteString("- `cat`, `ls`, `find`, `head`, `tail` — read-only file inspection\n\n")
	b.WriteString("DO NOT use any other commands.\n\n")

	b.WriteString("## Response style\n\n")
	b.WriteString("- Respond directly to what the user is asking.\n")
	b.WriteString("- Keep responses concise and conversational.\n")
	b.WriteString("- Do not volunteer project status unless asked.\n")
	b.WriteString("- Use `adaf` CLI tools only when the user asks you to take action or when you need project context to answer their question.\n\n")

	if basePrompt != "" {
		b.WriteString("## Project Context\n\n")
		b.WriteString(basePrompt)
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

	b.WriteString("## Current User Message\n\n")
	b.WriteString(userMessage)
	b.WriteString("\n\nRespond directly to the message above.\n")

	return b.String()
}

func (srv *Server) handleSavePMChatResponse(w http.ResponseWriter, r *http.Request) {
	handleSavePMChatResponseP(srv.store, w, r)
}

func handleSavePMChatResponseP(s *store.Store, w http.ResponseWriter, r *http.Request) {
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

	msg := &store.PMChatMessage{
		Role:    "assistant",
		Content: req.Content,
	}
	if err := s.CreatePMChatMessage(msg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save response")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "id": msg.ID})
}
