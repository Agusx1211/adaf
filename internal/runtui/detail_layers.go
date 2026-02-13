package runtui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/store"
)

type detailLayer int

const (
	detailLayerRaw detailLayer = iota
	detailLayerSimplified
	detailLayerPrompt
	detailLayerLastMessage
	detailLayerActivity
	detailLayerCount
)

type promptSnapshot struct {
	Prompt         string
	TurnHexID      string
	IsResume       bool
	Truncated      bool
	OriginalLength int
	UpdatedAt      time.Time
}

type finalMessageSnapshot struct {
	Text      string
	TurnHexID string
	UpdatedAt time.Time
}

type detailStats struct {
	ToolCalls         int
	AssistantMessages int
	Spawns            int
	Notes             int
	LoopMessages      int
	SpawnMessages     int
	Issues            int
	Docs              int
}

func (s *detailStats) add(other detailStats) {
	s.ToolCalls += other.ToolCalls
	s.AssistantMessages += other.AssistantMessages
	s.Spawns += other.Spawns
	s.Notes += other.Notes
	s.LoopMessages += other.LoopMessages
	s.SpawnMessages += other.SpawnMessages
	s.Issues += other.Issues
	s.Docs += other.Docs
}

func (m Model) detailLayerLabel() string {
	switch m.detailLayer {
	case detailLayerSimplified:
		return "simplified"
	case detailLayerPrompt:
		return "prompt"
	case detailLayerLastMessage:
		return "last-message"
	case detailLayerActivity:
		return "activity"
	default:
		return "raw"
	}
}

func (m *Model) cycleDetailLayer(delta int) {
	if m.leftSection != leftSectionAgents {
		return
	}
	n := int(detailLayerCount)
	next := (int(m.detailLayer) + delta) % n
	if next < 0 {
		next += n
	}
	m.detailLayer = detailLayer(next)
	switch m.detailLayer {
	case detailLayerRaw:
		m.scrollToBottom()
		m.autoScroll = true
	default:
		m.scrollPos = 0
		m.autoScroll = false
	}
	if ms := m.maxScroll(); m.scrollPos > ms {
		m.scrollPos = ms
	}
}

func (m Model) detailHeaderHeight() int {
	if m.leftSection != leftSectionAgents {
		return 0
	}
	return 2
}

func (m Model) detailViewportHeight() int {
	h := m.rcHeight() - m.detailHeaderHeight()
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) detailHeaderLines() []string {
	if m.leftSection != leftSectionAgents {
		return nil
	}
	label := "Detail · " + m.detailLayerLabel()
	selected := m.selectedScope()
	if selected != "" {
		label += " · " + m.scopePrefix(selected)
	}
	lines := []string{
		sectionTitleStyle.Render(label),
		m.renderLayerTabs(),
	}
	return lines
}

func (m Model) renderLayerTabs() string {
	type tabDef struct {
		layer detailLayer
		title string
	}
	tabs := []tabDef{
		{layer: detailLayerRaw, title: "Raw"},
		{layer: detailLayerSimplified, title: "Simplified"},
		{layer: detailLayerPrompt, title: "Prompt"},
		{layer: detailLayerLastMessage, title: "Last Msg"},
		{layer: detailLayerActivity, title: "Activity"},
	}

	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		style := detailTabInactiveStyle
		if tab.layer == m.detailLayer {
			style = detailTabActiveStyle
		}
		parts = append(parts, style.Render(tab.title))
	}
	return strings.Join(parts, " ")
}

func (m *Model) appendLayerLine(target *[]scopedLine, scope, line string, activeLayer detailLayer) {
	for _, part := range splitRenderableLines(line) {
		*target = append(*target, scopedLine{
			scope: scope,
			text:  part,
		})
	}
	if m.autoScroll && m.scopeVisible(scope) && m.leftSection == leftSectionAgents && m.detailLayer == activeLayer {
		m.scrollToBottom()
	}
}

func (m *Model) addSimplifiedLine(scope, line string) {
	added := false
	for _, part := range splitRenderableLines(line) {
		n := len(m.simplifiedLines)
		if n > 0 {
			last := m.simplifiedLines[n-1]
			if last.scope == scope && last.text == part {
				continue
			}
		}
		m.simplifiedLines = append(m.simplifiedLines, scopedLine{
			scope: scope,
			text:  part,
		})
		added = true
	}
	if added && m.autoScroll && m.scopeVisible(scope) && m.leftSection == leftSectionAgents && m.detailLayer == detailLayerSimplified {
		m.scrollToBottom()
	}
}

func (m *Model) addActivityLine(scope, line string) {
	m.appendLayerLine(&m.activityLines, scope, line, detailLayerActivity)
}

func (m *Model) ensureStats(scope string) *detailStats {
	if m.statsByScope == nil {
		m.statsByScope = make(map[string]*detailStats)
	}
	if st, ok := m.statsByScope[scope]; ok {
		return st
	}
	st := &detailStats{}
	m.statsByScope[scope] = st
	return st
}

func (m *Model) bumpStats(scope string, fn func(*detailStats)) {
	st := m.ensureStats(scope)
	fn(st)
}

func (m *Model) recordAssistantText(scope, text string) {
	compact := strings.TrimSpace(compactWhitespace(text))
	if compact == "" {
		return
	}
	chunks := m.assistantMessagesByScope[scope]
	if n := len(chunks); n == 0 || chunks[n-1] != compact {
		m.assistantMessagesByScope[scope] = append(chunks, compact)
	}
	m.lastMessageByScope[scope] = compact
	if turnHexID, closed := m.finalizedTurnHexByScope[scope]; closed {
		m.updateFinalMessageSnapshot(scope, turnHexID)
	}
	m.bumpStats(scope, func(st *detailStats) { st.AssistantMessages++ })
	m.addSimplifiedLine(scope, dimStyle.Render("assistant message"))
}

func (m *Model) latestAssistantMessage(scope string) string {
	chunks := m.assistantMessagesByScope[scope]
	if len(chunks) > 0 {
		return strings.TrimSpace(strings.Join(chunks, "\n\n"))
	}
	return strings.TrimSpace(m.lastMessageByScope[scope])
}

func (m *Model) updateFinalMessageSnapshot(scope, turnHexID string) {
	final := m.latestAssistantMessage(scope)
	if final == "" {
		return
	}
	snap := finalMessageSnapshot{
		Text:      final,
		TurnHexID: turnHexID,
		UpdatedAt: time.Now(),
	}
	if snap.TurnHexID == "" {
		if prev, ok := m.finalMessageByScope[scope]; ok && prev.TurnHexID != "" {
			snap.TurnHexID = prev.TurnHexID
		}
	}
	m.finalMessageByScope[scope] = snap
	m.latestFinalScope = scope
}

func (m *Model) recordToolCall(scope, toolName string) {
	_ = toolName
	m.bumpStats(scope, func(st *detailStats) { st.ToolCalls++ })
	m.addSimplifiedLine(scope, dimStyle.Render("tool call"))
}

func (m *Model) recordAgentPrompt(scope string, msg AgentPromptMsg) {
	if m.promptsByScope == nil {
		m.promptsByScope = make(map[string]promptSnapshot)
	}
	m.promptsByScope[scope] = promptSnapshot{
		Prompt:         msg.Prompt,
		TurnHexID:      msg.TurnHexID,
		IsResume:       msg.IsResume,
		Truncated:      msg.Truncated,
		OriginalLength: msg.OriginalLength,
		UpdatedAt:      time.Now(),
	}
	m.latestPromptScope = scope
	if msg.IsResume {
		m.addSimplifiedLine(scope, dimStyle.Render("resume prompt ready"))
	} else {
		m.addSimplifiedLine(scope, dimStyle.Render("prompt ready"))
	}
}

func (m Model) filteredScopedEntries(entries []scopedLine) []string {
	if len(entries) == 0 {
		return nil
	}
	selected := m.selectedScope()
	prefixAll := shouldPrefixAllEntries(entries)

	out := make([]string, 0, len(entries))
	if selected == "" {
		for _, line := range entries {
			out = append(out, m.maybePrefixedLine(line.scope, line.text, prefixAll))
		}
		return out
	}
	for _, line := range entries {
		if line.scope == "" || line.scope == selected {
			out = append(out, m.maybePrefixedLine(line.scope, line.text, false))
		}
	}
	return out
}

func shouldPrefixAllEntries(entries []scopedLine) bool {
	scopes := make(map[string]struct{}, 4)
	for _, line := range entries {
		if line.scope == "" {
			continue
		}
		scopes[line.scope] = struct{}{}
		if len(scopes) > 1 {
			return true
		}
	}
	return false
}

func (m Model) rawDetailLines() []string {
	lines := append([]string(nil), m.filteredLines()...)
	if m.streamBuf != nil && m.streamBuf.Len() > 0 && m.scopeVisible(m.currentScope) {
		prefixAll := m.shouldPrefixAllOutput()
		partial := m.styleDelta(m.streamBuf.String())
		lines = append(lines, m.maybePrefixedLine(m.currentScope, partial, prefixAll))
	}
	if len(lines) == 0 {
		return []string{dimStyle.Render("No output yet.")}
	}
	return lines
}

func (m Model) scopeStatsForDisplay() detailStats {
	if len(m.statsByScope) == 0 {
		return detailStats{}
	}
	selected := m.selectedScope()
	var out detailStats
	if selected == "" {
		for _, st := range m.statsByScope {
			if st == nil {
				continue
			}
			out.add(*st)
		}
		return out
	}
	if st := m.statsByScope[""]; st != nil {
		out.add(*st)
	}
	if st := m.statsByScope[selected]; st != nil {
		out.add(*st)
	}
	return out
}

func (m Model) simplifiedDetailLines() []string {
	stats := m.scopeStatsForDisplay()
	lines := []string{
		dimStyle.Render(fmt.Sprintf(
			"tools=%d messages=%d spawns=%d notes=%d loop_msgs=%d spawn_msgs=%d issues=%d docs=%d",
			stats.ToolCalls,
			stats.AssistantMessages,
			stats.Spawns,
			stats.Notes,
			stats.LoopMessages,
			stats.SpawnMessages,
			stats.Issues,
			stats.Docs,
		)),
		"",
	}
	entries := m.filteredScopedEntries(m.simplifiedLines)
	if len(entries) == 0 {
		lines = append(lines, dimStyle.Render("No simplified events yet."))
		return lines
	}
	lines = append(lines, entries...)
	return lines
}

func (m Model) promptScopeForDisplay() string {
	selected := m.selectedScope()
	if selected != "" {
		return selected
	}
	if m.latestPromptScope != "" {
		return m.latestPromptScope
	}
	latestScope := ""
	var latestTime time.Time
	for scope, snap := range m.promptsByScope {
		if snap.UpdatedAt.After(latestTime) {
			latestTime = snap.UpdatedAt
			latestScope = scope
		}
	}
	return latestScope
}

func (m Model) promptDetailLines() []string {
	scope := m.promptScopeForDisplay()
	if scope == "" {
		return []string{
			dimStyle.Render("No prompt captured yet."),
			dimStyle.Render("Prompts appear when a new turn starts."),
		}
	}
	snap, ok := m.promptsByScope[scope]
	if !ok {
		return []string{dimStyle.Render("No prompt captured for this selection yet.")}
	}

	lines := make([]string, 0, 8)
	if m.selectedScope() == "" && scope != "" {
		lines = append(lines, dimStyle.Render("Showing latest prompt for "+m.scopePrefix(scope)))
		lines = append(lines, "")
	}
	mode := "fresh prompt"
	if snap.IsResume {
		mode = "resume prompt"
	}
	meta := fmt.Sprintf("%s · %s · %d chars", mode, snap.UpdatedAt.Format("15:04:05"), len(snap.Prompt))
	if snap.Truncated && snap.OriginalLength > len(snap.Prompt) {
		meta += fmt.Sprintf(" (truncated from %d chars)", snap.OriginalLength)
	}
	lines = append(lines, dimStyle.Render(meta))
	if snap.TurnHexID != "" {
		lines = append(lines, dimStyle.Render("turn="+snap.TurnHexID))
	}
	lines = append(lines, "")
	for _, line := range splitRenderableLines(snap.Prompt) {
		lines = append(lines, textStyle.Render(line))
	}
	return lines
}

func (m Model) finalScopeForDisplay() string {
	selected := m.selectedScope()
	if selected != "" {
		return selected
	}
	if m.latestFinalScope != "" {
		return m.latestFinalScope
	}
	latestScope := ""
	var latestTime time.Time
	for scope, snap := range m.finalMessageByScope {
		if snap.UpdatedAt.After(latestTime) {
			latestTime = snap.UpdatedAt
			latestScope = scope
		}
	}
	return latestScope
}

func (m Model) finalMessageDetailLines() []string {
	scope := m.finalScopeForDisplay()
	if scope == "" {
		if m.done {
			return []string{dimStyle.Render("No final assistant message captured.")}
		}
		return []string{dimStyle.Render("Last message appears after a turn finishes.")}
	}
	snap, ok := m.finalMessageByScope[scope]
	if !ok || strings.TrimSpace(snap.Text) == "" {
		if m.done {
			return []string{dimStyle.Render("No final assistant message captured for this scope.")}
		}
		return []string{dimStyle.Render("Waiting for this turn to finish before capturing last message.")}
	}

	lines := make([]string, 0, 8)
	if m.selectedScope() == "" && scope != "" {
		lines = append(lines, dimStyle.Render("Showing latest finished message for "+m.scopePrefix(scope)))
		lines = append(lines, "")
	}
	meta := fmt.Sprintf("captured %s", snap.UpdatedAt.Format("15:04:05"))
	if snap.TurnHexID != "" {
		meta += " · turn=" + snap.TurnHexID
	}
	lines = append(lines, dimStyle.Render(meta))
	lines = append(lines, "")
	for _, line := range splitRenderableLines(snap.Text) {
		lines = append(lines, textStyle.Render(line))
	}
	return lines
}

func (m Model) activityDetailLines() []string {
	entries := m.filteredScopedEntries(m.activityLines)
	if len(entries) == 0 {
		return []string{dimStyle.Render("No message/note/issue/doc activity yet.")}
	}
	return entries
}

func (m *Model) refreshStoreActivity() {
	if m.projectStore == nil {
		return
	}
	m.ensureActivityMaps()

	if !m.activityBaselineReady {
		m.seedActivityBaseline()
		m.activityBaselineReady = true
		return
	}

	m.trackIssueActivity()
	m.trackDocActivity()
	m.trackNoteActivity()
	m.trackLoopMessageActivity()
	m.trackSpawnActivity()
}

func (m *Model) ensureActivityMaps() {
	if m.knownIssues == nil {
		m.knownIssues = make(map[int]store.Issue)
	}
	if m.knownDocs == nil {
		m.knownDocs = make(map[string]store.Doc)
	}
	if m.knownNotes == nil {
		m.knownNotes = make(map[int]struct{})
	}
	if m.knownLoopMessages == nil {
		m.knownLoopMessages = make(map[string]struct{})
	}
	if m.knownSpawns == nil {
		m.knownSpawns = make(map[int]store.SpawnRecord)
	}
	if m.knownSpawnMessages == nil {
		m.knownSpawnMessages = make(map[string]struct{})
	}
}

func (m *Model) seedActivityBaseline() {
	for _, issue := range m.issues {
		m.knownIssues[issue.ID] = issue
	}
	for _, doc := range m.docs {
		m.knownDocs[doc.ID] = doc
	}
	if notes, err := m.projectStore.ListNotes(); err == nil {
		for _, note := range notes {
			m.knownNotes[note.ID] = struct{}{}
		}
	}
	if m.activeLoop != nil && m.activeLoop.ID > 0 {
		m.activityLoopRunID = m.activeLoop.ID
		if msgs, err := m.projectStore.ListLoopMessages(m.activeLoop.ID); err == nil {
			for _, msg := range msgs {
				key := fmt.Sprintf("%d:%d", msg.RunID, msg.ID)
				m.knownLoopMessages[key] = struct{}{}
			}
		}
	}
	if spawns, err := m.projectStore.ListSpawns(); err == nil {
		for _, rec := range spawns {
			if !m.spawnIsRelevant(rec) {
				continue
			}
			m.knownSpawns[rec.ID] = rec
			msgs, msgErr := m.projectStore.ListMessages(rec.ID)
			if msgErr != nil {
				continue
			}
			for _, msg := range msgs {
				key := fmt.Sprintf("%d:%d", rec.ID, msg.ID)
				m.knownSpawnMessages[key] = struct{}{}
			}
		}
	}
}

func (m *Model) trackIssueActivity() {
	current := make(map[int]store.Issue, len(m.issues))
	for _, issue := range m.issues {
		current[issue.ID] = issue
		prev, ok := m.knownIssues[issue.ID]
		if !ok {
			m.addActivityLine("", dimStyle.Render(fmt.Sprintf("[issue #%d] created: %s", issue.ID, truncate(compactWhitespace(issue.Title), 140))))
			m.addSimplifiedLine("", dimStyle.Render("issue created"))
			m.bumpStats("", func(st *detailStats) { st.Issues++ })
			continue
		}
		if issue.Updated.After(prev.Updated) ||
			issue.Status != prev.Status ||
			issue.Priority != prev.Priority ||
			issue.Title != prev.Title ||
			issue.Description != prev.Description {
			change := "updated"
			if issue.Status != prev.Status {
				change = fmt.Sprintf("status %s -> %s", prev.Status, issue.Status)
			}
			m.addActivityLine("", dimStyle.Render(fmt.Sprintf("[issue #%d] %s", issue.ID, change)))
			m.addSimplifiedLine("", dimStyle.Render("issue updated"))
			m.bumpStats("", func(st *detailStats) { st.Issues++ })
		}
	}
	m.knownIssues = current
}

func (m *Model) trackDocActivity() {
	current := make(map[string]store.Doc, len(m.docs))
	for _, doc := range m.docs {
		current[doc.ID] = doc
		prev, ok := m.knownDocs[doc.ID]
		if !ok {
			m.addActivityLine("", dimStyle.Render(fmt.Sprintf("[doc %s] created: %s", doc.ID, truncate(compactWhitespace(doc.Title), 140))))
			m.addSimplifiedLine("", dimStyle.Render("doc created"))
			m.bumpStats("", func(st *detailStats) { st.Docs++ })
			continue
		}
		if doc.Updated.After(prev.Updated) || doc.Title != prev.Title || doc.Content != prev.Content {
			m.addActivityLine("", dimStyle.Render(fmt.Sprintf("[doc %s] updated", doc.ID)))
			m.addSimplifiedLine("", dimStyle.Render("doc updated"))
			m.bumpStats("", func(st *detailStats) { st.Docs++ })
		}
	}
	m.knownDocs = current
}

func (m *Model) trackNoteActivity() {
	notes, err := m.projectStore.ListNotes()
	if err != nil {
		return
	}
	current := make(map[int]struct{}, len(notes))
	for _, note := range notes {
		current[note.ID] = struct{}{}
		if _, seen := m.knownNotes[note.ID]; seen {
			continue
		}
		scope := m.sessionScope(note.TurnID)
		author := strings.TrimSpace(note.Author)
		if author == "" {
			author = "supervisor"
		}
		m.addActivityLine(scope, dimStyle.Render(fmt.Sprintf("[note] %s: %s", author, truncate(compactWhitespace(note.Note), 180))))
		m.addSimplifiedLine(scope, dimStyle.Render("supervisor note received"))
		m.bumpStats(scope, func(st *detailStats) { st.Notes++ })
	}
	m.knownNotes = current
}

func (m *Model) trackLoopMessageActivity() {
	if m.activeLoop == nil || m.activeLoop.ID <= 0 {
		m.activityLoopRunID = 0
		m.knownLoopMessages = make(map[string]struct{})
		return
	}

	runID := m.activeLoop.ID
	msgs, err := m.projectStore.ListLoopMessages(runID)
	if err != nil {
		return
	}
	if m.activityLoopRunID != runID {
		m.activityLoopRunID = runID
		m.knownLoopMessages = make(map[string]struct{}, len(msgs))
		for _, msg := range msgs {
			key := fmt.Sprintf("%d:%d", msg.RunID, msg.ID)
			m.knownLoopMessages[key] = struct{}{}
		}
		return
	}

	for _, msg := range msgs {
		key := fmt.Sprintf("%d:%d", msg.RunID, msg.ID)
		if _, seen := m.knownLoopMessages[key]; seen {
			continue
		}
		m.knownLoopMessages[key] = struct{}{}
		text := truncate(compactWhitespace(msg.Content), 180)
		m.addActivityLine("", dimStyle.Render(fmt.Sprintf("[loop msg step %d] %s", msg.StepIndex+1, text)))
		m.addSimplifiedLine("", dimStyle.Render("loop message"))
		m.bumpStats("", func(st *detailStats) { st.LoopMessages++ })
	}
}

func (m Model) spawnIsRelevant(rec store.SpawnRecord) bool {
	if rec.ID <= 0 {
		return false
	}
	if _, ok := m.knownSpawns[rec.ID]; ok {
		return true
	}
	if _, ok := m.sessions[rec.ParentTurnID]; ok {
		return true
	}
	if m.activeLoop != nil {
		for _, turnID := range m.activeLoop.TurnIDs {
			if turnID == rec.ParentTurnID {
				return true
			}
		}
	}
	return false
}

func (m *Model) trackSpawnActivity() {
	spawns, err := m.projectStore.ListSpawns()
	if err != nil {
		return
	}
	sort.Slice(spawns, func(i, j int) bool { return spawns[i].ID < spawns[j].ID })

	current := make(map[int]store.SpawnRecord, len(spawns))
	for _, rec := range spawns {
		if !m.spawnIsRelevant(rec) {
			continue
		}
		current[rec.ID] = rec
		scope := m.spawnScope(rec.ID)
		prev, seen := m.knownSpawns[rec.ID]
		if !seen {
			m.addActivityLine(scope, dimStyle.Render(fmt.Sprintf("[spawn #%d] created for %s (parent turn #%d)", rec.ID, rec.ChildProfile, rec.ParentTurnID)))
			m.addSimplifiedLine(scope, dimStyle.Render("spawn created"))
			m.bumpStats(scope, func(st *detailStats) { st.Spawns++ })
		} else if prev.Status != rec.Status {
			m.addActivityLine(scope, dimStyle.Render(fmt.Sprintf("[spawn #%d] status %s -> %s", rec.ID, prev.Status, rec.Status)))
			m.addSimplifiedLine(scope, dimStyle.Render("spawn status changed"))
			m.bumpStats(scope, func(st *detailStats) { st.Spawns++ })
		}
		m.trackSpawnMessages(rec)
	}
	m.knownSpawns = current
}

func (m *Model) trackSpawnMessages(rec store.SpawnRecord) {
	msgs, err := m.projectStore.ListMessages(rec.ID)
	if err != nil {
		return
	}
	for _, msg := range msgs {
		key := fmt.Sprintf("%d:%d", rec.ID, msg.ID)
		if _, seen := m.knownSpawnMessages[key]; seen {
			continue
		}
		m.knownSpawnMessages[key] = struct{}{}
		scope := m.spawnScope(rec.ID)
		direction := msg.Direction
		if direction == "" {
			direction = "message"
		}
		kind := msg.Type
		if kind == "" {
			kind = "message"
		}
		body := truncate(compactWhitespace(msg.Content), 180)
		m.addActivityLine(scope, dimStyle.Render(fmt.Sprintf("[spawn msg #%d] %s/%s: %s", rec.ID, direction, kind, body)))
		m.addSimplifiedLine(scope, dimStyle.Render("spawn message"))
		m.bumpStats(scope, func(st *detailStats) { st.SpawnMessages++ })
	}
}
