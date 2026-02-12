package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/config"
)

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
	prof := m.globalCfg.FindProfile(step.Profile)
	return config.EffectiveStepRole(step.Role, prof)
}

func (m *AppModel) resetLoopStepSpawnOptions() {
	m.loopStepSpawnOpts = nil
	m.loopStepSpawnSelect = make(map[int]bool)
	m.loopStepSpawnSel = 0
	m.loopStepSpawnCfgSel = 0
	m.loopStepSpawnSpeed = make(map[int]int)
	m.loopStepSpawnHandoff = make(map[int]bool)
	for _, p := range m.globalCfg.Profiles {
		m.loopStepSpawnOpts = append(m.loopStepSpawnOpts, p.Name)
	}
}

func (m *AppModel) preselectLoopStepSpawn(step config.LoopStep) {
	for i, name := range m.loopStepSpawnOpts {
		if step.Delegation != nil && step.Delegation.HasProfile(name) {
			m.loopStepSpawnSelect[i] = true
			if dp := step.Delegation.FindProfile(name); dp != nil {
				switch dp.Speed {
				case "fast":
					m.loopStepSpawnSpeed[i] = 1
				case "medium":
					m.loopStepSpawnSpeed[i] = 2
				case "slow":
					m.loopStepSpawnSpeed[i] = 3
				}
				m.loopStepSpawnHandoff[i] = dp.Handoff
			}
			continue
		}
		// Backward compatibility: pre-populate from legacy profile settings
		// when the step has no explicit delegation.
		if step.Delegation == nil {
			if prof := m.globalCfg.FindProfile(step.Profile); prof != nil {
				for _, sp := range prof.SpawnableProfiles {
					if strings.EqualFold(sp, name) {
						m.loopStepSpawnSelect[i] = true
						break
					}
				}
			}
		}
	}
}

func loopStepSpawnCount(step config.LoopStep) int {
	if step.Delegation == nil {
		return 0
	}
	return len(step.Delegation.Profiles)
}

func defaultLoopStepRoleSel() int {
	roles := config.AllRoles()
	for i, role := range roles {
		if role == config.RoleJunior {
			return i
		}
	}
	return 0
}

func (m *AppModel) setLoopStepRoleSelection(role string) {
	m.loopStepRoleSel = defaultLoopStepRoleSel()
	for i, candidate := range config.AllRoles() {
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
			selectedProfile := ""
			if len(m.loopStepProfileOpts) > 0 {
				selectedProfile = m.loopStepProfileOpts[m.loopStepProfileSel]
			}
			m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg.FindProfile(selectedProfile)))
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
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("a: add  enter: edit  d: delete  J/K: reorder"))
	lines = append(lines, dimStyle.Render("s: save  esc: back"))

	content := fitLines(lines, cw, ch)
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
				selected := m.loopStepProfileOpts[m.loopStepProfileSel]
				m.resetLoopStepSpawnOptions()
				m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg.FindProfile(selected)))
			}
		case "k", "up":
			if len(m.loopStepProfileOpts) > 0 {
				m.loopStepProfileSel = (m.loopStepProfileSel - 1 + len(m.loopStepProfileOpts)) % len(m.loopStepProfileOpts)
				selected := m.loopStepProfileOpts[m.loopStepProfileSel]
				m.resetLoopStepSpawnOptions()
				m.setLoopStepRoleSelection(config.EffectiveStepRole("", m.globalCfg.FindProfile(selected)))
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
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(name))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: select  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Role ---

func (m AppModel) updateLoopStepRole(msg tea.Msg) (tea.Model, tea.Cmd) {
	roles := config.AllRoles()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.loopStepRoleSel = (m.loopStepRoleSel + 1) % len(roles)
		case "k", "up":
			m.loopStepRoleSel = (m.loopStepRoleSel - 1 + len(roles)) % len(roles)
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
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	roleDescs := map[string]string{
		"manager":    "Planning/delegation focused; no direct coding",
		"senior":     "Lead coder; can also orchestrate work",
		"junior":     "Execution focused developer",
		"supervisor": "Review and guidance; no direct coding",
	}

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Role"))
	lines = append(lines, "")

	roles := config.AllRoles()
	for i, role := range roles {
		label := role
		if desc, ok := roleDescs[role]; ok {
			label = fmt.Sprintf("%-12s %s", role, dimStyle.Render(desc))
		}
		if i == m.loopStepRoleSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+styled)
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: continue  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
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
		} else {
			lines = append(lines, "  "+check+" "+lipgloss.NewStyle().Foreground(ColorText).Render(label)+desc)
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: delegation  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Spawn Delegation ---

func (m AppModel) updateLoopStepSpawn(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if len(m.loopStepSpawnOpts) > 0 {
				m.loopStepSpawnSel = (m.loopStepSpawnSel + 1) % len(m.loopStepSpawnOpts)
			}
		case "k", "up":
			if len(m.loopStepSpawnOpts) > 0 {
				m.loopStepSpawnSel = (m.loopStepSpawnSel - 1 + len(m.loopStepSpawnOpts)) % len(m.loopStepSpawnOpts)
			}
		case " ":
			if len(m.loopStepSpawnOpts) > 0 {
				m.loopStepSpawnSelect[m.loopStepSpawnSel] = !m.loopStepSpawnSelect[m.loopStepSpawnSel]
			}
		case "enter":
			if len(m.selectedSpawnIndices()) > 0 {
				m.loopStepSpawnCfgSel = 0
				m.state = stateLoopStepSpawnCfg
				return m, nil
			}
			return m.finishLoopStep()
		case "esc":
			m.state = stateLoopStepTools
		}
	}
	return m, nil
}

// selectedSpawnIndices returns the indices of selected spawn profiles.
func (m AppModel) selectedSpawnIndices() []int {
	var indices []int
	for i := range m.loopStepSpawnOpts {
		if m.loopStepSpawnSelect[i] {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m AppModel) viewLoopStepSpawn() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Step Delegation"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Select which profiles this step can spawn (space to toggle):"))
	lines = append(lines, dimStyle.Render("Leave all unchecked to disallow spawning."))
	lines = append(lines, "")

	if len(m.loopStepSpawnOpts) == 0 {
		lines = append(lines, dimStyle.Render("No profiles available. Create a profile first."))
	}
	for i, name := range m.loopStepSpawnOpts {
		check := "[ ]"
		if m.loopStepSpawnSelect[i] {
			check = lipgloss.NewStyle().Foreground(ColorGreen).Render("[x]")
		}
		if i == m.loopStepSpawnSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+check+" "+nameStyled)
		} else {
			lines = append(lines, "  "+check+" "+lipgloss.NewStyle().Foreground(ColorText).Render(name))
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: save step  esc: back"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// --- Loop Step Spawn Config (Speed/Handoff) ---

func (m AppModel) updateLoopStepSpawnCfg(msg tea.Msg) (tea.Model, tea.Cmd) {
	selected := m.selectedSpawnIndices()
	if len(selected) == 0 {
		return m.finishLoopStep()
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			m.loopStepSpawnCfgSel = (m.loopStepSpawnCfgSel + 1) % len(selected)
		case "k", "up":
			m.loopStepSpawnCfgSel = (m.loopStepSpawnCfgSel - 1 + len(selected)) % len(selected)
		case "s":
			idx := selected[m.loopStepSpawnCfgSel]
			cur := m.loopStepSpawnSpeed[idx]
			m.loopStepSpawnSpeed[idx] = (cur + 1) % 4
		case " ":
			idx := selected[m.loopStepSpawnCfgSel]
			m.loopStepSpawnHandoff[idx] = !m.loopStepSpawnHandoff[idx]
		case "enter":
			return m.finishLoopStep()
		case "esc":
			m.state = stateLoopStepSpawn
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
	lines = append(lines, sectionStyle.Render(m.loopWizardTitle()+" — Delegation Configure"))
	lines = append(lines, "")

	speedLabels := []string{"(none)", "fast", "medium", "slow"}
	selected := m.selectedSpawnIndices()
	for si, idx := range selected {
		name := m.loopStepSpawnOpts[idx]
		spd := m.loopStepSpawnSpeed[idx]
		speedStr := speedLabels[spd]
		handoffStr := "[ ]"
		if m.loopStepSpawnHandoff[idx] {
			handoffStr = lipgloss.NewStyle().Foreground(ColorGreen).Render("[x]")
		}
		label := fmt.Sprintf("%-20s Speed: %-8s Handoff: %s", name, speedStr, handoffStr)
		if si == m.loopStepSpawnCfgSel {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			styled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(label)
			lines = append(lines, cursor+styled)
		} else {
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(ColorText).Render(label))
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("s: cycle speed  space: toggle handoff  j/k: navigate"))
	lines = append(lines, dimStyle.Render("enter: save step  esc: back"))

	content := fitLines(lines, cw, ch)
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

	role := config.RoleJunior
	roles := config.AllRoles()
	if m.loopStepRoleSel >= 0 && m.loopStepRoleSel < len(roles) {
		role = roles[m.loopStepRoleSel]
	}

	speedNames := []string{"fast", "medium", "slow"}
	var spawnProfiles []config.DelegationProfile
	for i := range m.loopStepSpawnOpts {
		if m.loopStepSpawnSelect[i] {
			dp := config.DelegationProfile{Name: m.loopStepSpawnOpts[i]}
			if spd, ok := m.loopStepSpawnSpeed[i]; ok && spd > 0 && spd <= len(speedNames) {
				dp.Speed = speedNames[spd-1]
			}
			if m.loopStepSpawnHandoff[i] {
				dp.Handoff = true
			}
			spawnProfiles = append(spawnProfiles, dp)
		}
	}

	var delegation *config.DelegationConfig
	if len(spawnProfiles) > 0 {
		delegation = &config.DelegationConfig{
			Profiles: spawnProfiles,
		}
	}

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
	} else {
		lines = append(lines, "  "+saveLabel)
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("j/k: navigate  enter: edit field  esc: cancel"))

	content := fitLines(lines, cw, ch)
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
