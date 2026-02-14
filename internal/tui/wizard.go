package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
)

// WizardState holds the common state for both Ask and PM wizards.
type WizardState struct {
	ProfileSel    int
	Profiles      []string
	ModelOverride string
	ConfigSel     int
	Msg           string
}

// clampProfileSelection ensures the profile selection is within valid bounds.
func clampProfileSelection(ws *WizardState) {
	if ws.ProfileSel >= len(ws.Profiles) {
		ws.ProfileSel = len(ws.Profiles) - 1
	}
	if ws.ProfileSel < 0 {
		ws.ProfileSel = 0
	}
}

// adjustProfileSelection moves the profile selection by delta positions.
func adjustProfileSelection(ws *WizardState, delta int) {
	if len(ws.Profiles) == 0 {
		ws.ProfileSel = 0
		return
	}
	sel := ws.ProfileSel + delta
	for sel < 0 {
		sel += len(ws.Profiles)
	}
	ws.ProfileSel = sel % len(ws.Profiles)
}

// selectedProfile returns the currently selected profile from the global config.
func selectedProfile(m *AppModel, ws *WizardState) *config.Profile {
	if m.globalCfg == nil {
		return nil
	}
	if len(ws.Profiles) == 0 {
		return nil
	}
	clampProfileSelection(ws)
	if ws.ProfileSel < 0 || ws.ProfileSel >= len(ws.Profiles) {
		return nil
	}
	return m.globalCfg.FindProfile(ws.Profiles[ws.ProfileSel])
}

// wizardNextField moves to the next configuration field.
func wizardNextField(ws *WizardState, total int) {
	ws.ConfigSel = (ws.ConfigSel + 1) % total
}

// wizardPrevField moves to the previous configuration field.
func wizardPrevField(ws *WizardState, total int) {
	ws.ConfigSel = (ws.ConfigSel - 1 + total) % total
}

// initWizardProfiles initializes the wizard profiles from the global configuration.
func initWizardProfiles(m *AppModel, ws *WizardState, totalFields int) {
	if m.globalCfg == nil {
		m.globalCfg = &config.GlobalConfig{}
	}

	prevName := ""
	if ws.ProfileSel >= 0 && ws.ProfileSel < len(ws.Profiles) {
		prevName = ws.Profiles[ws.ProfileSel]
	}

	profiles := make([]string, 0, len(m.globalCfg.Profiles))
	for _, prof := range m.globalCfg.Profiles {
		profiles = append(profiles, prof.Name)
	}
	ws.Profiles = profiles
	ws.ProfileSel = 0
	if prevName != "" {
		for i, name := range profiles {
			if strings.EqualFold(name, prevName) {
				ws.ProfileSel = i
				break
			}
		}
	}
	clampProfileSelection(ws)
	if ws.ConfigSel < 0 || ws.ConfigSel >= totalFields {
		ws.ConfigSel = 0
	}
	ws.Msg = ""
}

// viewWizardPrompt renders the prompt/message input view for both Ask and PM wizards.
func viewWizardPrompt(m AppModel, ws *WizardState, title, textareaID, text string) string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRed)

	m.ensureTextarea(textareaID, text)
	m.syncTextarea(text)

	var lines []string
	lines = append(lines, sectionStyle.Render(title))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Type your standalone prompt (multi-line supported)."))
	lines = append(lines, dimStyle.Render("ctrl+s or ctrl+enter: next  esc: cancel"))
	lines = append(lines, "")

	if msg := strings.TrimSpace(ws.Msg); msg != "" {
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

// viewWizardConfigCommon renders the common configuration sections for both Ask and PM wizards.
func viewWizardConfigCommon(m *AppModel, ws *WizardState, title string, renderField func(field int, label, value string), cursorLine *int) []string {
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)

	var lines []string
	lines = append(lines, sectionStyle.Render(title))
	lines = append(lines, "")

	selectedProfile := selectedProfile(m, ws)
	profileText := "(no profile)"
	if selectedProfile != nil {
		modelText := selectedProfile.Model
		if strings.TrimSpace(modelText) == "" {
			modelText = "(default)"
		}
		profileText = fmt.Sprintf("%s (%s, %s)", selectedProfile.Name, selectedProfile.Agent, modelText)
	}

	renderField(0, "Profile:", profileText)

	modelValue := strings.TrimSpace(ws.ModelOverride)
	if modelValue == "" {
		modelValue = "(default profile model)"
	}
	renderField(1, "Model:", modelValue)

	return lines
}

// viewWizardConfigFooter renders the common footer for configuration views.
func viewWizardConfigFooter(m *AppModel, ws *WizardState, lines []string, cursorLine int, cw, ch int) string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, _, _ := profileWizardPanel(*m)

	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	errStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRed)

	if msg := strings.TrimSpace(ws.Msg); msg != "" {
		lines = append(lines, "")
		lines = append(lines, errStyle.Render(msg))
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("tab/up/down: field  left/right: adjust  enter: run/select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
