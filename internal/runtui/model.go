package runtui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
	"github.com/agusx1211/adaf/internal/theme"
)

const leftPanelOuterWidth = 32

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
	lines      []string
	scrollPos  int
	autoScroll bool

	// Streaming delta accumulator. Raw text is collected here and flushed
	// to m.lines when a newline appears, the line exceeds viewport width,
	// or the content block ends.
	streamBuf    strings.Builder
	streamBufLen int // approximate visual width

	// Tool input accumulator: collects partial_json deltas for parsing on block stop.
	toolInputBuf strings.Builder

	// Current streaming block state.
	currentBlockType string
	currentToolName  string

	// Hierarchy: active spawns for this session.
	spawns []SpawnInfo

	// Event channel and lifecycle state.
	eventCh    chan any
	cancelFunc context.CancelFunc
	done       bool
	stopping   bool
	exitErr    error
}

// NewModel creates a new Model with the given configuration.
func NewModel(projectName string, plan *store.Plan, agentName, modelName string, eventCh chan any, cancel context.CancelFunc) Model {
	return Model{
		projectName: projectName,
		plan:        plan,
		agentName:   agentName,
		modelName:   modelName,
		startTime:   time.Now(),
		autoScroll:  true,
		eventCh:     eventCh,
		cancelFunc:  cancel,
	}
}

// SetSize sets the terminal dimensions on the model. This is used when
// embedding the Model inside a parent so the first render has correct sizing.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
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
		m.addLine(msg.Data)
		return m, waitForEvent(m.eventCh)

	case SpawnStatusMsg:
		m.spawns = msg.Spawns
		return m, waitForEvent(m.eventCh)

	case AgentStartedMsg:
		m.sessionID = msg.SessionID
		m.addLine(dimStyle.Render(fmt.Sprintf(">>> Session #%d started", msg.SessionID)))
		return m, waitForEvent(m.eventCh)

	case AgentFinishedMsg:
		if msg.Result != nil {
			m.addLine(dimStyle.Render(fmt.Sprintf("<<< Session #%d finished (exit=%d, %s)",
				msg.SessionID, msg.Result.ExitCode, msg.Result.Duration.Round(time.Second))))
		} else if msg.Err != nil {
			m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("<<< Session #%d error: %v", msg.SessionID, msg.Err)))
		}
		return m, waitForEvent(m.eventCh)

	case AgentLoopDoneMsg:
		m.done = true
		m.exitErr = msg.Err
		if msg.Err != nil {
			m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("Loop error: %v", msg.Err)))
		} else {
			m.addLine("")
			m.addLine(resultLabelStyle.Render("Agent loop finished."))
		}
		return m, nil

	case tickMsg:
		m.elapsed = time.Since(m.startTime)
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
		m.scrollDown(1)
	case "k", "up":
		m.scrollUp(1)
	case "pgdown", "ctrl+d":
		m.scrollDown(m.rcHeight() / 2)
	case "pgup", "ctrl+u":
		m.scrollUp(m.rcHeight() / 2)
	case "home", "g":
		m.scrollPos = 0
		m.autoScroll = false
	case "end", "G":
		m.scrollToBottom()
		m.autoScroll = true
	}
	return m, nil
}

// --- Scrolling ---

// totalLines returns the count of all lines including a partial streaming line.
func (m Model) totalLines() int {
	n := len(m.lines)
	if m.streamBuf.Len() > 0 {
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

// addLine adds a completed styled line.
func (m *Model) addLine(line string) {
	m.lines = append(m.lines, line)
	if m.autoScroll {
		m.scrollToBottom()
	}
}

// flushStream flushes the streaming buffer to a completed line.
func (m *Model) flushStream() {
	if m.streamBuf.Len() > 0 {
		styled := m.styleDelta(m.streamBuf.String())
		m.lines = append(m.lines, styled)
		m.streamBuf.Reset()
		m.streamBufLen = 0
		if m.autoScroll {
			m.scrollToBottom()
		}
	}
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
	if m.autoScroll {
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
func (m *Model) renderToolInput(toolName, rawJSON string) {
	var data map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		// Fallback: show truncated raw JSON.
		if len(rawJSON) > 200 {
			rawJSON = rawJSON[:200] + "..."
		}
		m.addLine(toolInputStyle.Render(rawJSON))
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
		m.addLine("  " + p)
	}
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// --- Event handling ---

func (m *Model) handleEvent(ev stream.ClaudeEvent) {
	switch ev.Type {
	case "system":
		m.addLine(initLabelStyle.Render(
			fmt.Sprintf("[init] session=%s model=%s", ev.SessionID, ev.Model)))
		if ev.Model != "" {
			m.modelName = ev.Model
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
					m.addLine(textLabelStyle.Render("[text]"))
					m.addLine("  " + textStyle.Render(text))
				case "tool_use":
					m.addLine(toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", block.Name)))
					m.renderToolInput(block.Name, string(block.Input))
				case "tool_result":
					m.addLine(toolResultStyle.Render("[tool_result]"))
				case "thinking":
					text := block.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					m.addLine(thinkingLabelStyle.Render("[thinking]"))
					m.addLine("  " + thinkingTextStyle.Render(compactWhitespace(text)))
				}
			}
		}

	case "user":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				if block.Type == "tool_result" {
					content := block.ToolContentText()
					if content == "" {
						m.addLine(dimStyle.Render("[empty result]"))
					} else {
						label := toolResultStyle.Render("[result]")
						if block.IsError {
							label = lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error]")
						}
						m.addLine(label)
						// Show first few lines of result, truncated.
						lines := strings.SplitN(content, "\n", 8)
						for i, line := range lines {
							if i >= 6 {
								m.addLine(dimStyle.Render("  ... (truncated)"))
								break
							}
							if len(line) > 200 {
								line = line[:200] + "..."
							}
							m.addLine(dimStyle.Render("  " + line))
						}
					}
				}
			}
		} else {
			m.addLine(dimStyle.Render("[tool response received]"))
		}

	case "content_block_start":
		m.flushStream()
		m.toolInputBuf.Reset()
		if ev.ContentBlock != nil {
			m.currentBlockType = ev.ContentBlock.Type
			m.currentToolName = ev.ContentBlock.Name
			switch ev.ContentBlock.Type {
			case "thinking":
				m.addLine(thinkingLabelStyle.Render("[thinking]"))
			case "tool_use":
				m.addLine(toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", ev.ContentBlock.Name)))
			case "text":
				m.addLine(textLabelStyle.Render("[text]"))
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
			m.renderToolInput(m.currentToolName, m.toolInputBuf.String())
		}
		m.addLine("")
		m.currentBlockType = ""
		m.currentToolName = ""
		m.toolInputBuf.Reset()

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
		m.addLine(resultLabelStyle.Render("[result]") + " " + summary)

	case "message":
		// Silently ignored.

	default:
		if ev.Type != "" {
			m.addLine(dimStyle.Render(fmt.Sprintf("[%s]", ev.Type)))
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
	title := fmt.Sprintf(" adaf run — %s ", m.projectName)
	return headerStyle.
		Width(m.width).
		MaxWidth(m.width).
		Render(title)
}

func (m Model) renderStatusBar() string {
	var parts []string
	parts = append(parts, shortcut("j/k", "scroll"))
	parts = append(parts, shortcut("pgup/dn", "page"))

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

	if m.done {
		parts = append(parts, shortcut("esc", "back"))
		parts = append(parts, shortcut("q", "quit"))
	} else {
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

	// Agent section.
	lines = append(lines, sectionTitleStyle.Render("Agent"))
	lines = append(lines, fieldLine("Name", m.agentName))
	if m.modelName != "" {
		lines = append(lines, fieldLine("Model", m.modelName))
	}
	if m.sessionID > 0 {
		lines = append(lines, fieldLine("Session", fmt.Sprintf("#%d", m.sessionID)))
	}
	lines = append(lines, fieldLine("Elapsed", m.elapsed.Round(time.Second).String()))
	lines = append(lines, "")

	// Usage section.
	lines = append(lines, sectionTitleStyle.Render("Usage"))
	if m.inputTokens > 0 || m.outputTokens > 0 {
		lines = append(lines, fieldLine("In", fmt.Sprintf("%d", m.inputTokens)))
		lines = append(lines, fieldLine("Out", fmt.Sprintf("%d", m.outputTokens)))
	}
	if m.costUSD > 0 {
		lines = append(lines, fieldLine("Cost", fmt.Sprintf("$%.4f", m.costUSD)))
	}
	if m.numTurns > 0 {
		lines = append(lines, fieldLine("Turns", fmt.Sprintf("%d", m.numTurns)))
	}
	lines = append(lines, "")

	// Hierarchy section.
	if len(m.spawns) > 0 {
		lines = append(lines, sectionTitleStyle.Render("Hierarchy"))
		for _, sp := range m.spawns {
			var statusStyle lipgloss.Style
			switch sp.Status {
			case "running":
				statusStyle = lipgloss.NewStyle().Foreground(theme.ColorYellow)
			case "awaiting_input":
				statusStyle = lipgloss.NewStyle().Foreground(theme.ColorMauve)
			case "completed", "merged":
				statusStyle = lipgloss.NewStyle().Foreground(theme.ColorGreen)
			case "failed", "rejected":
				statusStyle = lipgloss.NewStyle().Foreground(theme.ColorRed)
			default:
				statusStyle = lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
			}
			line := fmt.Sprintf("  [%d] %s (%s)", sp.ID, sp.Profile, statusStyle.Render(sp.Status))
			lines = append(lines, line)
			if sp.Status == "awaiting_input" && sp.Question != "" {
				lines = append(lines, dimStyle.Render("    Q: "+sp.Question))
			}
		}
		lines = append(lines, "")
	}

	// Plan section.
	if m.plan != nil && len(m.plan.Phases) > 0 {
		lines = append(lines, sectionTitleStyle.Render("Plan"))
		for _, phase := range m.plan.Phases {
			indicator := theme.PhaseStatusIndicator(phase.Status)
			title := phase.Title
			maxTitleW := cw - 4
			if maxTitleW > 0 && len(title) > maxTitleW {
				title = title[:maxTitleW-3] + "..."
			}
			lines = append(lines, indicator+title)
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
	total := m.totalLines()
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
	for i := start; i < end; i++ {
		if i < len(m.lines) {
			result = append(result, m.lines[i])
		} else if i == len(m.lines) && m.streamBuf.Len() > 0 {
			result = append(result, m.styleDelta(m.streamBuf.String()))
		}
	}
	return result
}

// --- Utility ---

// fitToSize takes a slice of styled lines and produces a string that is
// exactly w columns wide and h lines tall. Lines wider than w are truncated;
// shorter lines are right-padded with spaces; missing lines are blank.
func fitToSize(lines []string, w, h int) string {
	truncator := lipgloss.NewStyle().MaxWidth(w)
	emptyLine := strings.Repeat(" ", w)
	result := make([]string, h)

	for i := 0; i < h; i++ {
		if i < len(lines) {
			line := lines[i]
			lw := lipgloss.Width(line)
			if lw > w {
				line = truncator.Render(line)
				lw = lipgloss.Width(line)
			}
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
