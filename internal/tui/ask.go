package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
)

const (
	askConfigFieldProfile = iota
	askConfigFieldCount
	askConfigFieldChain
	askConfigFieldModel
	askConfigFieldRun
	askConfigFieldTotal
)

type AskWizardState struct {
	PromptText    string
	ProfileSel    int
	Profiles      []string
	Count         int
	Chain         bool
	ModelOverride string

	ConfigSel int
	Msg       string
}

func (m *AppModel) initAskWizard() {
	if m.globalCfg == nil {
		m.globalCfg = &config.GlobalConfig{}
	}

	prevName := ""
	if m.askWiz.ProfileSel >= 0 && m.askWiz.ProfileSel < len(m.askWiz.Profiles) {
		prevName = m.askWiz.Profiles[m.askWiz.ProfileSel]
	}

	profiles := make([]string, 0, len(m.globalCfg.Profiles))
	for _, prof := range m.globalCfg.Profiles {
		profiles = append(profiles, prof.Name)
	}
	m.askWiz.Profiles = profiles
	m.askWiz.ProfileSel = 0
	if prevName != "" {
		for i, name := range profiles {
			if strings.EqualFold(name, prevName) {
				m.askWiz.ProfileSel = i
				break
			}
		}
	}
	if m.askWiz.ProfileSel >= len(profiles) {
		m.askWiz.ProfileSel = len(profiles) - 1
	}
	if m.askWiz.ProfileSel < 0 {
		m.askWiz.ProfileSel = 0
	}
	if m.askWiz.Count < 1 {
		m.askWiz.Count = 1
	}
	if m.askWiz.ConfigSel < 0 || m.askWiz.ConfigSel >= askConfigFieldTotal {
		m.askWiz.ConfigSel = 0
	}
	m.askWiz.Msg = ""
}

func (m *AppModel) clampAskProfileSelection() {
	if m.askWiz.ProfileSel >= len(m.askWiz.Profiles) {
		m.askWiz.ProfileSel = len(m.askWiz.Profiles) - 1
	}
	if m.askWiz.ProfileSel < 0 {
		m.askWiz.ProfileSel = 0
	}
}

func (m *AppModel) adjustAskProfileSelection(delta int) {
	if len(m.askWiz.Profiles) == 0 {
		m.askWiz.ProfileSel = 0
		return
	}
	sel := m.askWiz.ProfileSel + delta
	for sel < 0 {
		sel += len(m.askWiz.Profiles)
	}
	m.askWiz.ProfileSel = sel % len(m.askWiz.Profiles)
}

func (m *AppModel) adjustAskCount(delta int) {
	next := m.askWiz.Count + delta
	if next < 1 {
		next = 1
	}
	if next > 999 {
		next = 999
	}
	m.askWiz.Count = next
}

func (m *AppModel) askSelectedProfile() *config.Profile {
	if m.globalCfg == nil {
		return nil
	}
	if len(m.askWiz.Profiles) == 0 {
		return nil
	}
	m.clampAskProfileSelection()
	if m.askWiz.ProfileSel < 0 || m.askWiz.ProfileSel >= len(m.askWiz.Profiles) {
		return nil
	}
	return m.globalCfg.FindProfile(m.askWiz.Profiles[m.askWiz.ProfileSel])
}

func (m *AppModel) askConfigNextField() {
	m.askWiz.ConfigSel = (m.askWiz.ConfigSel + 1) % askConfigFieldTotal
}

func (m *AppModel) askConfigPrevField() {
	m.askWiz.ConfigSel = (m.askWiz.ConfigSel - 1 + askConfigFieldTotal) % askConfigFieldTotal
}

func (m AppModel) startAskWizard() (tea.Model, tea.Cmd) {
	m.initAskWizard()
	m.state = stateAskPrompt
	return m, nil
}

func (m AppModel) startAskSession() (tea.Model, tea.Cmd) {
	if m.globalCfg == nil {
		m.askWiz.Msg = "Global configuration is unavailable."
		return m, nil
	}
	if len(m.askWiz.Profiles) == 0 {
		m.askWiz.Msg = "No profiles available. Create one from the selector first."
		return m, nil
	}

	prompt := strings.TrimSpace(m.askWiz.PromptText)
	if prompt == "" {
		m.askWiz.Msg = "Prompt is empty."
		m.state = stateAskPrompt
		return m, nil
	}

	prof := m.askSelectedProfile()
	if prof == nil {
		m.askWiz.Msg = "Selected profile is no longer available."
		return m, nil
	}

	runProfile := *prof
	if override := strings.TrimSpace(m.askWiz.ModelOverride); override != "" {
		runProfile.Model = override
	}

	loopDef := config.LoopDef{
		Name: "ask",
		Steps: []config.LoopStep{
			{
				Profile:      runProfile.Name,
				Role:         config.EffectiveStepRole("", m.globalCfg),
				Turns:        1,
				Instructions: m.askWiz.PromptText,
			},
		},
	}

	m.askWiz.Msg = ""
	count := m.askWiz.Count
	if count < 1 {
		count = 1
	}
	return m.startLoopSession(loopDef, []config.Profile{runProfile}, runProfile.Name, runProfile.Agent, nil, count)
}

func (m AppModel) updateAskPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextarea("ask-prompt", m.askWiz.PromptText)
	m.syncTextarea(m.askWiz.PromptText)

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEsc:
			m.askWiz.Msg = ""
			m.state = stateSelector
			return m, nil
		case tea.KeyCtrlS:
			m.askWiz.PromptText = m.textArea.Value()
			if strings.TrimSpace(m.askWiz.PromptText) == "" {
				m.askWiz.Msg = "Prompt is empty."
				return m, initCmd
			}
			m.askWiz.Msg = ""
			m.askWiz.ConfigSel = 0
			m.state = stateAskConfig
			return m, nil
		}
		switch keyMsg.String() {
		case "ctrl+enter", "ctrl+j":
			m.askWiz.PromptText = m.textArea.Value()
			if strings.TrimSpace(m.askWiz.PromptText) == "" {
				m.askWiz.Msg = "Prompt is empty."
				return m, initCmd
			}
			m.askWiz.Msg = ""
			m.askWiz.ConfigSel = 0
			m.state = stateAskConfig
			return m, nil
		}
	}

	cmd := m.updateTextarea(msg)
	m.askWiz.PromptText = m.textArea.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewAskPrompt() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRed)

	m.ensureTextarea("ask-prompt", m.askWiz.PromptText)
	m.syncTextarea(m.askWiz.PromptText)

	var lines []string
	lines = append(lines, sectionStyle.Render("Ask — Enter Prompt"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Type your standalone prompt (multi-line supported)."))
	lines = append(lines, dimStyle.Render("ctrl+s or ctrl+enter: next  esc: cancel"))
	lines = append(lines, "")

	if msg := strings.TrimSpace(m.askWiz.Msg); msg != "" {
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

func (m AppModel) updateAskConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.clampAskProfileSelection()
	if m.askWiz.Count < 1 {
		m.askWiz.Count = 1
	}
	if m.askWiz.ConfigSel < 0 || m.askWiz.ConfigSel >= askConfigFieldTotal {
		m.askWiz.ConfigSel = 0
	}

	// Model override uses the shared text input editor.
	if m.askWiz.ConfigSel == askConfigFieldModel {
		initCmd := m.ensureTextInput("ask-model-override", m.askWiz.ModelOverride, 0)
		m.syncTextInput(m.askWiz.ModelOverride)
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.askWiz.Msg = ""
				m.state = stateAskPrompt
				return m, nil
			case "tab", "down", "j":
				m.askConfigNextField()
				return m, nil
			case "shift+tab", "up", "k":
				m.askConfigPrevField()
				return m, nil
			case "enter":
				m.askConfigNextField()
				return m, nil
			}
		}
		cmd := m.updateTextInput(msg)
		m.askWiz.ModelOverride = m.textInput.Value()
		return m, tea.Batch(initCmd, cmd)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.askWiz.Msg = ""
			m.state = stateAskPrompt
			return m, nil
		case "tab", "down", "j":
			m.askConfigNextField()
			return m, nil
		case "shift+tab", "up", "k":
			m.askConfigPrevField()
			return m, nil
		}

		switch m.askWiz.ConfigSel {
		case askConfigFieldProfile:
			switch keyMsg.String() {
			case "left", "h":
				m.adjustAskProfileSelection(-1)
			case "right", "l":
				m.adjustAskProfileSelection(1)
			case "enter":
				m.askConfigNextField()
			}
		case askConfigFieldCount:
			switch keyMsg.String() {
			case "left", "h", "-":
				m.adjustAskCount(-1)
			case "right", "l", "+", "=":
				m.adjustAskCount(1)
			case "enter":
				m.askConfigNextField()
			}
		case askConfigFieldChain:
			switch keyMsg.String() {
			case "left", "right", " ", "space", "enter":
				m.askWiz.Chain = !m.askWiz.Chain
			}
		case askConfigFieldRun:
			if keyMsg.String() == "enter" {
				return m.startAskSession()
			}
		}
	}

	return m, nil
}

func (m AppModel) viewAskConfig() string {
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

	lines = append(lines, sectionStyle.Render("Ask — Configure"))
	lines = append(lines, "")

	selectedProfile := m.askSelectedProfile()
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
		if m.askWiz.ConfigSel == field {
			lines = append(lines, selectedStyle.Render("> "+line))
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+valueStyle.Render(line))
		}
	}

	renderField(askConfigFieldProfile, "Profile:", profileText)
	renderField(askConfigFieldCount, "Count:", fmt.Sprintf("%d", m.askWiz.Count))
	chainText := "off"
	if m.askWiz.Chain {
		chainText = "on"
	}
	renderField(askConfigFieldChain, "Chain:", chainText)

	modelValue := strings.TrimSpace(m.askWiz.ModelOverride)
	if modelValue == "" {
		modelValue = "(default profile model)"
	}
	renderField(askConfigFieldModel, "Model:", modelValue)

	runLabel := "[ Run Ask ]"
	if m.askWiz.ConfigSel == askConfigFieldRun {
		lines = append(lines, selectedStyle.Render("> "+runLabel))
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+valueStyle.Render(runLabel))
	}

	if m.askWiz.ConfigSel == askConfigFieldModel {
		m.ensureTextInput("ask-model-override", m.askWiz.ModelOverride, 0)
		m.syncTextInput(m.askWiz.ModelOverride)
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Model override input:"))
		lines = append(lines, m.viewTextInput(cw-4))
	}

	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Preview"))
	preview := strings.TrimSpace(m.askWiz.PromptText)
	if preview == "" {
		lines = append(lines, dimStyle.Render("(empty prompt)"))
	} else {
		lines = append(lines, dimStyle.Render(truncateInputForDisplay(strings.ReplaceAll(preview, "\n", " "), cw-4)))
	}
	if m.askWiz.Count > 1 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("Runs: %d cycles", m.askWiz.Count)))
	}
	if m.askWiz.Chain && m.askWiz.Count > 1 {
		lines = append(lines, dimStyle.Render("Chain: currently reuses the same prompt each cycle."))
	}

	if msg := strings.TrimSpace(m.askWiz.Msg); msg != "" {
		lines = append(lines, "")
		lines = append(lines, errStyle.Render(msg))
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("tab/up/down: field  left/right: adjust  enter: run/select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
