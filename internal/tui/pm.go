package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
)

const (
	pmConfigFieldProfile = iota
	pmConfigFieldModel
	pmConfigFieldRun
	pmConfigFieldTotal
)

const pmPromptPrefix = "# Role: Project Manager\n\n" +
	"You are operating in project manager mode. Focus on plans, issues, and documentation updates.\n" +
	"Keep outcomes concrete, prioritized, and actionable.\n\n" +
	"## User Message\n\n"

type PMWizardState struct {
	MessageText   string
	ProfileSel    int
	Profiles      []string
	ModelOverride string

	ConfigSel int
	Msg       string
}

func (m *AppModel) initPMWizard() {
	if m.globalCfg == nil {
		m.globalCfg = &config.GlobalConfig{}
	}

	prevName := ""
	if m.pmWiz.ProfileSel >= 0 && m.pmWiz.ProfileSel < len(m.pmWiz.Profiles) {
		prevName = m.pmWiz.Profiles[m.pmWiz.ProfileSel]
	}

	profiles := make([]string, 0, len(m.globalCfg.Profiles))
	for _, prof := range m.globalCfg.Profiles {
		profiles = append(profiles, prof.Name)
	}
	m.pmWiz.Profiles = profiles
	m.pmWiz.ProfileSel = 0
	if prevName != "" {
		for i, name := range profiles {
			if strings.EqualFold(name, prevName) {
				m.pmWiz.ProfileSel = i
				break
			}
		}
	}
	if m.pmWiz.ProfileSel >= len(profiles) {
		m.pmWiz.ProfileSel = len(profiles) - 1
	}
	if m.pmWiz.ProfileSel < 0 {
		m.pmWiz.ProfileSel = 0
	}
	if m.pmWiz.ConfigSel < 0 || m.pmWiz.ConfigSel >= pmConfigFieldTotal {
		m.pmWiz.ConfigSel = 0
	}
	m.pmWiz.Msg = ""
}

func (m *AppModel) clampPMProfileSelection() {
	if m.pmWiz.ProfileSel >= len(m.pmWiz.Profiles) {
		m.pmWiz.ProfileSel = len(m.pmWiz.Profiles) - 1
	}
	if m.pmWiz.ProfileSel < 0 {
		m.pmWiz.ProfileSel = 0
	}
}

func (m *AppModel) adjustPMProfileSelection(delta int) {
	if len(m.pmWiz.Profiles) == 0 {
		m.pmWiz.ProfileSel = 0
		return
	}
	sel := m.pmWiz.ProfileSel + delta
	for sel < 0 {
		sel += len(m.pmWiz.Profiles)
	}
	m.pmWiz.ProfileSel = sel % len(m.pmWiz.Profiles)
}

func (m *AppModel) pmSelectedProfile() *config.Profile {
	if m.globalCfg == nil {
		return nil
	}
	if len(m.pmWiz.Profiles) == 0 {
		return nil
	}
	m.clampPMProfileSelection()
	if m.pmWiz.ProfileSel < 0 || m.pmWiz.ProfileSel >= len(m.pmWiz.Profiles) {
		return nil
	}
	return m.globalCfg.FindProfile(m.pmWiz.Profiles[m.pmWiz.ProfileSel])
}

func (m *AppModel) pmConfigNextField() {
	m.pmWiz.ConfigSel = (m.pmWiz.ConfigSel + 1) % pmConfigFieldTotal
}

func (m *AppModel) pmConfigPrevField() {
	m.pmWiz.ConfigSel = (m.pmWiz.ConfigSel - 1 + pmConfigFieldTotal) % pmConfigFieldTotal
}

func (m AppModel) startPMWizard() (tea.Model, tea.Cmd) {
	m.initPMWizard()
	m.state = statePMPrompt
	return m, nil
}

func (m AppModel) startPMSession() (tea.Model, tea.Cmd) {
	if m.globalCfg == nil {
		m.pmWiz.Msg = "Global configuration is unavailable."
		return m, nil
	}
	if len(m.pmWiz.Profiles) == 0 {
		m.pmWiz.Msg = "No profiles available. Create one from the selector first."
		return m, nil
	}

	message := strings.TrimSpace(m.pmWiz.MessageText)
	if message == "" {
		m.pmWiz.Msg = "Message is empty."
		m.state = statePMPrompt
		return m, nil
	}

	prof := m.pmSelectedProfile()
	if prof == nil {
		m.pmWiz.Msg = "Selected profile is no longer available."
		return m, nil
	}

	runProfile := *prof
	if override := strings.TrimSpace(m.pmWiz.ModelOverride); override != "" {
		runProfile.Model = override
	}

	loopDef := config.LoopDef{
		Name: "pm",
		Steps: []config.LoopStep{
			{
				Profile:      runProfile.Name,
				Role:         config.RoleManager,
				Turns:        1,
				Instructions: pmPromptPrefix + message,
			},
		},
	}

	m.pmWiz.Msg = ""
	return m.startLoopSession(loopDef, []config.Profile{runProfile}, runProfile.Name, runProfile.Agent, nil, 1)
}

func (m AppModel) updatePMPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextarea("pm-message", m.pmWiz.MessageText)
	m.syncTextarea(m.pmWiz.MessageText)

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.pmWiz.Msg = ""
			m.state = stateSelector
			return m, nil
		case tea.KeyCtrlS:
			m.pmWiz.MessageText = m.textArea.Value()
			if strings.TrimSpace(m.pmWiz.MessageText) == "" {
				m.pmWiz.Msg = "Message is empty."
				return m, initCmd
			}
			m.pmWiz.Msg = ""
			m.pmWiz.ConfigSel = 0
			m.state = statePMConfig
			return m, nil
		}
		switch keyMsg.String() {
		case "ctrl+enter", "ctrl+j":
			m.pmWiz.MessageText = m.textArea.Value()
			if strings.TrimSpace(m.pmWiz.MessageText) == "" {
				m.pmWiz.Msg = "Message is empty."
				return m, initCmd
			}
			m.pmWiz.Msg = ""
			m.pmWiz.ConfigSel = 0
			m.state = statePMConfig
			return m, nil
		}
	}

	cmd := m.updateTextarea(msg)
	m.pmWiz.MessageText = m.textArea.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewPMPrompt() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRed)

	m.ensureTextarea("pm-message", m.pmWiz.MessageText)
	m.syncTextarea(m.pmWiz.MessageText)

	var lines []string
	lines = append(lines, sectionStyle.Render("Project Manager — Enter Message"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Type your project management message (multi-line supported)."))
	lines = append(lines, dimStyle.Render("ctrl+s or ctrl+enter: next  esc: cancel"))
	lines = append(lines, "")

	if msg := strings.TrimSpace(m.pmWiz.Msg); msg != "" {
		lines = append(lines, errStyle.Render(msg))
		lines = append(lines, "")
	}

	prefixLines := wrapRenderableLines(lines, cw)
	editorHeight := ch - len(prefixLines)
	if editorHeight < 3 {
		editorHeight = 3
	}
	editorView := m.viewTextarea(cw, editorHeight)
	lines = append(lines, splitRenderableLines(editorView)...)

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

func (m AppModel) updatePMConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.clampPMProfileSelection()
	if m.pmWiz.ConfigSel < 0 || m.pmWiz.ConfigSel >= pmConfigFieldTotal {
		m.pmWiz.ConfigSel = 0
	}

	if m.pmWiz.ConfigSel == pmConfigFieldModel {
		initCmd := m.ensureTextInput("pm-model-override", m.pmWiz.ModelOverride, 0)
		m.syncTextInput(m.pmWiz.ModelOverride)
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.pmWiz.Msg = ""
				m.state = statePMPrompt
				return m, nil
			case "tab", "down", "j":
				m.pmConfigNextField()
				return m, nil
			case "shift+tab", "up", "k":
				m.pmConfigPrevField()
				return m, nil
			case "enter":
				m.pmConfigNextField()
				return m, nil
			}
		}
		cmd := m.updateTextInput(msg)
		m.pmWiz.ModelOverride = m.textInput.Value()
		return m, tea.Batch(initCmd, cmd)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.pmWiz.Msg = ""
			m.state = statePMPrompt
			return m, nil
		case "tab", "down", "j":
			m.pmConfigNextField()
			return m, nil
		case "shift+tab", "up", "k":
			m.pmConfigPrevField()
			return m, nil
		}

		switch m.pmWiz.ConfigSel {
		case pmConfigFieldProfile:
			switch keyMsg.String() {
			case "left", "h":
				m.adjustPMProfileSelection(-1)
			case "right", "l":
				m.adjustPMProfileSelection(1)
			case "enter":
				m.pmConfigNextField()
			}
		case pmConfigFieldRun:
			if keyMsg.String() == "enter" {
				return m.startPMSession()
			}
		}
	}

	return m, nil
}

func (m AppModel) viewPMConfig() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRed)

	var lines []string
	cursorLine := -1

	lines = append(lines, sectionStyle.Render("Project Manager — Configure"))
	lines = append(lines, "")

	selectedProfile := m.pmSelectedProfile()
	profileText := "(no profile)"
	if selectedProfile != nil {
		modelText := selectedProfile.Model
		if strings.TrimSpace(modelText) == "" {
			modelText = "(default)"
		}
		profileText = fmt.Sprintf("%s (%s, %s)", selectedProfile.Name, selectedProfile.Agent, modelText)
	}

	renderField := func(field int, label, value string) {
		line := fmt.Sprintf("%-8s %s", label, value)
		if m.pmWiz.ConfigSel == field {
			lines = append(lines, selectedStyle.Render("> "+line))
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+valueStyle.Render(line))
		}
	}

	renderField(pmConfigFieldProfile, "Profile:", profileText)

	modelValue := strings.TrimSpace(m.pmWiz.ModelOverride)
	if modelValue == "" {
		modelValue = "(default profile model)"
	}
	renderField(pmConfigFieldModel, "Model:", modelValue)

	runLabel := "[ Run PM ]"
	if m.pmWiz.ConfigSel == pmConfigFieldRun {
		lines = append(lines, selectedStyle.Render("> "+runLabel))
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+valueStyle.Render(runLabel))
	}

	if m.pmWiz.ConfigSel == pmConfigFieldModel {
		m.ensureTextInput("pm-model-override", m.pmWiz.ModelOverride, 0)
		m.syncTextInput(m.pmWiz.ModelOverride)
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Model override input:"))
		lines = append(lines, m.viewTextInput(cw-4))
	}

	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Preview"))
	preview := strings.TrimSpace(m.pmWiz.MessageText)
	if preview == "" {
		lines = append(lines, dimStyle.Render("(empty message)"))
	} else {
		lines = append(lines, dimStyle.Render(truncateInputForDisplay(strings.ReplaceAll(preview, "\n", " "), cw-4)))
	}
	lines = append(lines, dimStyle.Render("Runs: 1 cycle"))

	if msg := strings.TrimSpace(m.pmWiz.Msg); msg != "" {
		lines = append(lines, "")
		lines = append(lines, errStyle.Render(msg))
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("tab/up/down: field  left/right: adjust  enter: run/select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
