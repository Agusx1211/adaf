package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/prompt"
)

const settingsRolesRulesMenuItemCount = 3 // Roles, Prompt Rules, Back

func normalizeRoleNameInput(in string) string {
	return strings.ToLower(strings.TrimSpace(in))
}

func normalizeRuleIDInput(in string) string {
	return strings.ToLower(strings.TrimSpace(in))
}

func deleteLastRune(s string) string {
	if s == "" {
		return s
	}
	_, size := utf8.DecodeLastRuneInString(s)
	if size <= 0 || size > len(s) {
		return s[:len(s)-1]
	}
	return s[:len(s)-size]
}

func ensureSettingsRoleCatalog(cfg *config.GlobalConfig) {
	config.EnsureDefaultRoleCatalog(cfg)
}

func saveSettingsRoleCatalog(cfg *config.GlobalConfig) {
	config.EnsureDefaultRoleCatalog(cfg)
	_ = config.Save(cfg)
}

func settingsPanelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSurface2).
		Padding(1, 2)
}

func renderSettingsPane(lines []string, outerW, outerH int) string {
	style := settingsPanelStyle()
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

func (m AppModel) renderSettingsSplitView(leftLines, rightLines []string) string {
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
		panel := renderSettingsPane(combined, m.width, panelH)
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

	left := renderSettingsPane(leftLines, leftOuter, panelH)
	right := renderSettingsPane(rightLines, rightOuter, panelH)
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + statusBar
}

func (m *AppModel) clampSettingsRolesSel() {
	if len(m.globalCfg.Roles) == 0 {
		m.settingsRolesSel = 0
		return
	}
	if m.settingsRolesSel < 0 {
		m.settingsRolesSel = 0
	}
	if m.settingsRolesSel >= len(m.globalCfg.Roles) {
		m.settingsRolesSel = len(m.globalCfg.Roles) - 1
	}
}

func (m *AppModel) clampSettingsRulesSel() {
	if len(m.globalCfg.PromptRules) == 0 {
		m.settingsRulesSel = 0
		return
	}
	if m.settingsRulesSel < 0 {
		m.settingsRulesSel = 0
	}
	if m.settingsRulesSel >= len(m.globalCfg.PromptRules) {
		m.settingsRulesSel = len(m.globalCfg.PromptRules) - 1
	}
}

func (m *AppModel) clampSettingsRoleRuleSel() {
	if len(m.globalCfg.PromptRules) == 0 {
		m.settingsRoleRuleSel = 0
		return
	}
	if m.settingsRoleRuleSel < 0 {
		m.settingsRoleRuleSel = 0
	}
	if m.settingsRoleRuleSel >= len(m.globalCfg.PromptRules) {
		m.settingsRoleRuleSel = len(m.globalCfg.PromptRules) - 1
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
			m.settingsRolesRulesSel = (m.settingsRolesRulesSel + 1) % settingsRolesRulesMenuItemCount
		case "k", "up":
			m.settingsRolesRulesSel = (m.settingsRolesRulesSel - 1 + settingsRolesRulesMenuItemCount) % settingsRolesRulesMenuItemCount
		case "enter":
			switch m.settingsRolesRulesSel {
			case 0:
				m.settingsRolesSel = 0
				m.state = stateSettingsRolesList
			case 1:
				m.settingsRulesSel = 0
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
	lines = append(lines, sectionStyle.Render("Settings — Roles & Rules"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Manage role definitions and reusable prompt rule sections."))
	lines = append(lines, "")
	for i, opt := range options {
		if i == m.settingsRolesRulesSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(opt)
			lines = append(lines, cursor+styled)
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(opt))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLines(lines, cw, ch)
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
		case "j", "down":
			if len(m.globalCfg.Roles) > 0 {
				m.settingsRolesSel = (m.settingsRolesSel + 1) % len(m.globalCfg.Roles)
			}
		case "k", "up":
			if len(m.globalCfg.Roles) > 0 {
				m.settingsRolesSel = (m.settingsRolesSel - 1 + len(m.globalCfg.Roles)) % len(m.globalCfg.Roles)
			}
		case "a":
			m.settingsEditRoleIdx = -1
			m.settingsRoleNameInput = ""
			m.state = stateSettingsRoleName
			return m, nil
		case "r":
			if len(m.globalCfg.Roles) == 0 {
				return m, nil
			}
			m.settingsEditRoleIdx = m.settingsRolesSel
			m.settingsRoleNameInput = m.globalCfg.Roles[m.settingsRolesSel].Name
			m.state = stateSettingsRoleName
			return m, nil
		case "d":
			if len(m.globalCfg.Roles) <= 1 {
				return m, nil
			}
			toDelete := m.globalCfg.Roles[m.settingsRolesSel].Name
			m.globalCfg.RemoveRoleDefinition(toDelete)
			m.clampSettingsRolesSel()
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "enter":
			if len(m.globalCfg.Roles) == 0 {
				return m, nil
			}
			m.settingsEditRoleIdx = m.settingsRolesSel
			m.settingsRoleRuleSel = 0
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
			if i == m.settingsRolesSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
				left = append(left, cursor+styled)
			} else {
				left = append(left, "  "+textStyle.Render(label))
			}
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("a: add  r: rename  d: delete"))
	left = append(left, dimStyle.Render("enter: edit role  esc: back"))

	var right []string
	right = append(right, sectionStyle.Render("Role Details"))
	right = append(right, "")
	if len(m.globalCfg.Roles) == 0 {
		right = append(right, dimStyle.Render("No role selected."))
	} else {
		m.clampSettingsRolesSel()
		role := m.globalCfg.Roles[m.settingsRolesSel]
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

	return m.renderSettingsSplitView(left, right)
}

// --- Role name input ---

func (m AppModel) updateSettingsRoleName(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := normalizeRoleNameInput(m.settingsRoleNameInput)
			if name == "" {
				return m, nil
			}

			for i := range m.globalCfg.Roles {
				if i == m.settingsEditRoleIdx {
					continue
				}
				if strings.EqualFold(m.globalCfg.Roles[i].Name, name) {
					return m, nil
				}
			}

			if m.settingsEditRoleIdx >= 0 && m.settingsEditRoleIdx < len(m.globalCfg.Roles) {
				old := m.globalCfg.Roles[m.settingsEditRoleIdx].Name
				m.globalCfg.Roles[m.settingsEditRoleIdx].Name = name
				if strings.EqualFold(m.globalCfg.DefaultRole, old) {
					m.globalCfg.DefaultRole = name
				}
				m.rewriteRoleReferences(old, name)
				m.settingsRolesSel = m.settingsEditRoleIdx
			} else {
				m.globalCfg.Roles = append(m.globalCfg.Roles, config.RoleDefinition{
					Name:         name,
					Title:        strings.ToUpper(name),
					Identity:     fmt.Sprintf("You are a %s role.", strings.ToUpper(strings.ReplaceAll(name, "-", " "))),
					CanWriteCode: true,
				})
				m.settingsRolesSel = len(m.globalCfg.Roles) - 1
			}
			saveSettingsRoleCatalog(m.globalCfg)
			m.state = stateSettingsRolesList
			return m, nil
		case "esc":
			m.state = stateSettingsRolesList
			return m, nil
		case "backspace":
			m.settingsRoleNameInput = deleteLastRune(m.settingsRoleNameInput)
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.settingsRoleNameInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRoleName() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	title := "Settings — New Role Name"
	if m.settingsEditRoleIdx >= 0 {
		title = "Settings — Rename Role"
	}

	var lines []string
	lines = append(lines, sectionStyle.Render(title))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Use lowercase role names (example: reviewer, architect, qa)."))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	display := truncateInputForDisplay(m.settingsRoleNameInput, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(display)
	lines = append(lines, "> "+inputText+cursor)
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
	if m.settingsEditRoleIdx < 0 || m.settingsEditRoleIdx >= len(m.globalCfg.Roles) {
		m.settingsEditRoleIdx = m.settingsRolesSel
	}
	if m.settingsEditRoleIdx < 0 || m.settingsEditRoleIdx >= len(m.globalCfg.Roles) {
		m.state = stateSettingsRolesList
		return m, nil
	}
	m.clampSettingsRoleRuleSel()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		role := &m.globalCfg.Roles[m.settingsEditRoleIdx]
		switch msg.String() {
		case "j", "down":
			if len(m.globalCfg.PromptRules) > 0 {
				m.settingsRoleRuleSel = (m.settingsRoleRuleSel + 1) % len(m.globalCfg.PromptRules)
			}
		case "k", "up":
			if len(m.globalCfg.PromptRules) > 0 {
				m.settingsRoleRuleSel = (m.settingsRoleRuleSel - 1 + len(m.globalCfg.PromptRules)) % len(m.globalCfg.PromptRules)
			}
		case " ":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			ruleID := m.globalCfg.PromptRules[m.settingsRoleRuleSel].ID
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
			m.settingsEditRoleIdx = m.settingsRolesSel
			m.settingsRoleNameInput = role.Name
			m.state = stateSettingsRoleName
			return m, nil
		case "e", "enter":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			m.settingsEditRuleIdx = m.settingsRoleRuleSel
			m.settingsRuleBodyInput = m.globalCfg.PromptRules[m.settingsEditRuleIdx].Body
			m.state = stateSettingsRuleBody
			return m, nil
		case "a":
			m.settingsEditRuleIdx = -1
			m.settingsRuleIDInput = ""
			m.state = stateSettingsRuleID
			return m, nil
		case "i":
			m.settingsEditRuleIdx = -1
			m.settingsRuleBodyInput = role.Identity
			m.state = stateSettingsRuleBody
			return m, nil
		case "esc":
			m.settingsRolesSel = m.settingsEditRoleIdx
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

	if m.settingsEditRoleIdx < 0 || m.settingsEditRoleIdx >= len(m.globalCfg.Roles) {
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
		return m.renderSettingsSplitView(left, right)
	}

	role := m.globalCfg.Roles[m.settingsEditRoleIdx]
	mode := "read-only"
	if role.CanWriteCode {
		mode = "can-write"
	}
	defaultTag := ""
	if strings.EqualFold(role.Name, m.globalCfg.DefaultRole) {
		defaultTag = " [default]"
	}

	var left []string
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
		if i == m.settingsRoleRuleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			left = append(left, cursor+styled)
		} else {
			left = append(left, "  "+textStyle.Render(label))
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("space: toggle rule  w: toggle write mode  t: set default"))
	left = append(left, dimStyle.Render("i: edit identity  r: rename role  e/enter: edit selected rule"))
	left = append(left, dimStyle.Render("a: new rule  esc: back"))

	var right []string
	right = append(right, sectionStyle.Render("Rule Content"))
	right = append(right, "")
	if len(m.globalCfg.PromptRules) == 0 {
		right = append(right, dimStyle.Render("No prompt rules configured."))
	} else {
		rule := m.globalCfg.PromptRules[m.settingsRoleRuleSel]
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

	return m.renderSettingsSplitView(left, right)
}

// --- Rules list ---

func (m AppModel) updateSettingsRulesList(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	m.clampSettingsRulesSel()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.globalCfg.PromptRules) > 0 {
				m.settingsRulesSel = (m.settingsRulesSel + 1) % len(m.globalCfg.PromptRules)
			}
		case "k", "up":
			if len(m.globalCfg.PromptRules) > 0 {
				m.settingsRulesSel = (m.settingsRulesSel - 1 + len(m.globalCfg.PromptRules)) % len(m.globalCfg.PromptRules)
			}
		case "a":
			m.settingsEditRuleIdx = -1
			m.settingsRuleIDInput = ""
			m.state = stateSettingsRuleID
			return m, nil
		case "r":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			m.settingsEditRuleIdx = m.settingsRulesSel
			m.settingsRuleIDInput = m.globalCfg.PromptRules[m.settingsRulesSel].ID
			m.state = stateSettingsRuleID
			return m, nil
		case "d":
			if len(m.globalCfg.PromptRules) <= 1 {
				return m, nil
			}
			ruleID := m.globalCfg.PromptRules[m.settingsRulesSel].ID
			m.globalCfg.RemovePromptRule(ruleID)
			m.clampSettingsRulesSel()
			saveSettingsRoleCatalog(m.globalCfg)
			return m, nil
		case "e", "enter":
			if len(m.globalCfg.PromptRules) == 0 {
				return m, nil
			}
			m.settingsEditRuleIdx = m.settingsRulesSel
			m.settingsRuleBodyInput = m.globalCfg.PromptRules[m.settingsEditRuleIdx].Body
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
	left = append(left, sectionStyle.Render("Settings — Prompt Rules"))
	left = append(left, "")
	left = append(left, dimStyle.Render("Select a rule to inspect full content on the right."))
	left = append(left, "")

	if len(m.globalCfg.PromptRules) == 0 {
		left = append(left, dimStyle.Render("No prompt rules configured. Press 'a' to add one."))
	} else {
		for i, rule := range m.globalCfg.PromptRules {
			label := rule.ID
			if i == m.settingsRulesSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
				left = append(left, cursor+styled)
			} else {
				left = append(left, "  "+textStyle.Render(label))
			}
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("a: add  r: rename id  d: delete"))
	left = append(left, dimStyle.Render("e/enter: edit body  esc: back"))

	var right []string
	right = append(right, sectionStyle.Render("Rule Content"))
	right = append(right, "")
	if len(m.globalCfg.PromptRules) == 0 {
		right = append(right, dimStyle.Render("No rule selected."))
	} else {
		m.clampSettingsRulesSel()
		rule := m.globalCfg.PromptRules[m.settingsRulesSel]
		right = append(right, textStyle.Render("Rule: "+rule.ID))
		right = append(right, "")
		if strings.TrimSpace(rule.Body) == "" {
			right = append(right, dimStyle.Render("(empty rule body)"))
		} else {
			right = appendMultiline(right, rule.Body)
		}
	}

	return m.renderSettingsSplitView(left, right)
}

// --- Rule ID input ---

func (m AppModel) updateSettingsRuleID(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			id := normalizeRuleIDInput(m.settingsRuleIDInput)
			if id == "" {
				return m, nil
			}
			for i := range m.globalCfg.PromptRules {
				if i == m.settingsEditRuleIdx {
					continue
				}
				if strings.EqualFold(m.globalCfg.PromptRules[i].ID, id) {
					return m, nil
				}
			}

			if m.settingsEditRuleIdx >= 0 && m.settingsEditRuleIdx < len(m.globalCfg.PromptRules) {
				oldID := m.globalCfg.PromptRules[m.settingsEditRuleIdx].ID
				m.globalCfg.PromptRules[m.settingsEditRuleIdx].ID = id
				m.rewriteRuleReferences(oldID, id)
				m.settingsRulesSel = m.settingsEditRuleIdx
				saveSettingsRoleCatalog(m.globalCfg)
				m.state = stateSettingsRulesList
				return m, nil
			}

			m.globalCfg.PromptRules = append(m.globalCfg.PromptRules, config.PromptRule{ID: id})
			m.settingsRulesSel = len(m.globalCfg.PromptRules) - 1
			m.settingsEditRuleIdx = m.settingsRulesSel
			m.settingsRuleBodyInput = ""
			saveSettingsRoleCatalog(m.globalCfg)
			m.state = stateSettingsRuleBody
			return m, nil
		case "esc":
			m.state = stateSettingsRulesList
			return m, nil
		case "backspace":
			m.settingsRuleIDInput = deleteLastRune(m.settingsRuleIDInput)
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.settingsRuleIDInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRuleID() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	title := "Settings — New Rule ID"
	if m.settingsEditRuleIdx >= 0 {
		title = "Settings — Rename Rule ID"
	}

	var lines []string
	lines = append(lines, sectionStyle.Render(title))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Use lowercase IDs like: reviewer_identity, qa_checks, handoff_policy"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	display := truncateInputForDisplay(m.settingsRuleIDInput, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(display)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue/save  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Rule body editor ---

func (m AppModel) updateSettingsRuleBody(msg tea.Msg) (tea.Model, tea.Cmd) {
	ensureSettingsRoleCatalog(m.globalCfg)
	editingRoleIdentity := m.settingsEditRuleIdx < 0 && m.settingsEditRoleIdx >= 0 && m.settingsEditRoleIdx < len(m.globalCfg.Roles)
	if !editingRoleIdentity && (m.settingsEditRuleIdx < 0 || m.settingsEditRuleIdx >= len(m.globalCfg.PromptRules)) {
		m.state = stateSettingsRulesList
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "S", "ctrl+s":
			if editingRoleIdentity {
				m.globalCfg.Roles[m.settingsEditRoleIdx].Identity = m.settingsRuleBodyInput
			} else {
				m.globalCfg.PromptRules[m.settingsEditRuleIdx].Body = m.settingsRuleBodyInput
			}
			saveSettingsRoleCatalog(m.globalCfg)
			if editingRoleIdentity {
				m.state = stateSettingsRoleEdit
			} else {
				m.settingsRulesSel = m.settingsEditRuleIdx
				m.state = stateSettingsRulesList
			}
			return m, nil
		case "esc":
			if editingRoleIdentity {
				m.state = stateSettingsRoleEdit
			} else {
				m.state = stateSettingsRulesList
			}
			return m, nil
		case "enter":
			m.settingsRuleBodyInput += "\n"
			return m, nil
		case "backspace":
			m.settingsRuleBodyInput = deleteLastRune(m.settingsRuleBodyInput)
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.settingsRuleBodyInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewSettingsRuleBody() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	editingRoleIdentity := m.settingsEditRuleIdx < 0 && m.settingsEditRoleIdx >= 0 && m.settingsEditRoleIdx < len(m.globalCfg.Roles)
	ruleID := ""
	roleName := ""
	if editingRoleIdentity {
		roleName = m.globalCfg.Roles[m.settingsEditRoleIdx].Name
	} else if m.settingsEditRuleIdx >= 0 && m.settingsEditRuleIdx < len(m.globalCfg.PromptRules) {
		ruleID = m.globalCfg.PromptRules[m.settingsEditRuleIdx].ID
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
	lines = append(lines, dimStyle.Render("enter: newline  ctrl+s/S: save  esc: cancel"))
	lines = append(lines, "")

	editorLines := strings.Split(m.settingsRuleBodyInput, "\n")
	if len(editorLines) == 0 {
		editorLines = []string{""}
	}
	editorLines[len(editorLines)-1] = editorLines[len(editorLines)-1] + lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	for _, ln := range editorLines {
		lines = append(lines, textStyle.Render(ln))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
