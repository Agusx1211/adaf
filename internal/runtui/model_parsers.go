package runtui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/theme"
)

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
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "loading context")
		}
	case "user":
		text := compactWhitespace(msg.Content)
		if strings.TrimSpace(text) == "" {
			text = "prompt received"
		}
		m.addScopedLine(scope, dimStyle.Render("[vibe:user] "+truncate(text, 200)))
		m.addSimplifiedLine(scope, dimStyle.Render("prompt received"))
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
			m.recordToolCall(scope, name)
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
			m.recordAssistantText(scope, msg.Content)
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "responding")
			}
		}
	case "tool":
		m.markAssistantBoundary(scope)
		m.addScopedLine(scope, toolResultStyle.Render("[result]"))
		m.addSimplifiedLine(scope, dimStyle.Render("tool result"))
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

// stderrCategory classifies a stderr line body.
type stderrCategory int

const (
	stderrGeneric     stderrCategory = iota
	stderrDeprecation                // Node.js deprecation warnings
	stderrInfo                       // Informational messages (YOLO mode, credentials, etc.)
)

// classifyStderr returns the category for a stderr line body (without the [stderr] prefix).
func classifyStderr(body string) stderrCategory {
	lower := strings.ToLower(body)
	// Deprecation warnings from Node.js.
	if strings.Contains(body, "DeprecationWarning:") ||
		strings.HasPrefix(body, "(Use `node --trace-deprecation") ||
		strings.HasPrefix(body, "(node:") {
		return stderrDeprecation
	}
	// Known informational messages.
	if strings.Contains(lower, "yolo mode") ||
		strings.Contains(lower, "loaded cached credentials") ||
		strings.Contains(lower, "hook registry initialized") ||
		strings.Contains(lower, "mode is enabled") {
		return stderrInfo
	}
	return stderrGeneric
}

// renderStderrLine handles lines that start with "[stderr] ". Returns true if
// the line was consumed (callers should skip further processing).
func (m *Model) renderStderrLine(scope, line string) bool {
	if !strings.HasPrefix(line, "[stderr] ") {
		return false
	}
	body := strings.TrimSpace(strings.TrimPrefix(line, "[stderr] "))
	if body == "" {
		// Empty stderr line — consume silently.
		return true
	}

	// Deduplication: suppress consecutive identical stderr within a scope.
	if last, ok := m.lastStderrByScope[scope]; ok && last == body {
		m.stderrRepeatCount[scope]++
		return true
	}
	// Flush any pending repeat annotation before rendering a new line.
	m.flushStderrRepeat(scope)
	m.lastStderrByScope[scope] = body
	m.stderrRepeatCount[scope] = 0

	truncBody := truncate(body, 400)
	switch classifyStderr(body) {
	case stderrDeprecation:
		m.addScopedLine(scope, stderrDimStyle.Render("[stderr] "+truncBody))
	case stderrInfo:
		m.addScopedLine(scope, dimStyle.Render("[stderr] "+truncBody))
	default: // stderrGeneric
		m.addScopedLine(scope, stderrLabelStyle.Render("[stderr]")+" "+dimStyle.Render(truncBody))
	}
	return true
}

// flushStderrRepeat emits a suppression annotation if consecutive identical
// stderr lines were deduplicated for the given scope, then resets state.
func (m *Model) flushStderrRepeat(scope string) {
	if count := m.stderrRepeatCount[scope]; count > 0 {
		note := fmt.Sprintf("  (%d identical lines suppressed)", count)
		m.addScopedLine(scope, stderrDimStyle.Render(note))
	}
	delete(m.lastStderrByScope, scope)
	delete(m.stderrRepeatCount, scope)
}

// renderStreamEventLine attempts to parse a raw JSON line as a codex, gemini,
// or claude stream event and renders it in a human-readable form. Returns true
// if the line was handled.
func (m *Model) renderStreamEventLine(scope, line string) bool {
	// Quick check: must look like JSON with a "type" field.
	if len(line) == 0 || line[0] != '{' {
		return false
	}

	// Peek at the type field to decide which parser to use.
	var peek struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &peek); err != nil || peek.Type == "" {
		return false
	}

	// Try codex event types first (they use dotted names).
	if m.renderCodexStreamLine(scope, line, peek.Type) {
		return true
	}

	// Try gemini event types.
	if m.renderGeminiStreamLine(scope, line, peek.Type) {
		return true
	}

	// Try claude event types.
	if m.renderClaudeStreamLine(scope, line, peek.Type) {
		return true
	}

	return false
}

// renderCodexStreamLine handles codex JSON events from claude_stream recordings.
func (m *Model) renderCodexStreamLine(scope, line, eventType string) bool {
	switch eventType {
	case "thread.started":
		var ev struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		m.addScopedLine(scope, initLabelStyle.Render(fmt.Sprintf("[init] session=%s", ev.ThreadID)))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		return true

	case "turn.started", "item.started", "item.updated":
		// Skip intermediate events silently.
		return true

	case "item.completed":
		var ev struct {
			Item *struct {
				Type             string            `json:"type"`
				Text             string            `json:"text"`
				Command          string            `json:"command"`
				AggregatedOutput string            `json:"aggregated_output"`
				ExitCode         *int              `json:"exit_code"`
				Status           string            `json:"status"`
				Server           string            `json:"server"`
				Tool             string            `json:"tool"`
				Arguments        json.RawMessage   `json:"arguments"`
				Changes          []json.RawMessage `json:"changes"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Item == nil {
			return false
		}
		switch ev.Item.Type {
		case "agent_message":
			if strings.TrimSpace(ev.Item.Text) == "" {
				return true
			}
			m.addScopedLine(scope, textLabelStyle.Render("[text]"))
			m.addScopedLine(scope, "  "+textStyle.Render(truncate(ev.Item.Text, 500)))
			m.recordAssistantText(scope, ev.Item.Text)
		case "reasoning":
			if strings.TrimSpace(ev.Item.Text) == "" {
				return true
			}
			m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
			m.addScopedLine(scope, "  "+thinkingTextStyle.Render(truncate(compactWhitespace(ev.Item.Text), 300)))
		case "command_execution":
			cmd := ev.Item.Command
			if cmd == "" {
				cmd = "command"
			}
			m.addScopedLine(scope, toolLabelStyle.Render("[tool:Bash]"))
			m.recordToolCall(scope, "Bash")
			m.addScopedLine(scope, "  "+toolInputStyle.Render(truncate(cmd, 200)))
			if ev.Item.AggregatedOutput != "" {
				m.addScopedLine(scope, "  "+dimStyle.Render(truncate(ev.Item.AggregatedOutput, 200)))
			}
		case "mcp_tool_call":
			toolName := strings.Trim(strings.Join([]string{ev.Item.Server, ev.Item.Tool}, "."), ".")
			if toolName == "" {
				toolName = "mcp"
			}
			m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", toolName)))
			m.recordToolCall(scope, toolName)
			if len(ev.Item.Arguments) > 0 {
				m.addScopedLine(scope, "  "+toolInputStyle.Render(truncate(string(ev.Item.Arguments), 200)))
			}
		case "file_change":
			m.addScopedLine(scope, toolLabelStyle.Render("[file]"))
			m.addSimplifiedLine(scope, dimStyle.Render("file change"))
		default:
			return true
		}
		return true

	case "turn.completed":
		var ev struct {
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		json.Unmarshal([]byte(line), &ev)
		summary := "done"
		if ev.Usage != nil {
			summary = fmt.Sprintf("in=%d out=%d", ev.Usage.InputTokens, ev.Usage.OutputTokens)
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		return true

	case "turn.failed":
		var ev struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal([]byte(line), &ev)
		msg := "failed"
		if ev.Error != nil && ev.Error.Message != "" {
			msg = truncate(ev.Error.Message, 200)
		}
		m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error] "+msg))
		m.addSimplifiedLine(scope, dimStyle.Render("error"))
		return true

	case "error":
		// Codex top-level error event.
		var ev struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal([]byte(line), &ev)
		msg := "unknown error"
		if ev.Error != nil && ev.Error.Message != "" {
			msg = truncate(ev.Error.Message, 200)
		}
		m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error] "+msg))
		m.addSimplifiedLine(scope, dimStyle.Render("error"))
		return true

	default:
		return false
	}
}

// renderGeminiStreamLine handles gemini JSON events from claude_stream recordings.
func (m *Model) renderGeminiStreamLine(scope, line, eventType string) bool {
	switch eventType {
	case "init":
		var ev struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		m.flushStream()
		m.currentBlockType = ""
		m.addScopedLine(scope, initLabelStyle.Render(fmt.Sprintf("[init] model=%s", ev.Model)))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		return true

	case "message":
		var ev struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			Delta   bool   `json:"delta"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		if ev.Role != "assistant" {
			// Skip user messages.
			return true
		}

		if ev.Delta {
			if m.currentBlockType != "text" {
				m.flushStream()
				m.currentBlockType = "text"
				m.addScopedLine(scope, textLabelStyle.Render("[text]"))
			}
			m.appendDelta(ev.Content)
			m.recordAssistantDelta(scope, ev.Content)
			return true
		}

		// Final message or non-streaming message.
		isNewBlock := m.currentBlockType != "text"
		m.flushStream()
		m.currentBlockType = ""

		if strings.TrimSpace(ev.Content) == "" {
			return true
		}
		if isNewBlock {
			m.addScopedLine(scope, textLabelStyle.Render("[text]"))
			m.addScopedLine(scope, "  "+textStyle.Render(truncate(ev.Content, 500)))
		}
		m.recordAssistantText(scope, ev.Content)
		return true

	case "tool_use":
		var ev struct {
			ToolName string `json:"tool_name"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		m.flushStream()
		m.currentBlockType = ""
		name := ev.ToolName
		if name == "" {
			name = "tool"
		}
		m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", name)))
		m.recordToolCall(scope, name)
		return true

	case "tool_result":
		m.flushStream()
		m.currentBlockType = ""
		m.markAssistantBoundary(scope)
		// Skip rendering tool results in detail raw text; they still define
		// the boundary for what counts as the final assistant message.
		return true

	case "result":
		m.flushStream()
		m.currentBlockType = ""
		var ev struct {
			Stats *struct {
				InputTokens  int     `json:"input_tokens"`
				OutputTokens int     `json:"output_tokens"`
				DurationMS   float64 `json:"duration_ms"`
			} `json:"stats"`
		}
		json.Unmarshal([]byte(line), &ev)
		summary := "done"
		if ev.Stats != nil {
			summary = fmt.Sprintf("in=%d out=%d", ev.Stats.InputTokens, ev.Stats.OutputTokens)
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		return true

	default:
		return false
	}
}

// renderClaudeStreamLine handles claude JSON events from claude_stream recordings.
func (m *Model) renderClaudeStreamLine(scope, line, eventType string) bool {
	switch eventType {
	case "system":
		var ev struct {
			Subtype string `json:"subtype"`
			Model   string `json:"model"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		if ev.Subtype == "init" {
			m.addScopedLine(scope, initLabelStyle.Render(fmt.Sprintf("[init] model=%s", ev.Model)))
			m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		}
		// Skip other system subtypes silently.
		return true

	case "assistant":
		var ev struct {
			Message *struct {
				Content []struct {
					Type  string          `json:"type"`
					Text  string          `json:"text"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Message == nil {
			return false
		}
		for _, block := range ev.Message.Content {
			switch block.Type {
			case "text":
				if strings.TrimSpace(block.Text) == "" {
					continue
				}
				text := block.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				m.addScopedLine(scope, textLabelStyle.Render("[text]"))
				m.addScopedLine(scope, "  "+textStyle.Render(text))
				m.recordAssistantText(scope, text)
			case "tool_use":
				name := block.Name
				if name == "" {
					name = "tool"
				}
				m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", name)))
				m.recordToolCall(scope, name)
				if len(block.Input) > 0 {
					m.renderToolInput(scope, name, string(block.Input))
				}
			case "thinking":
				if strings.TrimSpace(block.Text) == "" {
					continue
				}
				text := block.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
				m.addScopedLine(scope, "  "+thinkingTextStyle.Render(compactWhitespace(text)))
			}
		}
		return true

	case "user":
		// User events in Claude stream mode usually carry tool results; treat
		// them as the boundary for final-message capture.
		m.markAssistantBoundary(scope)
		return true

	case "content_block_start", "content_block_delta", "content_block_stop":
		// Skip partial streaming events — the full assistant message covers these.
		return true

	case "result":
		var ev struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
			DurationMS   float64 `json:"duration_ms"`
			NumTurns     int     `json:"num_turns"`
			Usage        *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		json.Unmarshal([]byte(line), &ev)
		var parts []string
		if ev.TotalCostUSD > 0 {
			parts = append(parts, fmt.Sprintf("cost=$%.4f", ev.TotalCostUSD))
		}
		if ev.DurationMS > 0 {
			parts = append(parts, fmt.Sprintf("duration=%.1fs", ev.DurationMS/1000))
		}
		if ev.NumTurns > 0 {
			parts = append(parts, fmt.Sprintf("turns=%d", ev.NumTurns))
		}
		if ev.Usage != nil {
			parts = append(parts, fmt.Sprintf("in=%d out=%d", ev.Usage.InputTokens, ev.Usage.OutputTokens))
		}
		summary := "done"
		if len(parts) > 0 {
			summary = strings.Join(parts, " ")
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		return true

	default:
		return false
	}
}
