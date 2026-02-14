package runtui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/theme"
)

// --- View rendering ---

func (m Model) View() string {
	if m.width == 0 || m.height < 3 {
		return "Loading..."
	}

	panelHeight := m.height - 2
	if panelHeight < 1 {
		panelHeight = 1
	}

	header := m.renderHeader()
	statusBar := m.renderStatusBar()

	var panels string
	rightOuterW := m.width - leftPanelOuterWidth
	if rightOuterW >= 20 {
		// Two-column layout.
		left := m.renderLeftPanel(leftPanelOuterWidth, panelHeight)
		right := m.renderRightPanel(rightOuterW, panelHeight)
		panels = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		// Narrow terminal: right panel only.
		panels = m.renderRightPanel(m.width, panelHeight)
	}

	return header + "\n" + panels + "\n" + statusBar
}

func (m Model) renderHeader() string {
	var title string
	if m.loopName != "" {
		title = fmt.Sprintf(" adaf loop — %s — %s ", m.projectName, m.loopName)
	} else {
		title = fmt.Sprintf(" adaf run — %s ", m.projectName)
	}
	return headerStyle.
		Width(m.width).
		MaxWidth(m.width).
		Render(title)
}

func (m Model) renderStatusBar() string {
	var parts []string
	if m.focus == focusCommand {
		parts = append(parts, shortcut("j/k", "select"))
		parts = append(parts, shortcut("tab", "detail"))
	} else {
		parts = append(parts, shortcut("j/k", "scroll"))
		parts = append(parts, shortcut("pgup/dn", "page"))
		parts = append(parts, shortcut("tab", "command"))
	}
	parts = append(parts, shortcut("1-5", "views"))
	if m.leftSection == leftSectionAgents {
		parts = append(parts, shortcut("[/]", "agent"))
		if m.focus == focusDetail {
			parts = append(parts, shortcut("t/T", "layer"))
		}
	}

	total := m.totalLines()
	vh := m.detailViewportHeight()
	if total > vh {
		pct := 0
		ms := m.maxScroll()
		if ms > 0 {
			pct = m.scrollPos * 100 / ms
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("%d%%", pct)))
	}

	parts = append(parts, statusValueStyle.Render("view="+m.leftSectionLabel()))
	if m.sessionModeID > 0 {
		if m.live {
			parts = append(parts, statusValueStyle.Render("stream=live"))
		} else {
			parts = append(parts, statusValueStyle.Render("stream=syncing"))
		}
	}
	if m.leftSection == leftSectionAgents {
		parts = append(parts, statusValueStyle.Render("layer="+m.detailLayerLabel()))
	}
	if selected := m.selectedScope(); selected != "" {
		parts = append(parts, statusValueStyle.Render("detail="+selected))
	}
	if m.leftSection == leftSectionIssues && len(m.issues) > 0 {
		idx := m.selectedIssue
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.issues) {
			idx = len(m.issues) - 1
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("issue=#%d", m.issues[idx].ID)))
	}
	if m.leftSection == leftSectionDocs && len(m.docs) > 0 {
		idx := m.selectedDoc
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.docs) {
			idx = len(m.docs) - 1
		}
		parts = append(parts, statusValueStyle.Render("doc="+m.docs[idx].ID))
	}
	if m.leftSection == leftSectionPlan && m.plan != nil && len(m.plan.Phases) > 0 {
		idx := m.selectedPhase
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.plan.Phases) {
			idx = len(m.plan.Phases) - 1
		}
		phaseID := strings.TrimSpace(m.plan.Phases[idx].ID)
		if phaseID == "" {
			phaseID = fmt.Sprintf("%d", idx+1)
		}
		parts = append(parts, statusValueStyle.Render("phase="+phaseID))
	}
	if m.leftSection == leftSectionLogs && len(m.turns) > 0 {
		idx := m.selectedTurn
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.turns) {
			idx = len(m.turns) - 1
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("turn=#%d", m.turns[idx].ID)))
	}

	if m.done {
		parts = append(parts, shortcut("esc", "back"))
		parts = append(parts, shortcut("q", "quit"))
	} else {
		if m.sessionModeID > 0 {
			parts = append(parts, shortcut("ctrl+d", "detach"))
		}
		parts = append(parts, shortcut("ctrl+c", "stop"))
	}

	content := strings.Join(parts, statusValueStyle.Render("  "))
	return statusBarStyle.
		Width(m.width).
		MaxWidth(m.width).
		Render(content)
}

func (m Model) leftSectionLabel() string {
	switch m.leftSection {
	case leftSectionIssues:
		return "issues"
	case leftSectionDocs:
		return "docs"
	case leftSectionPlan:
		return "plan"
	case leftSectionLogs:
		return "logs"
	default:
		return "agents"
	}
}

func shortcut(k, desc string) string {
	return statusKeyStyle.Render(k) + statusValueStyle.Render(" "+desc)
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "running", "awaiting_input", "waiting", "waiting_for_spawns":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case "completed", "merged":
		return lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case "failed", "rejected":
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	}
}

func issueStatusStyle(status string) lipgloss.Style {
	switch status {
	case "open":
		return lipgloss.NewStyle().Foreground(theme.ColorGreen).Bold(true)
	case "in_progress":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow).Bold(true)
	case "resolved":
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	case "wontfix":
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	}
}

func issuePriorityStyle(priority string) lipgloss.Style {
	switch priority {
	case "critical":
		return lipgloss.NewStyle().Foreground(theme.ColorRed).Bold(true)
	case "high":
		return lipgloss.NewStyle().Foreground(theme.ColorPeach).Bold(true)
	case "medium":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case "low":
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	}
}

func formatTimeAgoShort(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

func (m Model) commandEntries() []commandEntry {
	entries := []commandEntry{
		{
			scope:    "",
			title:    "All agents",
			status:   "running",
			action:   m.loopStepProfile,
			duration: m.elapsed.Round(time.Second).String(),
			depth:    0,
		},
	}
	if m.done {
		entries[0].status = "completed"
	}
	if entries[0].action == "" {
		entries[0].action = "monitoring"
	}

	spawns := append([]SpawnInfo(nil), m.spawns...)
	sort.Slice(spawns, func(i, j int) bool {
		return spawns[i].ID < spawns[j].ID
	})
	childrenByParent := make(map[string][]SpawnInfo, len(spawns))
	rootSpawns := make([]SpawnInfo, 0, len(spawns))
	for _, sp := range spawns {
		parentScope := ""
		if sp.ParentSpawnID > 0 {
			parentScope = m.spawnScope(sp.ParentSpawnID)
		} else if sp.ParentTurnID > 0 {
			parentScope = m.sessionScope(sp.ParentTurnID)
		}
		if parentScope == "" {
			rootSpawns = append(rootSpawns, sp)
			continue
		}
		childrenByParent[parentScope] = append(childrenByParent[parentScope], sp)
	}

	now := time.Now()
	appendSpawn := func(sp SpawnInfo, depth int, includeParentHint bool) {
		started := m.spawnFirstSeen[sp.ID]
		duration := ""
		if !started.IsZero() {
			d := now.Sub(started).Round(time.Second)
			if d < 0 {
				d = 0
			}
			duration = d.String()
		}
		title := fmt.Sprintf("#%d %s", sp.ID, sp.Profile)
		if role := strings.TrimSpace(sp.Role); role != "" {
			title += " as " + role
		}
		if includeParentHint && sp.ParentTurnID > 0 {
			title += fmt.Sprintf(" (turn #%d)", sp.ParentTurnID)
		}
		status := strings.TrimSpace(sp.Status)
		if status == "" {
			status = "running"
		}
		var actionParts []string
		if role := strings.TrimSpace(sp.Role); role != "" {
			actionParts = append(actionParts, "role="+role)
		}
		if sp.Status == "awaiting_input" || strings.TrimSpace(sp.Question) != "" {
			actionParts = append(actionParts, "awaiting input")
		}
		if includeParentHint && sp.ParentTurnID > 0 {
			actionParts = append(actionParts, fmt.Sprintf("parent turn #%d", sp.ParentTurnID))
		}
		if len(actionParts) == 0 {
			actionParts = append(actionParts, "delegated")
		}
		entries = append(entries, commandEntry{
			scope:    m.spawnScope(sp.ID),
			title:    title,
			status:   status,
			action:   strings.Join(actionParts, " · "),
			duration: duration,
			depth:    depth,
		})
	}
	seenSpawns := make(map[int]struct{}, len(spawns))
	var appendSpawnTree func(parentScope string, depth int)
	appendSpawnTree = func(parentScope string, depth int) {
		children := childrenByParent[parentScope]
		for _, sp := range children {
			if _, seen := seenSpawns[sp.ID]; seen {
				continue
			}
			seenSpawns[sp.ID] = struct{}{}
			appendSpawn(sp, depth, false)
			appendSpawnTree(m.spawnScope(sp.ID), depth+1)
		}
	}

	for _, sid := range m.sessionOrder {
		s := m.sessions[sid]
		if s == nil {
			continue
		}
		status := strings.TrimSpace(s.Status)
		if status == "" {
			status = "running"
		}
		title := fmt.Sprintf("turn #%d %s", s.ID, s.Agent)
		if s.Profile != "" {
			title = fmt.Sprintf("turn #%d %s (%s)", s.ID, s.Profile, s.Agent)
		}
		if s.Model != "" {
			title += " · " + s.Model
		}
		duration := "0s"
		if !s.StartedAt.IsZero() {
			end := now
			if !s.EndedAt.IsZero() {
				end = s.EndedAt
			}
			d := end.Sub(s.StartedAt).Round(time.Second)
			if d < 0 {
				d = 0
			}
			duration = d.String()
		}
		action := s.Action
		if action == "" {
			action = "idle"
		}
		entries = append(entries, commandEntry{
			scope:    m.sessionScope(s.ID),
			title:    title,
			status:   status,
			action:   action,
			duration: duration,
			depth:    0,
		})
		appendSpawnTree(m.sessionScope(s.ID), 1)
	}

	for _, sp := range rootSpawns {
		if _, seen := seenSpawns[sp.ID]; seen {
			continue
		}
		seenSpawns[sp.ID] = struct{}{}
		appendSpawn(sp, 0, sp.ParentTurnID > 0)
		appendSpawnTree(m.spawnScope(sp.ID), 1)
	}

	for _, sp := range spawns {
		if _, seen := seenSpawns[sp.ID]; seen {
			continue
		}
		seenSpawns[sp.ID] = struct{}{}
		appendSpawn(sp, 0, true)
		appendSpawnTree(m.spawnScope(sp.ID), 1)
	}

	return entries
}

func (m Model) renderLeftPanel(outerW, outerH int) string {
	hf, vf := leftPanelStyle.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionTitleStyle.Render("Command Center"))
	if m.focus == focusCommand {
		lines = append(lines, dimStyle.Render("focus: left panel"))
	} else {
		lines = append(lines, dimStyle.Render("focus: detail"))
	}
	helpLine := "tab focus · 1-5 views"
	if m.leftSection == leftSectionAgents {
		helpLine += " · t/T detail layer"
	}
	lines = append(lines, dimStyle.Render(helpLine))
	if m.leftSection == leftSectionAgents {
		lines = append(lines, dimStyle.Render("[/] cycle running agents"))
	} else {
		lines = append(lines, dimStyle.Render("j/k select entry"))
	}
	lines = append(lines, "")

	lines = append(lines, fieldLine("Agent", m.agentName))
	lines = append(lines, fieldLine("Elapsed", m.elapsed.Round(time.Second).String()))
	if m.loopName != "" {
		lines = append(lines, fieldLine("Loop", m.loopName))
	}
	if m.loopTotalSteps > 0 {
		lines = append(lines, fieldLine("Step", fmt.Sprintf("%d/%d", m.loopStep+1, m.loopTotalSteps)))
	}
	if m.loopStepProfile != "" {
		lines = append(lines, fieldLine("Profile", m.loopStepProfile))
	}
	if m.activeLoop != nil && m.activeLoop.Status == "running" {
		runLabel := fmt.Sprintf("#%d %s", m.activeLoop.ID, m.activeLoop.Status)
		if m.activeLoop.HexID != "" {
			runLabel = fmt.Sprintf("#%d [%s] %s", m.activeLoop.ID, m.activeLoop.HexID, m.activeLoop.Status)
		}
		lines = append(lines, fieldLine("Loop Run", runLabel))
	}
	lines = append(lines, "")

	entries := m.commandEntries()
	lines = append(lines, sectionTitleStyle.Render("Views"))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionAgents, fmt.Sprintf("1 Agents (%d)", len(entries))))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionIssues, fmt.Sprintf("2 Issues (%d)", len(m.issues))))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionDocs, fmt.Sprintf("3 Docs (%d)", len(m.docs))))
	planCount := 0
	if m.plan != nil {
		planCount = len(m.plan.Phases)
	}
	lines = append(lines, leftViewChip(m.leftSection == leftSectionPlan, fmt.Sprintf("4 Plan (%d)", planCount)))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionLogs, fmt.Sprintf("5 Logs (%d)", len(m.turns))))
	lines = append(lines, "")

	switch m.leftSection {
	case leftSectionIssues:
		cursorLine = m.appendIssuesList(&lines, cw)
	case leftSectionDocs:
		cursorLine = m.appendDocsList(&lines, cw)
	case leftSectionPlan:
		cursorLine = m.appendPlanList(&lines, cw)
	case leftSectionLogs:
		cursorLine = m.appendLogsList(&lines, cw)
	default:
		cursorLine = m.appendAgentsList(&lines, cw, entries)
	}
	lines = append(lines, "")

	lines = append(lines, sectionTitleStyle.Render("Usage"))
	usage := fmt.Sprintf("in=%d out=%d", m.inputTokens, m.outputTokens)
	if m.costUSD > 0 {
		usage += fmt.Sprintf(" cost=$%.4f", m.costUSD)
	}
	if m.numTurns > 0 {
		usage += fmt.Sprintf(" turns=%d", m.numTurns)
	}
	lines = append(lines, dimStyle.Render(truncate(usage, cw)))

	content := fitToSizeWithCursor(lines, cw, ch, cursorLine)
	return leftPanelStyle.Render(content)
}

func leftViewChip(active bool, text string) string {
	if active {
		return lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal).Render("> " + text)
	}
	return dimStyle.Render("  " + text)
}

func hierarchyPrefix(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("|  ", depth-1) + "+- "
}

func (m Model) appendAgentsList(lines *[]string, cw int, entries []commandEntry) int {
	*lines = append(*lines, sectionTitleStyle.Render("Agents"))
	if len(entries) == 0 {
		*lines = append(*lines, dimStyle.Render("  no active entries"))
		return -1
	}
	cursorLine := -1
	selected := m.selectedEntry
	if selected < 0 {
		selected = 0
	}
	if selected >= len(entries) {
		selected = len(entries) - 1
	}
	for i, entry := range entries {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		status := statusStyle(entry.status).Render(entry.status)
		title := entry.title
		if entry.depth > 0 {
			title = hierarchyPrefix(entry.depth) + title
		}
		line := fmt.Sprintf("%s%s [%s]", prefix, titleStyle.Render(truncate(title, cw-8)), status)
		*lines = append(*lines, line)
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		metaPrefix := "   "
		if entry.depth > 0 {
			metaPrefix += strings.Repeat("  ", entry.depth)
		}
		maxMetaWidth := cw - len(metaPrefix)
		if maxMetaWidth < 1 {
			maxMetaWidth = 1
		}
		meta := dimStyle.Render(metaPrefix + truncate(entry.duration+" · "+entry.action, maxMetaWidth))
		*lines = append(*lines, meta)
	}
	return cursorLine
}

func (m Model) appendIssuesList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Issues"))
	if len(m.issues) == 0 {
		*lines = append(*lines, dimStyle.Render("  no issues recorded"))
		return -1
	}
	cursorLine := -1
	selected := m.selectedIssue
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.issues) {
		selected = len(m.issues) - 1
	}
	for i, issue := range m.issues {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		title := fmt.Sprintf("#%d %s", issue.ID, truncate(issue.Title, cw-8))
		*lines = append(*lines, prefix+titleStyle.Render(title))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		meta := fmt.Sprintf("   %s · %s",
			issuePriorityStyle(issue.Priority).Render(issue.Priority),
			issueStatusStyle(issue.Status).Render(issue.Status))
		*lines = append(*lines, truncate(meta, cw))
	}
	return cursorLine
}

func (m Model) appendDocsList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Docs"))
	if len(m.docs) == 0 {
		*lines = append(*lines, dimStyle.Render("  no docs recorded"))
		return -1
	}
	cursorLine := -1
	selected := m.selectedDoc
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.docs) {
		selected = len(m.docs) - 1
	}
	for i, doc := range m.docs {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		title := fmt.Sprintf("[%s] %s", doc.ID, truncate(doc.Title, cw-8))
		*lines = append(*lines, prefix+titleStyle.Render(title))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		meta := fmt.Sprintf("   %s · %d chars", formatTimeAgoShort(doc.Updated), len(doc.Content))
		*lines = append(*lines, dimStyle.Render(truncate(meta, cw)))
	}
	return cursorLine
}

func (m Model) appendPlanList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Plan"))
	if m.plan == nil {
		*lines = append(*lines, dimStyle.Render("  no active plan"))
		return -1
	}

	title := strings.TrimSpace(m.plan.Title)
	if title == "" {
		title = "(untitled)"
	}
	status := strings.TrimSpace(m.plan.Status)
	if status == "" {
		status = "active"
	}
	*lines = append(*lines, valueStyle.Render("  "+truncate(m.plan.ID+" · "+title, cw-2)))
	*lines = append(*lines, dimStyle.Render("  status: "+status))
	if len(m.plan.Phases) == 0 {
		*lines = append(*lines, dimStyle.Render("  no phases"))
		return -1
	}

	*lines = append(*lines, "")
	*lines = append(*lines, sectionTitleStyle.Render("Phases"))
	selected := m.selectedPhase
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.plan.Phases) {
		selected = len(m.plan.Phases) - 1
	}
	cursorLine := -1
	for i, phase := range m.plan.Phases {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		indicator := theme.PhaseStatusIndicator(phase.Status)
		name := phase.Title
		if strings.TrimSpace(name) == "" {
			name = phase.ID
		}
		*lines = append(*lines, prefix+titleStyle.Render(indicator+truncate(name, cw-8)))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		metaID := strings.TrimSpace(phase.ID)
		if metaID == "" {
			metaID = fmt.Sprintf("phase-%d", i+1)
		}
		meta := fmt.Sprintf("   %s · %s", metaID, phase.Status)
		*lines = append(*lines, dimStyle.Render(truncate(meta, cw)))
	}
	return cursorLine
}

func (m Model) appendLogsList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Logs"))
	if len(m.turns) == 0 {
		*lines = append(*lines, dimStyle.Render("  no turn logs yet"))
		return -1
	}
	selected := m.selectedTurn
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.turns) {
		selected = len(m.turns) - 1
	}
	cursorLine := -1
	for i, turn := range m.turns {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		profile := strings.TrimSpace(turn.ProfileName)
		if profile == "" {
			profile = strings.TrimSpace(turn.Agent)
		}
		if profile == "" {
			profile = "turn"
		}
		title := fmt.Sprintf("#%d %s", turn.ID, profile)
		*lines = append(*lines, prefix+titleStyle.Render(truncate(title, cw-8)))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		meta := fmt.Sprintf("   %s · %s", turn.Date.Format("2006-01-02 15:04"), truncate(compactWhitespace(turn.Objective), cw-26))
		*lines = append(*lines, dimStyle.Render(truncate(meta, cw)))
	}
	return cursorLine
}

func fieldLine(label, value string) string {
	return labelStyle.Render(label) + valueStyle.Render(value)
}

func (m Model) renderRightPanel(outerW, outerH int) string {
	hf, vf := rightPanelStyle.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	headerLines := m.detailHeaderLines()
	viewportHeight := ch - len(headerLines)
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	visible := m.getVisibleLines(viewportHeight, cw)
	combined := make([]string, 0, len(headerLines)+len(visible))
	combined = append(combined, headerLines...)
	combined = append(combined, visible...)
	content := fitToSize(combined, cw, ch)
	return rightPanelStyle.Render(content)
}

// getVisibleLines returns the slice of lines visible in the viewport,
// including a partial streaming line if present.
func (m Model) getVisibleLines(height, width int) []string {
	wrapped := m.detailLines(width)
	total := len(wrapped)
	if total == 0 {
		return nil
	}

	start := m.scrollPos
	if start < 0 {
		start = 0
	}
	if start >= total {
		start = total - 1
	}

	end := start + height
	if end > total {
		end = total
	}

	return wrapped[start:end]
}

func (m Model) detailLines(width int) []string {
	switch m.leftSection {
	case leftSectionIssues:
		return wrapRenderableLines(m.issueDetailLines(), width)
	case leftSectionDocs:
		return wrapRenderableLines(m.docDetailLines(), width)
	case leftSectionPlan:
		return wrapRenderableLines(m.planDetailLines(), width)
	case leftSectionLogs:
		return wrapRenderableLines(m.logDetailLines(), width)
	}

	var lines []string
	switch m.detailLayer {
	case detailLayerSimplified:
		lines = m.simplifiedDetailLines()
	case detailLayerPrompt:
		lines = m.promptDetailLines()
	case detailLayerLastMessage:
		lines = m.finalMessageDetailLines()
	case detailLayerActivity:
		lines = m.activityDetailLines()
	default:
		lines = m.rawDetailLines()
	}
	return wrapRenderableLines(lines, width)
}

func (m Model) issueDetailLines() []string {
	if len(m.issues) == 0 {
		return []string{
			sectionTitleStyle.Render("Issues"),
			"",
			dimStyle.Render("No issues available."),
		}
	}

	idx := m.selectedIssue
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.issues) {
		idx = len(m.issues) - 1
	}
	issue := m.issues[idx]

	lines := []string{
		sectionTitleStyle.Render(fmt.Sprintf("Issue #%d", issue.ID)),
		fieldLine("Status", issueStatusStyle(issue.Status).Render(issue.Status)),
		fieldLine("Priority", issuePriorityStyle(issue.Priority).Render(issue.Priority)),
		fieldLine("Created", issue.Created.Format("2006-01-02 15:04")),
		fieldLine("Updated", issue.Updated.Format("2006-01-02 15:04")),
	}
	if issue.TurnID > 0 {
		lines = append(lines, fieldLine("Turn", fmt.Sprintf("#%d", issue.TurnID)))
	}
	if len(issue.Labels) > 0 {
		lines = append(lines, fieldLine("Labels", strings.Join(issue.Labels, ", ")))
	}
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Title"))
	lines = append(lines, textStyle.Render(issue.Title))
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Description"))
	if strings.TrimSpace(issue.Description) == "" {
		lines = append(lines, dimStyle.Render("No description."))
		return lines
	}
	for _, line := range splitRenderableLines(issue.Description) {
		lines = append(lines, textStyle.Render(line))
	}
	return lines
}

func (m Model) docDetailLines() []string {
	if len(m.docs) == 0 {
		return []string{
			sectionTitleStyle.Render("Docs"),
			"",
			dimStyle.Render("No documents available."),
		}
	}

	idx := m.selectedDoc
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.docs) {
		idx = len(m.docs) - 1
	}
	doc := m.docs[idx]

	lines := []string{
		sectionTitleStyle.Render(fmt.Sprintf("Doc %s", doc.ID)),
		fieldLine("Title", doc.Title),
		fieldLine("Updated", doc.Updated.Format("2006-01-02 15:04")),
		fieldLine("Size", fmt.Sprintf("%d chars", len(doc.Content))),
		"",
		sectionTitleStyle.Render("Content"),
	}
	if strings.TrimSpace(doc.Content) == "" {
		lines = append(lines, dimStyle.Render("Empty content."))
		return lines
	}
	for _, line := range splitRenderableLines(doc.Content) {
		lines = append(lines, textStyle.Render(line))
	}
	return lines
}

func (m Model) planDetailLines() []string {
	if m.plan == nil {
		return []string{
			sectionTitleStyle.Render("Plan"),
			"",
			dimStyle.Render("No active plan available."),
		}
	}

	status := strings.TrimSpace(m.plan.Status)
	if status == "" {
		status = "active"
	}
	title := strings.TrimSpace(m.plan.Title)
	if title == "" {
		title = "(untitled)"
	}
	lines := []string{
		sectionTitleStyle.Render("Plan " + m.plan.ID),
		fieldLine("Status", status),
		fieldLine("Updated", m.plan.Updated.Format("2006-01-02 15:04")),
		fieldLine("Phases", fmt.Sprintf("%d", len(m.plan.Phases))),
		"",
		sectionTitleStyle.Render("Title"),
		textStyle.Render(title),
	}
	if strings.TrimSpace(m.plan.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, sectionTitleStyle.Render("Description"))
		for _, line := range splitRenderableLines(m.plan.Description) {
			lines = append(lines, textStyle.Render(line))
		}
	}

	if len(m.plan.Phases) == 0 {
		lines = append(lines, "", dimStyle.Render("No phases defined."))
		return lines
	}

	idx := m.selectedPhase
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.plan.Phases) {
		idx = len(m.plan.Phases) - 1
	}
	phase := m.plan.Phases[idx]
	phaseID := strings.TrimSpace(phase.ID)
	if phaseID == "" {
		phaseID = fmt.Sprintf("phase-%d", idx+1)
	}
	phaseTitle := strings.TrimSpace(phase.Title)
	if phaseTitle == "" {
		phaseTitle = "(untitled phase)"
	}

	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render(fmt.Sprintf("Phase %d/%d", idx+1, len(m.plan.Phases))))
	lines = append(lines, fieldLine("ID", phaseID))
	lines = append(lines, fieldLine("Status", phase.Status))
	lines = append(lines, fieldLine("Priority", fmt.Sprintf("%d", phase.Priority)))
	if len(phase.DependsOn) > 0 {
		lines = append(lines, fieldLine("Depends", strings.Join(phase.DependsOn, ", ")))
	}
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Phase Title"))
	lines = append(lines, textStyle.Render(phaseTitle))
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Phase Description"))
	if strings.TrimSpace(phase.Description) == "" {
		lines = append(lines, dimStyle.Render("No phase description."))
	} else {
		for _, line := range splitRenderableLines(phase.Description) {
			lines = append(lines, textStyle.Render(line))
		}
	}
	return lines
}

func (m Model) logDetailLines() []string {
	if len(m.turns) == 0 {
		return []string{
			sectionTitleStyle.Render("Logs"),
			"",
			dimStyle.Render("No turn logs captured yet."),
		}
	}

	idx := m.selectedTurn
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.turns) {
		idx = len(m.turns) - 1
	}
	turn := m.turns[idx]

	profile := strings.TrimSpace(turn.ProfileName)
	if profile == "" {
		profile = "(unknown)"
	}
	agentLabel := strings.TrimSpace(turn.Agent)
	if strings.TrimSpace(turn.AgentModel) != "" {
		agentLabel += " · " + strings.TrimSpace(turn.AgentModel)
	}
	if strings.TrimSpace(turn.HexID) != "" {
		agentLabel += " [" + strings.TrimSpace(turn.HexID) + "]"
	}

	lines := []string{
		sectionTitleStyle.Render(fmt.Sprintf("Turn #%d", turn.ID)),
		fieldLine("Date", turn.Date.Format("2006-01-02 15:04:05")),
		fieldLine("Profile", profile),
		fieldLine("Agent", agentLabel),
		fieldLine("Duration", fmt.Sprintf("%ds", turn.DurationSecs)),
	}
	if strings.TrimSpace(turn.PlanID) != "" {
		lines = append(lines, fieldLine("Plan", turn.PlanID))
	}
	if strings.TrimSpace(turn.LoopRunHexID) != "" {
		lines = append(lines, fieldLine("Run", turn.LoopRunHexID))
	}
	if strings.TrimSpace(turn.StepHexID) != "" {
		lines = append(lines, fieldLine("Step", turn.StepHexID))
	}
	if strings.TrimSpace(turn.CommitHash) != "" {
		lines = append(lines, fieldLine("Commit", turn.CommitHash))
	}

	appendSection := func(title, body string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		lines = append(lines, "")
		lines = append(lines, sectionTitleStyle.Render(title))
		for _, line := range splitRenderableLines(body) {
			lines = append(lines, textStyle.Render(line))
		}
	}

	appendSection("Objective", turn.Objective)
	appendSection("What Was Built", turn.WhatWasBuilt)
	appendSection("Key Decisions", turn.KeyDecisions)
	appendSection("Challenges", turn.Challenges)
	appendSection("Current State", turn.CurrentState)
	appendSection("Known Issues", turn.KnownIssues)
	appendSection("Next Steps", turn.NextSteps)
	appendSection("Build State", turn.BuildState)

	if len(lines) == 0 {
		return []string{dimStyle.Render("No log details available.")}
	}
	return lines
}

// --- Utility ---

// fitToSize takes a slice of styled lines and produces a string that is
// exactly w columns wide and h lines tall. Lines wider than w are truncated;
// shorter lines are right-padded with spaces; missing lines are blank.
func fitToSize(lines []string, w, h int) string {
	emptyLine := strings.Repeat(" ", w)
	result := make([]string, h)

	for i := 0; i < h; i++ {
		if i < len(lines) {
			line := lines[i]
			parts := splitRenderableLines(line)
			if len(parts) > 0 {
				line = parts[0]
			}
			line = ansi.Truncate(line, w, "")
			lw := lipgloss.Width(line)
			pad := w - lw
			if pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			result[i] = line
		} else {
			result[i] = emptyLine
		}
	}
	return strings.Join(result, "\n")
}

func fitToSizeWithCursor(lines []string, w, h, cursorLine int) string {
	if cursorLine < 0 || len(lines) <= h {
		return fitToSize(lines, w, h)
	}
	start := cursorLine - (h / 2)
	if start < 0 {
		start = 0
	}
	maxStart := len(lines) - h
	if start > maxStart {
		start = maxStart
	}
	end := start + h
	if end > len(lines) {
		end = len(lines)
	}
	return fitToSize(lines[start:end], w, h)
}

func wrapRenderableLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if width < 1 {
		width = 1
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, part := range splitRenderableLines(line) {
			wrapped := ansi.Wrap(part, width, " ")
			out = append(out, splitRenderableLines(wrapped)...)
		}
	}
	return out
}

func compactWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}
