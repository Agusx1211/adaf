package runtui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/stream"
	"github.com/agusx1211/adaf/internal/theme"
)

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

func (m *Model) ensureStreamBuf() *strings.Builder {
	if m.streamBuf == nil {
		m.streamBuf = &strings.Builder{}
	}
	return m.streamBuf
}

func (m *Model) ensureToolInputBuf() *strings.Builder {
	if m.toolInputBuf == nil {
		m.toolInputBuf = &strings.Builder{}
	}
	return m.toolInputBuf
}

func (m *Model) switchStreamScope(scope string) {
	if m.currentScope == scope {
		return
	}
	m.flushStream()
	m.currentScope = scope
	m.currentBlockType = ""
	m.currentToolName = ""
	m.ensureToolInputBuf().Reset()
}

// flushStream flushes the streaming buffer to a completed line.
func (m *Model) flushStream() {
	if m.streamBuf == nil || m.streamBuf.Len() == 0 {
		return
	}
	styled := m.styleDelta(m.streamBuf.String())
	m.addScopedLine(m.currentScope, styled)
	m.streamBuf.Reset()
}

// appendDelta processes streaming delta text and flushes completed lines.
func (m *Model) appendDelta(text string) {
	buf := m.ensureStreamBuf()
	for _, r := range text {
		if r == '\n' {
			m.flushStream()
		} else {
			buf.WriteRune(r)
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
	m.flushStderrRepeat(scope)
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

	if m.renderStreamEventLine(scope, line) {
		return
	}

	if m.renderStderrLine(scope, line) {
		return
	}

	trimmed := truncate(line, 400)
	m.addScopedLine(scope, textStyle.Render(trimmed))
	if sid := m.sessionIDForScope(scope); sid > 0 {
		m.setSessionAction(sid, "processing output")
	}
}

// --- Event handling ---

func (m *Model) handleEvent(ev stream.ClaudeEvent) {
	scope := ""
	sessionID := 0
	if ev.TurnID != "" {
		if sid, err := strconv.Atoi(strings.TrimSpace(ev.TurnID)); err == nil && sid > 0 {
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
			fmt.Sprintf("[init] session=%s model=%s", ev.TurnID, ev.Model)))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
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
					m.recordAssistantText(scope, text)
					if sessionID > 0 {
						m.setSessionAction(sessionID, "responding")
					}
				case "tool_use":
					m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", block.Name)))
					m.recordToolCall(scope, block.Name)
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
					m.addSimplifiedLine(scope, dimStyle.Render("tool result"))
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
					m.markAssistantBoundary(scope)
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
		m.ensureToolInputBuf().Reset()
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
				m.recordToolCall(scope, ev.ContentBlock.Name)
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
				m.ensureToolInputBuf().WriteString(ev.Delta.PartialJSON)
			} else if ev.Delta.Text != "" {
				m.appendDelta(ev.Delta.Text)
			}
		}

	case "content_block_stop":
		m.flushStream()
		if m.currentBlockType == "tool_use" && m.toolInputBuf != nil && m.toolInputBuf.Len() > 0 {
			m.renderToolInput(scope, m.currentToolName, m.toolInputBuf.String())
		}
		m.addScopedLine(scope, "")
		m.currentBlockType = ""
		m.currentToolName = ""
		m.ensureToolInputBuf().Reset()
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
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		if sessionID > 0 {
			m.setSessionAction(sessionID, "turn complete")
		}

	case "message":
		m.addSimplifiedLine(scope, dimStyle.Render("message"))

	default:
		if ev.Type != "" {
			m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("[%s]", ev.Type)))
		}
	}
}
