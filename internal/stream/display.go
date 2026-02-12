package stream

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Display formats Claude stream events for terminal output.
type Display struct {
	w  io.Writer
	mu sync.Mutex

	needNewline bool
}

// NewDisplay creates a new Display that writes formatted output to w.
func NewDisplay(w io.Writer) *Display {
	return &Display{w: w}
}

// Handle formats and writes a single Claude event to the terminal.
func (d *Display) Handle(ev ClaudeEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch ev.Type {
	case "system":
		d.finishLine()
		switch ev.Subtype {
		case "init":
			model := ev.Model
			turnID := ev.TurnID
			if model != "" || turnID != "" {
				fmt.Fprintf(d.w, "\033[2m[init]\033[0m session=%s model=%s\n", turnID, model)
			}
		case "api_error":
			fmt.Fprintf(d.w, "\033[1;31m[api_error]\033[0m\n")
		default:
			// Other system subtypes (status, hook_started, hook_response,
			// informational, compact_boundary, etc.) are silently ignored
			// in the terminal display.
		}

	case "assistant":
		d.finishLine()
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				switch block.Type {
				case "text":
					text := block.Text
					if len(text) > 500 {
						text = text[:500] + "..."
					}
					fmt.Fprintf(d.w, "\033[1;36m[text]\033[0m %s\n", text)
				case "tool_use":
					name := block.Name
					inputStr := string(block.Input)
					if len(inputStr) > 100 {
						inputStr = inputStr[:100] + "..."
					}
					fmt.Fprintf(d.w, "\033[1;33m[tool:%s]\033[0m %s\n", name, inputStr)
				case "tool_result":
					// Tool results are embedded in subsequent assistant messages
					fmt.Fprintf(d.w, "\033[2m[tool_result]\033[0m\n")
				case "thinking":
					text := block.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					text = compactWhitespace(text)
					fmt.Fprintf(d.w, "\033[2m[thinking]\033[0m %s\n", text)
				}
			}
		}

	case "content_block_start":
		d.finishLine()
		if ev.ContentBlock != nil {
			switch ev.ContentBlock.Type {
			case "thinking":
				fmt.Fprintf(d.w, "\033[2m[thinking]\033[0m ")
				d.needNewline = true
			case "tool_use":
				fmt.Fprintf(d.w, "\033[1;33m[tool:%s]\033[0m ", ev.ContentBlock.Name)
				d.needNewline = true
			case "text":
				fmt.Fprintf(d.w, "\033[1;36m[text]\033[0m ")
				d.needNewline = true
			}
		}

	case "content_block_delta":
		if ev.Delta != nil {
			text := ev.Delta.Text
			if ev.Delta.PartialJSON != "" {
				text = ev.Delta.PartialJSON
			}
			if text != "" {
				fmt.Fprint(d.w, text)
				d.needNewline = true
			}
		}

	case "content_block_stop":
		d.finishLine()

	case "result":
		d.finishLine()
		var parts []string
		if ev.IsError {
			parts = append(parts, "ERROR")
		}
		if ev.Subtype != "" && ev.Subtype != "success" {
			parts = append(parts, ev.Subtype)
		}
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
			parts = append(parts, fmt.Sprintf("in=%d out=%d",
				ev.Usage.InputTokens, ev.Usage.OutputTokens))
		}
		color := "\033[1;32m" // green
		if ev.IsError {
			color = "\033[1;31m" // red
		}
		if len(parts) > 0 {
			fmt.Fprintf(d.w, "%s[result]\033[0m %s\n", color, strings.Join(parts, " "))
		} else {
			fmt.Fprintf(d.w, "%s[result]\033[0m done\n", color)
		}

	case "user":
		// User/tool result events — silently ignore in display.
		// The tool results are embedded in subsequent assistant messages.

	case "error":
		d.finishLine()
		msg := ev.ResultText
		if msg == "" {
			msg = "unknown error"
		}
		fmt.Fprintf(d.w, "\033[1;31m[error]\033[0m %s\n", msg)

	case "message":
		// message start/stop — ignore silently

	case "tool_use_summary", "tool_progress", "auth_status":
		// These are informational events — silently ignore in display.

	default:
		if ev.Type != "" {
			d.finishLine()
			fmt.Fprintf(d.w, "\033[2m[%s]\033[0m\n", ev.Type)
		}
	}
}

// finishLine writes a newline if the previous output didn't end with one.
func (d *Display) finishLine() {
	if d.needNewline {
		fmt.Fprintln(d.w)
		d.needNewline = false
	}
}

// Finish ensures any pending output is terminated with a newline.
func (d *Display) Finish() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.finishLine()
}

// compactWhitespace replaces runs of whitespace with a single space.
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
