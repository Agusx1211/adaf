package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
)

type ProfileWizardState struct {
	Editing           bool
	EditName          string
	NameInput         string
	Agents            []string
	AgentSel          int
	Models            []string
	ModelSel          int
	CustomModel       string
	CustomModelMode   bool
	SelectedModel     string
	ReasoningLevels   []agentmeta.ReasoningLevel
	ReasoningLevelSel int
	SelectedReasoning string
	IntelInput        string
	DescInput         string
	MenuSel           int
	MaxInstInput      string
	SpeedSel          int
}

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
	p := m.profiles[m.selector.Selected]
	prof := m.globalCfg.FindProfile(p.Name)
	if prof == nil {
		return m, nil
	}

	m.profileWiz.Editing = true
	m.profileWiz.EditName = prof.Name
	m.profileWiz.NameInput = prof.Name

	// Pre-populate agent list and selection.
	m.profileWiz.Agents = agentmeta.Names()
	m.profileWiz.AgentSel = 0
	for i, name := range m.profileWiz.Agents {
		if name == prof.Agent {
			m.profileWiz.AgentSel = i
			break
		}
	}

	// Store model/reasoning.
	m.profileWiz.SelectedModel = prof.Model
	m.profileWiz.SelectedReasoning = prof.ReasoningLevel

	// Pre-populate text inputs.
	m.profileWiz.DescInput = prof.Description
	m.profileWiz.IntelInput = ""
	if prof.Intelligence > 0 {
		m.profileWiz.IntelInput = strconv.Itoa(prof.Intelligence)
	}
	m.profileWiz.MaxInstInput = ""
	if prof.MaxInstances > 0 {
		m.profileWiz.MaxInstInput = strconv.Itoa(prof.MaxInstances)
	}

	m.profileWiz.SpeedSel = 0
	for i, opt := range profileSpeedOptions {
		if opt == prof.Speed {
			m.profileWiz.SpeedSel = i
			break
		}
	}

	m.profileWiz.MenuSel = 0
	m.state = stateProfileMenu
	return m, nil
}

// wizardTitle returns "Edit Profile" or "New Profile" depending on mode.
func (m AppModel) wizardTitle() string {
	if m.profileWiz.Editing {
		return "Edit Profile"
	}
	return "New Profile"
}

// profileSpeedOptions are the valid speed choices for profiles.
var profileSpeedOptions = []string{"(none)", "fast", "medium", "slow"}

// editMenuItemCount is the number of items in the edit menu (8 fields + Save).
const editMenuItemCount = 9

// updateProfileMenu handles the edit profile field picker menu.
func (m AppModel) updateProfileMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.profileWiz.MenuSel = (m.profileWiz.MenuSel + 1) % editMenuItemCount
		case "k", "up":
			m.profileWiz.MenuSel = (m.profileWiz.MenuSel - 1 + editMenuItemCount) % editMenuItemCount
		case "esc":
			m.profileWiz.Editing = false
			m.state = stateSelector
		case "enter":
			switch m.profileWiz.MenuSel {
			case 0: // Name
				m.state = stateProfileName
			case 1: // Agent
				m.profileWiz.Agents = agentmeta.Names()
				m.state = stateProfileAgent
			case 2: // Model
				selectedAgent := m.profileWiz.Agents[m.profileWiz.AgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig()
				m.profileWiz.Models = buildModelChoices(selectedAgent, agentsCfg)
				m.profileWiz.ModelSel = 0
				if m.profileWiz.SelectedModel != "" {
					for i, model := range m.profileWiz.Models {
						if model == m.profileWiz.SelectedModel {
							m.profileWiz.ModelSel = i
							break
						}
					}
				}
				m.profileWiz.CustomModelMode = false
				m.state = stateProfileModel
			case 3: // Reasoning
				selectedAgent := m.profileWiz.Agents[m.profileWiz.AgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig()
				levels := buildReasoningChoices(selectedAgent, agentsCfg)
				if len(levels) == 0 {
					return m, nil // no reasoning levels for this agent
				}
				m.profileWiz.ReasoningLevels = levels
				m.profileWiz.ReasoningLevelSel = 0
				if m.profileWiz.SelectedReasoning != "" {
					for i, l := range levels {
						if l.Name == m.profileWiz.SelectedReasoning {
							m.profileWiz.ReasoningLevelSel = i
							break
						}
					}
				}
				m.state = stateProfileReasoning
			case 4: // Intelligence
				m.state = stateProfileIntel
			case 5: // Description
				m.state = stateProfileDesc
			case 6: // Max Instances
				m.state = stateProfileMaxInst
			case 7: // Speed
				m.state = stateProfileSpeed
			case 8: // Save
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
	cursorLine := -1
	lines = append(lines, sectionStyle.Render("Edit Profile"))
	lines = append(lines, "")

	// Build field values for display.
	agentName := ""
	if m.profileWiz.AgentSel < len(m.profileWiz.Agents) {
		agentName = m.profileWiz.Agents[m.profileWiz.AgentSel]
	}
	model := m.profileWiz.SelectedModel
	if model == "" {
		model = "(default)"
	}
	reasoning := m.profileWiz.SelectedReasoning
	if reasoning == "" {
		reasoning = "(none)"
	}
	intel := m.profileWiz.IntelInput
	if intel == "" {
		intel = "-"
	}
	desc := m.profileWiz.DescInput
	if desc == "" {
		desc = "-"
	}
	maxDescW := cw - 18
	if maxDescW > 0 && len(desc) > maxDescW {
		desc = desc[:maxDescW-3] + "..."
	}
	maxInst := m.profileWiz.MaxInstInput
	if maxInst == "" {
		maxInst = "unlimited"
	}
	speed := "(none)"
	if m.profileWiz.SpeedSel > 0 && m.profileWiz.SpeedSel < len(profileSpeedOptions) {
		speed = profileSpeedOptions[m.profileWiz.SpeedSel]
	}

	type menuItem struct {
		label, value string
	}
	items := []menuItem{
		{"Name", m.profileWiz.NameInput},
		{"Agent", agentName},
		{"Model", model},
		{"Reasoning", reasoning},
		{"Intelligence", intel},
		{"Description", desc},
		{"Max Instances", maxInst},
		{"Speed", speed},
	}

	for i, item := range items {
		line := labelStyle.Render(item.label) + valueStyle.Render(item.value)
		if i == m.profileWiz.MenuSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			lines = append(lines, cursor+line)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+line)
		}
	}

	// Save item
	lines = append(lines, "")
	saveLabel := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("Save")
	if m.profileWiz.MenuSel == 8 {
		cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
		lines = append(lines, cursor+saveLabel)
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+saveLabel)
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: edit field  esc: cancel"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// updateProfileName handles input in the profile name state.
func (m AppModel) updateProfileName(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("profile-name", m.profileWiz.NameInput, 0)
	m.syncTextInput(m.profileWiz.NameInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			name := strings.TrimSpace(m.profileWiz.NameInput)
			if name == "" {
				return m, nil
			}
			// Allow same name when editing the same profile.
			if existing := m.globalCfg.FindProfile(name); existing != nil {
				if !m.profileWiz.Editing || !strings.EqualFold(name, m.profileWiz.EditName) {
					return m, nil // duplicate
				}
			}
			m.profileWiz.NameInput = name
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileWiz.Agents = agentmeta.Names()
			m.profileWiz.AgentSel = 0
			m.state = stateProfileAgent
			return m, nil
		case tea.KeyEsc:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileWiz.NameInput = ""
			m.state = stateSelector
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.profileWiz.NameInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

// updateProfileAgent handles agent selection in the profile wizard.
func (m AppModel) updateProfileAgent(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileWiz.Agents) > 0 {
				m.profileWiz.AgentSel = (m.profileWiz.AgentSel + 1) % len(m.profileWiz.Agents)
			}
		case "k", "up":
			if len(m.profileWiz.Agents) > 0 {
				m.profileWiz.AgentSel = (m.profileWiz.AgentSel - 1 + len(m.profileWiz.Agents)) % len(m.profileWiz.Agents)
			}
		case "enter":
			if len(m.profileWiz.Agents) > 0 {
				if m.profileWiz.Editing {
					m.state = stateProfileMenu
					return m, nil
				}
				selectedAgent := m.profileWiz.Agents[m.profileWiz.AgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig()
				m.profileWiz.Models = buildModelChoices(selectedAgent, agentsCfg)
				m.profileWiz.ModelSel = 0
				m.profileWiz.CustomModel = ""
				m.profileWiz.CustomModelMode = false
				m.state = stateProfileModel
			}
		case "esc":
			if m.profileWiz.Editing {
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
	if m.profileWiz.CustomModelMode {
		return m.updateCustomModelInput(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileWiz.Models) > 0 {
				m.profileWiz.ModelSel = (m.profileWiz.ModelSel + 1) % len(m.profileWiz.Models)
			}
		case "k", "up":
			if len(m.profileWiz.Models) > 0 {
				m.profileWiz.ModelSel = (m.profileWiz.ModelSel - 1 + len(m.profileWiz.Models)) % len(m.profileWiz.Models)
			}
		case "enter":
			if len(m.profileWiz.Models) > 0 {
				model := m.profileWiz.Models[m.profileWiz.ModelSel]
				if model == customModelSentinel {
					m.profileWiz.CustomModelMode = true
					m.profileWiz.CustomModel = ""
					m.textInputKey = ""
					return m, nil
				}
				if model == "(default)" {
					model = ""
				}
				m.profileWiz.SelectedModel = model
				if m.profileWiz.Editing {
					m.state = stateProfileMenu
					return m, nil
				}
				return m.transitionToReasoningOrFinish()
			}
		case "esc":
			if m.profileWiz.Editing {
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
	initCmd := m.ensureTextInput("profile-custom-model", m.profileWiz.CustomModel, 0)
	m.syncTextInput(m.profileWiz.CustomModel)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			model := strings.TrimSpace(m.profileWiz.CustomModel)
			if model == "" {
				return m, nil
			}
			m.profileWiz.SelectedModel = model
			m.profileWiz.CustomModelMode = false
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			return m.transitionToReasoningOrFinish()
		case tea.KeyEsc:
			m.profileWiz.CustomModelMode = false
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.profileWiz.CustomModel = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

// transitionToReasoningOrFinish moves to reasoning level selection if the agent
// supports it, otherwise skips directly to intelligence input.
func (m AppModel) transitionToReasoningOrFinish() (tea.Model, tea.Cmd) {
	selectedAgent := m.profileWiz.Agents[m.profileWiz.AgentSel]
	agentsCfg, _ := agent.LoadAgentsConfig()
	levels := buildReasoningChoices(selectedAgent, agentsCfg)
	if len(levels) > 0 {
		m.profileWiz.ReasoningLevels = levels
		m.profileWiz.ReasoningLevelSel = 0
		// When editing, pre-select existing reasoning level.
		if m.profileWiz.Editing && m.profileWiz.SelectedReasoning != "" {
			for i, l := range levels {
				if l.Name == m.profileWiz.SelectedReasoning {
					m.profileWiz.ReasoningLevelSel = i
					break
				}
			}
		}
		m.state = stateProfileReasoning
		return m, nil
	}
	m.profileWiz.SelectedReasoning = ""
	m.profileWiz.IntelInput = ""
	m.state = stateProfileIntel
	return m, nil
}

// updateProfileReasoning handles reasoning level selection.
func (m AppModel) updateProfileReasoning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.profileWiz.ReasoningLevels) > 0 {
				m.profileWiz.ReasoningLevelSel = (m.profileWiz.ReasoningLevelSel + 1) % len(m.profileWiz.ReasoningLevels)
			}
		case "k", "up":
			if len(m.profileWiz.ReasoningLevels) > 0 {
				m.profileWiz.ReasoningLevelSel = (m.profileWiz.ReasoningLevelSel - 1 + len(m.profileWiz.ReasoningLevels)) % len(m.profileWiz.ReasoningLevels)
			}
		case "enter":
			if len(m.profileWiz.ReasoningLevels) > 0 {
				level := m.profileWiz.ReasoningLevels[m.profileWiz.ReasoningLevelSel]
				m.profileWiz.SelectedReasoning = level.Name
				if m.profileWiz.Editing {
					m.state = stateProfileMenu
					return m, nil
				}
				m.profileWiz.IntelInput = ""
				m.state = stateProfileIntel
				return m, nil
			}
		case "esc":
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileModel
		}
	}
	return m, nil
}

// updateProfileIntel handles intelligence rating input.
func (m AppModel) updateProfileIntel(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("profile-intel", m.profileWiz.IntelInput, 2)
	m.syncTextInput(m.profileWiz.IntelInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			// Move to description.
			m.profileWiz.DescInput = ""
			m.state = stateProfileDesc
			return m, nil
		case tea.KeyEsc:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			if len(m.profileWiz.ReasoningLevels) > 0 {
				m.state = stateProfileReasoning
			} else {
				m.state = stateProfileModel
			}
			return m, nil
		}
	}
	filteredMsg, ok := sanitizeDigitsMsg(msg)
	if !ok {
		return m, initCmd
	}
	cmd := m.updateTextInput(filteredMsg)
	filtered := digitsOnly(m.textInput.Value(), 2)
	if filtered != m.textInput.Value() {
		m.textInput.SetValue(filtered)
		m.textInput.CursorEnd()
	}
	m.profileWiz.IntelInput = filtered
	return m, tea.Batch(initCmd, cmd)
}

// updateProfileDesc handles description text input.
func (m AppModel) updateProfileDesc(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("profile-desc", m.profileWiz.DescInput, 0)
	m.syncTextInput(m.profileWiz.DescInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileWiz.MaxInstInput = ""
			m.state = stateProfileMaxInst
			return m, nil
		case tea.KeyEsc:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileIntel
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.profileWiz.DescInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

// updateProfileMaxInst handles max instances input.
func (m AppModel) updateProfileMaxInst(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("profile-max-inst", m.profileWiz.MaxInstInput, 2)
	m.syncTextInput(m.profileWiz.MaxInstInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.profileWiz.SpeedSel = 0
			m.state = stateProfileSpeed
			return m, nil
		case tea.KeyEsc:
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileDesc
			return m, nil
		}
	}
	filteredMsg, ok := sanitizeDigitsMsg(msg)
	if !ok {
		return m, initCmd
	}
	cmd := m.updateTextInput(filteredMsg)
	filtered := digitsOnly(m.textInput.Value(), 2)
	if filtered != m.textInput.Value() {
		m.textInput.SetValue(filtered)
		m.textInput.CursorEnd()
	}
	m.profileWiz.MaxInstInput = filtered
	return m, tea.Batch(initCmd, cmd)
}

// finishProfileCreation creates the profile, saves config, and returns to selector.
func (m AppModel) finishProfileCreation() (tea.Model, tea.Cmd) {
	selectedAgent := m.profileWiz.Agents[m.profileWiz.AgentSel]

	intel := 0
	if m.profileWiz.IntelInput != "" {
		if v, err := strconv.Atoi(m.profileWiz.IntelInput); err == nil && v >= 1 && v <= 10 {
			intel = v
		}
	}

	maxInst := 0
	if m.profileWiz.MaxInstInput != "" {
		if v, err := strconv.Atoi(m.profileWiz.MaxInstInput); err == nil && v > 0 {
			maxInst = v
		}
	}

	speed := ""
	if m.profileWiz.SpeedSel > 0 && m.profileWiz.SpeedSel < len(profileSpeedOptions) {
		speed = profileSpeedOptions[m.profileWiz.SpeedSel]
	}

	p := config.Profile{
		Name:           m.profileWiz.NameInput,
		Agent:          selectedAgent,
		Model:          m.profileWiz.SelectedModel,
		ReasoningLevel: m.profileWiz.SelectedReasoning,
		Intelligence:   intel,
		Description:    strings.TrimSpace(m.profileWiz.DescInput),
		MaxInstances:   maxInst,
		Speed:          speed,
	}

	if m.profileWiz.Editing {
		// Remove old profile, then add updated one.
		m.globalCfg.RemoveProfile(m.profileWiz.EditName)
	}

	m.globalCfg.AddProfile(p)
	config.Save(m.globalCfg)
	m.rebuildProfiles()

	if m.profileWiz.Editing {
		// Select the edited profile by name.
		for i, pe := range m.profiles {
			if strings.EqualFold(pe.Name, p.Name) {
				m.selector.Selected = i
				break
			}
		}
	} else {
		// Select the new profile (last before sentinel).
		m.selector.Selected = len(m.profiles) - 2
		if m.selector.Selected < 0 {
			m.selector.Selected = 0
		}
	}

	m.profileWiz.Editing = false
	m.profileWiz.EditName = ""
	m.profileWiz.NameInput = ""
	m.profileWiz.SelectedModel = ""
	m.profileWiz.SelectedReasoning = ""
	m.profileWiz.DescInput = ""
	m.profileWiz.IntelInput = ""
	m.profileWiz.MaxInstInput = ""
	m.profileWiz.SpeedSel = 0
	m.state = stateSelector
	return m, nil
}

// updateProfileSpeed handles speed selection in the profile wizard.
func (m AppModel) updateProfileSpeed(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.profileWiz.SpeedSel = (m.profileWiz.SpeedSel + 1) % len(profileSpeedOptions)
		case "k", "up":
			m.profileWiz.SpeedSel = (m.profileWiz.SpeedSel - 1 + len(profileSpeedOptions)) % len(profileSpeedOptions)
		case "enter":
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			return m.finishProfileCreation()
		case "esc":
			if m.profileWiz.Editing {
				m.state = stateProfileMenu
				return m, nil
			}
			m.state = stateProfileMaxInst
		}
	}
	return m, nil
}

// viewProfileSpeed renders the speed selection screen.
func (m AppModel) viewProfileSpeed() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Speed"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Select speed rating for this profile:"))
	lines = append(lines, "")

	for i, opt := range profileSpeedOptions {
		if i == m.profileWiz.SpeedSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(opt)
			lines = append(lines, cursor+styled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(opt))
		}
	}
	lines = append(lines, "")
	if m.profileWiz.Editing {
		lines = append(lines, dimStyle.Render("j/k: navigate  enter: save  esc: back"))
	} else {
		lines = append(lines, dimStyle.Render("j/k: navigate  enter: create profile  esc: back"))
	}

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
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
	m.ensureTextInput("profile-name", m.profileWiz.NameInput, 0)
	m.syncTextInput(m.profileWiz.NameInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Name"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter a name for the new profile:"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
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
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Select Agent"))
	lines = append(lines, "")

	for i, name := range m.profileWiz.Agents {
		if i == m.profileWiz.AgentSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+nameStyled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(name))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
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
	if m.profileWiz.AgentSel < len(m.profileWiz.Agents) {
		agentName = m.profileWiz.Agents[m.profileWiz.AgentSel]
	}

	var lines []string
	cursorLine := -1

	if m.profileWiz.CustomModelMode {
		m.ensureTextInput("profile-custom-model", m.profileWiz.CustomModel, 0)
		m.syncTextInput(m.profileWiz.CustomModel)
		lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Custom Model"))
		lines = append(lines, dimStyle.Render("Agent: "+agentName))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Type the model name:"))
		lines = append(lines, "")
		lines = append(lines, m.viewTextInput(cw-4))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("enter: confirm  esc: back to list"))
	} else {
		lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Select Model"))
		lines = append(lines, dimStyle.Render("Agent: "+agentName))
		lines = append(lines, "")

		for i, model := range m.profileWiz.Models {
			styled := model
			if model == customModelSentinel {
				if i == m.profileWiz.ModelSel {
					cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
					nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render(styled)
					lines = append(lines, cursor+nameStyled)
					cursorLine = len(lines) - 1
				} else {
					lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorOverlay0).Render(styled))
				}
			} else if i == m.profileWiz.ModelSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				modelStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(styled)
				lines = append(lines, cursor+modelStyled)
				cursorLine = len(lines) - 1
			} else {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(styled))
			}
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))
	}

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
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
	if m.profileWiz.AgentSel < len(m.profileWiz.Agents) {
		agentName = m.profileWiz.Agents[m.profileWiz.AgentSel]
	}

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Reasoning Level"))
	lines = append(lines, dimStyle.Render("Agent: "+agentName+"  Model: "+m.profileWiz.SelectedModel))
	lines = append(lines, "")

	for i, level := range m.profileWiz.ReasoningLevels {
		label := level.Name
		if i == m.profileWiz.ReasoningLevelSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+styled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
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
	m.ensureTextInput("profile-intel", m.profileWiz.IntelInput, 2)
	m.syncTextInput(m.profileWiz.IntelInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Intelligence"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Rate this profile's capability (1-10, or empty to skip):"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
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
	m.ensureTextInput("profile-desc", m.profileWiz.DescInput, 0)
	m.syncTextInput(m.profileWiz.DescInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Description"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Describe strengths/weaknesses (or enter to skip):"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
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
	m.ensureTextInput("profile-max-inst", m.profileWiz.MaxInstInput, 2)
	m.syncTextInput(m.profileWiz.MaxInstInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.wizardTitle()+" — Max Instances"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Max concurrent instances of this profile"))
	lines = append(lines, dimStyle.Render("(empty = unlimited):"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
