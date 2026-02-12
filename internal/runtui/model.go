package runtui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
	"github.com/agusx1211/adaf/internal/theme"
)

const leftPanelOuterWidth = 44

type paneFocus int

const (
	focusDetail paneFocus = iota
	focusCommand
)

type scopedLine struct {
	scope string
	text  string
}

type sessionStatus struct {
	ID         int
	Agent      string
	Profile    string
	Model      string
	Status     string
	Action     string
	StartedAt  time.Time
	EndedAt    time.Time
	LastUpdate time.Time
}

type commandEntry struct {
	scope    string
	title    string
	status   string
	action   string
	duration string
}

// Model is the bubbletea model for the adaf run TUI.
type Model struct {
	width  int
	height int

	// Project/plan data.
	projectName string
	plan        *store.Plan

	// Agent info.
	agentName string
	modelName string
	sessionID int
	startTime time.Time
	elapsed   time.Duration

	// Usage stats (populated from stream events).
	inputTokens  int
	outputTokens int
	costUSD      float64
	numTurns     int

	// Right panel: completed styled lines and scroll position.
	lines      []scopedLine
	scrollPos  int
	autoScroll bool
	focus      paneFocus

	// Streaming delta accumulator. Raw text is collected here and flushed
	// to m.lines when a newline appears, the line exceeds viewport width,
	// or the content block ends.
	streamBuf    strings.Builder
	streamBufLen int // approximate visual width
	currentScope string

	// Tool input accumulator: collects partial_json deltas for parsing on block stop.
	toolInputBuf strings.Builder

	// Current streaming block state.
	currentBlockType string
	currentToolName  string

	// Hierarchy: active spawns for this session.
	spawns []SpawnInfo
	// Track first-seen and last-seen status for spawn entries.
	spawnFirstSeen map[int]time.Time
	spawnStatus    map[int]string

	// Loop state.
	loopName        string
	loopCycle       int
	loopStep        int
	loopTotalSteps  int
	loopStepProfile string

	// Event channel and lifecycle state.
	eventCh    chan any
	cancelFunc context.CancelFunc
	done       bool
	stopping   bool
	exitErr    error

	// Session mode: when non-zero, this model is attached to a session daemon
	// and supports detach (Ctrl+D).
	sessionModeID int

	// Command center state.
	sessions      map[int]*sessionStatus
	sessionOrder  []int
	selectedEntry int

	// Raw output accumulators per scope for line-based rendering.
	rawRemainder map[string]string
}

// NewModel creates a new Model with the given configuration.
func NewModel(projectName string, plan *store.Plan, agentName, modelName string, eventCh chan any, cancel context.CancelFunc) Model {
	return Model{
		projectName:    projectName,
		plan:           plan,
		agentName:      agentName,
		modelName:      modelName,
		startTime:      time.Now(),
		autoScroll:     true,
		focus:          focusDetail,
		eventCh:        eventCh,
		cancelFunc:     cancel,
		sessions:       make(map[int]*sessionStatus),
		spawnFirstSeen: make(map[int]time.Time),
		spawnStatus:    make(map[int]string),
		rawRemainder:   make(map[string]string),
	}
}

// SetSize sets the terminal dimensions on the model. This is used when
// embedding the Model inside a parent so the first render has correct sizing.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetLoopInfo configures loop display information on the model.
func (m *Model) SetLoopInfo(name string, totalSteps int) {
	m.loopName = name
	m.loopTotalSteps = totalSteps
}

// SetSessionMode marks this model as attached to a session daemon.
// When in session mode, Ctrl+D detaches instead of scrolling.
func (m *Model) SetSessionMode(sessionID int) {
	m.sessionModeID = sessionID
}

// SessionMode returns the session ID if in session mode, or 0.
func (m Model) SessionMode() int {
	return m.sessionModeID
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.eventCh),
		tickEvery(),
		tea.SetWindowTitle("adaf run"),
	)
}

// waitForEvent returns a Cmd that waits for the next event on the channel.
func waitForEvent(ch chan any) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return AgentLoopDoneMsg{}
		}
		return msg
	}
}

// tickEvery returns a Cmd that sends a tickMsg after 1 second.
func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case AgentEventMsg:
		m.handleEvent(msg.Event)
		return m, waitForEvent(m.eventCh)

	case AgentRawOutputMsg:
		scope := m.sessionScope(msg.SessionID)
		if scope == "" && m.sessionID > 0 {
			scope = m.sessionScope(m.sessionID)
		}
		m.handleRawChunk(scope, msg.Data)
		return m, waitForEvent(m.eventCh)

	case SpawnStatusMsg:
		m.spawns = msg.Spawns
		now := time.Now()
		nextSeen := make(map[int]time.Time, len(msg.Spawns))
		nextStatus := make(map[int]string, len(msg.Spawns))
		for _, sp := range msg.Spawns {
			firstSeen, ok := m.spawnFirstSeen[sp.ID]
			if !ok {
				firstSeen = now
			}
			nextSeen[sp.ID] = firstSeen
			nextStatus[sp.ID] = sp.Status
			prev, hadPrev := m.spawnStatus[sp.ID]
			if !hadPrev || prev != sp.Status {
				scope := m.spawnScope(sp.ID)
				m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("[spawn #%d] %s -> %s", sp.ID, sp.Profile, sp.Status)))
				if sp.Question != "" {
					m.addScopedLine(scope, dimStyle.Render("  Q: "+truncate(sp.Question, 180)))
				}
			}
		}
		m.spawnFirstSeen = nextSeen
		m.spawnStatus = nextStatus
		return m, waitForEvent(m.eventCh)

	case AgentStartedMsg:
		m.sessionID = msg.SessionID
		now := time.Now()
		s := m.ensureSession(msg.SessionID)
		if s.Agent == "" {
			s.Agent = m.agentName
		}
		if s.Profile == "" {
			s.Profile = m.loopStepProfile
		}
		s.Status = "running"
		s.Action = "starting"
		s.StartedAt = now
		s.LastUpdate = now
		scope := m.sessionScope(msg.SessionID)
		m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf(">>> Session #%d started", msg.SessionID)))
		return m, waitForEvent(m.eventCh)

	case AgentFinishedMsg:
		scope := m.sessionScope(msg.SessionID)
		m.flushRawRemainder(scope)
		s := m.ensureSession(msg.SessionID)
		s.LastUpdate = time.Now()
		if msg.Result != nil {
			s.EndedAt = s.LastUpdate
			if msg.Result.ExitCode == 0 {
				s.Status = "completed"
			} else {
				s.Status = "failed"
			}
			s.Action = fmt.Sprintf("finished (exit=%d)", msg.Result.ExitCode)
			m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("<<< Session #%d finished (exit=%d, %s)",
				msg.SessionID, msg.Result.ExitCode, msg.Result.Duration.Round(time.Second))))
		} else if msg.Err != nil {
			s.EndedAt = s.LastUpdate
			s.Status = "failed"
			s.Action = "error"
			m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("<<< Session #%d error: %v", msg.SessionID, msg.Err)))
		}
		return m, waitForEvent(m.eventCh)

	case AgentLoopDoneMsg:
		m.flushAllRawRemainders()
		m.done = true
		m.exitErr = msg.Err
		if msg.Err != nil {
			m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("Loop error: %v", msg.Err)))
		} else {
			m.addLine("")
			m.addLine(resultLabelStyle.Render("Agent loop finished."))
		}
		// Show cost/token summary.
		if m.costUSD > 0 || m.inputTokens > 0 {
			m.addLine(dimStyle.Render(fmt.Sprintf("  Total: $%.4f, %d in / %d out tokens, %d turns",
				m.costUSD, m.inputTokens, m.outputTokens, m.numTurns)))
		}
		return m, nil

	case LoopStepStartMsg:
		m.loopCycle = msg.Cycle
		m.loopStep = msg.StepIndex
		m.loopStepProfile = msg.Profile
		m.addLine("")
		m.addLine(initLabelStyle.Render(fmt.Sprintf("[loop] Cycle %d, Step %d/%d: %s (x%d)",
			msg.Cycle+1, msg.StepIndex+1, m.loopTotalSteps, msg.Profile, msg.Turns)))
		return m, waitForEvent(m.eventCh)

	case LoopStepEndMsg:
		m.addLine(dimStyle.Render(fmt.Sprintf("[loop] Step %d/%d: %s completed",
			msg.StepIndex+1, m.loopTotalSteps, msg.Profile)))
		return m, waitForEvent(m.eventCh)

	case LoopDoneMsg:
		m.flushAllRawRemainders()
		m.done = true
		m.exitErr = msg.Err
		if msg.Err != nil && msg.Reason != "cancelled" {
			m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("Loop error: %v", msg.Err)))
		} else {
			m.addLine("")
			m.addLine(resultLabelStyle.Render(fmt.Sprintf("Loop finished (%s).", msg.Reason)))
		}
		// Show cost/token summary.
		if m.costUSD > 0 || m.inputTokens > 0 {
			m.addLine(dimStyle.Render(fmt.Sprintf("  Total: $%.4f, %d in / %d out tokens, %d turns",
				m.costUSD, m.inputTokens, m.outputTokens, m.numTurns)))
		}
		return m, nil

	case tickMsg:
		m.elapsed = time.Since(m.startTime)
		// keep scroll clamped as filtered line counts change with selection/focus.
		ms := m.maxScroll()
		if m.scrollPos > ms {
			m.scrollPos = ms
		}
		return m, tickEvery()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// Done returns whether the agent loop has finished.
func (m Model) Done() bool {
	return m.done
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	moveSelection := func(delta int) {
		entries := m.commandEntries()
		if len(entries) == 0 {
			m.selectedEntry = 0
			return
		}
		m.selectedEntry += delta
		if m.selectedEntry < 0 {
			m.selectedEntry = 0
		}
		if m.selectedEntry >= len(entries) {
			m.selectedEntry = len(entries) - 1
		}
		m.scrollToBottom()
		m.autoScroll = true
	}
	cycleActiveAgent := func(delta int) {
		entries := m.commandEntries()
		if len(entries) <= 1 {
			return
		}

		active := make([]int, 0, len(entries)-1)
		for i := 1; i < len(entries); i++ {
			if entries[i].scope == "" {
				continue
			}
			if entries[i].status == "running" || entries[i].status == "awaiting_input" {
				active = append(active, i)
			}
		}
		if len(active) == 0 {
			for i := 1; i < len(entries); i++ {
				if entries[i].scope != "" {
					active = append(active, i)
				}
			}
		}
		if len(active) == 0 {
			return
		}

		pos := -1
		for i, idx := range active {
			if idx == m.selectedEntry {
				pos = i
				break
			}
		}
		if pos == -1 {
			if delta >= 0 {
				m.selectedEntry = active[0]
			} else {
				m.selectedEntry = active[len(active)-1]
			}
			m.scrollToBottom()
			m.autoScroll = true
			return
		}

		if delta >= 0 {
			pos = (pos + 1) % len(active)
		} else {
			pos = (pos - 1 + len(active)) % len(active)
		}
		m.selectedEntry = active[pos]
		m.scrollToBottom()
		m.autoScroll = true
	}

	switch msg.String() {
	case "q":
		if m.done {
			return m, tea.Quit
		}
		return m, nil
	case "esc", "backspace":
		if m.done {
			return m, func() tea.Msg { return BackToSelectorMsg{} }
		}
		return m, nil
	case "ctrl+d":
		if m.sessionModeID > 0 && !m.done {
			// Detach from the session without stopping the agent.
			return m, func() tea.Msg {
				return DetachMsg{SessionID: m.sessionModeID}
			}
		}
		// Not in session mode: page down.
		m.scrollDown(m.rcHeight() / 2)
	case "tab":
		if m.focus == focusDetail {
			m.focus = focusCommand
		} else {
			m.focus = focusDetail
		}
	case "left", "h":
		m.focus = focusCommand
	case "right", "l":
		m.focus = focusDetail
	case "]", "n":
		cycleActiveAgent(1)
	case "[", "p":
		cycleActiveAgent(-1)
	case "ctrl+c":
		if m.done {
			return m, tea.Quit
		}
		if m.stopping {
			// Second Ctrl+C: force quit.
			return m, tea.Quit
		}
		// First Ctrl+C: cancel the agent.
		m.stopping = true
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		m.addLine("")
		m.addLine(lipgloss.NewStyle().Foreground(theme.ColorYellow).Bold(true).Render(
			"Stopping agent... (press Ctrl+C again to force quit)"))
		return m, nil
	case "j", "down":
		if m.focus == focusCommand {
			moveSelection(1)
		} else {
			m.scrollDown(1)
		}
	case "k", "up":
		if m.focus == focusCommand {
			moveSelection(-1)
		} else {
			m.scrollUp(1)
		}
	case "pgdown":
		if m.focus == focusDetail {
			m.scrollDown(m.rcHeight() / 2)
		}
	case "pgup", "ctrl+u":
		if m.focus == focusDetail {
			m.scrollUp(m.rcHeight() / 2)
		}
	case "home", "g":
		if m.focus == focusDetail {
			m.scrollPos = 0
			m.autoScroll = false
		} else {
			m.selectedEntry = 0
			m.scrollToBottom()
			m.autoScroll = true
		}
	case "end", "G":
		if m.focus == focusDetail {
			m.scrollToBottom()
			m.autoScroll = true
		} else {
			entries := m.commandEntries()
			if len(entries) > 0 {
				m.selectedEntry = len(entries) - 1
				m.scrollToBottom()
				m.autoScroll = true
			}
		}
	}
	return m, nil
}

// --- Scrolling ---

// totalLines returns the count of visible lines including a partial streaming line.
func (m Model) totalLines() int {
	n := len(m.filteredLines())
	if m.streamBuf.Len() > 0 && m.scopeVisible(m.currentScope) {
		n++
	}
	return n
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
	vh := m.rcHeight()
	total := m.totalLines()
	if total <= vh {
		return 0
	}
	return total - vh
}

func (m Model) selectedScope() string {
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
					return fmt.Sprintf("[#%d:%s]", sid, s.Profile)
				}
			}
			return fmt.Sprintf("[#%d]", sid)
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
	if sessionID <= 0 {
		return ""
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

// addLine adds a completed styled line in the global scope.
func (m *Model) addLine(line string) {
	m.addScopedLine("", line)
}

// addScopedLine adds a completed styled line under a specific scope.
func (m *Model) addScopedLine(scope, line string) {
	for _, part := range splitRenderableLines(line) {
		m.lines = append(m.lines, scopedLine{
			scope: scope,
			text:  part,
		})
	}
	if m.autoScroll && m.scopeVisible(scope) {
		m.scrollToBottom()
	}
}

func (m *Model) switchStreamScope(scope string) {
	if m.currentScope == scope {
		return
	}
	m.flushStream()
	m.currentScope = scope
	m.currentBlockType = ""
	m.currentToolName = ""
	m.toolInputBuf.Reset()
}

func (m *Model) ensureSession(sessionID int) *sessionStatus {
	if sessionID <= 0 {
		return nil
	}
	if s, ok := m.sessions[sessionID]; ok {
		return s
	}
	now := time.Now()
	s := &sessionStatus{
		ID:         sessionID,
		Agent:      m.agentName,
		Profile:    m.loopStepProfile,
		Status:     "running",
		Action:     "starting",
		StartedAt:  now,
		LastUpdate: now,
	}
	m.sessions[sessionID] = s
	m.sessionOrder = append(m.sessionOrder, sessionID)
	return s
}

func (m *Model) setSessionAction(sessionID int, action string) {
	if sessionID <= 0 {
		return
	}
	s := m.ensureSession(sessionID)
	if s == nil {
		return
	}
	if action != "" {
		s.Action = action
	}
	s.LastUpdate = time.Now()
}

// flushStream flushes the streaming buffer to a completed line.
func (m *Model) flushStream() {
	if m.streamBuf.Len() == 0 {
		return
	}
	styled := m.styleDelta(m.streamBuf.String())
	m.addScopedLine(m.currentScope, styled)
	m.streamBuf.Reset()
	m.streamBufLen = 0
}

// appendDelta processes streaming delta text with line wrapping.
func (m *Model) appendDelta(text string) {
	cw := m.rcWidth()
	if cw < 1 {
		cw = 80
	}
	for _, r := range text {
		if r == '\n' {
			m.flushStream()
		} else {
			m.streamBuf.WriteRune(r)
			m.streamBufLen++
			if m.streamBufLen >= cw {
				m.flushStream()
			}
		}
	}
	if m.autoScroll && m.scopeVisible(m.currentScope) {
		m.scrollToBottom()
	}
}

// styleDelta applies the appropriate style based on current block type.
func (m Model) styleDelta(text string) string {
	switch m.currentBlockType {
	case "thinking":
		return thinkingTextStyle.Render(text)
	case "tool_use":
		return toolInputStyle.Render(text)
	default:
		return textStyle.Render(text)
	}
}

// renderToolInput parses the accumulated tool JSON and shows key fields.
func (m *Model) renderToolInput(scope, toolName, rawJSON string) {
	var data map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		// Fallback: show truncated raw JSON.
		if len(rawJSON) > 200 {
			rawJSON = rawJSON[:200] + "..."
		}
		m.addScopedLine(scope, toolInputStyle.Render(rawJSON))
		return
	}

	// Extract the most useful fields based on tool name.
	var parts []string
	switch toolName {
	case "Bash":
		if cmd, ok := data["command"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(cmd, 200)))
		}
		if desc, ok := data["description"].(string); ok {
			parts = append(parts, dimStyle.Render("# "+truncate(desc, 100)))
		}
	case "Read":
		if fp, ok := data["file_path"].(string); ok {
			parts = append(parts, toolInputStyle.Render(fp))
		}
	case "Write":
		if fp, ok := data["file_path"].(string); ok {
			parts = append(parts, toolInputStyle.Render(fp))
		}
	case "Edit":
		if fp, ok := data["file_path"].(string); ok {
			parts = append(parts, toolInputStyle.Render(fp))
		}
	case "Grep":
		if pat, ok := data["pattern"].(string); ok {
			parts = append(parts, toolInputStyle.Render("pattern="+truncate(pat, 100)))
		}
		if p, ok := data["path"].(string); ok {
			parts = append(parts, dimStyle.Render("path="+p))
		}
	case "Glob":
		if pat, ok := data["pattern"].(string); ok {
			parts = append(parts, toolInputStyle.Render("pattern="+truncate(pat, 100)))
		}
	case "Task":
		if desc, ok := data["description"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(desc, 200)))
		}
	case "WebFetch":
		if url, ok := data["url"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(url, 200)))
		}
	case "WebSearch":
		if q, ok := data["query"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(q, 200)))
		}
	}

	if len(parts) == 0 {
		// Generic fallback: show all string-valued keys.
		for k, v := range data {
			if s, ok := v.(string); ok {
				parts = append(parts, dimStyle.Render(k+"=")+toolInputStyle.Render(truncate(s, 100)))
				if len(parts) >= 3 {
					break
				}
			}
		}
	}

	for _, p := range parts {
		m.addScopedLine(scope, "  "+p)
	}
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		if max <= 3 {
			return s[:max]
		}
		return s[:max-3] + "..."
	}
	return s
}

func (m *Model) handleRawChunk(scope, chunk string) {
	if chunk == "" {
		return
	}
	if scope == "" {
		scope = m.sessionScope(m.sessionID)
	}
	if scope == "" {
		scope = ""
	}

	// Raw output is line-oriented and independent from Claude block deltas.
	m.switchStreamScope(scope)
	data := m.rawRemainder[scope] + strings.ReplaceAll(chunk, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	lines := strings.Split(data, "\n")
	if len(lines) == 0 {
		return
	}
	m.rawRemainder[scope] = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		m.handleRawLine(scope, line)
	}
}

func (m *Model) flushRawRemainder(scope string) {
	if rem, ok := m.rawRemainder[scope]; ok && strings.TrimSpace(rem) != "" {
		m.handleRawLine(scope, rem)
	}
	delete(m.rawRemainder, scope)
}

func (m *Model) flushAllRawRemainders() {
	for scope := range m.rawRemainder {
		m.flushRawRemainder(scope)
	}
}

func (m *Model) handleRawLine(scope, line string) {
	if strings.TrimSpace(line) == "" {
		m.addScopedLine(scope, "")
		return
	}

	if m.renderVibeStreamingLine(scope, line) {
		return
	}

	trimmed := truncate(line, 400)
	m.addScopedLine(scope, textStyle.Render(trimmed))
	if sid := m.sessionIDForScope(scope); sid > 0 {
		m.setSessionAction(sid, "processing output")
	}
}

func (m *Model) renderVibeStreamingLine(scope, line string) bool {
	var msg struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
		ToolCalls        []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return false
	}
	if msg.Role == "" {
		return false
	}

	switch msg.Role {
	case "system":
		m.addScopedLine(scope, initLabelStyle.Render("[vibe:system] context loaded"))
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "loading context")
		}
	case "user":
		text := compactWhitespace(msg.Content)
		if strings.TrimSpace(text) == "" {
			text = "prompt received"
		}
		m.addScopedLine(scope, dimStyle.Render("[vibe:user] "+truncate(text, 200)))
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "processing prompt")
		}
	case "assistant":
		if msg.ReasoningContent != "" {
			text := compactWhitespace(msg.ReasoningContent)
			m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
			m.addScopedLine(scope, "  "+thinkingTextStyle.Render(truncate(text, 300)))
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "thinking")
			}
		}
		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			if name == "" {
				name = "tool"
			}
			m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", name)))
			if tc.Function.Arguments != "" {
				m.addScopedLine(scope, "  "+toolInputStyle.Render(truncate(compactWhitespace(tc.Function.Arguments), 200)))
			}
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "running "+name)
			}
		}
		if strings.TrimSpace(msg.Content) != "" {
			m.addScopedLine(scope, textLabelStyle.Render("[text]"))
			m.addScopedLine(scope, "  "+textStyle.Render(truncate(msg.Content, 500)))
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "responding")
			}
		}
	case "tool":
		m.addScopedLine(scope, toolResultStyle.Render("[result]"))
		if strings.TrimSpace(msg.Content) != "" {
			for _, part := range splitRenderableLines(truncate(msg.Content, 400)) {
				m.addScopedLine(scope, dimStyle.Render("  "+part))
			}
		}
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "processing tool result")
		}
	default:
		return false
	}

	return true
}

// --- Event handling ---

func (m *Model) handleEvent(ev stream.ClaudeEvent) {
	scope := ""
	sessionID := 0
	if ev.SessionID != "" {
		if sid, err := strconv.Atoi(strings.TrimSpace(ev.SessionID)); err == nil && sid > 0 {
			sessionID = sid
			scope = m.sessionScope(sid)
			m.ensureSession(sid)
		}
	}
	if scope == "" && m.sessionID > 0 {
		scope = m.sessionScope(m.sessionID)
		sessionID = m.sessionID
	}
	m.switchStreamScope(scope)

	switch ev.Type {
	case "system":
		m.addScopedLine(scope, initLabelStyle.Render(
			fmt.Sprintf("[init] session=%s model=%s", ev.SessionID, ev.Model)))
		if ev.Model != "" {
			m.modelName = ev.Model
			if sessionID > 0 {
				s := m.ensureSession(sessionID)
				s.Model = ev.Model
				s.LastUpdate = time.Now()
				m.setSessionAction(sessionID, "initialized")
			}
		}

	case "assistant":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				switch block.Type {
				case "text":
					text := block.Text
					if len(text) > 500 {
						text = text[:500] + "..."
					}
					m.addScopedLine(scope, textLabelStyle.Render("[text]"))
					m.addScopedLine(scope, "  "+textStyle.Render(text))
					if sessionID > 0 {
						m.setSessionAction(sessionID, "responding")
					}
				case "tool_use":
					m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", block.Name)))
					m.renderToolInput(scope, block.Name, string(block.Input))
					if sessionID > 0 {
						action := "running tool"
						if block.Name != "" {
							action = "running " + block.Name
						}
						m.setSessionAction(sessionID, action)
					}
				case "tool_result":
					m.addScopedLine(scope, toolResultStyle.Render("[tool_result]"))
					if sessionID > 0 {
						m.setSessionAction(sessionID, "processing tool result")
					}
				case "thinking":
					text := block.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
					m.addScopedLine(scope, "  "+thinkingTextStyle.Render(compactWhitespace(text)))
					if sessionID > 0 {
						m.setSessionAction(sessionID, "thinking")
					}
				}
			}
		}

	case "user":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				if block.Type == "tool_result" {
					content := block.ToolContentText()
					if content == "" {
						m.addScopedLine(scope, dimStyle.Render("[empty result]"))
					} else {
						label := toolResultStyle.Render("[result]")
						if block.IsError {
							label = lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error]")
						}
						m.addScopedLine(scope, label)
						// Show first few lines of result, truncated.
						lines := strings.SplitN(content, "\n", 8)
						for i, line := range lines {
							if i >= 6 {
								m.addScopedLine(scope, dimStyle.Render("  ... (truncated)"))
								break
							}
							if len(line) > 200 {
								line = line[:200] + "..."
							}
							m.addScopedLine(scope, dimStyle.Render("  "+line))
						}
					}
				}
			}
		} else {
			m.addScopedLine(scope, dimStyle.Render("[tool response received]"))
		}
		if sessionID > 0 {
			m.setSessionAction(sessionID, "processing tool result")
		}

	case "content_block_start":
		m.flushStream()
		m.toolInputBuf.Reset()
		if ev.ContentBlock != nil {
			m.currentBlockType = ev.ContentBlock.Type
			m.currentToolName = ev.ContentBlock.Name
			switch ev.ContentBlock.Type {
			case "thinking":
				m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
				if sessionID > 0 {
					m.setSessionAction(sessionID, "thinking")
				}
			case "tool_use":
				m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", ev.ContentBlock.Name)))
				if sessionID > 0 {
					action := "running tool"
					if ev.ContentBlock.Name != "" {
						action = "running " + ev.ContentBlock.Name
					}
					m.setSessionAction(sessionID, action)
				}
			case "text":
				m.addScopedLine(scope, textLabelStyle.Render("[text]"))
				if sessionID > 0 {
					m.setSessionAction(sessionID, "responding")
				}
			}
		}

	case "content_block_delta":
		if ev.Delta != nil {
			if ev.Delta.PartialJSON != "" {
				// Accumulate tool JSON silently — will format on block stop.
				m.toolInputBuf.WriteString(ev.Delta.PartialJSON)
			} else if ev.Delta.Text != "" {
				m.appendDelta(ev.Delta.Text)
			}
		}

	case "content_block_stop":
		m.flushStream()
		if m.currentBlockType == "tool_use" && m.toolInputBuf.Len() > 0 {
			m.renderToolInput(scope, m.currentToolName, m.toolInputBuf.String())
		}
		m.addScopedLine(scope, "")
		m.currentBlockType = ""
		m.currentToolName = ""
		m.toolInputBuf.Reset()
		if sessionID > 0 {
			m.setSessionAction(sessionID, "waiting")
		}

	case "result":
		var parts []string
		if ev.TotalCostUSD > 0 {
			parts = append(parts, fmt.Sprintf("cost=$%.4f", ev.TotalCostUSD))
			m.costUSD = ev.TotalCostUSD
		}
		if ev.DurationMS > 0 {
			parts = append(parts, fmt.Sprintf("duration=%.1fs", ev.DurationMS/1000))
		}
		if ev.NumTurns > 0 {
			parts = append(parts, fmt.Sprintf("turns=%d", ev.NumTurns))
			m.numTurns = ev.NumTurns
		}
		if ev.Usage != nil {
			parts = append(parts, fmt.Sprintf("in=%d out=%d",
				ev.Usage.InputTokens, ev.Usage.OutputTokens))
			m.inputTokens = ev.Usage.InputTokens
			m.outputTokens = ev.Usage.OutputTokens
		}
		summary := "done"
		if len(parts) > 0 {
			summary = strings.Join(parts, " ")
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		if sessionID > 0 {
			m.setSessionAction(sessionID, "turn complete")
		}

	case "message":
		// Silently ignored.

	default:
		if ev.Type != "" {
			m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("[%s]", ev.Type)))
		}
	}
}

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
	parts = append(parts, shortcut("[/]", "agent"))

	total := m.totalLines()
	vh := m.rcHeight()
	if total > vh {
		pct := 0
		ms := m.maxScroll()
		if ms > 0 {
			pct = m.scrollPos * 100 / ms
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("%d%%", pct)))
	}

	if selected := m.selectedScope(); selected != "" {
		parts = append(parts, statusValueStyle.Render("detail="+selected))
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

func shortcut(k, desc string) string {
	return statusKeyStyle.Render(k) + statusValueStyle.Render(" "+desc)
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "running", "awaiting_input":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case "completed", "merged":
		return lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case "failed", "rejected":
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
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
		},
	}
	if m.done {
		entries[0].status = "completed"
	}
	if entries[0].action == "" {
		entries[0].action = "monitoring"
	}

	for _, sid := range m.sessionOrder {
		s := m.sessions[sid]
		if s == nil {
			continue
		}
		title := fmt.Sprintf("#%d %s", s.ID, s.Agent)
		if s.Profile != "" {
			title = fmt.Sprintf("#%d %s (%s)", s.ID, s.Profile, s.Agent)
		}
		if s.Model != "" {
			title += " · " + s.Model
		}
		duration := "0s"
		if !s.StartedAt.IsZero() {
			end := time.Now()
			if !s.EndedAt.IsZero() {
				end = s.EndedAt
			}
			d := end.Sub(s.StartedAt).Round(time.Second)
			if d < 0 {
				d = 0
			}
			duration = d.String()
		}
		status := s.Status
		if status == "" {
			status = "running"
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
		})
	}

	if len(m.spawns) > 0 {
		spawns := append([]SpawnInfo(nil), m.spawns...)
		sort.Slice(spawns, func(i, j int) bool {
			return spawns[i].ID < spawns[j].ID
		})
		for _, sp := range spawns {
			started := m.spawnFirstSeen[sp.ID]
			duration := ""
			if !started.IsZero() {
				d := time.Since(started).Round(time.Second)
				if d < 0 {
					d = 0
				}
				duration = d.String()
			}
			action := "spawn"
			if sp.Question != "" {
				action = "awaiting input"
			}
			entries = append(entries, commandEntry{
				scope:    m.spawnScope(sp.ID),
				title:    fmt.Sprintf("spawn #%d %s", sp.ID, sp.Profile),
				status:   sp.Status,
				action:   action,
				duration: duration,
			})
		}
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
	lines = append(lines, sectionTitleStyle.Render("Command Center"))
	if m.focus == focusCommand {
		lines = append(lines, dimStyle.Render("focus: agents"))
	} else {
		lines = append(lines, dimStyle.Render("focus: detail"))
	}
	lines = append(lines, dimStyle.Render("tab focus · [/] cycle"))
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
	lines = append(lines, "")

	lines = append(lines, sectionTitleStyle.Render("Agents"))
	entries := m.commandEntries()
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
		line := fmt.Sprintf("%s%s [%s]", prefix, titleStyle.Render(truncate(entry.title, cw-8)), status)
		lines = append(lines, line)
		meta := dimStyle.Render("   " + truncate(entry.duration+" · "+entry.action, cw-3))
		lines = append(lines, meta)
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

	if m.plan != nil && len(m.plan.Phases) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionTitleStyle.Render("Plan"))
		limit := len(m.plan.Phases)
		if limit > 6 {
			limit = 6
		}
		for i := 0; i < limit; i++ {
			phase := m.plan.Phases[i]
			indicator := theme.PhaseStatusIndicator(phase.Status)
			lines = append(lines, indicator+truncate(phase.Title, cw-2))
		}
	}

	content := fitToSize(lines, cw, ch)
	return leftPanelStyle.Render(content)
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

	visible := m.getVisibleLines(ch)
	content := fitToSize(visible, cw, ch)
	return rightPanelStyle.Render(content)
}

// getVisibleLines returns the slice of lines visible in the viewport,
// including a partial streaming line if present.
func (m Model) getVisibleLines(height int) []string {
	filtered := m.filteredLines()
	total := len(filtered)
	if m.streamBuf.Len() > 0 && m.scopeVisible(m.currentScope) {
		total++
	}
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

	result := make([]string, 0, end-start)
	prefixAll := m.shouldPrefixAllOutput()
	for i := start; i < end; i++ {
		if i < len(filtered) {
			result = append(result, filtered[i])
		} else if i == len(filtered) && m.streamBuf.Len() > 0 && m.scopeVisible(m.currentScope) {
			partial := m.styleDelta(m.streamBuf.String())
			partial = m.maybePrefixedLine(m.currentScope, partial, prefixAll)
			result = append(result, partial)
		}
	}
	return result
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
