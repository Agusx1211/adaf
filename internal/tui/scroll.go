package tui

import tea "github.com/charmbracelet/bubbletea"

func (m *AppModel) syncScrollState(prev appState) {
	if m.state != prev {
		m.clearStateScroll(m.state)
		m.clearPaneFocus(m.state)
		m.resetInputEditors()
	}
}

func (m AppModel) stateScrollOffset() int {
	if m.viewScroll == nil {
		return 0
	}
	if offset, ok := m.viewScroll[m.state]; ok && offset > 0 {
		return offset
	}
	return 0
}

func (m *AppModel) setStateScroll(offset int) {
	if offset <= 0 {
		m.clearStateScroll(m.state)
		return
	}
	if m.viewScroll == nil {
		m.viewScroll = map[appState]int{}
	}
	m.viewScroll[m.state] = offset
}

func (m *AppModel) adjustStateScroll(delta int) {
	offset := m.stateScrollOffset() + delta
	if offset < 0 {
		offset = 0
	}
	m.setStateScroll(offset)
}

func (m *AppModel) clearStateScroll(state appState) {
	if m.viewScroll == nil {
		return
	}
	delete(m.viewScroll, state)
}

func (m *AppModel) resetStateScroll() {
	m.clearStateScroll(m.state)
}

func (m AppModel) pageScrollStep() int {
	step := (m.height - 2) / 2
	if step < 3 {
		step = 3
	}
	return step
}

func (m AppModel) stateSupportsManualScroll() bool {
	switch m.state {
	case stateSelector, stateSettingsRolesList, stateSettingsRoleEdit, stateSettingsRulesList, stateLoopStepRole, stateLoopStepSpawnRoles:
		return true
	default:
		return false
	}
}

func (m AppModel) stateSupportsPaneFocus() bool {
	switch m.state {
	case stateSelector, stateSettingsRolesList, stateSettingsRoleEdit, stateSettingsRulesList, stateLoopStepRole, stateLoopStepSpawnRoles:
		return true
	default:
		return false
	}
}

func (m AppModel) stateHasSplitPaneLayout() bool {
	switch m.state {
	case stateSelector:
		return m.width > selectorLeftWidth+20
	case stateSettingsRolesList, stateSettingsRoleEdit, stateSettingsRulesList:
		return m.width >= 80
	case stateLoopStepRole, stateLoopStepSpawnRoles:
		return m.width >= 90
	default:
		return false
	}
}

func (m AppModel) isRightPaneFocused() bool {
	if !m.stateSupportsPaneFocus() || !m.stateHasSplitPaneLayout() || m.viewPaneFocus == nil {
		return false
	}
	return m.viewPaneFocus[m.state]
}

func (m *AppModel) setRightPaneFocused(focused bool) {
	if !m.stateSupportsPaneFocus() || !m.stateHasSplitPaneLayout() {
		return
	}
	if m.viewPaneFocus == nil {
		m.viewPaneFocus = map[appState]bool{}
	}
	if focused {
		m.viewPaneFocus[m.state] = true
		return
	}
	delete(m.viewPaneFocus, m.state)
}

func (m *AppModel) clearPaneFocus(state appState) {
	if m.viewPaneFocus == nil {
		return
	}
	delete(m.viewPaneFocus, state)
}

func (m AppModel) canAdjustDetailScroll() bool {
	if !m.stateSupportsManualScroll() {
		return false
	}
	switch m.state {
	case stateSelector:
		return m.stateHasSplitPaneLayout()
	case stateSettingsRolesList, stateSettingsRoleEdit, stateSettingsRulesList, stateLoopStepRole, stateLoopStepSpawnRoles:
		if !m.stateHasSplitPaneLayout() {
			return true
		}
		return m.isRightPaneFocused()
	default:
		return true
	}
}

func (m *AppModel) handleGlobalScrollKey(msg tea.KeyMsg) bool {
	if !m.canAdjustDetailScroll() {
		return false
	}
	switch msg.String() {
	case "pgdown", "ctrl+d":
		m.adjustStateScroll(m.pageScrollStep())
		return true
	case "pgup", "ctrl+u":
		m.adjustStateScroll(-m.pageScrollStep())
		return true
	}
	return false
}
