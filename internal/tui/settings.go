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
const settingsMenuItemCount = 3 // Pushover User Key, Pushover App Token, Back

// --- Settings Menu ---

func (m AppModel) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.settingsSel = (m.settingsSel + 1) % settingsMenuItemCount
		case "k", "up":
			m.settingsSel = (m.settingsSel - 1 + settingsMenuItemCount) % settingsMenuItemCount
		case "esc":
			m.state = stateSelector
		case "enter":
			switch m.settingsSel {
			case 0: // Pushover User Key
				m.state = stateSettingsPushoverUserKey
			case 1: // Pushover App Token
				m.state = stateSettingsPushoverAppToken
			case 2: // Back
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

	maskedUserKey := maskValue(m.settingsPushoverUserKey)
	maskedAppToken := maskValue(m.settingsPushoverAppToken)

	items := []menuItem{
		{"Pushover User Key", maskedUserKey},
		{"Pushover App Token", maskedAppToken},
	}

	for i, item := range items {
		line := labelStyle.Render(item.label) + valueStyle.Render(item.value)
		if i == m.settingsSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			lines = append(lines, cursor+line)
		} else {
			lines = append(lines, "  "+line)
		}
	}

	// Back item.
	lines = append(lines, "")
	backLabel := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render("Back")
	if m.settingsSel == 2 {
		cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render("> ")
		lines = append(lines, cursor+backLabel)
	} else {
		lines = append(lines, "  "+backLabel)
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: edit  esc: back"))

	content := fitLines(lines, cw, ch)
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			key := strings.TrimSpace(m.settingsPushoverUserKey)
			m.settingsPushoverUserKey = key
			m.globalCfg.Pushover.UserKey = key
			config.Save(m.globalCfg)
			m.state = stateSettings
		case "esc":
			// Revert to saved value.
			m.settingsPushoverUserKey = m.globalCfg.Pushover.UserKey
			m.state = stateSettings
		case "backspace":
			if len(m.settingsPushoverUserKey) > 0 {
				m.settingsPushoverUserKey = m.settingsPushoverUserKey[:len(m.settingsPushoverUserKey)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.settingsPushoverUserKey += msg.String()
			}
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsPushoverUserKey() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render("Settings — Pushover User Key"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter your Pushover User Key:"))
	lines = append(lines, dimStyle.Render("(found on your Pushover dashboard at pushover.net)"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	displayKey := truncateInputForDisplay(m.settingsPushoverUserKey, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(displayKey)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: save  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Pushover App Token Input ---

func (m AppModel) updateSettingsPushoverAppToken(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			token := strings.TrimSpace(m.settingsPushoverAppToken)
			m.settingsPushoverAppToken = token
			m.globalCfg.Pushover.AppToken = token
			config.Save(m.globalCfg)
			m.state = stateSettings
		case "esc":
			// Revert to saved value.
			m.settingsPushoverAppToken = m.globalCfg.Pushover.AppToken
			m.state = stateSettings
		case "backspace":
			if len(m.settingsPushoverAppToken) > 0 {
				m.settingsPushoverAppToken = m.settingsPushoverAppToken[:len(m.settingsPushoverAppToken)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.settingsPushoverAppToken += msg.String()
			}
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsPushoverAppToken() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render("Settings — Pushover App Token"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter your Pushover Application Token:"))
	lines = append(lines, dimStyle.Render("(create an app at pushover.net/apps/build)"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	displayToken := truncateInputForDisplay(m.settingsPushoverAppToken, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(displayToken)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: save  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
