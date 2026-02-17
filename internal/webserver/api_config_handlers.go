package webserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
	loopctrl "github.com/agusx1211/adaf/internal/loop"
	"github.com/agusx1211/adaf/internal/looprun"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/store"
)

var errInvalidRequestBody = errors.New("invalid request body")

func (srv *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func loadConfigOrError(w http.ResponseWriter) (*config.GlobalConfig, bool) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return nil, false
	}
	return cfg, true
}

func saveConfigOrError(w http.ResponseWriter, cfg *config.GlobalConfig) bool {
	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return false
	}
	return true
}

func pathValueRequired(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	value := strings.TrimSpace(r.PathValue(key))
	if value == "" {
		writeError(w, http.StatusBadRequest, key+" is required")
		return "", false
	}
	return value, true
}

func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (srv *Server) createConfigEntry(
	w http.ResponseWriter,
	r *http.Request,
	payload any,
	validate func() string,
	add func(cfg *config.GlobalConfig) error,
) {
	if !decodeJSONBody(w, r, payload) {
		return
	}
	if msg := strings.TrimSpace(validate()); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	if err := add(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !saveConfigOrError(w, cfg) {
		return
	}
	writeJSON(w, http.StatusCreated, payload)
}

func (srv *Server) updateConfigEntry(
	w http.ResponseWriter,
	r *http.Request,
	pathKey string,
	notFoundMsg string,
	update func(cfg *config.GlobalConfig, key string) (bool, error),
) {
	key, ok := pathValueRequired(w, r, pathKey)
	if !ok {
		return
	}
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	found, err := update(cfg, key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, notFoundMsg)
		return
	}
	if !saveConfigOrError(w, cfg) {
		return
	}
	writeOK(w)
}

func (srv *Server) deleteConfigEntry(
	w http.ResponseWriter,
	r *http.Request,
	pathKey string,
	remove func(cfg *config.GlobalConfig, key string),
) {
	key, ok := pathValueRequired(w, r, pathKey)
	if !ok {
		return
	}
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	remove(cfg, key)
	if !saveConfigOrError(w, cfg) {
		return
	}
	writeOK(w)
}

func updateMatchingEntry[T any](
	items []T,
	matches func(item T) bool,
	decode func(item *T) error,
	finalize func(item *T),
) (bool, error) {
	for i := range items {
		if !matches(items[i]) {
			continue
		}
		var updated T
		if err := decode(&updated); err != nil {
			return false, errInvalidRequestBody
		}
		if finalize != nil {
			finalize(&updated)
		}
		items[i] = updated
		return true, nil
	}
	return false, nil
}

func updateConfigSliceEntry[T any](
	srv *Server,
	w http.ResponseWriter,
	r *http.Request,
	pathKey string,
	notFoundMsg string,
	beforeUpdate func(cfg *config.GlobalConfig),
	selectItems func(cfg *config.GlobalConfig) []T,
	matches func(item T, key string) bool,
	stabilize func(item *T, key string),
) {
	srv.updateConfigEntry(w, r, pathKey, notFoundMsg, func(cfg *config.GlobalConfig, key string) (bool, error) {
		if beforeUpdate != nil {
			beforeUpdate(cfg)
		}
		return updateMatchingEntry(
			selectItems(cfg),
			func(item T) bool { return matches(item, key) },
			func(item *T) error { return json.NewDecoder(r.Body).Decode(item) },
			func(item *T) {
				if stabilize != nil {
					stabilize(item, key)
				}
			},
		)
	})
}

func createTypedConfigEntry[T any](
	srv *Server,
	w http.ResponseWriter,
	r *http.Request,
	payload *T,
	validate func(item T) string,
	add func(cfg *config.GlobalConfig, item T) error,
) {
	srv.createConfigEntry(
		w,
		r,
		payload,
		func() string { return validate(*payload) },
		func(cfg *config.GlobalConfig) error { return add(cfg, *payload) },
	)
}

func validateRequiredName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "name is required"
	}
	return ""
}

func validateCreateProfile(prof config.Profile) string {
	if strings.TrimSpace(prof.Name) == "" || strings.TrimSpace(prof.Agent) == "" {
		return "name and agent are required"
	}
	return ""
}

func validateCreateLoop(loop config.LoopDef) string {
	if strings.TrimSpace(loop.Name) == "" {
		return "name is required"
	}
	if len(loop.Steps) == 0 {
		return "at least one step is required"
	}
	for _, step := range loop.Steps {
		if strings.TrimSpace(step.Profile) == "" {
			return "each step must have a profile"
		}
		position := config.EffectiveStepPosition(step)
		if !config.PositionCanOwnTurn(position) {
			return "worker position is not valid for loop steps"
		}
		if config.PositionRequiresTeam(position) && strings.TrimSpace(step.Team) == "" {
			return "manager position steps require a team"
		}
		if !config.PositionAllowsTeam(position) && strings.TrimSpace(step.Team) != "" {
			return "supervisor steps cannot have teams"
		}
	}
	return ""
}

func validateCreateRule(rule config.PromptRule) string {
	if strings.TrimSpace(rule.ID) == "" || strings.TrimSpace(rule.Body) == "" {
		return "id and body are required"
	}
	return ""
}

func validateCreateRole(role config.RoleDefinition) string {
	return validateRequiredName(role.Name)
}

func validateCreateTeam(team config.Team) string {
	return validateRequiredName(team.Name)
}

func validateCreateSkill(skill config.Skill) string {
	if strings.TrimSpace(skill.ID) == "" {
		return "id is required"
	}
	return ""
}

func selectProfiles(cfg *config.GlobalConfig) []config.Profile { return cfg.Profiles }

func profileMatchesName(p config.Profile, name string) bool { return p.Name == name }

func setProfileName(p *config.Profile, name string) { p.Name = name }

func selectLoops(cfg *config.GlobalConfig) []config.LoopDef { return cfg.Loops }

func loopMatchesName(loop config.LoopDef, name string) bool { return loop.Name == name }

func setLoopName(loop *config.LoopDef, name string) { loop.Name = name }

func selectTeams(cfg *config.GlobalConfig) []config.Team { return cfg.Teams }

func teamMatchesName(team config.Team, name string) bool { return team.Name == name }

func setTeamName(team *config.Team, name string) { team.Name = name }

func (srv *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
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
	createTypedConfigEntry(srv, w, r, &prof, validateCreateProfile, (*config.GlobalConfig).AddProfile)
}

func (srv *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	// Keep URL name authoritative.
	updateConfigSliceEntry(srv, w, r, "name", "profile not found", nil, selectProfiles, profileMatchesName, setProfileName)
}

func (srv *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	srv.deleteConfigEntry(w, r, "name", func(cfg *config.GlobalConfig, name string) {
		cfg.RemoveProfile(name)
	})
}

func (srv *Server) handleListLoopDefs(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	loops := cfg.Loops
	if loops == nil {
		loops = []config.LoopDef{}
	}
	writeJSON(w, http.StatusOK, loops)
}

type loopPromptPreviewRequest struct {
	ProjectID     string         `json:"project_id,omitempty"`
	Loop          config.LoopDef `json:"loop"`
	StepIndex     int            `json:"step_index"`
	Cycle         int            `json:"cycle,omitempty"`
	PlanID        string         `json:"plan_id,omitempty"`
	InitialPrompt string         `json:"initial_prompt,omitempty"`
}

type loopPromptPreviewScenario struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Exact       bool   `json:"exact"`
}

type loopPromptPreviewResponse struct {
	RuntimePath string                      `json:"runtime_path"`
	LoopName    string                      `json:"loop_name"`
	StepIndex   int                         `json:"step_index"`
	StepCount   int                         `json:"step_count"`
	Profile     string                      `json:"profile"`
	Position    string                      `json:"position"`
	Role        string                      `json:"role,omitempty"`
	Scenarios   []loopPromptPreviewScenario `json:"scenarios"`
}

type teamPromptPreviewRequest struct {
	ProjectID     string      `json:"project_id,omitempty"`
	Team          config.Team `json:"team"`
	ChildProfile  string      `json:"child_profile,omitempty"`
	ChildRole     string      `json:"child_role,omitempty"`
	Task          string      `json:"task,omitempty"`
	ReadOnly      bool        `json:"read_only,omitempty"`
	Profile       string      `json:"profile,omitempty"`  // legacy alias for child_profile
	Position      string      `json:"position,omitempty"` // legacy, ignored (workers only)
	Cycle         int         `json:"cycle,omitempty"`
	PlanID        string      `json:"plan_id,omitempty"`
	InitialPrompt string      `json:"initial_prompt,omitempty"`
}

type teamPromptPreviewResponse struct {
	RuntimePath string                      `json:"runtime_path"`
	TeamName    string                      `json:"team_name"`
	Profile     string                      `json:"profile"`
	Position    string                      `json:"position"`
	Role        string                      `json:"role,omitempty"`
	Scenarios   []loopPromptPreviewScenario `json:"scenarios"`
}

func (srv *Server) handleLoopPromptPreview(w http.ResponseWriter, r *http.Request) {
	var req loopPromptPreviewRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.Loop.Steps) == 0 {
		writeError(w, http.StatusBadRequest, "loop must include at least one step")
		return
	}
	if req.StepIndex < 0 || req.StepIndex >= len(req.Loop.Steps) {
		writeError(w, http.StatusBadRequest, "step_index is out of range")
		return
	}

	step := req.Loop.Steps[req.StepIndex]
	if strings.TrimSpace(step.Profile) == "" {
		writeError(w, http.StatusBadRequest, "selected step must include a profile")
		return
	}

	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	config.EnsureDefaultRoleCatalog(cfg)
	config.EnsureDefaultSkillCatalog(cfg)

	prof := cfg.FindProfile(step.Profile)
	if prof == nil {
		writeError(w, http.StatusBadRequest, "profile not found: "+step.Profile)
		return
	}
	if err := config.ValidateLoopStepPosition(step, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var projectStore *store.Store
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		projectStore = srv.defaultStore()
	} else {
		var found bool
		projectStore, found = srv.registry.Get(projectID)
		if !found {
			writeError(w, http.StatusNotFound, "project not found: "+projectID)
			return
		}
	}

	var projectCfg *store.ProjectConfig
	if projectStore != nil {
		var err error
		projectCfg, err = projectStore.LoadProject()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load project config")
			return
		}
	}

	var effectiveDelegation *config.DelegationConfig
	if teamName := strings.TrimSpace(step.Team); teamName != "" {
		team := cfg.FindTeam(teamName)
		if team == nil {
			writeError(w, http.StatusBadRequest, "team not found: "+teamName)
			return
		}
		effectiveDelegation = team.Delegation
	}

	loopName := strings.TrimSpace(req.Loop.Name)
	if loopName == "" {
		loopName = "preview-loop"
	}
	cycle := req.Cycle
	if cycle < 0 {
		cycle = 0
	}

	stepPromptInput := looprun.StepPromptInput{
		Store:         projectStore,
		Project:       projectCfg,
		GlobalCfg:     cfg,
		PlanID:        strings.TrimSpace(req.PlanID),
		InitialPrompt: req.InitialPrompt,
		LoopName:      loopName,
		RunID:         1,
		Cycle:         cycle,
		StepIndex:     req.StepIndex,
		TotalSteps:    len(req.Loop.Steps),
		Step:          step,
		LoopSteps:     req.Loop.Steps,
		Profile:       prof,
		Delegation:    effectiveDelegation,
	}

	freshPrompt, err := looprun.BuildStepPrompt(stepPromptInput)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build loop prompt: "+err.Error())
		return
	}
	resumePrompt := loopctrl.BuildResumePrompt(nil, false, "", true)

	position := config.EffectiveStepPosition(step)
	workerRole := config.EffectiveWorkerRoleForPosition(position, step.Role, cfg)
	resp := loopPromptPreviewResponse{
		RuntimePath: "looprun.BuildStepPrompt + loop.BuildResumePrompt",
		LoopName:    loopName,
		StepIndex:   req.StepIndex,
		StepCount:   len(req.Loop.Steps),
		Profile:     prof.Name,
		Position:    position,
		Role:        workerRole,
		Scenarios: []loopPromptPreviewScenario{
			{
				ID:          "fresh_turn",
				Title:       "Turn 1 (fresh prompt)",
				Description: "Exact prompt sent for the first turn of this step.",
				Prompt:      freshPrompt,
				Exact:       true,
			},
			{
				ID:          "resume_turn",
				Title:       "Turn 2+ (resume continuation)",
				Description: "Exact continuation prompt when the same agent session is resumed with no wait results or interrupts.",
				Prompt:      resumePrompt,
				Exact:       true,
			},
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func (srv *Server) handleTeamPromptPreview(w http.ResponseWriter, r *http.Request) {
	var req teamPromptPreviewRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	config.EnsureDefaultRoleCatalog(cfg)
	config.EnsureDefaultSkillCatalog(cfg)

	teamName := strings.TrimSpace(req.Team.Name)
	if teamName == "" {
		teamName = "preview-team"
	}
	teamDelegation := req.Team.Delegation
	if teamDelegation == nil || len(teamDelegation.Profiles) == 0 {
		writeError(w, http.StatusBadRequest, "team must include at least one delegation profile")
		return
	}

	childProfileName := strings.TrimSpace(req.ChildProfile)
	if childProfileName == "" {
		// Backward compatibility for older clients.
		childProfileName = strings.TrimSpace(req.Profile)
	}
	if childProfileName == "" {
		writeError(w, http.StatusBadRequest, "child_profile is required")
		return
	}

	childProf := cfg.FindProfile(childProfileName)
	if childProf == nil {
		writeError(w, http.StatusBadRequest, "profile not found: "+childProfileName)
		return
	}

	resolved, resolvedRole, resolvedPosition, err := teamDelegation.ResolveProfileWithPosition(
		childProfileName,
		strings.TrimSpace(req.ChildRole),
		config.PositionWorker,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if resolvedPosition != config.PositionWorker {
		writeError(w, http.StatusBadRequest, "invalid child position: teams are composed only by workers")
		return
	}
	if !config.ValidRole(resolvedRole, cfg) {
		writeError(w, http.StatusBadRequest, "child role is not defined in the roles catalog: "+resolvedRole)
		return
	}
	if resolved.Delegation != nil && len(resolved.Delegation.Profiles) > 0 {
		writeError(w, http.StatusBadRequest, "invalid child delegation: workers cannot have teams")
		return
	}
	childDelegation := &config.DelegationConfig{}
	if resolved.Delegation != nil {
		childDelegation = resolved.Delegation.Clone()
	}
	childSkills := append([]string(nil), resolved.Skills...)

	var projectStore *store.Store
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		projectStore = srv.defaultStore()
	} else {
		var found bool
		projectStore, found = srv.registry.Get(projectID)
		if !found {
			writeError(w, http.StatusNotFound, "project not found: "+projectID)
			return
		}
	}

	var projectCfg *store.ProjectConfig
	if projectStore != nil {
		var err error
		projectCfg, err = projectStore.LoadProject()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load project config")
			return
		}
	}

	task := strings.TrimSpace(req.Task)
	if task == "" {
		task = "Preview task: implement the delegated sub-task and report clear results back to the parent agent."
	}

	freshPrompt, err := promptpkg.Build(promptpkg.BuildOpts{
		Store:        projectStore,
		Project:      projectCfg,
		PlanID:       strings.TrimSpace(req.PlanID),
		Profile:      childProf,
		Role:         resolvedRole,
		Position:     resolvedPosition,
		GlobalCfg:    cfg,
		Task:         task,
		ReadOnly:     req.ReadOnly,
		ParentTurnID: 1, // any positive value forces sub-agent prompt path
		Delegation:   childDelegation,
		Skills:       childSkills,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build team prompt preview: "+err.Error())
		return
	}
	resumePrompt := loopctrl.BuildResumePrompt(nil, false, "", true)

	resp := teamPromptPreviewResponse{
		RuntimePath: "prompt.Build (sub-agent) + loop.BuildResumePrompt",
		TeamName:    teamName,
		Profile:     childProf.Name,
		Position:    resolvedPosition,
		Role:        resolvedRole,
		Scenarios: []loopPromptPreviewScenario{
			{
				ID:          "fresh_turn",
				Title:       "Turn 1 (fresh prompt)",
				Description: "Exact worker sub-agent prompt template. The task text is injected when spawning.",
				Prompt:      freshPrompt,
				Exact:       true,
			},
			{
				ID:          "resume_turn",
				Title:       "Turn 2+ (resume continuation)",
				Description: "Exact continuation prompt when the same agent session is resumed with no wait results or interrupts.",
				Prompt:      resumePrompt,
				Exact:       true,
			},
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (srv *Server) handleCreateLoopDef(w http.ResponseWriter, r *http.Request) {
	var loop config.LoopDef
	createTypedConfigEntry(srv, w, r, &loop, validateCreateLoop, (*config.GlobalConfig).AddLoop)
}

func (srv *Server) handleUpdateLoopDef(w http.ResponseWriter, r *http.Request) {
	updateConfigSliceEntry(srv, w, r, "name", "loop not found", nil, selectLoops, loopMatchesName, setLoopName)
}

func (srv *Server) handleDeleteLoopDef(w http.ResponseWriter, r *http.Request) {
	srv.deleteConfigEntry(w, r, "name", func(cfg *config.GlobalConfig, name string) {
		cfg.RemoveLoop(name)
	})
}

func (srv *Server) handleListRoles(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
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
	createTypedConfigEntry(srv, w, r, &role, validateCreateRole, (*config.GlobalConfig).AddRoleDefinition)
}

func (srv *Server) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	updateConfigSliceEntry(
		srv,
		w,
		r,
		"name",
		"role not found: "+r.PathValue("name"),
		func(cfg *config.GlobalConfig) { config.EnsureDefaultRoleCatalog(cfg) },
		func(cfg *config.GlobalConfig) []config.RoleDefinition { return cfg.Roles },
		func(role config.RoleDefinition, name string) bool { return strings.EqualFold(role.Name, name) },
		func(role *config.RoleDefinition, name string) { role.Name = name },
	)
}

func (srv *Server) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	srv.deleteConfigEntry(w, r, "name", func(cfg *config.GlobalConfig, name string) {
		cfg.RemoveRoleDefinition(name)
	})
}

func (srv *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
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
	createTypedConfigEntry(srv, w, r, &rule, validateCreateRule, (*config.GlobalConfig).AddPromptRule)
}

func (srv *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	srv.deleteConfigEntry(w, r, "id", func(cfg *config.GlobalConfig, id string) {
		cfg.RemovePromptRule(id)
	})
}

// ── Team handlers ──

func (srv *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	teams := cfg.Teams
	if teams == nil {
		teams = []config.Team{}
	}
	writeJSON(w, http.StatusOK, teams)
}

func (srv *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var t config.Team
	createTypedConfigEntry(srv, w, r, &t, validateCreateTeam, (*config.GlobalConfig).AddTeam)
}

func (srv *Server) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	updateConfigSliceEntry(srv, w, r, "name", "team not found", nil, selectTeams, teamMatchesName, setTeamName)
}

func (srv *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	srv.deleteConfigEntry(w, r, "name", func(cfg *config.GlobalConfig, name string) {
		cfg.RemoveTeam(name)
	})
}

func (srv *Server) handleListRecentCombinations(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	combos := cfg.RecentCombinations
	if combos == nil {
		combos = []config.RecentCombination{}
	}
	writeJSON(w, http.StatusOK, combos)
}

func (srv *Server) handleGetPushover(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
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
	if !decodeJSONBody(w, r, &req) {
		return
	}

	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}

	cfg.Pushover = req

	if !saveConfigOrError(w, cfg) {
		return
	}

	writeOK(w)
}

// ── Skill handlers ──

func (srv *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	cfg, ok := loadConfigOrError(w)
	if !ok {
		return
	}
	config.EnsureDefaultSkillCatalog(cfg)
	skills := cfg.Skills
	if skills == nil {
		skills = []config.Skill{}
	}
	writeJSON(w, http.StatusOK, skills)
}

func (srv *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	var sk config.Skill
	createTypedConfigEntry(srv, w, r, &sk, validateCreateSkill, (*config.GlobalConfig).AddSkill)
}

func (srv *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	updateConfigSliceEntry(
		srv,
		w,
		r,
		"id",
		"skill not found",
		nil,
		func(cfg *config.GlobalConfig) []config.Skill { return cfg.Skills },
		func(skill config.Skill, id string) bool { return strings.EqualFold(skill.ID, id) },
		func(skill *config.Skill, id string) { skill.ID = id },
	)
}

func (srv *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	srv.deleteConfigEntry(w, r, "id", func(cfg *config.GlobalConfig, id string) {
		cfg.RemoveSkill(id)
	})
}
