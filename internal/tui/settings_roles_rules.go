package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/prompt"
)

type SettingsState struct {
	Sel                 int
	PushoverUserKey     string
	PushoverAppToken    string
	RolesRulesSel       int
	RolesSel            int
	RoleRuleSel         int
	RulesSel            int
	EditRoleIdx         int
	EditRuleIdx         int
	RuleBodyReturnState appState
	RoleNameInput       string
	RuleIDInput         string
	RuleBodyInput       string
}

const settingsRolesRulesMenuItemCount = 3 // Roles, Prompt Rules, Back

func normalizeRoleNameInput(in string) string {
	return strings.ToLower(strings.TrimSpace(in))
}

func normalizeRuleIDInput(in string) string {
	return strings.ToLower(strings.TrimSpace(in))
}

func (m AppModel) ruleBodyEditorContext() (editorKey string, editingRoleIdentity bool, ok bool) {
	editingRoleIdentity = m.settings.EditRuleIdx < 0 &&
		m.settings.EditRoleIdx >= 0 &&
		m.settings.EditRoleIdx < len(m.globalCfg.Roles)
	if editingRoleIdentity {
		return fmt.Sprintf("settings-role-identity-%d", m.settings.EditRoleIdx), true, true
	}
	if m.settings.EditRuleIdx >= 0 && m.settings.EditRuleIdx < len(m.globalCfg.PromptRules) {
		return fmt.Sprintf("settings-rule-body-%d", m.settings.EditRuleIdx), false, true
	}
	return "", false, false
}

func ensureSettingsRoleCatalog(cfg *config.GlobalConfig) {
	config.EnsureDefaultRoleCatalog(cfg)
}

func saveSettingsRoleCatalog(cfg *config.GlobalConfig) {
	config.EnsureDefaultRoleCatalog(cfg)
	_ = config.Save(cfg)
}

func settingsPanelStyle(focused bool) lipgloss.Style {
	borderColor := ColorSurface2
	if focused {
		borderColor = ColorMauve
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2)
}

func renderSettingsPane(lines []string, outerW, outerH int, focused bool) string {
	style := settingsPanelStyle(focused)
	hf, vf := style.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	content := fitLines(wrapRenderableLines(lines, cw), cw, ch)
	return style.Render(content)
}

func renderSettingsPaneWithOffset(lines []string, outerW, outerH, offset int, focused bool) string {
	style := settingsPanelStyle(focused)
	hf, vf := style.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	content := fitLinesWithOffset(wrapRenderableLines(lines, cw), cw, ch, offset)
	return style.Render(content)
}

func renderSettingsPaneWithCursor(lines []string, outerW, outerH, cursorLine int, focused bool) string {
	style := settingsPanelStyle(focused)
	hf, vf := style.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	content := fitLinesWithCursor(wrapRenderableLines(lines, cw), cw, ch, cursorLine)
	return style.Render(content)
}

func appendMultiline(lines []string, text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return append(lines, "")
	}
	return append(lines, splitRenderableLines(trimmed)...)
}

func rolePromptPreview(roleName string, cfg *config.GlobalConfig) string {
	if strings.TrimSpace(roleName) == "" {
		return ""
	}
	return strings.TrimSpace(prompt.RolePrompt(&config.Profile{}, roleName, cfg))
}

func (m AppModel) renderSettingsSplitView(leftLines, rightLines []string, leftCursorLine int) string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()

	panelH := m.height - 2
	if panelH < 1 {
		panelH = 1
	}

	if m.width < 80 {
		combined := make([]string, 0, len(leftLines)+len(rightLines)+3)
		combined = append(combined, leftLines...)
		combined = append(combined, "")
		combined = append(combined, lipgloss.NewStyle().Bold(true).Foreground(ColorLavender).Render("Details"))
		combined = append(combined, rightLines...)
		scroll := m.stateScrollOffset()
		panel := ""
		if scroll > 0 {
			panel = renderSettingsPaneWithOffset(combined, m.width, panelH, scroll, true)
		} else {
			panel = renderSettingsPaneWithCursor(combined, m.width, panelH, leftCursorLine, true)
		}
		return header + "\n" + panel + "\n" + statusBar
	}

	leftOuter := m.width / 2
	minPane := 32
	if leftOuter < minPane {
		leftOuter = minPane
	}
	if m.width-leftOuter < minPane {
		leftOuter = m.width - minPane
	}
	if leftOuter < 1 {
		leftOuter = 1
	}
	if leftOuter >= m.width {
		leftOuter = m.width - 1
	}
	rightOuter := m.width - leftOuter
	if rightOuter < 1 {
		rightOuter = 1
	}

	leftFocused := !m.isRightPaneFocused()
	rightFocused := m.isRightPaneFocused()
	left := renderSettingsPaneWithCursor(leftLines, leftOuter, panelH, leftCursorLine, leftFocused)
	right := renderSettingsPaneWithOffset(rightLines, rightOuter, panelH, m.stateScrollOffset(), rightFocused)
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + statusBar
}

func (m *AppModel) clampSettingsRolesSel() {
	if len(m.globalCfg.Roles) == 0 {
		m.settings.RolesSel = 0
		return
	}
	if m.settings.RolesSel < 0 {
		m.settings.RolesSel = 0
	}
	if m.settings.RolesSel >= len(m.globalCfg.Roles) {
		m.settings.RolesSel = len(m.globalCfg.Roles) - 1
	}
}

func (m *AppModel) clampSettingsRulesSel() {
	if len(m.globalCfg.PromptRules) == 0 {
		m.settings.RulesSel = 0
		return
	}
	if m.settings.RulesSel < 0 {
		m.settings.RulesSel = 0
	}
	if m.settings.RulesSel >= len(m.globalCfg.PromptRules) {
		m.settings.RulesSel = len(m.globalCfg.PromptRules) - 1
	}
}

func (m *AppModel) clampSettingsRoleRuleSel() {
	if len(m.globalCfg.PromptRules) == 0 {
		m.settings.RoleRuleSel = 0
		return
	}
	if m.settings.RoleRuleSel < 0 {
		m.settings.RoleRuleSel = 0
	}
	if m.settings.RoleRuleSel >= len(m.globalCfg.PromptRules) {
		m.settings.RoleRuleSel = len(m.globalCfg.PromptRules) - 1
	}
}

func roleHasRule(role *config.RoleDefinition, ruleID string) bool {
	if role == nil {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(ruleID))
	for _, rid := range role.RuleIDs {
		if strings.ToLower(strings.TrimSpace(rid)) == key {
			return true
		}
	}
	return false
}

func toggleRoleRule(role *config.RoleDefinition, ruleID string) {
	if role == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(ruleID))
	if key == "" {
		return
	}
	for i, rid := range role.RuleIDs {
		if strings.ToLower(strings.TrimSpace(rid)) == key {
			role.RuleIDs = append(role.RuleIDs[:i], role.RuleIDs[i+1:]...)
			return
		}
	}
	role.RuleIDs = append(role.RuleIDs, key)
}

func rewriteDelegationRoleRefs(deleg *config.DelegationConfig, oldRole, newRole string) {
	if deleg == nil {
		return
	}
	for i := range deleg.Profiles {
		if strings.EqualFold(strings.TrimSpace(deleg.Profiles[i].Role), oldRole) {
			deleg.Profiles[i].Role = newRole
		}
		if len(deleg.Profiles[i].Roles) > 0 {
			for r := range deleg.Profiles[i].Roles {
				if strings.EqualFold(strings.TrimSpace(deleg.Profiles[i].Roles[r]), oldRole) {
					deleg.Profiles[i].Roles[r] = newRole
				}
			}
		}
		rewriteDelegationRoleRefs(deleg.Profiles[i].Delegation, oldRole, newRole)
	}
}

func (m *AppModel) rewriteRoleReferences(oldRole, newRole string) {
	for li := range m.globalCfg.Loops {
		for si := range m.globalCfg.Loops[li].Steps {
			if strings.EqualFold(strings.TrimSpace(m.globalCfg.Loops[li].Steps[si].Role), oldRole) {
				m.globalCfg.Loops[li].Steps[si].Role = newRole
			}
			rewriteDelegationRoleRefs(m.globalCfg.Loops[li].Steps[si].Delegation, oldRole, newRole)
		}
	}
}

func (m *AppModel) rewriteRuleReferences(oldID, newID string) {
	oldKey := strings.ToLower(strings.TrimSpace(oldID))
	newKey := strings.ToLower(strings.TrimSpace(newID))
	for i := range m.globalCfg.Roles {
		for r := range m.globalCfg.Roles[i].RuleIDs {
			if strings.ToLower(strings.TrimSpace(m.globalCfg.Roles[i].RuleIDs[r])) == oldKey {
				m.globalCfg.Roles[i].RuleIDs[r] = newKey
			}
		}
	}
}

// --- Roles & Rules menu ---

func (m AppModel) updateSettingsRolesRulesMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.settings.RolesRulesSel = (m.settings.RolesRulesSel + 1) % settingsRolesRulesMenuItemCount
		case "k", "up":
			m.settings.RolesRulesSel = (m.settings.RolesRulesSel - 1 + settingsRolesRulesMenuItemCount) % settingsRolesRulesMenuItemCount
		case "enter":
			switch m.settings.RolesRulesSel {
			case 0:
				m.settings.RolesSel = 0
				m.state = stateSettingsRolesList
			case 1:
				m.settings.RulesSel = 0
				m.state = stateSettingsRulesList
			default:
				m.state = stateSettings
			}
			return m, nil
		case "esc":
			m.state = stateSettings
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRolesRulesMenu() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	options := []string{
		"Roles",
		"Prompt Rules",
		"Back",
	}

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render("Settings — Roles & Rules"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Manage role definitions and reusable prompt rule sections."))
	lines = append(lines, "")
	for i, opt := range options {
		if i == m.settings.RolesRulesSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(opt)
			lines = append(lines, cursor+styled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(opt))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Roles list ---

func (m AppModel) updateSettingsRolesList(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	m.clampSettingsRolesSel()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "l", "right":
			m.setRightPaneFocused(true)
			return m, nil
		case "shift+tab", "h", "left":
			m.setRightPaneFocused(false)
			return m, nil
		case "j", "down":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(1)
				return m, nil
			}
			if len(m.globalCfg.Roles) > 0 {
				m.settings.RolesSel = (m.settings.RolesSel + 1) % len(m.globalCfg.Roles)
				m.resetStateScroll()
			}
		case "k", "up":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(-1)
				return m, nil
			}
			if len(m.globalCfg.Roles) > 0 {
				m.settings.RolesSel = (m.settings.RolesSel - 1 + len(m.globalCfg.Roles)) % len(m.globalCfg.Roles)
				m.resetStateScroll()
			}
		case "a":
			m.settings.EditRoleIdx = -1
			m.settings.RoleNameInput = ""
			m.state = stateSettingsRoleName
			return m, nil
		case "r":
			if len(m.globalCfg.Roles) == 0 {
				return m, nil
			}
			m.settings.EditRoleIdx = m.settings.RolesSel
			m.settings.RoleNameInput = m.globalCfg.Roles[m.settings.RolesSel].Name
			m.state = stateSettingsRoleName
			return m, nil
		case "d":
			if len(m.globalCfg.Roles) <= 1 {
				return m, nil
			}
			toDelete := m.globalCfg.Roles[m.settings.RolesSel].Name
			m.globalCfg.RemoveRoleDefinition(toDelete)
			m.clampSettingsRolesSel()
			m.resetStateScroll()
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "enter":
			if len(m.globalCfg.Roles) == 0 {
				return m, nil
			}
			m.settings.EditRoleIdx = m.settings.RolesSel
			m.settings.RoleRuleSel = 0
			m.state = stateSettingsRoleEdit
			return m, nil
		case "esc":
			m.state = stateSettingsRolesRulesMenu
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRolesList() string {
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	var left []string
	cursorLine := -1
	left = append(left, sectionStyle.Render("Settings — Roles"))
	left = append(left, "")
	left = append(left, dimStyle.Render("Browse roles on the left. Full composed prompt is on the right."))
	left = append(left, "")

	if len(m.globalCfg.Roles) == 0 {
		left = append(left, dimStyle.Render("No roles configured. Press 'a' to add one."))
	} else {
		for i, role := range m.globalCfg.Roles {
			writeMode := "read-only"
			if role.CanWriteCode {
				writeMode = "can-write"
			}
			defaultTag := ""
			if strings.EqualFold(role.Name, m.globalCfg.DefaultRole) {
				defaultTag = " [default]"
			}
			label := fmt.Sprintf("%s%s  (%s, rules:%d)", role.Name, defaultTag, writeMode, len(role.RuleIDs))
			if i == m.settings.RolesSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
				left = append(left, cursor+styled)
				cursorLine = len(left) - 1
			} else {
				left = append(left, "  "+textStyle.Render(label))
			}
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("a: add  r: rename  d: delete"))
	left = append(left, dimStyle.Render("enter: edit role  esc: back  tab/h/l: pane focus"))

	var right []string
	rightTitle := "Role Details"
	if m.isRightPaneFocused() {
		rightTitle += " [focus]"
	}
	right = append(right, sectionStyle.Render(rightTitle))
	right = append(right, "")
	if len(m.globalCfg.Roles) == 0 {
		right = append(right, dimStyle.Render("No role selected."))
	} else {
		m.clampSettingsRolesSel()
		role := m.globalCfg.Roles[m.settings.RolesSel]
		mode := "read-only"
		if role.CanWriteCode {
			mode = "can-write"
		}
		right = append(right, textStyle.Render("Name: "+role.Name))
		if strings.TrimSpace(role.Title) != "" {
			right = append(right, textStyle.Render("Title: "+role.Title))
		}
		right = append(right, textStyle.Render("Mode: "+mode))
		right = append(right, textStyle.Render(fmt.Sprintf("Rules: %d", len(role.RuleIDs))))
		right = append(right, "")
		right = append(right, sectionStyle.Render("Identity"))
		if strings.TrimSpace(role.Identity) == "" {
			right = append(right, dimStyle.Render("(none)"))
		} else {
			right = appendMultiline(right, role.Identity)
		}
		right = append(right, "")
		right = append(right, sectionStyle.Render("Description"))
		if strings.TrimSpace(role.Description) == "" {
			right = append(right, dimStyle.Render("(none)"))
		} else {
			right = appendMultiline(right, role.Description)
		}
		right = append(right, "")
		right = append(right, sectionStyle.Render("Composed Prompt"))
		right = append(right, "")
		promptPreview := rolePromptPreview(role.Name, m.globalCfg)
		if promptPreview == "" {
			right = append(right, dimStyle.Render("(empty prompt)"))
		} else {
			right = appendMultiline(right, promptPreview)
		}
	}

	return m.renderSettingsSplitView(left, right, cursorLine)
}

// --- Role name input ---

func (m AppModel) updateSettingsRoleName(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	initCmd := m.ensureTextInput("settings-role-name", m.settings.RoleNameInput, 0)
	m.syncTextInput(m.settings.RoleNameInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			name := normalizeRoleNameInput(m.settings.RoleNameInput)
			if name == "" {
				return m, nil
			}

			for i := range m.globalCfg.Roles {
				if i == m.settings.EditRoleIdx {
					continue
				}
				if strings.EqualFold(m.globalCfg.Roles[i].Name, name) {
					return m, nil
				}
			}

			if m.settings.EditRoleIdx >= 0 && m.settings.EditRoleIdx < len(m.globalCfg.Roles) {
				old := m.globalCfg.Roles[m.settings.EditRoleIdx].Name
				m.globalCfg.Roles[m.settings.EditRoleIdx].Name = name
				if strings.EqualFold(m.globalCfg.DefaultRole, old) {
					m.globalCfg.DefaultRole = name
				}
				m.rewriteRoleReferences(old, name)
				m.settings.RolesSel = m.settings.EditRoleIdx
			} else {
				m.globalCfg.Roles = append(m.globalCfg.Roles, config.RoleDefinition{
					Name:         name,
					Title:        strings.ToUpper(name),
					Identity:     fmt.Sprintf("You are a %s role.", strings.ToUpper(strings.ReplaceAll(name, "-", " "))),
					CanWriteCode: true,
				})
				m.settings.RolesSel = len(m.globalCfg.Roles) - 1
			}
			saveSettingsRoleCatalog(m.globalCfg)
			m.state = stateSettingsRolesList
			return m, nil
		case tea.KeyEsc:
			m.state = stateSettingsRolesList
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.settings.RoleNameInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewSettingsRoleName() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("settings-role-name", m.settings.RoleNameInput, 0)
	m.syncTextInput(m.settings.RoleNameInput)

	title := "Settings — New Role Name"
	if m.settings.EditRoleIdx >= 0 {
		title = "Settings — Rename Role"
	}

	var lines []string
	lines = append(lines, sectionStyle.Render(title))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Use lowercase role names (example: reviewer, architect, qa)."))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: save  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Role edit ---

func (m AppModel) updateSettingsRoleEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	m.clampSettingsRolesSel()
	if m.settings.EditRoleIdx < 0 || m.settings.EditRoleIdx >= len(m.globalCfg.Roles) {
		m.settings.EditRoleIdx = m.settings.RolesSel
	}
	if m.settings.EditRoleIdx < 0 || m.settings.EditRoleIdx >= len(m.globalCfg.Roles) {
		m.state = stateSettingsRolesList
		return m, nil
	}
	m.clampSettingsRoleRuleSel()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		role := &m.globalCfg.Roles[m.settings.EditRoleIdx]
		switch msg.String() {
		case "tab", "l", "right":
			m.setRightPaneFocused(true)
			return m, nil
		case "shift+tab", "h", "left":
			m.setRightPaneFocused(false)
			return m, nil
		case "j", "down":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(1)
				return m, nil
			}
			if len(m.globalCfg.PromptRules) > 0 {
				m.settings.RoleRuleSel = (m.settings.RoleRuleSel + 1) % len(m.globalCfg.PromptRules)
				m.resetStateScroll()
			}
		case "k", "up":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(-1)
				return m, nil
			}
			if len(m.globalCfg.PromptRules) > 0 {
				m.settings.RoleRuleSel = (m.settings.RoleRuleSel - 1 + len(m.globalCfg.PromptRules)) % len(m.globalCfg.PromptRules)
				m.resetStateScroll()
			}
		case " ":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			ruleID := m.globalCfg.PromptRules[m.settings.RoleRuleSel].ID
			toggleRoleRule(role, ruleID)
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "w":
			role.CanWriteCode = !role.CanWriteCode
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "t":
			m.globalCfg.DefaultRole = role.Name
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "r":
			m.settings.EditRoleIdx = m.settings.RolesSel
			m.settings.RoleNameInput = role.Name
			m.state = stateSettingsRoleName
			return m, nil
		case "e", "enter":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			m.settings.EditRuleIdx = m.settings.RoleRuleSel
			m.settings.RuleBodyInput = m.globalCfg.PromptRules[m.settings.EditRuleIdx].Body
			m.settings.RuleBodyReturnState = stateSettingsRoleEdit
			m.state = stateSettingsRuleBody
			return m, nil
		case "a":
			m.settings.EditRuleIdx = -1
			m.settings.RuleIDInput = ""
			m.state = stateSettingsRuleID
			return m, nil
		case "i":
			m.settings.EditRuleIdx = -1
			m.settings.RuleBodyInput = role.Identity
			m.settings.RuleBodyReturnState = stateSettingsRoleEdit
			m.state = stateSettingsRuleBody
			return m, nil
		case "esc":
			m.settings.RolesSel = m.settings.EditRoleIdx
			m.state = stateSettingsRolesList
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRoleEdit() string {
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	if m.settings.EditRoleIdx < 0 || m.settings.EditRoleIdx >= len(m.globalCfg.Roles) {
		left := []string{
			sectionStyle.Render("Settings — Role Edit"),
			"",
			dimStyle.Render("Role not found."),
		}
		right := []string{
			sectionStyle.Render("Rule Content"),
			"",
			dimStyle.Render("No selected role."),
		}
		return m.renderSettingsSplitView(left, right, -1)
	}

	role := m.globalCfg.Roles[m.settings.EditRoleIdx]
	mode := "read-only"
	if role.CanWriteCode {
		mode = "can-write"
	}
	defaultTag := ""
	if strings.EqualFold(role.Name, m.globalCfg.DefaultRole) {
		defaultTag = " [default]"
	}

	var left []string
	cursorLine := -1
	left = append(left, sectionStyle.Render("Settings — Role: "+role.Name+defaultTag))
	left = append(left, textStyle.Render("Mode: "+mode))
	left = append(left, "")
	left = append(left, dimStyle.Render("Toggle rules for this role (space)."))
	left = append(left, "")

	for i, rule := range m.globalCfg.PromptRules {
		check := "[ ]"
		if roleHasRule(&role, rule.ID) {
			check = lipgloss.NewStyle().Foreground(ColorGreen).Render("[x]")
		}
		label := fmt.Sprintf("%s %s", check, rule.ID)
		if i == m.settings.RoleRuleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			left = append(left, cursor+styled)
			cursorLine = len(left) - 1
		} else {
			left = append(left, "  "+textStyle.Render(label))
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("space: toggle rule  w: toggle write mode  t: set default"))
	left = append(left, dimStyle.Render("i: edit identity  r: rename role  e/enter: edit selected rule"))
	left = append(left, dimStyle.Render("a: new rule  esc: back  tab/h/l: pane focus"))

	var right []string
	rightTitle := "Rule Content"
	if m.isRightPaneFocused() {
		rightTitle += " [focus]"
	}
	right = append(right, sectionStyle.Render(rightTitle))
	right = append(right, "")
	if len(m.globalCfg.PromptRules) == 0 {
		right = append(right, dimStyle.Render("No prompt rules configured."))
	} else {
		rule := m.globalCfg.PromptRules[m.settings.RoleRuleSel]
		right = append(right, textStyle.Render("Rule: "+rule.ID))
		if roleHasRule(&role, rule.ID) {
			right = append(right, textStyle.Render("Assigned: yes"))
		} else {
			right = append(right, dimStyle.Render("Assigned: no"))
		}
		right = append(right, "")
		if strings.TrimSpace(rule.Body) == "" {
			right = append(right, dimStyle.Render("(empty rule body)"))
		} else {
			right = appendMultiline(right, rule.Body)
		}
	}

	return m.renderSettingsSplitView(left, right, cursorLine)
}

// --- Rules list ---

func (m AppModel) updateSettingsRulesList(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	m.clampSettingsRulesSel()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "l", "right":
			m.setRightPaneFocused(true)
			return m, nil
		case "shift+tab", "h", "left":
			m.setRightPaneFocused(false)
			return m, nil
		case "j", "down":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(1)
				return m, nil
			}
			if len(m.globalCfg.PromptRules) > 0 {
				m.settings.RulesSel = (m.settings.RulesSel + 1) % len(m.globalCfg.PromptRules)
				m.resetStateScroll()
			}
		case "k", "up":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(-1)
				return m, nil
			}
			if len(m.globalCfg.PromptRules) > 0 {
				m.settings.RulesSel = (m.settings.RulesSel - 1 + len(m.globalCfg.PromptRules)) % len(m.globalCfg.PromptRules)
				m.resetStateScroll()
			}
		case "a":
			m.settings.EditRuleIdx = -1
			m.settings.RuleIDInput = ""
			m.state = stateSettingsRuleID
			return m, nil
		case "r":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			m.settings.EditRuleIdx = m.settings.RulesSel
			m.settings.RuleIDInput = m.globalCfg.PromptRules[m.settings.RulesSel].ID
			m.state = stateSettingsRuleID
			return m, nil
		case "d":
			if len(m.globalCfg.PromptRules) <= 1 {
				return m, nil
			}
			ruleID := m.globalCfg.PromptRules[m.settings.RulesSel].ID
			m.globalCfg.RemovePromptRule(ruleID)
			m.clampSettingsRulesSel()
			m.resetStateScroll()
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "e", "enter":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			m.settings.EditRuleIdx = m.settings.RulesSel
			m.settings.RuleBodyInput = m.globalCfg.PromptRules[m.settings.EditRuleIdx].Body
			m.settings.RuleBodyReturnState = stateSettingsRulesList
			m.state = stateSettingsRuleBody
			return m, nil
		case "esc":
			m.state = stateSettingsRolesRulesMenu
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRulesList() string {
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	var left []string
	cursorLine := -1
	left = append(left, sectionStyle.Render("Settings — Prompt Rules"))
	left = append(left, "")
	left = append(left, dimStyle.Render("Select a rule to inspect full content on the right."))
	left = append(left, "")

	if len(m.globalCfg.PromptRules) == 0 {
		left = append(left, dimStyle.Render("No prompt rules configured. Press 'a' to add one."))
	} else {
		for i, rule := range m.globalCfg.PromptRules {
			label := rule.ID
			if i == m.settings.RulesSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
				left = append(left, cursor+styled)
				cursorLine = len(left) - 1
			} else {
				left = append(left, "  "+textStyle.Render(label))
			}
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("a: add  r: rename id  d: delete"))
	left = append(left, dimStyle.Render("e/enter: edit body  esc: back  tab/h/l: pane focus"))

	var right []string
	rightTitle := "Rule Content"
	if m.isRightPaneFocused() {
		rightTitle += " [focus]"
	}
	right = append(right, sectionStyle.Render(rightTitle))
	right = append(right, "")
	if len(m.globalCfg.PromptRules) == 0 {
		right = append(right, dimStyle.Render("No rule selected."))
	} else {
		m.clampSettingsRulesSel()
		rule := m.globalCfg.PromptRules[m.settings.RulesSel]
		right = append(right, textStyle.Render("Rule: "+rule.ID))
		right = append(right, "")
		if strings.TrimSpace(rule.Body) == "" {
			right = append(right, dimStyle.Render("(empty rule body)"))
		} else {
			right = appendMultiline(right, rule.Body)
		}
	}

	return m.renderSettingsSplitView(left, right, cursorLine)
}

// --- Rule ID input ---

func (m AppModel) updateSettingsRuleID(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	initCmd := m.ensureTextInput("settings-rule-id", m.settings.RuleIDInput, 0)
	m.syncTextInput(m.settings.RuleIDInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			id := normalizeRuleIDInput(m.settings.RuleIDInput)
			if id == "" {
				return m, nil
			}
			for i := range m.globalCfg.PromptRules {
				if i == m.settings.EditRuleIdx {
					continue
				}
				if strings.EqualFold(m.globalCfg.PromptRules[i].ID, id) {
					return m, nil
				}
			}

			if m.settings.EditRuleIdx >= 0 && m.settings.EditRuleIdx < len(m.globalCfg.PromptRules) {
				oldID := m.globalCfg.PromptRules[m.settings.EditRuleIdx].ID
				m.globalCfg.PromptRules[m.settings.EditRuleIdx].ID = id
				m.rewriteRuleReferences(oldID, id)
				m.settings.RulesSel = m.settings.EditRuleIdx
				saveSettingsRoleCatalog(m.globalCfg)
				m.state = stateSettingsRulesList
				return m, nil
			}

			m.globalCfg.PromptRules = append(m.globalCfg.PromptRules, config.PromptRule{ID: id})
			m.settings.RulesSel = len(m.globalCfg.PromptRules) - 1
			m.settings.EditRuleIdx = m.settings.RulesSel
			m.settings.RuleBodyInput = ""
			m.settings.RuleBodyReturnState = stateSettingsRulesList
			saveSettingsRoleCatalog(m.globalCfg)
			m.state = stateSettingsRuleBody
			return m, nil
		case tea.KeyEsc:
			m.state = stateSettingsRulesList
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.settings.RuleIDInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewSettingsRuleID() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("settings-rule-id", m.settings.RuleIDInput, 0)
	m.syncTextInput(m.settings.RuleIDInput)

	title := "Settings — New Rule ID"
	if m.settings.EditRuleIdx >= 0 {
		title = "Settings — Rename Rule ID"
	}

	var lines []string
	lines = append(lines, sectionStyle.Render(title))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Use lowercase IDs like: reviewer_identity, qa_checks, handoff_policy"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue/save  esc: cancel"))

	content := fitLinesWithCursor(lines, cw, ch, len(lines)-1)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Rule body editor ---

func (m AppModel) updateSettingsRuleBody(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	editorKey, editingRoleIdentity, ok := m.ruleBodyEditorContext()
	if !ok {
		m.state = stateSettingsRulesList
		return m, nil
	}
	returnState := m.settings.RuleBodyReturnState
	if returnState != stateSettingsRoleEdit && returnState != stateSettingsRulesList {
		if editingRoleIdentity {
			returnState = stateSettingsRoleEdit
		} else {
			returnState = stateSettingsRulesList
		}
	}
	initCmd := m.ensureTextarea(editorKey, m.settings.RuleBodyInput)
	m.syncTextarea(m.settings.RuleBodyInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyCtrlS:
			if editingRoleIdentity {
				m.globalCfg.Roles[m.settings.EditRoleIdx].Identity = m.settings.RuleBodyInput
			} else {
				m.globalCfg.PromptRules[m.settings.EditRuleIdx].Body = m.settings.RuleBodyInput
			}
			saveSettingsRoleCatalog(m.globalCfg)
			if returnState == stateSettingsRulesList && !editingRoleIdentity {
				m.settings.RulesSel = m.settings.EditRuleIdx
			}
			m.settings.RuleBodyReturnState = 0
			m.state = returnState
			return m, nil
		case tea.KeyEsc:
			m.settings.RuleBodyReturnState = 0
			m.state = returnState
			return m, nil
		}
	}
	cmd := m.updateTextarea(msg)
	m.settings.RuleBodyInput = m.textArea.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewSettingsRuleBody() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	editorKey, editingRoleIdentity, ok := m.ruleBodyEditorContext()
	ruleID := ""
	roleName := ""
	if editingRoleIdentity {
		roleName = m.globalCfg.Roles[m.settings.EditRoleIdx].Name
	} else if ok {
		ruleID = m.globalCfg.PromptRules[m.settings.EditRuleIdx].ID
	}

	var lines []string
	if editingRoleIdentity {
		lines = append(lines, sectionStyle.Render("Settings — Edit Role Identity"))
	} else {
		lines = append(lines, sectionStyle.Render("Settings — Edit Rule Body"))
	}
	if roleName != "" {
		lines = append(lines, dimStyle.Render("Role: "+roleName))
	} else if ruleID != "" {
		lines = append(lines, dimStyle.Render("Rule: "+ruleID))
	}
	lines = append(lines, "")
	if editingRoleIdentity {
		lines = append(lines, dimStyle.Render("Type identity text for this role."))
	} else {
		lines = append(lines, dimStyle.Render("Type rule text directly."))
	}
	lines = append(lines, dimStyle.Render("enter: newline  ctrl+s: save  esc: cancel"))
	lines = append(lines, "")

	if ok {
		m.ensureTextarea(editorKey, m.settings.RuleBodyInput)
		m.syncTextarea(m.settings.RuleBodyInput)

		prefixLines := wrapRenderableLines(lines, cw)
		editorHeight := ch - len(prefixLines)
		if editorHeight < 3 {
			editorHeight = 3
		}
		editorView := m.viewTextarea(cw, editorHeight)
		for _, ln := range splitRenderableLines(editorView) {
			lines = append(lines, ln)
		}
	} else {
		lines = append(lines, dimStyle.Render("No editable rule body selected."))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
