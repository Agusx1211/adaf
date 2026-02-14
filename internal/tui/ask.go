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

// AskWizardState holds the state for the Ask wizard, embedding the common WizardState.
type AskWizardState struct {
	WizardState
	PromptText string
	Count      int
	Chain      bool
}

func (m *AppModel) initAskWizard() {
	initWizardProfiles(m, &m.askWiz.WizardState, askConfigFieldTotal)
	if m.askWiz.Count < 1 {
		m.askWiz.Count = 1
	}
}

// adjustAskProfileSelection delegates to the shared adjustProfileSelection.
func (m *AppModel) adjustAskProfileSelection(delta int) {
	adjustProfileSelection(&m.askWiz.WizardState, delta)
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

// askSelectedProfile delegates to the shared selectedProfile.
func (m *AppModel) askSelectedProfile() *config.Profile {
	return selectedProfile(m, &m.askWiz.WizardState)
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

// viewAskPrompt delegates to the shared viewWizardPrompt.
func (m AppModel) viewAskPrompt() string {
	return viewWizardPrompt(m, &m.askWiz.WizardState, "Ask — Enter Prompt", "ask-prompt", m.askWiz.PromptText)
}

func (m AppModel) updateAskConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	clampProfileSelection(&m.askWiz.WizardState)
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
				wizardNextField(&m.askWiz.WizardState, askConfigFieldTotal)
				return m, nil
			case "shift+tab", "up", "k":
				wizardPrevField(&m.askWiz.WizardState, askConfigFieldTotal)
				return m, nil
			case "enter":
				wizardNextField(&m.askWiz.WizardState, askConfigFieldTotal)
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
			wizardNextField(&m.askWiz.WizardState, askConfigFieldTotal)
			return m, nil
		case "shift+tab", "up", "k":
			wizardPrevField(&m.askWiz.WizardState, askConfigFieldTotal)
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
				wizardNextField(&m.askWiz.WizardState, askConfigFieldTotal)
			}
		case askConfigFieldCount:
			switch keyMsg.String() {
			case "left", "h", "-":
				m.adjustAskCount(-1)
			case "right", "l", "+", "=":
				m.adjustAskCount(1)
			case "enter":
				wizardNextField(&m.askWiz.WizardState, askConfigFieldTotal)
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
	_, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)

	var lines []string
	cursorLine := -1

	// Use shared function for common fields
	lines = viewWizardConfigCommon(&m, &m.askWiz.WizardState, "Ask — Configure", askConfigFieldProfile, askConfigFieldModel, func(field int, label, value string) {
		line := fmt.Sprintf("%-8s %s", label, value)
		if m.askWiz.ConfigSel == field {
			lines = append(lines, selectedStyle.Render("> "+line))
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+valueStyle.Render(line))
		}
	}, &cursorLine)

	// Ask-specific fields
	renderField := func(field int, label, value string) {
		line := fmt.Sprintf("%-8s %s", label, value)
		if m.askWiz.ConfigSel == field {
			lines = append(lines, selectedStyle.Render("> "+line))
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+valueStyle.Render(line))
		}
	}

	renderField(askConfigFieldCount, "Count:", fmt.Sprintf("%d", m.askWiz.Count))
	chainText := "off"
	if m.askWiz.Chain {
		chainText = "on"
	}
	renderField(askConfigFieldChain, "Chain:", chainText)

	// Model field (already handled by shared function, but we need to handle the input editor)
	if m.askWiz.ConfigSel == askConfigFieldModel {
		m.ensureTextInput("ask-model-override", m.askWiz.ModelOverride, 0)
		m.syncTextInput(m.askWiz.ModelOverride)
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Model override input:"))
		lines = append(lines, m.viewTextInput(cw-4))
	}

	runLabel := "[ Run Ask ]"
	if m.askWiz.ConfigSel == askConfigFieldRun {
		lines = append(lines, selectedStyle.Render("> "+runLabel))
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+valueStyle.Render(runLabel))
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

	// Use shared function for footer
	return viewWizardConfigFooter(&m, &m.askWiz.WizardState, lines, cursorLine, cw, ch)
}
