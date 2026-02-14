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

type LoopWizardState struct {
	Editing          bool
	EditName         string
	NameInput        string
	Steps            []config.LoopStep
	StepSel          int
	StepEditIdx      int
	StepProfileOpts  []string
	StepProfileSel   int
	StepRoleSel      int
	StepTurnsInput   string
	StepInstrInput   string
	StepCanStop      bool
	StepCanMsg       bool
	StepCanPushover  bool
	StepToolsSel     int
	StepSpawnOpts    []string
	StepSpawnSel     int
	StepSpawnCfgSel  int
	StepSpawnRoleSel int
	StepDelegRoots   []*loopDelegationNode
	StepDelegPath    []int
	MenuSel          int
}

// loopWizardTitle returns "Edit Loop" or "New Loop" depending on mode.
func (m AppModel) loopWizardTitle() string {
	if m.loopWiz.Editing {
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
	m.loopWiz.StepSpawnOpts = nil
	m.loopWiz.StepSpawnSel = 0
	m.loopWiz.StepSpawnCfgSel = 0
	m.loopWiz.StepDelegRoots = nil
	m.loopWiz.StepDelegPath = nil
	for _, p := range m.globalCfg.Profiles {
		m.loopWiz.StepSpawnOpts = append(m.loopWiz.StepSpawnOpts, p.Name)
	}
}

func (m *AppModel) preselectLoopStepSpawn(step config.LoopStep) {
	m.loopWiz.StepDelegRoots = delegationConfigToNodes(step.Delegation, m.globalCfg)
	m.loopWiz.StepDelegPath = nil
	m.loopWiz.StepSpawnSel = 0
	m.loopWiz.StepSpawnRoleSel = 0
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
	level := &m.loopWiz.StepDelegRoots
	for _, idx := range m.loopWiz.StepDelegPath {
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
		m.loopWiz.StepSpawnSel = 0
		return
	}
	if m.loopWiz.StepSpawnSel < 0 {
		m.loopWiz.StepSpawnSel = 0
	}
	if m.loopWiz.StepSpawnSel >= len(*level) {
		m.loopWiz.StepSpawnSel = len(*level) - 1
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
	m.loopWiz.StepRoleSel = defaultLoopStepRoleSel(m.globalCfg)
	for i, candidate := range config.AllRoles(m.globalCfg) {
		if candidate == role {
			m.loopWiz.StepRoleSel = i
			return
		}
	}
}

// --- Loop Name ---

func (m AppModel) updateLoopName(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("loop-name", m.loopWiz.NameInput, 0)
	m.syncTextInput(m.loopWiz.NameInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			name := strings.TrimSpace(m.loopWiz.NameInput)
			if name == "" {
				return m, nil
			}
			if existing := m.globalCfg.FindLoop(name); existing != nil {
				if !m.loopWiz.Editing || !strings.EqualFold(name, m.loopWiz.EditName) {
					return m, nil // duplicate
				}
			}
			m.loopWiz.NameInput = name
			if m.loopWiz.Editing {
				m.state = stateLoopMenu
				return m, nil
			}
			// New loop: go to step list.
			m.loopWiz.Steps = nil
			m.loopWiz.StepSel = 0
			m.state = stateLoopStepList
			return m, nil
		case tea.KeyEsc:
			if m.loopWiz.Editing {
				m.state = stateLoopMenu
				return m, nil
			}
			m.state = stateSelector
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.loopWiz.NameInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewLoopName() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("loop-name", m.loopWiz.NameInput, 0)
	m.syncTextInput(m.loopWiz.NameInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Name"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter a name for the loop:"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
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
			if len(m.loopWiz.Steps) > 0 {
				m.loopWiz.StepSel = (m.loopWiz.StepSel + 1) % len(m.loopWiz.Steps)
			}
		case "k", "up":
			if len(m.loopWiz.Steps) > 0 {
				m.loopWiz.StepSel = (m.loopWiz.StepSel - 1 + len(m.loopWiz.Steps)) % len(m.loopWiz.Steps)
			}
		case "a":
			// Add new step.
			m.loopWiz.StepEditIdx = -1
			m.loopWiz.StepProfileOpts = nil
			for _, p := range m.globalCfg.Profiles {
				m.loopWiz.StepProfileOpts = append(m.loopWiz.StepProfileOpts, p.Name)
			}
			m.loopWiz.StepProfileSel = 0
			m.loopWiz.StepTurnsInput = "1"
			m.loopWiz.StepInstrInput = ""
			m.loopWiz.StepCanStop = false
			m.loopWiz.StepCanMsg = false
			m.loopWiz.StepCanPushover = false
			m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg))
			m.resetLoopStepSpawnOptions()
			m.loopWiz.StepToolsSel = 0
			m.state = stateLoopStepProfile
			return m, nil
		case "enter":
			// Edit selected step.
			if len(m.loopWiz.Steps) > 0 && m.loopWiz.StepSel < len(m.loopWiz.Steps) {
				m.loopWiz.StepEditIdx = m.loopWiz.StepSel
				step := m.loopWiz.Steps[m.loopWiz.StepSel]
				m.loopWiz.StepProfileOpts = nil
				for _, p := range m.globalCfg.Profiles {
					m.loopWiz.StepProfileOpts = append(m.loopWiz.StepProfileOpts, p.Name)
				}
				m.loopWiz.StepProfileSel = 0
				for i, name := range m.loopWiz.StepProfileOpts {
					if strings.EqualFold(name, step.Profile) {
						m.loopWiz.StepProfileSel = i
						break
					}
				}
				effectiveRole := m.effectiveLoopStepRole(step)
				m.setLoopStepRoleSelection(effectiveRole)
				turns := step.Turns
				if turns <= 0 {
					turns = 1
				}
				m.loopWiz.StepTurnsInput = strconv.Itoa(turns)
				m.loopWiz.StepInstrInput = step.Instructions
				m.loopWiz.StepCanStop = step.CanStop
				m.loopWiz.StepCanMsg = step.CanMessage
				m.loopWiz.StepCanPushover = step.CanPushover
				m.resetLoopStepSpawnOptions()
				m.preselectLoopStepSpawn(step)
				m.loopWiz.StepToolsSel = 0
				m.state = stateLoopStepProfile
			}
			return m, nil
		case "d":
			// Delete selected step.
			if len(m.loopWiz.Steps) > 0 && m.loopWiz.StepSel < len(m.loopWiz.Steps) {
				m.loopWiz.Steps = append(m.loopWiz.Steps[:m.loopWiz.StepSel], m.loopWiz.Steps[m.loopWiz.StepSel+1:]...)
				if m.loopWiz.StepSel >= len(m.loopWiz.Steps) && m.loopWiz.StepSel > 0 {
					m.loopWiz.StepSel--
				}
			}
			return m, nil
		case "J": // Shift+J: move step down
			if len(m.loopWiz.Steps) > 1 && m.loopWiz.StepSel < len(m.loopWiz.Steps)-1 {
				m.loopWiz.Steps[m.loopWiz.StepSel], m.loopWiz.Steps[m.loopWiz.StepSel+1] = m.loopWiz.Steps[m.loopWiz.StepSel+1], m.loopWiz.Steps[m.loopWiz.StepSel]
				m.loopWiz.StepSel++
			}
			return m, nil
		case "K": // Shift+K: move step up
			if len(m.loopWiz.Steps) > 1 && m.loopWiz.StepSel > 0 {
				m.loopWiz.Steps[m.loopWiz.StepSel], m.loopWiz.Steps[m.loopWiz.StepSel-1] = m.loopWiz.Steps[m.loopWiz.StepSel-1], m.loopWiz.Steps[m.loopWiz.StepSel]
				m.loopWiz.StepSel--
			}
			return m, nil
		case "s":
			// Save.
			return m.finishLoopCreation()
		case "esc":
			if m.loopWiz.Editing {
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
	lines = append(lines, dimStyle.Render("Loop: "+m.loopWiz.NameInput))
	lines = append(lines, "")

	if len(m.loopWiz.Steps) == 0 {
		lines = append(lines, dimStyle.Render("No steps yet. Press 'a' to add one."))
	}
	for i, step := range m.loopWiz.Steps {
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
		if i == m.loopWiz.StepSel {
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
			if len(m.loopWiz.StepProfileOpts) > 0 {
				m.loopWiz.StepProfileSel = (m.loopWiz.StepProfileSel + 1) % len(m.loopWiz.StepProfileOpts)
				m.resetLoopStepSpawnOptions()
				m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg))
			}
		case "k", "up":
			if len(m.loopWiz.StepProfileOpts) > 0 {
				m.loopWiz.StepProfileSel = (m.loopWiz.StepProfileSel - 1 + len(m.loopWiz.StepProfileOpts)) % len(m.loopWiz.StepProfileOpts)
				m.resetLoopStepSpawnOptions()
				m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg))
			}
		case "enter":
			if len(m.loopWiz.StepProfileOpts) > 0 {
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

	if len(m.loopWiz.StepProfileOpts) == 0 {
		lines = append(lines, dimStyle.Render("No profiles available. Create a profile first."))
	}
	for i, name := range m.loopWiz.StepProfileOpts {
		if i == m.loopWiz.StepProfileSel {
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
			m.loopWiz.StepRoleSel = (m.loopWiz.StepRoleSel + 1) % len(roles)
			m.resetStateScroll()
		case "k", "up":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(-1)
				return m, nil
			}
			m.loopWiz.StepRoleSel = (m.loopWiz.StepRoleSel - 1 + len(roles)) % len(roles)
			m.resetStateScroll()
		case "enter":
			m.state = stateLoopStepTurns
		case "esc":
			m.state = stateLoopStepProfile
		}
	}
	return m, nil
}

func selectedRoleAt(roles []string, idx int) string {
	if len(roles) == 0 || idx < 0 || idx >= len(roles) {
		return ""
	}
	return roles[idx]
}

func rolePromptPreviewLines(selectedRole string, globalCfg *config.GlobalConfig, sectionStyle, dimStyle, textStyle lipgloss.Style) []string {
	if selectedRole == "" {
		return []string{dimStyle.Render("No role selected.")}
	}

	lines := []string{textStyle.Render("Role: " + selectedRole)}
	if globalCfg != nil {
		if def := globalCfg.FindRoleDefinition(selectedRole); def != nil {
			if title := strings.TrimSpace(def.Title); title != "" {
				lines = append(lines, textStyle.Render("Title: "+title))
			}
			lines = append(lines, "")
			if desc := strings.TrimSpace(def.Description); desc != "" {
				lines = append(lines, sectionStyle.Render("Description"))
				lines = appendMultiline(lines, desc)
				lines = append(lines, "")
			}
		}
	}

	lines = append(lines, sectionStyle.Render("Composed Prompt"))
	promptText := rolePromptPreview(selectedRole, globalCfg)
	if promptText == "" {
		lines = append(lines, dimStyle.Render("(empty prompt)"))
	} else {
		lines = appendMultiline(lines, promptText)
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("pgup/pgdn or ctrl+u/ctrl+d to page"))
	return lines
}

func (m AppModel) renderLoopRoleSplitView(leftLines, rightLines []string, leftCursorLine int) string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()

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

	leftPanel := renderSettingsPaneWithCursor(leftLines, leftOuter, panelH, leftCursorLine, !m.isRightPaneFocused())
	rightPanel := renderSettingsPaneWithOffset(rightLines, rightOuter, panelH, m.stateScrollOffset(), m.isRightPaneFocused())
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel) + "\n" + statusBar
}

func (m AppModel) viewLoopStepRole() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	roles := config.AllRoles(m.globalCfg)
	selectedRole := selectedRoleAt(roles, m.loopWiz.StepRoleSel)

	if m.width < 90 {
		style, cw, ch := profileWizardPanel(m)
		var lines []string
		cursorLine := -1
		lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Role"))
		lines = append(lines, "")
		for i, role := range roles {
			if i == m.loopWiz.StepRoleSel {
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

	var left []string
	cursorLine := -1
	leftTitle := m.loopWizardTitle() + " — Step Role"
	if !m.isRightPaneFocused() {
		leftTitle += " [focus]"
	}
	left = append(left, sectionStyle.Render(leftTitle))
	left = append(left, "")
	for i, role := range roles {
		if i == m.loopWiz.StepRoleSel {
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
	right = append(right, rolePromptPreviewLines(selectedRole, m.globalCfg, sectionStyle, dimStyle, textStyle)...)

	return m.renderLoopRoleSplitView(left, right, cursorLine)
}

// --- Loop Step Turns ---

func (m AppModel) updateLoopStepTurns(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("loop-step-turns", m.loopWiz.StepTurnsInput, 3)
	m.syncTextInput(m.loopWiz.StepTurnsInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.state = stateLoopStepInstr
			return m, nil
		case tea.KeyEsc:
			m.state = stateLoopStepRole
			return m, nil
		}
	}
	filteredMsg, ok := sanitizeDigitsMsg(msg)
	if !ok {
		return m, initCmd
	}
	cmd := m.updateTextInput(filteredMsg)
	filtered := digitsOnly(m.textInput.Value(), 3)
	if filtered != m.textInput.Value() {
		m.textInput.SetValue(filtered)
		m.textInput.CursorEnd()
	}
	m.loopWiz.StepTurnsInput = filtered
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewLoopStepTurns() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("loop-step-turns", m.loopWiz.StepTurnsInput, 3)
	m.syncTextInput(m.loopWiz.StepTurnsInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Turns"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Number of turns for this step (0 = 1):"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Instructions ---

func (m AppModel) updateLoopStepInstr(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("loop-step-instr", m.loopWiz.StepInstrInput, 0)
	m.syncTextInput(m.loopWiz.StepInstrInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			m.loopWiz.StepToolsSel = 0
			m.state = stateLoopStepTools
			return m, nil
		case tea.KeyEsc:
			m.state = stateLoopStepTurns
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.loopWiz.StepInstrInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) viewLoopStepInstr() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	m.ensureTextInput("loop-step-instr", m.loopWiz.StepInstrInput, 0)
	m.syncTextInput(m.loopWiz.StepInstrInput)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Instructions"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Custom instructions for this step (or enter to skip):"))
	lines = append(lines, "")

	lines = append(lines, m.viewTextInput(cw-4))
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
			m.loopWiz.StepToolsSel = (m.loopWiz.StepToolsSel + 1) % loopStepToolCount
		case "k", "up":
			m.loopWiz.StepToolsSel = (m.loopWiz.StepToolsSel - 1 + loopStepToolCount) % loopStepToolCount
		case " ":
			switch m.loopWiz.StepToolsSel {
			case 0:
				m.loopWiz.StepCanStop = !m.loopWiz.StepCanStop
			case 1:
				m.loopWiz.StepCanMsg = !m.loopWiz.StepCanMsg
			case 2:
				m.loopWiz.StepCanPushover = !m.loopWiz.StepCanPushover
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
		{"Stop Loop", "Can signal the loop to stop", m.loopWiz.StepCanStop},
		{"Send Messages", "Can send messages to subsequent steps", m.loopWiz.StepCanMsg},
		{"Pushover Notify", "Can send push notifications to your device", m.loopWiz.StepCanPushover},
	}

	for i, tool := range tools {
		check := "[ ]"
		if tool.enabled {
			check = lipgloss.NewStyle().Foreground(ColorGreen).Render("[x]")
		}
		label := tool.label
		desc := dimStyle.Render(" — " + tool.desc)
		if i == m.loopWiz.StepToolsSel {
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
				m.loopWiz.StepSpawnSel = (m.loopWiz.StepSpawnSel + 1) % len(*level)
			}
		case "k", "up":
			level := m.currentDelegationLevel()
			if len(*level) > 0 {
				m.loopWiz.StepSpawnSel = (m.loopWiz.StepSpawnSel - 1 + len(*level)) % len(*level)
			}
		case "a":
			if len(m.loopWiz.StepSpawnOpts) == 0 {
				return m, nil
			}
			m.loopWiz.StepSpawnCfgSel = 0
			m.state = stateLoopStepSpawnCfg
			return m, nil
		case "d":
			level := m.currentDelegationLevel()
			m.clampDelegationSelection()
			if len(*level) == 0 {
				return m, nil
			}
			idx := m.loopWiz.StepSpawnSel
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
			m.loopWiz.StepSpawnRoleSel = 0
			if len(roles) > 0 {
				for i, role := range roles {
					if role == node.Roles[0] {
						m.loopWiz.StepSpawnRoleSel = i
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
				if len(m.loopWiz.StepSpawnOpts) > 0 {
					m.loopWiz.StepSpawnCfgSel = 0
					m.state = stateLoopStepSpawnCfg
				}
				return m, nil
			}
			m.loopWiz.StepDelegPath = append(m.loopWiz.StepDelegPath, m.loopWiz.StepSpawnSel)
			m.loopWiz.StepSpawnSel = 0
			return m, nil
		case "S", "ctrl+s":
			return m.finishLoopStep()
		case "esc":
			if len(m.loopWiz.StepDelegPath) > 0 {
				parentSel := m.loopWiz.StepDelegPath[len(m.loopWiz.StepDelegPath)-1]
				m.loopWiz.StepDelegPath = m.loopWiz.StepDelegPath[:len(m.loopWiz.StepDelegPath)-1]
				m.loopWiz.StepSpawnSel = parentSel
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
	level := m.loopWiz.StepDelegRoots
	for _, idx := range m.loopWiz.StepDelegPath {
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
	return (*level)[m.loopWiz.StepSpawnSel]
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
	if len(m.loopWiz.StepSpawnOpts) == 0 {
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
		if i == m.loopWiz.StepSpawnSel {
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
	if m.loopWiz.StepSpawnRoleSel < 0 {
		m.loopWiz.StepSpawnRoleSel = 0
	}
	if m.loopWiz.StepSpawnRoleSel >= len(roles) {
		m.loopWiz.StepSpawnRoleSel = len(roles) - 1
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
			m.loopWiz.StepSpawnRoleSel = (m.loopWiz.StepSpawnRoleSel + 1) % len(roles)
			m.resetStateScroll()
		case "k", "up":
			if m.isRightPaneFocused() {
				m.adjustStateScroll(-1)
				return m, nil
			}
			m.loopWiz.StepSpawnRoleSel = (m.loopWiz.StepSpawnRoleSel - 1 + len(roles)) % len(roles)
			m.resetStateScroll()
		case " ":
			selectedRole := roles[m.loopWiz.StepSpawnRoleSel]
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

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)

	node := m.currentDelegationNode()
	roles := config.AllRoles(m.globalCfg)
	if node == nil || len(roles) == 0 {
		style, cw, ch := profileWizardPanel(m)
		var lines []string
		lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Delegation Roles"))
		lines = append(lines, dimStyle.Render("Level: "+m.delegationPathLabel()))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("No selected rule or no roles available."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("esc: back"))
		content := fitLines(lines, cw, ch)
		panel := style.Render(content)
		return header + "\n" + panel + "\n" + statusBar
	}

	node.Roles = ensureDelegationRoles(node.Roles, m.globalCfg)
	selectedRole := selectedRoleAt(roles, m.loopWiz.StepSpawnRoleSel)

	if m.width < 90 {
		style, cw, ch := profileWizardPanel(m)
		var lines []string
		cursorLine := -1
		lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Delegation Roles"))
		lines = append(lines, dimStyle.Render("Level: "+m.delegationPathLabel()))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Toggle roles for: "+node.Profile))
		lines = append(lines, dimStyle.Render("At least one role must remain selected."))
		lines = append(lines, "")

		for i, role := range roles {
			marker := "[ ]"
			if delegationRoleListContains(node.Roles, role) {
				marker = "[x]"
			}
			label := fmt.Sprintf("%s %s", marker, role)
			if i == m.loopWiz.StepSpawnRoleSel {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
				styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
				lines = append(lines, cursor+styled)
				cursorLine = len(lines) - 1
			} else {
				lines = append(lines, "  "+textStyle.Render(label))
			}
		}

		lines = append(lines, "")
		lines = append(lines, sectionStyle.Render("Role Prompt Preview"))
		lines = append(lines, textStyle.Render("Rule: "+node.Profile))
		lines = append(lines, "")
		lines = append(lines, rolePromptPreviewLines(selectedRole, m.globalCfg, sectionStyle, dimStyle, textStyle)...)
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: done  esc: back"))

		content := fitLinesWithCursor(wrapRenderableLines(lines, cw), cw, ch, cursorLine)
		panel := style.Render(content)
		return header + "\n" + panel + "\n" + statusBar
	}

	var left []string
	cursorLine := -1
	leftTitle := m.loopWizardTitle() + " — Delegation Roles"
	if !m.isRightPaneFocused() {
		leftTitle += " [focus]"
	}
	left = append(left, sectionStyle.Render(leftTitle))
	left = append(left, dimStyle.Render("Level: "+m.delegationPathLabel()))
	left = append(left, "")
	left = append(left, dimStyle.Render("Toggle roles for: "+node.Profile))
	left = append(left, dimStyle.Render("At least one role must remain selected."))
	left = append(left, "")

	for i, role := range roles {
		marker := "[ ]"
		if delegationRoleListContains(node.Roles, role) {
			marker = "[x]"
		}
		label := fmt.Sprintf("%s %s", marker, role)
		if i == m.loopWiz.StepSpawnRoleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			left = append(left, cursor+styled)
			cursorLine = len(left) - 1
		} else {
			left = append(left, "  "+textStyle.Render(label))
		}
	}

	left = append(left, "")
	left = append(left, dimStyle.Render("space: toggle  j/k: navigate  enter: done  esc: back"))
	left = append(left, dimStyle.Render("tab/h/l: pane focus"))

	var right []string
	rightTitle := "Role Prompt Preview"
	if m.isRightPaneFocused() {
		rightTitle += " [focus]"
	}
	right = append(right, sectionStyle.Render(rightTitle))
	right = append(right, "")
	right = append(right, textStyle.Render("Rule: "+node.Profile))
	right = append(right, "")
	right = append(right, rolePromptPreviewLines(selectedRole, m.globalCfg, sectionStyle, dimStyle, textStyle)...)

	return m.renderLoopRoleSplitView(left, right, cursorLine)
}

// --- Loop Step Spawn Add Rule ---

func (m AppModel) updateLoopStepSpawnCfg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.loopWiz.StepSpawnOpts) > 0 {
				m.loopWiz.StepSpawnCfgSel = (m.loopWiz.StepSpawnCfgSel + 1) % len(m.loopWiz.StepSpawnOpts)
			}
		case "k", "up":
			if len(m.loopWiz.StepSpawnOpts) > 0 {
				m.loopWiz.StepSpawnCfgSel = (m.loopWiz.StepSpawnCfgSel - 1 + len(m.loopWiz.StepSpawnOpts)) % len(m.loopWiz.StepSpawnOpts)
			}
		case "enter":
			if len(m.loopWiz.StepSpawnOpts) == 0 {
				m.state = stateLoopStepSpawn
				return m, nil
			}
			profile := m.loopWiz.StepSpawnOpts[m.loopWiz.StepSpawnCfgSel]
			level := m.currentDelegationLevel()
			defaultRole := config.DefaultRole(m.globalCfg)
			*level = append(*level, &loopDelegationNode{
				Profile: profile,
				Roles:   []string{defaultRole},
			})
			m.loopWiz.StepSpawnSel = len(*level) - 1
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

	if len(m.loopWiz.StepSpawnOpts) == 0 {
		lines = append(lines, dimStyle.Render("No profiles available. Create a profile first."))
	} else {
		defaultRole := config.DefaultRole(m.globalCfg)
		for i, name := range m.loopWiz.StepSpawnOpts {
			label := fmt.Sprintf("%s (default role: %s)", name, defaultRole)
			if i == m.loopWiz.StepSpawnCfgSel {
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
	if m.loopWiz.StepProfileSel < len(m.loopWiz.StepProfileOpts) {
		profileName = m.loopWiz.StepProfileOpts[m.loopWiz.StepProfileSel]
	}
	if profileName == "" {
		m.state = stateLoopStepProfile
		return m, nil
	}

	turns := 1
	if m.loopWiz.StepTurnsInput != "" {
		if v, err := strconv.Atoi(m.loopWiz.StepTurnsInput); err == nil && v > 0 {
			turns = v
		}
	}

	role := config.DefaultRole(m.globalCfg)
	roles := config.AllRoles(m.globalCfg)
	if m.loopWiz.StepRoleSel >= 0 && m.loopWiz.StepRoleSel < len(roles) {
		role = roles[m.loopWiz.StepRoleSel]
	}

	delegation := nodesToDelegationConfig(cloneDelegationNodes(m.loopWiz.StepDelegRoots), m.globalCfg)

	step := config.LoopStep{
		Profile:      profileName,
		Role:         role,
		Turns:        turns,
		Instructions: strings.TrimSpace(m.loopWiz.StepInstrInput),
		CanStop:      m.loopWiz.StepCanStop,
		CanMessage:   m.loopWiz.StepCanMsg,
		CanPushover:  m.loopWiz.StepCanPushover,
		Delegation:   delegation,
	}

	if m.loopWiz.StepEditIdx >= 0 && m.loopWiz.StepEditIdx < len(m.loopWiz.Steps) {
		m.loopWiz.Steps[m.loopWiz.StepEditIdx] = step
	} else {
		m.loopWiz.Steps = append(m.loopWiz.Steps, step)
		m.loopWiz.StepSel = len(m.loopWiz.Steps) - 1
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
			m.loopWiz.MenuSel = (m.loopWiz.MenuSel + 1) % loopMenuItemCount
		case "k", "up":
			m.loopWiz.MenuSel = (m.loopWiz.MenuSel - 1 + loopMenuItemCount) % loopMenuItemCount
		case "esc":
			m.loopWiz.Editing = false
			m.state = stateSelector
		case "enter":
			switch m.loopWiz.MenuSel {
			case 0: // Name
				m.state = stateLoopName
			case 1: // Steps
				m.loopWiz.StepSel = 0
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
		{"Name", m.loopWiz.NameInput},
		{"Steps", fmt.Sprintf("%d steps", len(m.loopWiz.Steps))},
	}

	for i, item := range items {
		line := labelStyle.Render(item.label) + valueStyle.Render(item.value)
		if i == m.loopWiz.MenuSel {
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
	if m.loopWiz.MenuSel == 2 {
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
	name := strings.TrimSpace(m.loopWiz.NameInput)
	if name == "" || len(m.loopWiz.Steps) == 0 {
		return m, nil
	}

	loopDef := config.LoopDef{
		Name:  name,
		Steps: make([]config.LoopStep, len(m.loopWiz.Steps)),
	}
	copy(loopDef.Steps, m.loopWiz.Steps)

	if m.loopWiz.Editing {
		m.globalCfg.RemoveLoop(m.loopWiz.EditName)
	}

	m.globalCfg.AddLoop(loopDef)
	config.Save(m.globalCfg)
	m.rebuildProfiles()

	// Select the loop in the list.
	for i, pe := range m.profiles {
		if pe.IsLoop && strings.EqualFold(pe.LoopName, name) {
			m.selector.Selected = i
			break
		}
	}

	m.loopWiz.Editing = false
	m.loopWiz.EditName = ""
	m.loopWiz.NameInput = ""
	m.loopWiz.Steps = nil
	m.state = stateSelector
	return m, nil
}
