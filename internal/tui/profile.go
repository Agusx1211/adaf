package tui

import (
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

	// Prefer detected models from agents.json (authoritative when present).
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
			if m.globalCfg.FindProfile(name) != nil {
				return m, nil // duplicate
			}
			m.profileNameInput = name
			m.profileAgents = agentmeta.Names()
			m.profileAgentSel = 0
			m.state = stateProfileAgent
			return m, nil
		case "esc":
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
				selectedAgent := m.profileAgents[m.profileAgentSel]
				agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())
				m.profileModels = buildModelChoices(selectedAgent, agentsCfg)
				m.profileModelSel = 0
				m.profileCustomModel = ""
				m.profileCustomModelMode = false
				m.state = stateProfileModel
			}
		case "esc":
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
				return m.transitionToReasoningOrFinish()
			}
		case "esc":
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
// supports it, otherwise creates the profile immediately.
func (m AppModel) transitionToReasoningOrFinish() (tea.Model, tea.Cmd) {
	selectedAgent := m.profileAgents[m.profileAgentSel]
	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())
	levels := buildReasoningChoices(selectedAgent, agentsCfg)
	if len(levels) > 0 {
		m.profileReasoningLevels = levels
		m.profileReasoningLevelSel = 0
		m.state = stateProfileReasoning
		return m, nil
	}
	return m.finishProfileCreation("")
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
				return m.finishProfileCreation(level.Name)
			}
		case "esc":
			m.state = stateProfileModel
		}
	}
	return m, nil
}

// finishProfileCreation creates the profile, saves config, and returns to selector.
func (m AppModel) finishProfileCreation(reasoningLevel string) (tea.Model, tea.Cmd) {
	selectedAgent := m.profileAgents[m.profileAgentSel]
	p := config.Profile{
		Name:           m.profileNameInput,
		Agent:          selectedAgent,
		Model:          m.profileSelectedModel,
		ReasoningLevel: reasoningLevel,
	}
	m.globalCfg.AddProfile(p)
	config.Save(m.globalCfg)
	m.rebuildProfiles()
	// Select the new profile (last before sentinel).
	m.selected = len(m.profiles) - 2
	if m.selected < 0 {
		m.selected = 0
	}
	m.profileNameInput = ""
	m.profileSelectedModel = ""
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
	lines = append(lines, sectionStyle.Render("New Profile — Name"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter a name for the new profile:"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.profileNameInput)
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
	lines = append(lines, sectionStyle.Render("New Profile — Select Agent"))
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
		lines = append(lines, sectionStyle.Render("New Profile — Custom Model"))
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
		lines = append(lines, sectionStyle.Render("New Profile — Select Model"))
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
	lines = append(lines, sectionStyle.Render("New Profile — Reasoning Level"))
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
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: create  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
