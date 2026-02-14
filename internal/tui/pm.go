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

// PMWizardState holds the state for the PM wizard, embedding the common WizardState.
type PMWizardState struct {
	WizardState
	MessageText string
}

func (m *AppModel) initPMWizard() {
	initWizardProfiles(m, &m.pmWiz.WizardState, pmConfigFieldTotal)
}

// adjustPMProfileSelection delegates to the shared adjustProfileSelection.
func (m *AppModel) adjustPMProfileSelection(delta int) {
	adjustProfileSelection(&m.pmWiz.WizardState, delta)
}

// pmSelectedProfile delegates to the shared selectedProfile.
func (m *AppModel) pmSelectedProfile() *config.Profile {
	return selectedProfile(m, &m.pmWiz.WizardState)
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

// viewPMPrompt delegates to the shared viewWizardPrompt.
func (m AppModel) viewPMPrompt() string {
	return viewWizardPrompt(m, &m.pmWiz.WizardState, "Project Manager — Enter Message", "pm-message", m.pmWiz.MessageText)
}

func (m AppModel) updatePMConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	clampProfileSelection(&m.pmWiz.WizardState)
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
				wizardNextField(&m.pmWiz.WizardState, pmConfigFieldTotal)
				return m, nil
			case "shift+tab", "up", "k":
				wizardPrevField(&m.pmWiz.WizardState, pmConfigFieldTotal)
				return m, nil
			case "enter":
				wizardNextField(&m.pmWiz.WizardState, pmConfigFieldTotal)
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
			wizardNextField(&m.pmWiz.WizardState, pmConfigFieldTotal)
			return m, nil
		case "shift+tab", "up", "k":
			wizardPrevField(&m.pmWiz.WizardState, pmConfigFieldTotal)
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
				wizardNextField(&m.pmWiz.WizardState, pmConfigFieldTotal)
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
	_, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)

	var lines []string
	cursorLine := -1

	// Use shared function for common fields
	lines = viewWizardConfigCommon(&m, &m.pmWiz.WizardState, "Project Manager — Configure", pmConfigFieldProfile, pmConfigFieldModel, func(field int, label, value string) {
		line := fmt.Sprintf("%-8s %s", label, value)
		if m.pmWiz.ConfigSel == field {
			lines = append(lines, selectedStyle.Render("> "+line))
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+valueStyle.Render(line))
		}
	}, &cursorLine)

	// Model field (already handled by shared function, but we need to handle the input editor)
	if m.pmWiz.ConfigSel == pmConfigFieldModel {
		m.ensureTextInput("pm-model-override", m.pmWiz.ModelOverride, 0)
		m.syncTextInput(m.pmWiz.ModelOverride)
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Model override input:"))
		lines = append(lines, m.viewTextInput(cw-4))
	}

	runLabel := "[ Run PM ]"
	if m.pmWiz.ConfigSel == pmConfigFieldRun {
		lines = append(lines, selectedStyle.Render("> "+runLabel))
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+valueStyle.Render(runLabel))
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

	// Use shared function for footer
	return viewWizardConfigFooter(&m, &m.pmWiz.WizardState, lines, cursorLine, cw, ch)
}
