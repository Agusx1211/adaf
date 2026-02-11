package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

// GeminiEvent represents a single event from the Gemini CLI's
// --output-format stream-json NDJSON output.
type GeminiEvent struct {
	Type      string            `json:"type"`
	Timestamp string            `json:"timestamp,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	Model     string            `json:"model,omitempty"`
	Role      string            `json:"role,omitempty"`
	Content   string            `json:"content,omitempty"`
	Thought   bool              `json:"thought,omitempty"`
	Delta     bool              `json:"delta,omitempty"`
	ToolName  string            `json:"tool_name,omitempty"`
	ToolID    string            `json:"tool_id,omitempty"`
	Parameters json.RawMessage  `json:"parameters,omitempty"`
	Status    string            `json:"status,omitempty"`
	Output    string            `json:"output,omitempty"`
	Message   string            `json:"message,omitempty"`
	Stats     *GeminiStreamStats `json:"stats,omitempty"`
}

// GeminiStreamStats holds usage/performance statistics from a Gemini result event.
type GeminiStreamStats struct {
	TotalTokens  int     `json:"total_tokens,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	DurationMS   float64 `json:"duration_ms,omitempty"`
	ToolCalls    int     `json:"tool_calls,omitempty"`
}

// GeminiToClaudeEvent converts a Gemini stream event into the common ClaudeEvent
// format used by the Display and TUI layers.
func GeminiToClaudeEvent(ge GeminiEvent) (ClaudeEvent, bool) {
	switch ge.Type {
	case "init":
		return ClaudeEvent{
			Type:      "system",
			Subtype:   "init",
			SessionID: ge.SessionID,
			Model:     ge.Model,
		}, true

	case "message":
		if ge.Role != "assistant" {
			// Skip user/tool_result messages.
			return ClaudeEvent{}, false
		}

		if ge.Thought {
			// Thinking content block.
			return ClaudeEvent{
				Type: "assistant",
				AssistantMessage: &AssistantMessage{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "thinking", Text: ge.Content},
					},
				},
			}, true
		}

		if ge.Delta {
			// Streaming text delta.
			return ClaudeEvent{
				Type: "content_block_delta",
				Delta: &Delta{
					Type: "text_delta",
					Text: ge.Content,
				},
			}, true
		}

		// Full text message.
		return ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &AssistantMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "text", Text: ge.Content},
				},
			},
		}, true

	case "tool_use":
		return ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &AssistantMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type:  "tool_use",
						Name:  ge.ToolName,
						ID:    ge.ToolID,
						Input: ge.Parameters,
					},
				},
			},
		}, true

	case "tool_result":
		// Tool results are user-side events; skip for display purposes.
		return ClaudeEvent{}, false

	case "result":
		ev := ClaudeEvent{
			Type: "result",
		}
		if ge.Stats != nil {
			ev.DurationMS = ge.Stats.DurationMS
			ev.Usage = &Usage{
				InputTokens:  ge.Stats.InputTokens,
				OutputTokens: ge.Stats.OutputTokens,
			}
		}
		return ev, true

	case "error":
		// Render as a generic event so Display.Handle prints it.
		return ClaudeEvent{
			Type: "error",
		}, true

	default:
		return ClaudeEvent{}, false
	}
}

// ParseGemini reads NDJSON lines from r, parses each as a GeminiEvent,
// converts to ClaudeEvent via GeminiToClaudeEvent, and sends on the returned
// channel. The channel is closed when the reader reaches EOF or the context
// is cancelled.
func ParseGemini(ctx context.Context, r io.Reader) <-chan RawEvent {
	ch := make(chan RawEvent, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, maxLineSize), maxLineSize)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			raw := make([]byte, len(line))
			copy(raw, line)

			var ge GeminiEvent
			if err := json.Unmarshal(raw, &ge); err != nil {
				ch <- RawEvent{Raw: raw, Err: err}
				continue
			}

			ce, ok := GeminiToClaudeEvent(ge)
			if !ok {
				// Still record the raw line even if we skip the event.
				ch <- RawEvent{Raw: raw}
				continue
			}

			ch <- RawEvent{Raw: raw, Parsed: ce}
		}

		if err := scanner.Err(); err != nil {
			ch <- RawEvent{Err: err}
		}
	}()
	return ch
}
