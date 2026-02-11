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
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.loopNameInput)
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
				turns := step.Turns
				if turns <= 0 {
					turns = 1
				}
				m.loopStepTurnsInput = strconv.Itoa(turns)
				m.loopStepInstrInput = step.Instructions
				m.loopStepCanStop = step.CanStop
				m.loopStepCanMsg = step.CanMessage
				m.loopStepCanPushover = step.CanPushover
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
		label := fmt.Sprintf("%d. %s x%d%s", i+1, step.Profile, turns, flags)
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
			}
		case "k", "up":
			if len(m.loopStepProfileOpts) > 0 {
				m.loopStepProfileSel = (m.loopStepProfileSel - 1 + len(m.loopStepProfileOpts)) % len(m.loopStepProfileOpts)
			}
		case "enter":
			if len(m.loopStepProfileOpts) > 0 {
				m.state = stateLoopStepTurns
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

// --- Loop Step Turns ---

func (m AppModel) updateLoopStepTurns(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.state = stateLoopStepInstr
		case "esc":
			m.state = stateLoopStepProfile
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
	inputText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(m.loopStepInstrInput)
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
			return m.finishLoopStep()
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
	lines = append(lines, dimStyle.Render("space: toggle  j/k: navigate  enter: save step  esc: back"))

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

	step := config.LoopStep{
		Profile:      profileName,
		Turns:        turns,
		Instructions: strings.TrimSpace(m.loopStepInstrInput),
		CanStop:      m.loopStepCanStop,
		CanMessage:   m.loopStepCanMsg,
		CanPushover:  m.loopStepCanPushover,
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
