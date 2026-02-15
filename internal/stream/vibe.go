package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

// VibeMessage represents a single NDJSON line from the Vibe CLI's
// --output streaming mode. Each line is a complete LLMMessage.
//
// Message roles and their semantics:
//   - "user":      the user's prompt
//   - "assistant": model response, may contain tool_calls
//   - "tool":      tool result, has name and tool_call_id
//   - "system":    system prompt (usually skipped)
type VibeMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []VibeToolCall `json:"tool_calls,omitempty"`
	Name             string         `json:"name,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	MessageID        string         `json:"message_id,omitempty"`
}

// VibeToolCall represents a tool invocation inside an assistant message.
type VibeToolCall struct {
	ID       string           `json:"id,omitempty"`
	Index    int              `json:"index,omitempty"`
	Function VibeFunctionCall `json:"function"`
	Type     string           `json:"type,omitempty"`
}

// VibeFunctionCall holds the function name and JSON-encoded arguments.
type VibeFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// VibeToClaudeEvent converts a Vibe streaming message into the common
// ClaudeEvent format used by terminal display and event stream consumers.
func VibeToClaudeEvent(vm VibeMessage) ([]ClaudeEvent, bool) {
	switch vm.Role {
	case "assistant":
		var events []ClaudeEvent

		// Emit thinking block if reasoning_content is present.
		if vm.ReasoningContent != "" {
			events = append(events, ClaudeEvent{
				Type: "assistant",
				AssistantMessage: &AssistantMessage{
					Role: "assistant",
					Content: []ContentBlock{
						{Type: "thinking", Text: vm.ReasoningContent},
					},
				},
			})
		}

		// Build content blocks for text and tool calls.
		var blocks []ContentBlock
		if vm.Content != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: vm.Content})
		}
		for _, tc := range vm.ToolCalls {
			var inputRaw json.RawMessage
			if tc.Function.Arguments != "" {
				inputRaw = json.RawMessage(tc.Function.Arguments)
			}
			blocks = append(blocks, ContentBlock{
				Type:  "tool_use",
				Name:  tc.Function.Name,
				ID:    tc.ID,
				Input: inputRaw,
			})
		}
		if len(blocks) > 0 {
			events = append(events, ClaudeEvent{
				Type: "assistant",
				AssistantMessage: &AssistantMessage{
					Role:    "assistant",
					Content: blocks,
				},
			})
		}

		if len(events) == 0 {
			return nil, false
		}
		return events, true

	case "tool":
		content, _ := json.Marshal(vm.Content)
		return []ClaudeEvent{{
			Type: "user",
			AssistantMessage: &AssistantMessage{
				Role: "user",
				Content: []ContentBlock{
					{
						Type:        "tool_result",
						Name:        vm.Name,
						ToolUseID:   vm.ToolCallID,
						ToolContent: content,
						IsError:     false,
					},
				},
			},
		}}, true

	default:
		// Skip user and system messages.
		return nil, false
	}
}

// ParseVibe reads NDJSON lines from r, parses each as a VibeMessage,
// converts to ClaudeEvent(s) via VibeToClaudeEvent, and sends on the
// returned channel. The channel is closed when the reader reaches EOF
// or the context is cancelled.
func ParseVibe(ctx context.Context, r io.Reader) <-chan RawEvent {
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

			var vm VibeMessage
			if err := json.Unmarshal(raw, &vm); err != nil {
				offerRawEvent(ctx, ch, RawEvent{Raw: raw, Err: err}, "vibe")
				continue
			}

			events, ok := VibeToClaudeEvent(vm)
			if !ok {
				// Still record the raw line even if we skip the message.
				offerRawEvent(ctx, ch, RawEvent{Raw: raw}, "vibe")
				continue
			}

			for _, ce := range events {
				offerRawEvent(ctx, ch, RawEvent{Raw: raw, Parsed: ce}, "vibe")
			}
		}

		if err := scanner.Err(); err != nil {
			offerRawEvent(ctx, ch, RawEvent{Err: err}, "vibe")
		}
	}()
	return ch
}
