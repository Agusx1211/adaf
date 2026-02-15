package webserver

import (
	"encoding/json"
	"net/http"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
)

func (srv *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (srv *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	profiles := cfg.Profiles
	if profiles == nil {
		profiles = []config.Profile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (srv *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var prof config.Profile
	if err := json.NewDecoder(r.Body).Decode(&prof); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if prof.Name == "" || prof.Agent == "" {
		writeError(w, http.StatusBadRequest, "name and agent are required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	if err := cfg.AddProfile(prof); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusCreated, prof)
}

func (srv *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	found := false
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == name {
			if err := json.NewDecoder(r.Body).Decode(&cfg.Profiles[i]); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			// Ensure name doesn't change via body if we want to keep it consistent with URL
			cfg.Profiles[i].Name = name
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	cfg.RemoveProfile(name)

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleListLoopDefs(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	loops := cfg.Loops
	if loops == nil {
		loops = []config.LoopDef{}
	}
	writeJSON(w, http.StatusOK, loops)
}

func (srv *Server) handleCreateLoopDef(w http.ResponseWriter, r *http.Request) {
	var loop config.LoopDef
	if err := json.NewDecoder(r.Body).Decode(&loop); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if loop.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(loop.Steps) == 0 {
		writeError(w, http.StatusBadRequest, "at least one step is required")
		return
	}
	for _, step := range loop.Steps {
		if step.Profile == "" {
			writeError(w, http.StatusBadRequest, "each step must have a profile")
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	if err := cfg.AddLoop(loop); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusCreated, loop)
}

func (srv *Server) handleUpdateLoopDef(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	found := false
	for i := range cfg.Loops {
		if cfg.Loops[i].Name == name {
			if err := json.NewDecoder(r.Body).Decode(&cfg.Loops[i]); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			cfg.Loops[i].Name = name
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "loop not found")
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleDeleteLoopDef(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	cfg.RemoveLoop(name)

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	config.EnsureDefaultRoleCatalog(cfg)
	roles := cfg.Roles
	if roles == nil {
		roles = []config.RoleDefinition{}
	}
	writeJSON(w, http.StatusOK, roles)
}

func (srv *Server) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	var role config.RoleDefinition
	if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if role.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	if err := cfg.AddRoleDefinition(role); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusCreated, role)
}

func (srv *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	cfg.RemoveRoleDefinition(name)

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	rules := cfg.PromptRules
	if rules == nil {
		rules = []config.PromptRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (srv *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var rule config.PromptRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rule.ID == "" || rule.Body == "" {
		writeError(w, http.StatusBadRequest, "id and body are required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	if err := cfg.AddPromptRule(rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusCreated, rule)
}

func (srv *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	cfg.RemovePromptRule(id)

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleListStandaloneProfiles(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	profiles := cfg.StandaloneProfiles
	if profiles == nil {
		profiles = []config.StandaloneProfile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (srv *Server) handleCreateStandaloneProfile(w http.ResponseWriter, r *http.Request) {
	var sp config.StandaloneProfile
	if err := json.NewDecoder(r.Body).Decode(&sp); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if sp.Name == "" || sp.Profile == "" {
		writeError(w, http.StatusBadRequest, "name and profile are required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	if cfg.FindProfile(sp.Profile) == nil {
		writeError(w, http.StatusBadRequest, "referenced profile not found: "+sp.Profile)
		return
	}

	if err := cfg.AddStandaloneProfile(sp); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusCreated, sp)
}

func (srv *Server) handleUpdateStandaloneProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	found := false
	for i := range cfg.StandaloneProfiles {
		if cfg.StandaloneProfiles[i].Name == name {
			if err := json.NewDecoder(r.Body).Decode(&cfg.StandaloneProfiles[i]); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
			cfg.StandaloneProfiles[i].Name = name
			if cfg.StandaloneProfiles[i].Profile != "" && cfg.FindProfile(cfg.StandaloneProfiles[i].Profile) == nil {
				writeError(w, http.StatusBadRequest, "referenced profile not found: "+cfg.StandaloneProfiles[i].Profile)
				return
			}
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "standalone profile not found")
		return
	}

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleDeleteStandaloneProfile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	cfg.RemoveStandaloneProfile(name)

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) handleGetPushover(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	writeJSON(w, http.StatusOK, cfg.Pushover)
}

type agentInfoDTO struct {
	Name            string                     `json:"name"`
	Detected        bool                       `json:"detected"`
	DefaultModel    string                     `json:"default_model"`
	SupportedModels []string                   `json:"supported_models"`
	ReasoningLevels []agentmeta.ReasoningLevel `json:"reasoning_levels"`
}

func buildAgentInfoList(agentsCfg *agent.AgentsConfig) []agentInfoDTO {
	var result []agentInfoDTO
	for _, name := range agentmeta.Names() {
		info, _ := agentmeta.InfoFor(name)
		ai := agentInfoDTO{
			Name:            name,
			DefaultModel:    info.DefaultModel,
			SupportedModels: info.SupportedModels,
			ReasoningLevels: info.ReasoningLevels,
		}
		if ai.SupportedModels == nil {
			ai.SupportedModels = []string{}
		}
		if ai.ReasoningLevels == nil {
			ai.ReasoningLevels = []agentmeta.ReasoningLevel{}
		}

		if agentsCfg != nil {
			if rec, ok := agentsCfg.Agents[name]; ok {
				ai.Detected = rec.Detected
				if len(rec.SupportedModels) > 0 {
					ai.SupportedModels = rec.SupportedModels
				}
				if len(rec.ReasoningLevels) > 0 {
					ai.ReasoningLevels = rec.ReasoningLevels
				}
				if rec.DefaultModel != "" {
					ai.DefaultModel = rec.DefaultModel
				}
			}
		}

		result = append(result, ai)
	}
	return result
}

func (srv *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	// Try cached detection data first; if empty, trigger a fresh scan so the
	// dropdown gets full model IDs (e.g. "claude-opus-4-6") instead of just
	// the short aliases from the built-in catalog.
	agentsCfg, _ := agent.LoadAgentsConfig()
	if len(agentsCfg.Agents) == 0 {
		globalCfg, _ := config.Load()
		if synced, err := agent.LoadAndSyncAgentsConfig(globalCfg); err == nil {
			agentsCfg = synced
		}
	}

	writeJSON(w, http.StatusOK, buildAgentInfoList(agentsCfg))
}

func (srv *Server) handleDetectAgents(w http.ResponseWriter, r *http.Request) {
	globalCfg, _ := config.Load()
	agentsCfg, err := agent.LoadAndSyncAgentsConfig(globalCfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "detection failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, buildAgentInfoList(agentsCfg))
}

func (srv *Server) handleUpdatePushover(w http.ResponseWriter, r *http.Request) {
	var req config.PushoverConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}

	cfg.Pushover = req

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
