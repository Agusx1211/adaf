package runtui

import (
	"fmt"
	"strconv"
	"strings"
)

// --- Scrolling ---

// totalLines returns the count of visible lines including a partial streaming line.
func (m Model) totalLines() int {
	return len(m.detailLines(m.rcWidth()))
}

func (m *Model) scrollDown(n int) {
	ms := m.maxScroll()
	m.scrollPos += n
	if m.scrollPos > ms {
		m.scrollPos = ms
	}
	m.autoScroll = m.scrollPos >= ms
}

func (m *Model) scrollUp(n int) {
	m.scrollPos -= n
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}
	m.autoScroll = false
}

func (m *Model) scrollToBottom() {
	m.scrollPos = m.maxScroll()
}

func (m Model) maxScroll() int {
	vh := m.detailViewportHeight()
	total := m.totalLines()
	if total <= vh {
		return 0
	}
	return total - vh
}

func (m Model) selectedScope() string {
	if m.leftSection != leftSectionAgents {
		return ""
	}
	entries := m.commandEntries()
	if len(entries) == 0 {
		return ""
	}
	idx := m.selectedEntry
	if idx < 0 {
		idx = 0
	}
	if idx >= len(entries) {
		idx = len(entries) - 1
	}
	return entries[idx].scope
}

func (m Model) scopeVisible(scope string) bool {
	selected := m.selectedScope()
	if selected == "" {
		return true
	}
	// Global lines remain visible in every filtered detail view.
	return scope == "" || scope == selected
}

func (m Model) shouldPrefixAllOutput() bool {
	if m.selectedScope() != "" {
		return false
	}
	scopes := make(map[string]struct{}, 4)
	for _, line := range m.lines {
		if line.scope == "" {
			continue
		}
		scopes[line.scope] = struct{}{}
		if len(scopes) > 1 {
			return true
		}
	}
	if m.currentScope != "" {
		scopes[m.currentScope] = struct{}{}
	}
	return len(scopes) > 1
}

func (m Model) scopePrefix(scope string) string {
	if strings.HasPrefix(scope, "session:") {
		if sid := m.sessionIDForScope(scope); sid > 0 {
			if s, ok := m.sessions[sid]; ok && s != nil {
				if s.Profile != "" {
					return fmt.Sprintf("[turn #%d:%s]", sid, s.Profile)
				}
			}
			return fmt.Sprintf("[turn #%d]", sid)
		}
	}
	if strings.HasPrefix(scope, "spawn:") {
		id := strings.TrimPrefix(scope, "spawn:")
		if id != "" {
			return "[spawn#" + id + "]"
		}
	}
	return "[scope]"
}

func (m Model) maybePrefixedLine(scope, text string, enable bool) string {
	if !enable || scope == "" {
		return text
	}
	return dimStyle.Render(m.scopePrefix(scope)+" ") + text
}

func (m Model) filteredLines() []string {
	if len(m.lines) == 0 {
		return nil
	}
	selected := m.selectedScope()
	prefixAll := m.shouldPrefixAllOutput()
	if selected == "" {
		out := make([]string, 0, len(m.lines))
		for _, line := range m.lines {
			out = append(out, m.maybePrefixedLine(line.scope, line.text, prefixAll))
		}
		return out
	}
	out := make([]string, 0, len(m.lines))
	for _, line := range m.lines {
		if line.scope == "" || line.scope == selected {
			out = append(out, m.maybePrefixedLine(line.scope, line.text, false))
		}
	}
	return out
}

// rcHeight returns the right panel content height (lines of text visible).
func (m Model) rcHeight() int {
	_, vf := rightPanelStyle.GetFrameSize()
	ph := m.height - 2 // header + status bar
	h := ph - vf
	if h < 1 {
		return 1
	}
	return h
}

// rcWidth returns the right panel content width (chars per line).
func (m Model) rcWidth() int {
	hf, _ := rightPanelStyle.GetFrameSize()
	rw := m.width - leftPanelOuterWidth
	if rw < 1 {
		rw = m.width
	}
	w := rw - hf
	if w < 1 {
		return 1
	}
	return w
}

// --- Content management ---

func (m Model) sessionScope(sessionID int) string {
	if sessionID == 0 {
		return ""
	}
	if sessionID < 0 {
		return m.spawnScope(-sessionID)
	}
	return fmt.Sprintf("session:%d", sessionID)
}

func (m Model) spawnScope(spawnID int) string {
	if spawnID <= 0 {
		return ""
	}
	return fmt.Sprintf("spawn:%d", spawnID)
}

func (m Model) sessionIDForScope(scope string) int {
	if !strings.HasPrefix(scope, "session:") {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimPrefix(scope, "session:"))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func splitRenderableLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}
