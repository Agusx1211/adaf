package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
)

// customModelSentinel is shown in the model list to let users type a custom model.
const customModelSentinel = "(custom)"

// ensureDefaultProfiles seeds default profiles when the list is empty.
func ensureDefaultProfiles(cfg *config.GlobalConfig) bool {
	if len(cfg.Profiles) > 0 {
		return false
	}
	defaults := []config.Profile{
		{Name: "claude sonnet", Agent: "claude", Model: "sonnet"},
		{Name: "claude opus", Agent: "claude", Model: "opus"},
		{Name: "codex", Agent: "codex"},
		{Name: "codex fast", Agent: "codex", ReasoningLevel: "low"},
	}
	for _, p := range defaults {
		cfg.AddProfile(p)
	}
	return true
}

// buildModelChoices returns the model list for the profile wizard.
// When the agent has non-empty detected models in agentsCfg, those are used
// exclusively (they are authoritative). Otherwise falls back to catalog.
func buildModelChoices(agentName string, agentsCfg *agent.AgentsConfig) []string {
	var models []string
	seen := make(map[string]struct{})

	// Prefer detected models from the agents config (authoritative when present).
	if agentsCfg != nil {
		if rec, ok := agentsCfg.Agents[agentName]; ok && len(rec.SupportedModels) > 0 {
			for _, m := range rec.SupportedModels {
				lower := strings.ToLower(m)
				if _, ok := seen[lower]; !ok {
					seen[lower] = struct{}{}
					models = append(models, m)
				}
			}
		}
	}

	// Fall back to catalog models only if no detected models.
	if len(models) == 0 {
		for _, m := range agent.SupportedModels(agentName) {
			lower := strings.ToLower(m)
			if _, ok := seen[lower]; !ok {
				seen[lower] = struct{}{}
				models = append(models, m)
			}
		}
	}

	result := []string{"(default)"}
	result = append(result, models...)
	result = append(result, customModelSentinel)
	return result
}

// buildReasoningChoices returns the reasoning level list for the profile wizard.
// Prefers detected levels from agentsCfg, falls back to catalog.
func buildReasoningChoices(agentName string, agentsCfg *agent.AgentsConfig) []agentmeta.ReasoningLevel {
	// Try detected levels first.
	if agentsCfg != nil {
		if rec, ok := agentsCfg.Agents[agentName]; ok && len(rec.ReasoningLevels) > 0 {
			return append([]agentmeta.ReasoningLevel(nil), rec.ReasoningLevels...)
		}
	}
	// Fall back to catalog.
	return agent.ReasoningLevels(agentName)
}

// startEditProfile pre-populates the wizard fields from an existing profile
// and opens the edit menu.
func (m AppModel) startEditProfile() (tea.Model, tea.Cmd) {
	p := m.profiles[m.selected]
	prof := m.globalCfg.FindProfile(p.Name)
	if prof == nil {
		return m, nil
	}

	m.profileEditing = true
	m.profileEditName = prof.Name
	m.profileNameInput = prof.Name

	// Pre-populate agent list and selection.
	m.profileAgents = agentmeta.Names()
	m.profileAgentSel = 0
	for i, name := range m.profileAgents {
		if name == prof.Agent {
			m.profileAgentSel = i
			break
		}
	}

	// Store model/reasoning.
	m.profileSelectedModel = prof.Model
	m.profileSelectedReasoning = prof.ReasoningLevel

	// Pre-populate role selection.
	m.profileRoleSel = 2 // default "junior"
	roles := config.AllRoles()
	effectiveRole := config.EffectiveRole(prof.Role)
	for i, r := range roles {
		if r == effectiveRole {
			m.profileRoleSel = i
			break
		}
	}

	// Pre-populate text inputs.
	m.profileDescInput = prof.Description
	m.profileIntelInput = ""
	if prof.Intelligence > 0 {
		m.profileIntelInput = strconv.Itoa(prof.Intelligence)
	}
	m.profileMaxInstInput = ""
	if prof.MaxInstances > 0 {
		m.profileMaxInstInput = strconv.Itoa(prof.MaxInstances)
	}
	m.profileMaxParInput = ""
	if prof.MaxParallel > 0 {
		m.profileMaxParInput = strconv.Itoa(prof.MaxParallel)
	}

	// Pre-populate spawnable selections.
	m.profileSpawnableOptions = nil
	m.profileSpawnableSelected = make(map[int]bool)
	m.profileSpawnableSel = 0
	for _, sp := range m.globalCfg.Profiles {
		if !strings.EqualFold(sp.Name, prof.Name) {
			m.profileSpawnableOptions = append(m.profileSpawnableOptions, sp.Name)
		}
	}
	for i, name := range m.profileSpawnableOptions {
		for _, sp := range prof.SpawnableProfiles {
			if strings.EqualFold(name, sp) {
				m.profileSpawnableSelected[i] = true
				break
			}
		}
	}

	m.profileMenuSel = 0
	m.state = stateProfileMenu
	return m, nil
}

// wizardTitle returns "Edit Profile" or "New Profile" depending on mode.
func (m AppModel) wizardTitle() string {
	if m.profileEditing {
		return "Edit Profile"
	}
	return "New Profile"
}

// editMenuItemCount is the number of items in the edit menu (10 fields + Save).
const editMenuItemCount = 11

// updateProfileMenu handles the edit profile field picker menu.
func (m AppModel) updateProfileMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.profileMenuSel = (m.profileMenuSel + 1) % editMenuItemCount
		case "k", "up":
			m.profileMenuSel = (m.profileMenuSel - 1 + editMenuItemCount) % editMenuItemCount
		case "esc":
			m.profileEditing = false
			m.state = stateSelector
		case "enter":
			switch m.profileMenuSel {
			case 0: // Name
				m.state = stateProfileName
			case 1: // Agent
				m.profileAgents = agentmeta.Names()
				m.state = stateProfileAgent
			case 2: // Model
				selectedAgent := m.profileAgents[m.profileAgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig()
				m.profileModels = buildModelChoices(selectedAgent, agentsCfg)
				m.profileModelSel = 0
				if m.profileSelectedModel != "" {
					for i, model := range m.profileModels {
						if model == m.profileSelectedModel {
							m.profileModelSel = i
							break
						}
					}
				}
				m.profileCustomModelMode = false
				m.state = stateProfileModel
			case 3: // Reasoning
				selectedAgent := m.profileAgents[m.profileAgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig()
				levels := buildReasoningChoices(selectedAgent, agentsCfg)
				if len(levels) == 0 {
					return m, nil // no reasoning levels for this agent
				}
				m.profileReasoningLevels = levels
				m.profileReasoningLevelSel = 0
				if m.profileSelectedReasoning != "" {
					for i, l := range levels {
						if l.Name == m.profileSelectedReasoning {
							m.profileReasoningLevelSel = i
							break
						}
					}
				}
				m.state = stateProfileReasoning
			case 4: // Role
				m.state = stateProfileRole
			case 5: // Intelligence
				m.state = stateProfileIntel
			case 6: // Description
				m.state = stateProfileDesc
			case 7: // Max Instances
				m.state = stateProfileMaxInst
			case 8: // Spawnable Profiles
				// Rebuild options, preserving existing selections by name.
				oldSel := make(map[string]bool)
				for i, name := range m.profileSpawnableOptions {
					if m.profileSpawnableSelected[i] {
						oldSel[name] = true
					}
				}
				m.profileSpawnableOptions = nil
				m.profileSpawnableSelected = make(map[int]bool)
				m.profileSpawnableSel = 0
				for _, p := range m.globalCfg.Profiles {
					if !strings.EqualFold(p.Name, m.profileNameInput) {
						m.profileSpawnableOptions = append(m.profileSpawnableOptions, p.Name)
					}
				}
				for i, name := range m.profileSpawnableOptions {
					if oldSel[name] {
						m.profileSpawnableSelected[i] = true
					}
				}
				m.state = stateProfileSpawnable
			case 9: // Max Parallel
				m.state = stateProfileMaxPar
			case 10: // Save
				return m.finishProfileCreation()
			}
		}
	}
	return m, nil
}

// viewProfileMenu renders the edit profile field picker menu.
func (m AppModel) viewProfileMenu() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	var lines []string
	lines = append(lines, sectionStyle.Render("Edit Profile"))
	lines = append(lines, "")

	// Build field values for display.
	agentName := ""
	if m.profileAgentSel < len(m.profileAgents) {
		agentName = m.profileAgents[m.profileAgentSel]
	}
	model := m.profileSelectedModel
	if model == "" {
		model = "(default)"
	}
	reasoning := m.profileSelectedReasoning
	if reasoning == "" {
		reasoning = "(none)"
	}
	role := ""
	roles := config.AllRoles()
	if m.profileRoleSel < len(roles) {
		role = roles[m.profileRoleSel]
	}
	intel := m.profileIntelInput
	if intel == "" {
		intel = "-"
	}
	desc := m.profileDescInput
	if desc == "" {
		desc = "-"
	}
	maxDescW := cw - 18
	if maxDescW > 0 && len(desc) > maxDescW {
		desc = desc[:maxDescW-3] + "..."
	}
	maxInst := m.profileMaxInstInput
	if maxInst == "" {
		maxInst = "unlimited"
	}
	spawnCount := 0
	for _, sel := range m.profileSpawnableSelected {
		if sel {
			spawnCount++
		}
	}
	spawnVal := fmt.Sprintf("%d selected", spawnCount)
	if spawnCount == 0 {
		spawnVal = "none"
	}
	maxPar := m.profileMaxParInput
	if maxPar == "" {
		maxPar = "-"
	}

	type menuItem struct {
		label, value string
	}
	items := []menuItem{
		{"Name", m.profileNameInput},
		{"Agent", agentName},
		{"Model", model},
		{"Reasoning", reasoning},
		{"Role", role + " " + roleBadge(role)},
		{"Intelligence", intel},
		{"Description", desc},
		{"Max Instances", maxInst},
		{"Spawnable", spawnVal},
		{"Max Parallel", maxPar},
	}

	for i, item := range items {
		line := labelStyle.Render(item.label) + valueStyle.Render(item.value)
		if i == m.profileMenuSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			lines = append(lines, cursor+line)
		} else {
			lines = append(lines, "  "+line)
		}
	}

	// Save item
	lines = append(lines, "")
	saveLabel := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("Save")
	if m.profileMenuSel == 10 {
		cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
		lines = append(lines, cursor+saveLabel)
	} else {
		lines = append(lines, "  "+saveLabel)
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: edit field  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// updateProfileName handles input in the profile name state.
func (m AppModel) updateProfileName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.profileNameInput)
			if name == "" {
				return m, nil
			}
			// Allow same name when editing the same profile.
			if existing := m.globalCfg.FindProfile(name); existing != nil {
				if !m.profileEditing || !strings.EqualFold(name, m.profileEditName) {
					return m, nil // duplicate
				}
			}
			m.profileNameInput = name
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileAgents = agentmeta.Names()
			m.profileAgentSel = 0
			m.state = stateProfileAgent
			return m, nil
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileNameInput = ""
			m.state = stateSelector
			return m, nil
		case "backspace":
			if len(m.profileNameInput) > 0 {
				m.profileNameInput = m.profileNameInput[:len(m.profileNameInput)-1]
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.profileNameInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

// updateProfileAgent handles agent selection in the profile wizard.
func (m AppModel) updateProfileAgent(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileAgents) > 0 {
				m.profileAgentSel = (m.profileAgentSel + 1) % len(m.profileAgents)
			}
		case "k", "up":
			if len(m.profileAgents) > 0 {
				m.profileAgentSel = (m.profileAgentSel - 1 + len(m.profileAgents)) % len(m.profileAgents)
			}
		case "enter":
			if len(m.profileAgents) > 0 {
				if m.profileEditing {
					m.state = stateProfileMenu
					return m, nil
				}
				selectedAgent := m.profileAgents[m.profileAgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig()
				m.profileModels = buildModelChoices(selectedAgent, agentsCfg)
				m.profileModelSel = 0
				m.profileCustomModel = ""
				m.profileCustomModelMode = false
				m.state = stateProfileModel
			}
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileName
		}
	}
	return m, nil
}

// updateProfileModel handles model selection in the profile wizard.
func (m AppModel) updateProfileModel(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Custom model text input mode.
	if m.profileCustomModelMode {
		return m.updateCustomModelInput(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileModels) > 0 {
				m.profileModelSel = (m.profileModelSel + 1) % len(m.profileModels)
			}
		case "k", "up":
			if len(m.profileModels) > 0 {
				m.profileModelSel = (m.profileModelSel - 1 + len(m.profileModels)) % len(m.profileModels)
			}
		case "enter":
			if len(m.profileModels) > 0 {
				model := m.profileModels[m.profileModelSel]
				if model == customModelSentinel {
					m.profileCustomModelMode = true
					m.profileCustomModel = ""
					return m, nil
				}
				if model == "(default)" {
					model = ""
				}
				m.profileSelectedModel = model
				if m.profileEditing {
					m.state = stateProfileMenu
					return m, nil
				}
				return m.transitionToReasoningOrFinish()
			}
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileAgent
		}
	}
	return m, nil
}

// updateCustomModelInput handles free-text model input.
func (m AppModel) updateCustomModelInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			model := strings.TrimSpace(m.profileCustomModel)
			if model == "" {
				return m, nil
			}
			m.profileSelectedModel = model
			m.profileCustomModelMode = false
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			return m.transitionToReasoningOrFinish()
		case "esc":
			m.profileCustomModelMode = false
			return m, nil
		case "backspace":
			if len(m.profileCustomModel) > 0 {
				m.profileCustomModel = m.profileCustomModel[:len(m.profileCustomModel)-1]
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.profileCustomModel += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

// transitionToReasoningOrFinish moves to reasoning level selection if the agent
// supports it, otherwise skips to role selection.
func (m AppModel) transitionToReasoningOrFinish() (tea.Model, tea.Cmd) {
	selectedAgent := m.profileAgents[m.profileAgentSel]
	agentsCfg, _ := agent.LoadAgentsConfig()
	levels := buildReasoningChoices(selectedAgent, agentsCfg)
	if len(levels) > 0 {
		m.profileReasoningLevels = levels
		m.profileReasoningLevelSel = 0
		// When editing, pre-select existing reasoning level.
		if m.profileEditing && m.profileSelectedReasoning != "" {
			for i, l := range levels {
				if l.Name == m.profileSelectedReasoning {
					m.profileReasoningLevelSel = i
					break
				}
			}
		}
		m.state = stateProfileReasoning
		return m, nil
	}
	m.profileSelectedReasoning = ""
	return m.transitionToRole()
}

// updateProfileReasoning handles reasoning level selection.
func (m AppModel) updateProfileReasoning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileReasoningLevels) > 0 {
				m.profileReasoningLevelSel = (m.profileReasoningLevelSel + 1) % len(m.profileReasoningLevels)
			}
		case "k", "up":
			if len(m.profileReasoningLevels) > 0 {
				m.profileReasoningLevelSel = (m.profileReasoningLevelSel - 1 + len(m.profileReasoningLevels)) % len(m.profileReasoningLevels)
			}
		case "enter":
			if len(m.profileReasoningLevels) > 0 {
				level := m.profileReasoningLevels[m.profileReasoningLevelSel]
				m.profileSelectedReasoning = level.Name
				if m.profileEditing {
					m.state = stateProfileMenu
					return m, nil
				}
				return m.transitionToRole()
			}
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileModel
		}
	}
	return m, nil
}

// transitionToRole moves to role selection.
func (m AppModel) transitionToRole() (tea.Model, tea.Cmd) {
	m.profileRoleSel = 2 // default to "junior" (index 2)
	// When editing, pre-select existing role.
	if m.profileEditing {
		prof := m.globalCfg.FindProfile(m.profileEditName)
		if prof != nil {
			roles := config.AllRoles()
			effectiveRole := config.EffectiveRole(prof.Role)
			for i, r := range roles {
				if r == effectiveRole {
					m.profileRoleSel = i
					break
				}
			}
		}
	}
	m.state = stateProfileRole
	return m, nil
}

// updateProfileRole handles role selection.
func (m AppModel) updateProfileRole(msg tea.Msg) (tea.Model, tea.Cmd) {
	roles := config.AllRoles()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.profileRoleSel = (m.profileRoleSel + 1) % len(roles)
		case "k", "up":
			m.profileRoleSel = (m.profileRoleSel - 1 + len(roles)) % len(roles)
		case "enter":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			// Move to intelligence input.
			m.profileIntelInput = ""
			m.state = stateProfileIntel
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			// Go back to reasoning or model.
			if len(m.profileReasoningLevels) > 0 {
				m.state = stateProfileReasoning
			} else {
				m.state = stateProfileModel
			}
		}
	}
	return m, nil
}

// updateProfileIntel handles intelligence rating input.
func (m AppModel) updateProfileIntel(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			// Move to description.
			m.profileDescInput = ""
			m.state = stateProfileDesc
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileRole
		case "backspace":
			if len(m.profileIntelInput) > 0 {
				m.profileIntelInput = m.profileIntelInput[:len(m.profileIntelInput)-1]
			}
		default:
			ch := msg.String()
			if len(ch) == 1 && ch >= "0" && ch <= "9" && len(m.profileIntelInput) < 2 {
				m.profileIntelInput += ch
			}
		}
	}
	return m, nil
}

// updateProfileDesc handles description text input.
func (m AppModel) updateProfileDesc(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileMaxInstInput = ""
			m.state = stateProfileMaxInst
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileIntel
		case "backspace":
			if len(m.profileDescInput) > 0 {
				m.profileDescInput = m.profileDescInput[:len(m.profileDescInput)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.profileDescInput += msg.String()
			}
		}
	}
	return m, nil
}

// updateProfileMaxInst handles max instances input.
func (m AppModel) updateProfileMaxInst(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			roles := config.AllRoles()
			selectedRole := roles[m.profileRoleSel]
			if config.CanSpawn(selectedRole) {
				return m.transitionToSpawnable()
			}
			return m.finishProfileCreation()
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileDesc
		case "backspace":
			if len(m.profileMaxInstInput) > 0 {
				m.profileMaxInstInput = m.profileMaxInstInput[:len(m.profileMaxInstInput)-1]
			}
		default:
			ch := msg.String()
			if len(ch) == 1 && ch >= "0" && ch <= "9" && len(m.profileMaxInstInput) < 2 {
				m.profileMaxInstInput += ch
			}
		}
	}
	return m, nil
}

// transitionToSpawnable shows the spawnable profile multi-select.
func (m AppModel) transitionToSpawnable() (tea.Model, tea.Cmd) {
	// Build list of other profile names.
	m.profileSpawnableOptions = nil
	m.profileSpawnableSelected = make(map[int]bool)
	m.profileSpawnableSel = 0
	for _, p := range m.globalCfg.Profiles {
		if !strings.EqualFold(p.Name, m.profileNameInput) {
			m.profileSpawnableOptions = append(m.profileSpawnableOptions, p.Name)
		}
	}
	// When editing, pre-select existing spawnable profiles.
	if m.profileEditing {
		prof := m.globalCfg.FindProfile(m.profileEditName)
		if prof != nil {
			for i, name := range m.profileSpawnableOptions {
				for _, sp := range prof.SpawnableProfiles {
					if strings.EqualFold(name, sp) {
						m.profileSpawnableSelected[i] = true
						break
					}
				}
			}
		}
	}
	m.state = stateProfileSpawnable
	return m, nil
}

// updateProfileSpawnable handles multi-select of spawnable profiles.
func (m AppModel) updateProfileSpawnable(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileSpawnableOptions) > 0 {
				m.profileSpawnableSel = (m.profileSpawnableSel + 1) % len(m.profileSpawnableOptions)
			}
		case "k", "up":
			if len(m.profileSpawnableOptions) > 0 {
				m.profileSpawnableSel = (m.profileSpawnableSel - 1 + len(m.profileSpawnableOptions)) % len(m.profileSpawnableOptions)
			}
		case " ":
			// Toggle selection.
			m.profileSpawnableSelected[m.profileSpawnableSel] = !m.profileSpawnableSelected[m.profileSpawnableSel]
		case "enter":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileMaxParInput = "2"
			m.state = stateProfileMaxPar
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileMaxInst
		}
	}
	return m, nil
}

// updateProfileMaxPar handles max parallel input.
func (m AppModel) updateProfileMaxPar(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			return m.finishProfileCreation()
		case "esc":
			if m.profileEditing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileSpawnable
		case "backspace":
			if len(m.profileMaxParInput) > 0 {
				m.profileMaxParInput = m.profileMaxParInput[:len(m.profileMaxParInput)-1]
			}
		default:
			ch := msg.String()
			if len(ch) == 1 && ch >= "0" && ch <= "9" && len(m.profileMaxParInput) < 2 {
				m.profileMaxParInput += ch
			}
		}
	}
	return m, nil
}

// finishProfileCreation creates the profile, saves config, and returns to selector.
func (m AppModel) finishProfileCreation() (tea.Model, tea.Cmd) {
	selectedAgent := m.profileAgents[m.profileAgentSel]
	roles := config.AllRoles()
	selectedRole := roles[m.profileRoleSel]

	intel := 0
	if m.profileIntelInput != "" {
		if v, err := strconv.Atoi(m.profileIntelInput); err == nil && v >= 1 && v <= 10 {
			intel = v
		}
	}

	maxInst := 0
	if m.profileMaxInstInput != "" {
		if v, err := strconv.Atoi(m.profileMaxInstInput); err == nil && v > 0 {
			maxInst = v
		}
	}

	maxPar := 0
	if m.profileMaxParInput != "" {
		if v, err := strconv.Atoi(m.profileMaxParInput); err == nil && v > 0 {
			maxPar = v
		}
	}

	var spawnable []string
	for i, selected := range m.profileSpawnableSelected {
		if selected && i < len(m.profileSpawnableOptions) {
			spawnable = append(spawnable, m.profileSpawnableOptions[i])
		}
	}

	p := config.Profile{
		Name:              m.profileNameInput,
		Agent:             selectedAgent,
		Model:             m.profileSelectedModel,
		ReasoningLevel:    m.profileSelectedReasoning,
		Role:              selectedRole,
		Intelligence:      intel,
		Description:       strings.TrimSpace(m.profileDescInput),
		MaxInstances:      maxInst,
		SpawnableProfiles: spawnable,
		MaxParallel:       maxPar,
	}

	if m.profileEditing {
		// Remove old profile, then add updated one.
		m.globalCfg.RemoveProfile(m.profileEditName)
	}

	m.globalCfg.AddProfile(p)
	config.Save(m.globalCfg)
	m.rebuildProfiles()

	if m.profileEditing {
		// Select the edited profile by name.
		for i, pe := range m.profiles {
			if strings.EqualFold(pe.Name, p.Name) {
				m.selected = i
				break
			}
		}
	} else {
		// Select the new profile (last before sentinel).
		m.selected = len(m.profiles) - 2
		if m.selected < 0 {
			m.selected = 0
		}
	}

	m.profileEditing = false
	m.profileEditName = ""
	m.profileNameInput = ""
	m.profileSelectedModel = ""
	m.profileSelectedReasoning = ""
	m.profileDescInput = ""
	m.profileIntelInput = ""
	m.profileMaxInstInput = ""
	m.profileMaxParInput = ""
	m.profileSpawnableOptions = nil
	m.profileSpawnableSelected = nil
	m.state = stateSelector
	return m, nil
}

// --- Views ---

func profileWizardPanel(m AppModel) (lipgloss.Style, int, int) {
	panelH := m.height - 2
	if panelH < 1 {
		panelH = 1
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSurface2).
		Padding(1, 2)
	hf, vf := style.GetFrameSize()
	cw := m.width - hf
	ch := panelH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	return style, cw, ch
}

// viewProfileName renders the profile name input screen.
func (m AppModel) viewProfileName() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Name"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter a name for the new profile:"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	displayText := truncateInputForDisplay(m.profileNameInput, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(displayText)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: confirm  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileAgent renders the agent selection screen.
func (m AppModel) viewProfileAgent() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Select Agent"))
	lines = append(lines, "")

	for i, name := range m.profileAgents {
		if i == m.profileAgentSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+nameStyled)
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(name))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileModel renders the model selection screen.
func (m AppModel) viewProfileModel() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	agentName := ""
	if m.profileAgentSel < len(m.profileAgents) {
		agentName = m.profileAgents[m.profileAgentSel]
	}

	var lines []string

	if m.profileCustomModelMode {
		lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Custom Model"))
		lines = append(lines, dimStyle.Render("Agent: "+agentName))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Type the model name:"))
		lines = append(lines, "")
		cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
		inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.profileCustomModel)
		lines = append(lines, "> "+inputText+cursor)
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("enter: confirm  esc: back to list"))
	} else {
		lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Select Model"))
		lines = append(lines, dimStyle.Render("Agent: "+agentName))
		lines = append(lines, "")

		for i, model := range m.profileModels {
			styled := model
			if model == customModelSentinel {
				if i == m.profileModelSel {
					cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
					nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render(styled)
					lines = append(lines, cursor+nameStyled)
				} else {
					lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorOverlay0).Render(styled))
				}
			} else if i == m.profileModelSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				modelStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(styled)
				lines = append(lines, cursor+modelStyled)
			} else {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(styled))
			}
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileReasoning renders the reasoning level selection screen.
func (m AppModel) viewProfileReasoning() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	agentName := ""
	if m.profileAgentSel < len(m.profileAgents) {
		agentName = m.profileAgents[m.profileAgentSel]
	}

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Reasoning Level"))
	lines = append(lines, dimStyle.Render("Agent: "+agentName+"  Model: "+m.profileSelectedModel))
	lines = append(lines, "")

	for i, level := range m.profileReasoningLevels {
		label := level.Name
		if i == m.profileReasoningLevelSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+styled)
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileRole renders the role selection screen.
func (m AppModel) viewProfileRole() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Role"))
	lines = append(lines, "")

	roles := config.AllRoles()
	roleDescs := map[string]string{
		"manager":    "Breaks down tasks, delegates, reviews diffs",
		"senior":     "Writes code and can delegate to juniors",
		"junior":     "Focuses on assigned tasks, no spawning",
		"supervisor": "Reviews progress, sends notes, no code",
	}
	for i, role := range roles {
		label := role
		if desc, ok := roleDescs[role]; ok {
			label = fmt.Sprintf("%-12s %s", role, dimStyle.Render(desc))
		}
		if i == m.profileRoleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+styled)
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileIntel renders the intelligence rating input.
func (m AppModel) viewProfileIntel() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Intelligence"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Rate this profile's capability (1-10, or empty to skip):"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.profileIntelInput)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileDesc renders the description input screen.
func (m AppModel) viewProfileDesc() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Description"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Describe strengths/weaknesses (or enter to skip):"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	displayDesc := truncateInputForDisplay(m.profileDescInput, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(displayDesc)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileMaxInst renders the max instances input.
func (m AppModel) viewProfileMaxInst() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Max Instances"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Max concurrent instances of this profile"))
	lines = append(lines, dimStyle.Render("(empty = unlimited):"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.profileMaxInstInput)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileSpawnable renders the spawnable profiles multi-select.
func (m AppModel) viewProfileSpawnable() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Spawnable Profiles"))
	lines = append(lines, dimStyle.Render("Select profiles this agent can spawn (space to toggle):"))
	lines = append(lines, "")

	if len(m.profileSpawnableOptions) == 0 {
		lines = append(lines, dimStyle.Render("No other profiles available."))
	}
	for i, name := range m.profileSpawnableOptions {
		check := "[ ]"
		if m.profileSpawnableSelected[i] {
			check = lipgloss.NewStyle().Foreground(ColorGreen).Render("[x]")
		}
		if i == m.profileSpawnableSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+check+" "+nameStyled)
		} else {
			lines = append(lines, "  "+check+" "+lipgloss.NewStyle().Foreground(ColorText).Render(name))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// viewProfileMaxPar renders the max parallel input.
func (m AppModel) viewProfileMaxPar() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Max Parallel"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Maximum concurrent sub-agents (default: 2):"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.profileMaxParInput)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	if m.profileEditing {
		lines = append(lines, dimStyle.Render("enter: save profile  esc: back"))
	} else {
		lines = append(lines, dimStyle.Render("enter: create profile  esc: back"))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
