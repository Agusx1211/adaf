package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/buildinfo"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/pushover"
)

// settingsMenuItemCount is the number of items in the settings menu.
const settingsMenuItemCount = 4 // Pushover User Key, Pushover App Token, Roles & Rules, Back

// --- Settings Menu ---

func (m AppModel) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.settings.Sel = (m.settings.Sel + 1) % settingsMenuItemCount
		case "k", "up":
			m.settings.Sel = (m.settings.Sel - 1 + settingsMenuItemCount) % settingsMenuItemCount
		case "esc":
			m.state = stateSelector
		case "enter":
			switch m.settings.Sel {
			case 0: // Pushover User Key
				m.state = stateSettingsPushoverUserKey
			case 1: // Pushover App Token
				m.state = stateSettingsPushoverAppToken
			case 2: // Roles & Rules
				m.settings.RolesRulesSel = 0
				m.settings.RolesSel = 0
				m.settings.RoleRuleSel = 0
				m.settings.RulesSel = 0
				m.state = stateSettingsRolesRulesMenu
			case 3: // Back
				m.state = stateSelector
			}
		}
	}
	return m, nil
}

func (m AppModel) viewSettings() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render("Settings"))
	lines = append(lines, "")

	info := buildinfo.Current()
	lines = append(lines, sectionStyle.Render("Build Info"))
	lines = append(lines, labelStyle.Render("Build Date")+valueStyle.Render(info.BuildDate))
	lines = append(lines, labelStyle.Render("Commit Hash")+valueStyle.Render(info.CommitHash))
	lines = append(lines, "")

	// Pushover status.
	configured := pushover.Configured(&m.globalCfg.Pushover)
	if configured {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorGreen).Render("Pushover: configured"))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorYellow).Render("Pushover: not configured"))
	}
	lines = append(lines, "")

	type menuItem struct {
		label, value string
	}

	maskedUserKey := maskValue(m.settings.PushoverUserKey)
	maskedAppToken := maskValue(m.settings.PushoverAppToken)

	items := []menuItem{
		{"Pushover User Key", maskedUserKey},
		{"Pushover App Token", maskedAppToken},
		{"Roles & Rules", "manage role definitions and prompt rules"},
	}

	for i, item := range items {
		line := labelStyle.Render(item.label) + valueStyle.Render(item.value)
		if i == m.settings.Sel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			lines = append(lines, cursor+line)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+line)
		}
	}

	// Back item.
	lines = append(lines, "")
	backLabel := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render("Back")
	if m.settings.Sel == 3 {
		cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render("> ")
		lines = append(lines, cursor+backLabel)
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+backLabel)
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: edit  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// maskValue shows the first 4 characters and masks the rest, or "(not set)" if empty.
func maskValue(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 4 {
		return s
	}
	return s[:4] + strings.Repeat("*", len(s)-4)
}

// --- Pushover User Key Input ---

func (m AppModel) updateSettingsPushoverUserKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("settings-pushover-user", m.settings.PushoverUserKey, 0)
	m.syncTextInput(m.settings.PushoverUserKey)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			key := strings.TrimSpace(m.settings.PushoverUserKey)
			m.settings.PushoverUserKey = key
			m.globalCfg.Pushover.UserKey = key
			config.Save(m.globalCfg)
			m.state = stateSettings
			return m, nil
		case tea.KeyEsc:
			// Revert to saved value.
			m.settings.PushoverUserKey = m.globalCfg.Pushover.UserKey
			m.state = stateSettings
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.settings.PushoverUserKey = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewSettingsPushoverUserKey() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("settings-pushover-user", m.settings.PushoverUserKey, 0)
	m.syncTextInput(m.settings.PushoverUserKey)

	var lines []string
	lines = append(lines, sectionStyle.Render("Settings — Pushover User Key"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter your Pushover User Key:"))
	lines = append(lines, dimStyle.Render("(found on your Pushover dashboard at pushover.net)"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: save  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Pushover App Token Input ---

func (m AppModel) updateSettingsPushoverAppToken(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("settings-pushover-token", m.settings.PushoverAppToken, 0)
	m.syncTextInput(m.settings.PushoverAppToken)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			token := strings.TrimSpace(m.settings.PushoverAppToken)
			m.settings.PushoverAppToken = token
			m.globalCfg.Pushover.AppToken = token
			config.Save(m.globalCfg)
			m.state = stateSettings
			return m, nil
		case tea.KeyEsc:
			// Revert to saved value.
			m.settings.PushoverAppToken = m.globalCfg.Pushover.AppToken
			m.state = stateSettings
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.settings.PushoverAppToken = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewSettingsPushoverAppToken() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("settings-pushover-token", m.settings.PushoverAppToken, 0)
	m.syncTextInput(m.settings.PushoverAppToken)

	var lines []string
	lines = append(lines, sectionStyle.Render("Settings — Pushover App Token"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter your Pushover Application Token:"))
	lines = append(lines, dimStyle.Render("(create an app at pushover.net/apps/build)"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: save  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
