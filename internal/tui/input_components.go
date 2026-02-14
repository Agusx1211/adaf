package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newStyledTextInput() textinput.Model {
	input := textinput.New()
	input.Prompt = "> "
	input.PromptStyle = lipgloss.NewStyle().Foreground(ColorMauve)
	input.TextStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorOverlay0)
	input.Cursor.Style = lipgloss.NewStyle().Foreground(ColorMauve)
	return input
}

func newStyledTextarea() textarea.Model {
	editor := textarea.New()
	editor.Prompt = ""
	editor.ShowLineNumbers = false
	return editor
}

func (m *AppModel) resetInputEditors() {
	m.textInputKey = ""
	m.textAreaKey = ""
}

func (m *AppModel) ensureTextInput(key, value string, charLimit int) tea.Cmd {
	if m.textInputKey == key {
		return nil
	}
	input := newStyledTextInput()
	if charLimit > 0 {
		input.CharLimit = charLimit
	}
	input.SetValue(value)
	input.CursorEnd()
	m.textInput = input
	m.textInputKey = key
	return m.textInput.Focus()
}

func (m *AppModel) syncTextInput(value string) {
	if m.textInput.Value() == value {
		return
	}
	m.textInput.SetValue(value)
	m.textInput.CursorEnd()
}

func (m *AppModel) updateTextInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return cmd
}

func (m AppModel) viewTextInput(width int) string {
	input := m.textInput
	if width < 8 {
		width = 8
	}
	input.Width = width
	return input.View()
}

func (m *AppModel) ensureTextarea(key, value string) tea.Cmd {
	if m.textAreaKey == key {
		return nil
	}
	editor := newStyledTextarea()
	editor.SetValue(value)
	m.textArea = editor
	m.textAreaKey = key
	return m.textArea.Focus()
}

func (m *AppModel) syncTextarea(value string) {
	if m.textArea.Value() == value {
		return
	}
	m.textArea.SetValue(value)
}

func (m *AppModel) updateTextarea(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.textArea, cmd = m.textArea.Update(msg)
	return cmd
}

func (m AppModel) viewTextarea(width, height int) string {
	editor := m.textArea
	if width < 8 {
		width = 8
	}
	if height < 3 {
		height = 3
	}
	editor.SetWidth(width)
	editor.SetHeight(height)
	return editor.View()
}

func digitsOnly(input string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range input {
		if r < '0' || r > '9' {
			continue
		}
		if b.Len() >= maxLen {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sanitizeDigitsMsg(msg tea.Msg) (tea.Msg, bool) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok || keyMsg.Type != tea.KeyRunes {
		return msg, true
	}
	digits := make([]rune, 0, len(keyMsg.Runes))
	for _, r := range keyMsg.Runes {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return nil, false
	}
	keyMsg.Runes = digits
	return keyMsg, true
}

func (m *AppModel) initInputEditorForState() tea.Cmd {
	switch m.state {
	case statePlanCreateID:
		return m.ensureTextInput("plan-create-id", m.planWiz.CreateIDInput, 64)
	case statePlanCreateTitle:
		return m.ensureTextInput("plan-create-title", m.planWiz.CreateTitleInput, 0)
	case stateProfileName:
		return m.ensureTextInput("profile-name", m.profileWiz.NameInput, 0)
	case stateProfileModel:
		if m.profileWiz.CustomModelMode {
			return m.ensureTextInput("profile-custom-model", m.profileWiz.CustomModel, 0)
		}
	case stateProfileIntel:
		return m.ensureTextInput("profile-intel", m.profileWiz.IntelInput, 2)
	case stateProfileDesc:
		return m.ensureTextInput("profile-desc", m.profileWiz.DescInput, 0)
	case stateProfileMaxInst:
		return m.ensureTextInput("profile-max-inst", m.profileWiz.MaxInstInput, 2)
	case stateLoopName:
		return m.ensureTextInput("loop-name", m.loopWiz.NameInput, 0)
	case stateLoopStepTurns:
		return m.ensureTextInput("loop-step-turns", m.loopWiz.StepTurnsInput, 3)
	case stateLoopStepInstr:
		return m.ensureTextInput("loop-step-instr", m.loopWiz.StepInstrInput, 0)
	case stateSettingsPushoverUserKey:
		return m.ensureTextInput("settings-pushover-user", m.settings.PushoverUserKey, 0)
	case stateSettingsPushoverAppToken:
		return m.ensureTextInput("settings-pushover-token", m.settings.PushoverAppToken, 0)
	case stateSettingsRoleName:
		return m.ensureTextInput("settings-role-name", m.settings.RoleNameInput, 0)
	case stateSettingsRuleID:
		return m.ensureTextInput("settings-rule-id", m.settings.RuleIDInput, 0)
	case stateSettingsRuleBody:
		key, _, ok := m.ruleBodyEditorContext()
		if ok {
			return m.ensureTextarea(key, m.settings.RuleBodyInput)
		}
	case stateAskPrompt:
		return m.ensureTextarea("ask-prompt", m.askWiz.PromptText)
	case stateAskConfig:
		if m.askWiz.ConfigSel == askConfigFieldModel {
			return m.ensureTextInput("ask-model-override", m.askWiz.ModelOverride, 0)
		}
	case statePMPrompt:
		return m.ensureTextarea("pm-message", m.pmWiz.MessageText)
	case statePMConfig:
		if m.pmWiz.ConfigSel == pmConfigFieldModel {
			return m.ensureTextInput("pm-model-override", m.pmWiz.ModelOverride, 0)
		}
	}
	return nil
}
