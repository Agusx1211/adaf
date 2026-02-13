package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
)

type loopDelegationNode struct {
	Profile  string
	Roles    []string
	Speed    string
	Handoff  bool
	Children []*loopDelegationNode
}

// loopWizardTitle returns "Edit Loop" or "New Loop" depending on mode.
func (m AppModel) loopWizardTitle() string {
	if m.loopEditing {
		return "Edit Loop"
	}
	return "New Loop"
}

// loopMenuItemCount is the number of items in the edit menu (Name, Steps, Save).
const loopMenuItemCount = 3

// loopStepToolCount is the number of toggleable tools in the step tools form.
const loopStepToolCount = 3

func (m AppModel) effectiveLoopStepRole(step config.LoopStep) string {
	return config.EffectiveStepRole(step.Role, m.globalCfg)
}

func (m *AppModel) resetLoopStepSpawnOptions() {
	m.loopStepSpawnOpts = nil
	m.loopStepSpawnSel = 0
	m.loopStepSpawnCfgSel = 0
	m.loopStepDelegRoots = nil
	m.loopStepDelegPath = nil
	for _, p := range m.globalCfg.Profiles {
		m.loopStepSpawnOpts = append(m.loopStepSpawnOpts, p.Name)
	}
}

func (m *AppModel) preselectLoopStepSpawn(step config.LoopStep) {
	m.loopStepDelegRoots = delegationConfigToNodes(step.Delegation, m.globalCfg)
	m.loopStepDelegPath = nil
	m.loopStepSpawnSel = 0
	m.loopStepSpawnRoleSel = 0
}

func countDelegationProfiles(deleg *config.DelegationConfig) int {
	if deleg == nil {
		return 0
	}
	total := 0
	for _, dp := range deleg.Profiles {
		total++
		total += countDelegationProfiles(dp.Delegation)
	}
	return total
}

func loopStepSpawnCount(step config.LoopStep) int {
	return countDelegationProfiles(step.Delegation)
}

func cloneDelegationNodes(nodes []*loopDelegationNode) []*loopDelegationNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]*loopDelegationNode, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		cp := &loopDelegationNode{
			Profile: n.Profile,
			Roles:   append([]string(nil), n.Roles...),
			Speed:   n.Speed,
			Handoff: n.Handoff,
		}
		cp.Children = cloneDelegationNodes(n.Children)
		out = append(out, cp)
	}
	return out
}

func normalizeDelegationRoles(roles []string) []string {
	out := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, raw := range roles {
		role := strings.ToLower(strings.TrimSpace(raw))
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	return out
}

func ensureDelegationRoles(roles []string, globalCfg *config.GlobalConfig) []string {
	norm := normalizeDelegationRoles(roles)
	if len(norm) == 0 {
		return []string{config.DefaultRole(globalCfg)}
	}
	return norm
}

func delegationRoleListContains(roles []string, role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, existing := range roles {
		if existing == role {
			return true
		}
	}
	return false
}

func toggleDelegationRole(roles []string, role string, globalCfg *config.GlobalConfig) []string {
	normRoles := ensureDelegationRoles(roles, globalCfg)
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return normRoles
	}
	for i, existing := range normRoles {
		if existing != role {
			continue
		}
		// Keep at least one role selected at all times.
		if len(normRoles) == 1 {
			return normRoles
		}
		return append(normRoles[:i], normRoles[i+1:]...)
	}
	return append(normRoles, role)
}

func delegationRolesLabel(roles []string, globalCfg *config.GlobalConfig) string {
	return strings.Join(ensureDelegationRoles(roles, globalCfg), "/")
}

func delegationConfigToNodes(deleg *config.DelegationConfig, globalCfg *config.GlobalConfig) []*loopDelegationNode {
	if deleg == nil || len(deleg.Profiles) == 0 {
		return nil
	}
	nodes := make([]*loopDelegationNode, 0, len(deleg.Profiles))
	for _, dp := range deleg.Profiles {
		roles, err := dp.EffectiveRoles()
		if err != nil {
			roles = nil
		}
		node := &loopDelegationNode{
			Profile: dp.Name,
			Roles:   ensureDelegationRoles(roles, globalCfg),
			Speed:   dp.Speed,
			Handoff: dp.Handoff,
		}
		node.Children = delegationConfigToNodes(dp.Delegation, globalCfg)
		nodes = append(nodes, node)
	}
	return nodes
}

func nodesToDelegationConfig(nodes []*loopDelegationNode, globalCfg *config.GlobalConfig) *config.DelegationConfig {
	if len(nodes) == 0 {
		return nil
	}
	out := &config.DelegationConfig{
		Profiles: make([]config.DelegationProfile, 0, len(nodes)),
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		name := strings.TrimSpace(node.Profile)
		if name == "" {
			continue
		}
		roles := ensureDelegationRoles(node.Roles, globalCfg)
		dp := config.DelegationProfile{
			Name: name,
		}
		if len(roles) == 1 {
			dp.Role = roles[0]
		} else {
			dp.Roles = append(dp.Roles, roles...)
		}
		if node.Speed != "" {
			dp.Speed = node.Speed
		}
		if node.Handoff {
			dp.Handoff = true
		}
		dp.Delegation = nodesToDelegationConfig(node.Children, globalCfg)
		out.Profiles = append(out.Profiles, dp)
	}
	if len(out.Profiles) == 0 {
		return nil
	}
	return out
}

func (m *AppModel) currentDelegationLevel() *[]*loopDelegationNode {
	level := &m.loopStepDelegRoots
	for _, idx := range m.loopStepDelegPath {
		if idx < 0 || idx >= len(*level) {
			break
		}
		node := (*level)[idx]
		if node == nil {
			break
		}
		level = &node.Children
	}
	return level
}

func (m *AppModel) clampDelegationSelection() {
	level := m.currentDelegationLevel()
	if len(*level) == 0 {
		m.loopStepSpawnSel = 0
		return
	}
	if m.loopStepSpawnSel < 0 {
		m.loopStepSpawnSel = 0
	}
	if m.loopStepSpawnSel >= len(*level) {
		m.loopStepSpawnSel = len(*level) - 1
	}
}

func defaultLoopStepRoleSel(globalCfg *config.GlobalConfig) int {
	roles := config.AllRoles(globalCfg)
	defaultRole := config.DefaultRole(globalCfg)
	for i, role := range roles {
		if role == defaultRole {
			return i
		}
	}
	return 0
}

func (m *AppModel) setLoopStepRoleSelection(role string) {
	role = config.EffectiveRole(role, m.globalCfg)
	m.loopStepRoleSel = defaultLoopStepRoleSel(m.globalCfg)
	for i, candidate := range config.AllRoles(m.globalCfg) {
		if candidate == role {
			m.loopStepRoleSel = i
			return
		}
	}
}

// --- Loop Name ---

func (m AppModel) updateLoopName(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			name := strings.TrimSpace(m.loopNameInput)
			if name == "" {
				return m, nil
			}
			if existing := m.globalCfg.FindLoop(name); existing != nil {
				if !m.loopEditing || !strings.EqualFold(name, m.loopEditName) {
					return m, nil // duplicate
				}
			}
			m.loopNameInput = name
			if m.loopEditing {
				m.state = stateLoopMenu
				return m, nil
			}
			// New loop: go to step list.
			m.loopSteps = nil
			m.loopStepSel = 0
			m.state = stateLoopStepList
			return m, nil
		case "esc":
			if m.loopEditing {
				m.state = stateLoopMenu
				return m, nil
			}
			m.state = stateSelector
			return m, nil
		case "backspace":
			if len(m.loopNameInput) > 0 {
				m.loopNameInput = m.loopNameInput[:len(m.loopNameInput)-1]
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.loopNameInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewLoopName() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Name"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter a name for the loop:"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	displayName := truncateInputForDisplay(m.loopNameInput, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(displayName)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: confirm  esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step List ---

func (m AppModel) updateLoopStepList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.loopSteps) > 0 {
				m.loopStepSel = (m.loopStepSel + 1) % len(m.loopSteps)
			}
		case "k", "up":
			if len(m.loopSteps) > 0 {
				m.loopStepSel = (m.loopStepSel - 1 + len(m.loopSteps)) % len(m.loopSteps)
			}
		case "a":
			// Add new step.
			m.loopStepEditIdx = -1
			m.loopStepProfileOpts = nil
			for _, p := range m.globalCfg.Profiles {
				m.loopStepProfileOpts = append(m.loopStepProfileOpts, p.Name)
			}
			m.loopStepProfileSel = 0
			m.loopStepTurnsInput = "1"
			m.loopStepInstrInput = ""
			m.loopStepCanStop = false
			m.loopStepCanMsg = false
			m.loopStepCanPushover = false
			m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg))
			m.resetLoopStepSpawnOptions()
			m.loopStepToolsSel = 0
			m.state = stateLoopStepProfile
			return m, nil
		case "enter":
			// Edit selected step.
			if len(m.loopSteps) > 0 && m.loopStepSel < len(m.loopSteps) {
				m.loopStepEditIdx = m.loopStepSel
				step := m.loopSteps[m.loopStepSel]
				m.loopStepProfileOpts = nil
				for _, p := range m.globalCfg.Profiles {
					m.loopStepProfileOpts = append(m.loopStepProfileOpts, p.Name)
				}
				m.loopStepProfileSel = 0
				for i, name := range m.loopStepProfileOpts {
					if strings.EqualFold(name, step.Profile) {
						m.loopStepProfileSel = i
						break
					}
				}
				effectiveRole := m.effectiveLoopStepRole(step)
				m.setLoopStepRoleSelection(effectiveRole)
				turns := step.Turns
				if turns <= 0 {
					turns = 1
				}
				m.loopStepTurnsInput = strconv.Itoa(turns)
				m.loopStepInstrInput = step.Instructions
				m.loopStepCanStop = step.CanStop
				m.loopStepCanMsg = step.CanMessage
				m.loopStepCanPushover = step.CanPushover
				m.resetLoopStepSpawnOptions()
				m.preselectLoopStepSpawn(step)
				m.loopStepToolsSel = 0
				m.state = stateLoopStepProfile
			}
			return m, nil
		case "d":
			// Delete selected step.
			if len(m.loopSteps) > 0 && m.loopStepSel < len(m.loopSteps) {
				m.loopSteps = append(m.loopSteps[:m.loopStepSel], m.loopSteps[m.loopStepSel+1:]...)
				if m.loopStepSel >= len(m.loopSteps) && m.loopStepSel > 0 {
					m.loopStepSel--
				}
			}
			return m, nil
		case "J": // Shift+J: move step down
			if len(m.loopSteps) > 1 && m.loopStepSel < len(m.loopSteps)-1 {
				m.loopSteps[m.loopStepSel], m.loopSteps[m.loopStepSel+1] = m.loopSteps[m.loopStepSel+1], m.loopSteps[m.loopStepSel]
				m.loopStepSel++
			}
			return m, nil
		case "K": // Shift+K: move step up
			if len(m.loopSteps) > 1 && m.loopStepSel > 0 {
				m.loopSteps[m.loopStepSel], m.loopSteps[m.loopStepSel-1] = m.loopSteps[m.loopStepSel-1], m.loopSteps[m.loopStepSel]
				m.loopStepSel--
			}
			return m, nil
		case "s":
			// Save.
			return m.finishLoopCreation()
		case "esc":
			if m.loopEditing {
				m.state = stateLoopMenu
			} else {
				m.state = stateLoopName
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepList() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Steps"))
	lines = append(lines, dimStyle.Render("Loop: "+m.loopNameInput))
	lines = append(lines, "")

	if len(m.loopSteps) == 0 {
		lines = append(lines, dimStyle.Render("No steps yet. Press 'a' to add one."))
	}
	for i, step := range m.loopSteps {
		turns := step.Turns
		if turns <= 0 {
			turns = 1
		}
		role := m.effectiveLoopStepRole(step)
		flags := ""
		if step.CanStop {
			flags += " [stop]"
		}
		if step.CanMessage {
			flags += " [msg]"
		}
		if step.CanPushover {
			flags += " [push]"
		}
		spawnCount := loopStepSpawnCount(step)
		spawnTag := " [no-spawn]"
		if spawnCount > 0 {
			spawnTag = fmt.Sprintf(" [spawn:%d]", spawnCount)
		}
		label := fmt.Sprintf("%d. %s %s x%d%s%s", i+1, step.Profile, roleBadge(role), turns, spawnTag, flags)
		if i == m.loopStepSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render(label)
			lines = append(lines, cursor+styled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("a: add  enter: edit  d: delete  J/K: reorder"))
	lines = append(lines, dimStyle.Render("s: save  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Profile ---

func (m AppModel) updateLoopStepProfile(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.loopStepProfileOpts) > 0 {
				m.loopStepProfileSel = (m.loopStepProfileSel + 1) % len(m.loopStepProfileOpts)
				m.resetLoopStepSpawnOptions()
				m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg))
			}
		case "k", "up":
			if len(m.loopStepProfileOpts) > 0 {
				m.loopStepProfileSel = (m.loopStepProfileSel - 1 + len(m.loopStepProfileOpts)) % len(m.loopStepProfileOpts)
				m.resetLoopStepSpawnOptions()
				m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg))
			}
		case "enter":
			if len(m.loopStepProfileOpts) > 0 {
				m.state = stateLoopStepRole
			}
		case "esc":
			m.state = stateLoopStepList
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepProfile() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Profile"))
	lines = append(lines, "")

	if len(m.loopStepProfileOpts) == 0 {
		lines = append(lines, dimStyle.Render("No profiles available. Create a profile first."))
	}
	for i, name := range m.loopStepProfileOpts {
		if i == m.loopStepProfileSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+styled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(name))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Role ---

func (m AppModel) updateLoopStepRole(msg tea.Msg) (tea.Model, tea.Cmd) {
	roles := config.AllRoles(m.globalCfg)
	if len(roles) == 0 {
		return m, nil
	}
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
			m.loopStepRoleSel = (m.loopStepRoleSel + 1) % len(roles)
			m.resetStateScroll()
		case "k", "up":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(-1)
				return m, nil
			}
			m.loopStepRoleSel = (m.loopStepRoleSel - 1 + len(roles)) % len(roles)
			m.resetStateScroll()
		case "enter":
			m.state = stateLoopStepTurns
		case "esc":
			m.state = stateLoopStepProfile
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepRole() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	roles := config.AllRoles(m.globalCfg)
	selectedRole := ""
	if len(roles) > 0 && m.loopStepRoleSel >= 0 && m.loopStepRoleSel < len(roles) {
		selectedRole = roles[m.loopStepRoleSel]
	}

	if m.width < 90 {
		style, cw, ch := profileWizardPanel(m)
		var lines []string
		cursorLine := -1
		lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Role"))
		lines = append(lines, "")
		for i, role := range roles {
			if i == m.loopStepRoleSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(role)
				lines = append(lines, cursor+styled)
				cursorLine = len(lines) - 1
			} else {
				lines = append(lines, "  "+textStyle.Render(role))
			}
		}

		if selectedRole != "" {
			lines = append(lines, "")
			lines = append(lines, sectionStyle.Render("Composed Prompt"))
			promptText := rolePromptPreview(selectedRole, m.globalCfg)
			if promptText == "" {
				lines = append(lines, dimStyle.Render("(empty prompt)"))
			} else {
				lines = appendMultiline(lines, promptText)
			}
		}

		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("j/k: navigate  enter: continue  esc: back"))
		content := fitLinesWithCursor(wrapRenderableLines(lines, cw), cw, ch, cursorLine)
		panel := style.Render(content)
		return header + "\n" + panel + "\n" + statusBar
	}

	panelH := m.height - 2
	if panelH < 1 {
		panelH = 1
	}
	leftOuter := m.width / 2
	minPane := 34
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

	var left []string
	cursorLine := -1
	leftTitle := m.loopWizardTitle() + " — Step Role"
	if !m.isRightPaneFocused() {
		leftTitle += " [focus]"
	}
	left = append(left, sectionStyle.Render(leftTitle))
	left = append(left, "")
	for i, role := range roles {
		if i == m.loopStepRoleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(role)
			left = append(left, cursor+styled)
			cursorLine = len(left) - 1
		} else {
			left = append(left, "  "+textStyle.Render(role))
		}
	}
	left = append(left, "")
	left = append(left, dimStyle.Render("j/k: navigate roles  enter: continue  esc: back"))
	left = append(left, dimStyle.Render("tab/h/l: pane focus"))

	var right []string
	rightTitle := "Role Prompt Preview"
	if m.isRightPaneFocused() {
		rightTitle += " [focus]"
	}
	right = append(right, sectionStyle.Render(rightTitle))
	right = append(right, "")
	if selectedRole == "" {
		right = append(right, dimStyle.Render("No role selected."))
	} else {
		right = append(right, textStyle.Render("Role: "+selectedRole))
		if m.globalCfg != nil {
			if def := m.globalCfg.FindRoleDefinition(selectedRole); def != nil {
				if title := strings.TrimSpace(def.Title); title != "" {
					right = append(right, textStyle.Render("Title: "+title))
				}
				right = append(right, "")
				if desc := strings.TrimSpace(def.Description); desc != "" {
					right = append(right, sectionStyle.Render("Description"))
					right = appendMultiline(right, desc)
					right = append(right, "")
				}
			}
		}
		right = append(right, sectionStyle.Render("Composed Prompt"))
		promptText := rolePromptPreview(selectedRole, m.globalCfg)
		if promptText == "" {
			right = append(right, dimStyle.Render("(empty prompt)"))
		} else {
			right = appendMultiline(right, promptText)
		}
		right = append(right, "")
		right = append(right, dimStyle.Render("pgup/pgdn or ctrl+u/ctrl+d to page"))
	}

	leftPanel := renderSettingsPaneWithCursor(left, leftOuter, panelH, cursorLine, !m.isRightPaneFocused())
	rightPanel := renderSettingsPaneWithOffset(right, rightOuter, panelH, m.stateScrollOffset(), m.isRightPaneFocused())
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel) + "\n" + statusBar
}

// --- Loop Step Turns ---

func (m AppModel) updateLoopStepTurns(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.state = stateLoopStepInstr
		case "esc":
			m.state = stateLoopStepRole
		case "backspace":
			if len(m.loopStepTurnsInput) > 0 {
				m.loopStepTurnsInput = m.loopStepTurnsInput[:len(m.loopStepTurnsInput)-1]
			}
		default:
			ch := msg.String()
			if len(ch) == 1 && ch >= "0" && ch <= "9" && len(m.loopStepTurnsInput) < 3 {
				m.loopStepTurnsInput += ch
			}
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepTurns() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Turns"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Number of turns for this step (0 = 1):"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.loopStepTurnsInput)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Instructions ---

func (m AppModel) updateLoopStepInstr(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.loopStepToolsSel = 0
			m.state = stateLoopStepTools
		case "esc":
			m.state = stateLoopStepTurns
		case "backspace":
			if len(m.loopStepInstrInput) > 0 {
				m.loopStepInstrInput = m.loopStepInstrInput[:len(m.loopStepInstrInput)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.loopStepInstrInput += msg.String()
			}
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepInstr() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Instructions"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Custom instructions for this step (or enter to skip):"))
	lines = append(lines, "")

	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")
	displayInstr := truncateInputForDisplay(m.loopStepInstrInput, cw-4)
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(displayInstr)
	lines = append(lines, "> "+inputText+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Tools (consolidated multi-select) ---

func (m AppModel) updateLoopStepTools(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.loopStepToolsSel = (m.loopStepToolsSel + 1) % loopStepToolCount
		case "k", "up":
			m.loopStepToolsSel = (m.loopStepToolsSel - 1 + loopStepToolCount) % loopStepToolCount
		case " ":
			switch m.loopStepToolsSel {
			case 0:
				m.loopStepCanStop = !m.loopStepCanStop
			case 1:
				m.loopStepCanMsg = !m.loopStepCanMsg
			case 2:
				m.loopStepCanPushover = !m.loopStepCanPushover
			}
			return m, nil
		case "enter":
			m.state = stateLoopStepSpawn
			return m, nil
		case "esc":
			m.state = stateLoopStepInstr
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepTools() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Tools"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Which tools should this step have? (space to toggle)"))
	lines = append(lines, "")

	type toolOption struct {
		label   string
		desc    string
		enabled bool
	}
	tools := []toolOption{
		{"Stop Loop", "Can signal the loop to stop", m.loopStepCanStop},
		{"Send Messages", "Can send messages to subsequent steps", m.loopStepCanMsg},
		{"Pushover Notify", "Can send push notifications to your device", m.loopStepCanPushover},
	}

	for i, tool := range tools {
		check := "[ ]"
		if tool.enabled {
			check = lipgloss.NewStyle().Foreground(ColorGreen).Render("[x]")
		}
		label := tool.label
		desc := dimStyle.Render(" — " + tool.desc)
		if i == m.loopStepToolsSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+check+" "+nameStyled+desc)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+check+" "+lipgloss.NewStyle().Foreground(ColorText).Render(label)+desc)
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: delegation  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Spawn Delegation ---

func (m AppModel) updateLoopStepSpawn(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			level := m.currentDelegationLevel()
			if len(*level) > 0 {
				m.loopStepSpawnSel = (m.loopStepSpawnSel + 1) % len(*level)
			}
		case "k", "up":
			level := m.currentDelegationLevel()
			if len(*level) > 0 {
				m.loopStepSpawnSel = (m.loopStepSpawnSel - 1 + len(*level)) % len(*level)
			}
		case "a":
			if len(m.loopStepSpawnOpts) == 0 {
				return m, nil
			}
			m.loopStepSpawnCfgSel = 0
			m.state = stateLoopStepSpawnCfg
			return m, nil
		case "d":
			level := m.currentDelegationLevel()
			m.clampDelegationSelection()
			if len(*level) == 0 {
				return m, nil
			}
			idx := m.loopStepSpawnSel
			*level = append((*level)[:idx], (*level)[idx+1:]...)
			m.clampDelegationSelection()
			return m, nil
		case "r":
			node := m.currentDelegationNode()
			if node == nil {
				return m, nil
			}
			node.Roles = ensureDelegationRoles(node.Roles, m.globalCfg)
			roles := config.AllRoles(m.globalCfg)
			m.loopStepSpawnRoleSel = 0
			if len(roles) > 0 {
				for i, role := range roles {
					if role == node.Roles[0] {
						m.loopStepSpawnRoleSel = i
						break
					}
				}
			}
			m.state = stateLoopStepSpawnRoles
			return m, nil
		case "s":
			node := m.currentDelegationNode()
			if node == nil {
				return m, nil
			}
			switch node.Speed {
			case "":
				node.Speed = "fast"
			case "fast":
				node.Speed = "medium"
			case "medium":
				node.Speed = "slow"
			default:
				node.Speed = ""
			}
			return m, nil
		case " ":
			node := m.currentDelegationNode()
			if node == nil {
				return m, nil
			}
			node.Handoff = !node.Handoff
			return m, nil
		case "enter":
			node := m.currentDelegationNode()
			if node == nil {
				if len(m.loopStepSpawnOpts) > 0 {
					m.loopStepSpawnCfgSel = 0
					m.state = stateLoopStepSpawnCfg
				}
				return m, nil
			}
			m.loopStepDelegPath = append(m.loopStepDelegPath, m.loopStepSpawnSel)
			m.loopStepSpawnSel = 0
			return m, nil
		case "S", "ctrl+s":
			return m.finishLoopStep()
		case "esc":
			if len(m.loopStepDelegPath) > 0 {
				parentSel := m.loopStepDelegPath[len(m.loopStepDelegPath)-1]
				m.loopStepDelegPath = m.loopStepDelegPath[:len(m.loopStepDelegPath)-1]
				m.loopStepSpawnSel = parentSel
				m.clampDelegationSelection()
				return m, nil
			}
			m.state = stateLoopStepTools
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) delegationPathLabel() string {
	parts := []string{"root"}
	level := m.loopStepDelegRoots
	for _, idx := range m.loopStepDelegPath {
		if idx < 0 || idx >= len(level) {
			break
		}
		node := level[idx]
		if node == nil {
			break
		}
		parts = append(parts, fmt.Sprintf("%s/%s", node.Profile, delegationRolesLabel(node.Roles, m.globalCfg)))
		level = node.Children
	}
	return strings.Join(parts, " > ")
}

func (m *AppModel) currentDelegationNode() *loopDelegationNode {
	level := m.currentDelegationLevel()
	m.clampDelegationSelection()
	if len(*level) == 0 {
		return nil
	}
	return (*level)[m.loopStepSpawnSel]
}

func (m AppModel) viewLoopStepSpawn() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Delegation Tree"))
	lines = append(lines, dimStyle.Render("Level: "+m.delegationPathLabel()))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Define spawn rules at this level. Enter opens children of selected node."))
	lines = append(lines, "")

	level := m.currentDelegationLevel()
	m.clampDelegationSelection()
	if len(m.loopStepSpawnOpts) == 0 {
		lines = append(lines, dimStyle.Render("No profiles available. Create a profile first."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("esc: back"))
		content := fitLines(lines, cw, ch)
		panel := style.Render(content)
		return header + "\n" + panel + "\n" + statusBar
	}
	if len(*level) == 0 {
		lines = append(lines, dimStyle.Render("No rules at this level. Press 'a' to add one."))
	}
	for i, node := range *level {
		if node == nil {
			continue
		}
		label := fmt.Sprintf("%s as %s", node.Profile, delegationRolesLabel(node.Roles, m.globalCfg))
		if node.Speed != "" {
			label += " speed=" + node.Speed
		}
		if node.Handoff {
			label += " handoff"
		}
		if len(node.Children) > 0 {
			label += fmt.Sprintf(" children=%d", len(node.Children))
		}
		if i == m.loopStepSpawnSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+nameStyled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("a: add  d: delete  r: roles  s: speed  space: handoff"))
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: open children  esc: up/back  S: save step"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

func (m AppModel) updateLoopStepSpawnRoles(msg tea.Msg) (tea.Model, tea.Cmd) {
	node := m.currentDelegationNode()
	roles := config.AllRoles(m.globalCfg)
	if node == nil || len(roles) == 0 {
		m.state = stateLoopStepSpawn
		return m, nil
	}
	node.Roles = ensureDelegationRoles(node.Roles, m.globalCfg)
	if m.loopStepSpawnRoleSel < 0 {
		m.loopStepSpawnRoleSel = 0
	}
	if m.loopStepSpawnRoleSel >= len(roles) {
		m.loopStepSpawnRoleSel = len(roles) - 1
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.loopStepSpawnRoleSel = (m.loopStepSpawnRoleSel + 1) % len(roles)
		case "k", "up":
			m.loopStepSpawnRoleSel = (m.loopStepSpawnRoleSel - 1 + len(roles)) % len(roles)
		case " ":
			selectedRole := roles[m.loopStepSpawnRoleSel]
			node.Roles = toggleDelegationRole(node.Roles, selectedRole, m.globalCfg)
			return m, nil
		case "enter", "esc":
			m.state = stateLoopStepSpawn
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepSpawnRoles() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Delegation Roles"))
	lines = append(lines, dimStyle.Render("Level: "+m.delegationPathLabel()))
	lines = append(lines, "")

	node := m.currentDelegationNode()
	roles := config.AllRoles(m.globalCfg)
	if node == nil || len(roles) == 0 {
		lines = append(lines, dimStyle.Render("No selected rule or no roles available."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("esc: back"))
		content := fitLines(lines, cw, ch)
		panel := style.Render(content)
		return header + "\n" + panel + "\n" + statusBar
	}

	node.Roles = ensureDelegationRoles(node.Roles, m.globalCfg)
	lines = append(lines, dimStyle.Render("Toggle roles for: "+node.Profile))
	lines = append(lines, dimStyle.Render("At least one role must remain selected."))
	lines = append(lines, "")

	for i, role := range roles {
		marker := "[ ]"
		if delegationRoleListContains(node.Roles, role) {
			marker = "[x]"
		}
		label := fmt.Sprintf("%s %s", marker, role)
		if i == m.loopStepSpawnRoleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+styled)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: done  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Spawn Add Rule ---

func (m AppModel) updateLoopStepSpawnCfg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.loopStepSpawnOpts) > 0 {
				m.loopStepSpawnCfgSel = (m.loopStepSpawnCfgSel + 1) % len(m.loopStepSpawnOpts)
			}
		case "k", "up":
			if len(m.loopStepSpawnOpts) > 0 {
				m.loopStepSpawnCfgSel = (m.loopStepSpawnCfgSel - 1 + len(m.loopStepSpawnOpts)) % len(m.loopStepSpawnOpts)
			}
		case "enter":
			if len(m.loopStepSpawnOpts) == 0 {
				m.state = stateLoopStepSpawn
				return m, nil
			}
			profile := m.loopStepSpawnOpts[m.loopStepSpawnCfgSel]
			level := m.currentDelegationLevel()
			defaultRole := config.DefaultRole(m.globalCfg)
			*level = append(*level, &loopDelegationNode{
				Profile: profile,
				Roles:   []string{defaultRole},
			})
			m.loopStepSpawnSel = len(*level) - 1
			m.state = stateLoopStepSpawn
			return m, nil
		case "esc":
			m.state = stateLoopStepSpawn
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) viewLoopStepSpawnCfg() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Add Delegation Rule"))
	lines = append(lines, dimStyle.Render("Level: "+m.delegationPathLabel()))
	lines = append(lines, "")

	if len(m.loopStepSpawnOpts) == 0 {
		lines = append(lines, dimStyle.Render("No profiles available. Create a profile first."))
	} else {
		defaultRole := config.DefaultRole(m.globalCfg)
		for i, name := range m.loopStepSpawnOpts {
			label := fmt.Sprintf("%s (default role: %s)", name, defaultRole)
			if i == m.loopStepSpawnCfgSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
				lines = append(lines, cursor+styled)
				cursorLine = len(lines) - 1
			} else {
				lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: add rule  esc: back"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// finishLoopStep saves the current step being edited and returns to step list.
func (m AppModel) finishLoopStep() (tea.Model, tea.Cmd) {
	profileName := ""
	if m.loopStepProfileSel < len(m.loopStepProfileOpts) {
		profileName = m.loopStepProfileOpts[m.loopStepProfileSel]
	}
	if profileName == "" {
		m.state = stateLoopStepProfile
		return m, nil
	}

	turns := 1
	if m.loopStepTurnsInput != "" {
		if v, err := strconv.Atoi(m.loopStepTurnsInput); err == nil && v > 0 {
			turns = v
		}
	}

	role := config.DefaultRole(m.globalCfg)
	roles := config.AllRoles(m.globalCfg)
	if m.loopStepRoleSel >= 0 && m.loopStepRoleSel < len(roles) {
		role = roles[m.loopStepRoleSel]
	}

	delegation := nodesToDelegationConfig(cloneDelegationNodes(m.loopStepDelegRoots), m.globalCfg)

	step := config.LoopStep{
		Profile:      profileName,
		Role:         role,
		Turns:        turns,
		Instructions: strings.TrimSpace(m.loopStepInstrInput),
		CanStop:      m.loopStepCanStop,
		CanMessage:   m.loopStepCanMsg,
		CanPushover:  m.loopStepCanPushover,
		Delegation:   delegation,
	}

	if m.loopStepEditIdx >= 0 && m.loopStepEditIdx < len(m.loopSteps) {
		m.loopSteps[m.loopStepEditIdx] = step
	} else {
		m.loopSteps = append(m.loopSteps, step)
		m.loopStepSel = len(m.loopSteps) - 1
	}

	m.state = stateLoopStepList
	return m, nil
}

// --- Loop Menu (Edit) ---

func (m AppModel) updateLoopMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.loopMenuSel = (m.loopMenuSel + 1) % loopMenuItemCount
		case "k", "up":
			m.loopMenuSel = (m.loopMenuSel - 1 + loopMenuItemCount) % loopMenuItemCount
		case "esc":
			m.loopEditing = false
			m.state = stateSelector
		case "enter":
			switch m.loopMenuSel {
			case 0: // Name
				m.state = stateLoopName
			case 1: // Steps
				m.loopStepSel = 0
				m.state = stateLoopStepList
			case 2: // Save
				return m.finishLoopCreation()
			}
		}
	}
	return m, nil
}

func (m AppModel) viewLoopMenu() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render("Edit Loop"))
	lines = append(lines, "")

	type menuItem struct {
		label, value string
	}
	items := []menuItem{
		{"Name", m.loopNameInput},
		{"Steps", fmt.Sprintf("%d steps", len(m.loopSteps))},
	}

	for i, item := range items {
		line := labelStyle.Render(item.label) + valueStyle.Render(item.value)
		if i == m.loopMenuSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			lines = append(lines, cursor+line)
			cursorLine = len(lines) - 1
		} else {
			lines = append(lines, "  "+line)
		}
	}

	// Save item
	lines = append(lines, "")
	saveLabel := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("Save")
	if m.loopMenuSel == 2 {
		cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
		lines = append(lines, cursor+saveLabel)
		cursorLine = len(lines) - 1
	} else {
		lines = append(lines, "  "+saveLabel)
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: edit field  esc: cancel"))

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// finishLoopCreation saves the loop definition and returns to the selector.
func (m AppModel) finishLoopCreation() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.loopNameInput)
	if name == "" || len(m.loopSteps) == 0 {
		return m, nil
	}

	loopDef := config.LoopDef{
		Name:  name,
		Steps: make([]config.LoopStep, len(m.loopSteps)),
	}
	copy(loopDef.Steps, m.loopSteps)

	if m.loopEditing {
		m.globalCfg.RemoveLoop(m.loopEditName)
	}

	m.globalCfg.AddLoop(loopDef)
	config.Save(m.globalCfg)
	m.rebuildProfiles()

	// Select the loop in the list.
	for i, pe := range m.profiles {
		if pe.IsLoop && strings.EqualFold(pe.LoopName, name) {
			m.selected = i
			break
		}
	}

	m.loopEditing = false
	m.loopEditName = ""
	m.loopNameInput = ""
	m.loopSteps = nil
	m.state = stateSelector
	return m, nil
}
