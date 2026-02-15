package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

// GeminiEvent represents a single event from the Gemini CLI's
// --output-format stream-json NDJSON output.
//
// Event types and their fields (from the Gemini CLI source):
//   - init:        session_id, model
//   - message:     role ("user"|"assistant"), content, delta (bool)
//   - tool_use:    tool_name, tool_id, parameters
//   - tool_result: tool_id, status ("success"|"error"), output, error
//   - error:       severity ("warning"|"error"), message
//   - result:      status ("success"|"error"), error, stats
type GeminiEvent struct {
	Type       string             `json:"type"`
	Timestamp  string             `json:"timestamp,omitempty"`
	TurnID     string             `json:"session_id,omitempty"`
	Model      string             `json:"model,omitempty"`
	Role       string             `json:"role,omitempty"`
	Content    string             `json:"content,omitempty"`
	Delta      bool               `json:"delta,omitempty"`
	ToolName   string             `json:"tool_name,omitempty"`
	ToolID     string             `json:"tool_id,omitempty"`
	Parameters json.RawMessage    `json:"parameters,omitempty"`
	Status     string             `json:"status,omitempty"`
	Output     string             `json:"output,omitempty"`
	Message    string             `json:"message,omitempty"`
	Severity   string             `json:"severity,omitempty"`
	Error      *GeminiErrorInfo   `json:"error,omitempty"`
	Stats      *GeminiStreamStats `json:"stats,omitempty"`
}

// GeminiErrorInfo holds error details from tool_result or result events.
type GeminiErrorInfo struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

// GeminiStreamStats holds usage/performance statistics from a Gemini result event.
type GeminiStreamStats struct {
	TotalTokens  int     `json:"total_tokens,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	Cached       int     `json:"cached,omitempty"`
	Input        int     `json:"input,omitempty"`
	DurationMS   float64 `json:"duration_ms,omitempty"`
	ToolCalls    int     `json:"tool_calls,omitempty"`
}

// GeminiToClaudeEvent converts a Gemini stream event into the common ClaudeEvent
// format used by terminal display and event stream consumers.
func GeminiToClaudeEvent(ge GeminiEvent) (ClaudeEvent, bool) {
	switch ge.Type {
	case "init":
		return ClaudeEvent{
			Type:    "system",
			Subtype: "init",
			TurnID:  ge.TurnID,
			Model:   ge.Model,
		}, true

	case "message":
		if ge.Role != "assistant" {
			// Skip user messages.
			return ClaudeEvent{}, false
		}

		if ge.Delta {
			// Streaming text delta (most common assistant event).
			return ClaudeEvent{
				Type: "content_block_delta",
				Delta: &Delta{
					Type: "text_delta",
					Text: ge.Content,
				},
			}, true
		}

		// Full text message (non-delta).
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
		output := ge.Output
		isError := ge.Status == "error"
		if isError && ge.Error != nil && ge.Error.Message != "" {
			output = ge.Error.Message
		}
		content, _ := json.Marshal(output)
		return ClaudeEvent{
			Type: "user",
			AssistantMessage: &AssistantMessage{
				Role: "user",
				Content: []ContentBlock{
					{
						Type:        "tool_result",
						ToolUseID:   ge.ToolID,
						ToolContent: content,
						IsError:     isError,
					},
				},
			},
		}, true

	case "result":
		ev := ClaudeEvent{
			Type: "result",
		}
		if ge.Status == "error" && ge.Error != nil {
			ev.IsError = true
			ev.ResultText = ge.Error.Message
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
		// The Gemini error event has severity ("warning"|"error") and message fields.
		// Render as a system event with the message for Display.Handle.
		ev := ClaudeEvent{
			Type: "error",
		}
		// Stash the error message in ResultText so Display can show it.
		if ge.Message != "" {
			ev.ResultText = ge.Message
		}
		return ev, true

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
				offerRawEvent(ctx, ch, RawEvent{Raw: raw, Err: err}, "gemini")
				continue
			}

			ce, ok := GeminiToClaudeEvent(ge)
			if !ok {
				// Still record the raw line even if we skip the event.
				offerRawEvent(ctx, ch, RawEvent{Raw: raw}, "gemini")
				continue
			}

			offerRawEvent(ctx, ch, RawEvent{Raw: raw, Parsed: ce}, "gemini")
		}

		if err := scanner.Err(); err != nil {
			offerRawEvent(ctx, ch, RawEvent{Err: err}, "gemini")
		}
	}()
	return ch
}
