package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var codexMarshalJSON = json.Marshal

type codexEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id,omitempty"`
	Usage    *codexUsage `json:"usage,omitempty"`
	Error    *codexError `json:"error,omitempty"`
	Item     *codexItem  `json:"item,omitempty"`
}

type codexUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

type codexError struct {
	Message string `json:"message"`
}

type codexItem struct {
	ID               string            `json:"id,omitempty"`
	Type             string            `json:"type,omitempty"`
	Text             string            `json:"text,omitempty"`
	Command          string            `json:"command,omitempty"`
	AggregatedOutput string            `json:"aggregated_output,omitempty"`
	ExitCode         *int              `json:"exit_code,omitempty"`
	Status           string            `json:"status,omitempty"`
	Server           string            `json:"server,omitempty"`
	Tool             string            `json:"tool,omitempty"`
	Arguments        json.RawMessage   `json:"arguments,omitempty"`
	Result           json.RawMessage   `json:"result,omitempty"`
	Error            *codexError       `json:"error,omitempty"`
	Changes          []codexFileChange `json:"changes,omitempty"`
	Items            []codexTodoItem   `json:"items,omitempty"`
	Query            string            `json:"query,omitempty"`
}

type codexFileChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type codexTodoItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// ParseCodex reads Codex --json JSONL output and maps known events to
// ClaudeEvent so existing display layers can format it consistently.
func ParseCodex(ctx context.Context, r io.Reader) <-chan RawEvent {
	ch := make(chan RawEvent, 64)
	go func() {
		defer close(ch)

		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			raw := append([]byte(nil), scanner.Bytes()...)

			parsed, ok, err := parseCodexLine(raw)
			if err != nil {
				offerRawEvent(ctx, ch, RawEvent{Raw: raw, Err: err}, "codex")
				continue
			}
			if !ok {
				offerRawEvent(ctx, ch, RawEvent{Raw: raw}, "codex")
				continue
			}
			offerRawEvent(ctx, ch, RawEvent{Raw: raw, Parsed: parsed}, "codex")
		}

		if err := scanner.Err(); err != nil {
			offerRawEvent(ctx, ch, RawEvent{Err: err}, "codex")
		}
	}()
	return ch
}

func parseCodexLine(raw []byte) (ClaudeEvent, bool, error) {
	var ev codexEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ClaudeEvent{}, false, err
	}

	switch ev.Type {
	case "thread.started":
		return ClaudeEvent{
			Type:    "system",
			Subtype: "init",
			TurnID:  ev.ThreadID,
		}, true, nil
	case "turn.completed":
		usage := &Usage{}
		if ev.Usage != nil {
			usage = &Usage{
				InputTokens:              ev.Usage.InputTokens,
				OutputTokens:             ev.Usage.OutputTokens,
				CacheReadInputTokens:     ev.Usage.CachedInputTokens,
				CacheCreationInputTokens: 0,
			}
		}
		return ClaudeEvent{
			Type:     "result",
			Subtype:  "success",
			Usage:    usage,
			NumTurns: 1,
		}, true, nil
	case "turn.failed":
		msg := ""
		if ev.Error != nil {
			msg = ev.Error.Message
		}
		return ClaudeEvent{
			Type:       "result",
			Subtype:    "error_during_execution",
			IsError:    true,
			ResultText: msg,
		}, true, nil
	case "error":
		msg := "unknown error"
		if ev.Error != nil && ev.Error.Message != "" {
			msg = ev.Error.Message
		}
		return ClaudeEvent{
			Type:       "error",
			ResultText: msg,
		}, true, nil
	case "item.started", "item.updated", "item.completed":
		if ev.Item == nil {
			return ClaudeEvent{}, false, nil
		}
		mapped, ok, err := codexItemToClaude(ev.Type, *ev.Item)
		if err != nil {
			return ClaudeEvent{}, false, err
		}
		return mapped, ok, nil
	default:
		return ClaudeEvent{}, false, nil
	}
}

func codexItemToClaude(eventType string, item codexItem) (ClaudeEvent, bool, error) {
	switch item.Type {
	case "agent_message":
		if strings.TrimSpace(item.Text) == "" {
			return ClaudeEvent{}, false, nil
		}
		return ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &AssistantMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: item.Text,
					},
				},
			},
		}, true, nil
	case "reasoning":
		if strings.TrimSpace(item.Text) == "" {
			return ClaudeEvent{}, false, nil
		}
		return ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &AssistantMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "thinking",
						Text: item.Text,
					},
				},
			},
		}, true, nil
	case "command_execution":
		return codexCommandExecutionToClaude(eventType, item)
	case "mcp_tool_call":
		return codexMCPToolToClaude(eventType, item)
	case "file_change":
		summary := codexFileChangeSummary(item)
		if summary == "" {
			return ClaudeEvent{}, false, nil
		}
		return ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &AssistantMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "text",
						Text: summary,
					},
				},
			},
		}, true, nil
	case "todo_list":
		if len(item.Items) == 0 {
			return ClaudeEvent{}, false, nil
		}
		var b strings.Builder
		for _, todo := range item.Items {
			if todo.Text == "" {
				continue
			}
			check := " "
			if todo.Completed {
				check = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", check, todo.Text)
		}
		text := strings.TrimSpace(b.String())
		if text == "" {
			return ClaudeEvent{}, false, nil
		}
		return ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &AssistantMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{
						Type: "thinking",
						Text: text,
					},
				},
			},
		}, true, nil
	case "web_search":
		if eventType == "item.started" {
			input := `{}`
			if strings.TrimSpace(item.Query) != "" {
				input = fmt.Sprintf(`{"query":%q}`, item.Query)
			}
			return codexToolUseEvent(item.ID, "web_search", []byte(input)), true, nil
		}
		resultText := item.Query
		if resultText == "" {
			resultText = "web search completed"
		}
		return codexToolResultEvent(item.ID, false, []byte(fmt.Sprintf("%q", resultText))), true, nil
	case "error":
		msg := "unknown error"
		if item.Error != nil && strings.TrimSpace(item.Error.Message) != "" {
			msg = item.Error.Message
		}
		return ClaudeEvent{
			Type:       "error",
			ResultText: msg,
		}, true, nil
	default:
		return ClaudeEvent{}, false, nil
	}
}

func codexCommandExecutionToClaude(eventType string, item codexItem) (ClaudeEvent, bool, error) {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	isStart := eventType == "item.started" || status == "" || status == "in_progress"
	if isStart {
		inputJSON, err := codexMarshalJSON(map[string]string{
			"command": item.Command,
		})
		if err != nil {
			return ClaudeEvent{}, false, fmt.Errorf("marshal command_execution input: %w", err)
		}
		return codexToolUseEvent(item.ID, "Bash", inputJSON), true, nil
	}

	isError := status == "failed" || status == "declined" || (item.ExitCode != nil && *item.ExitCode != 0)
	text := item.AggregatedOutput
	if strings.TrimSpace(text) == "" {
		text = strings.TrimSpace(item.Command)
	}
	if text == "" {
		if item.ExitCode != nil {
			text = fmt.Sprintf("command finished (exit=%d)", *item.ExitCode)
		} else {
			text = "command finished"
		}
	}
	content, err := codexMarshalJSON(text)
	if err != nil {
		return ClaudeEvent{}, false, fmt.Errorf("marshal command_execution result: %w", err)
	}
	return codexToolResultEvent(item.ID, isError, content), true, nil
}

func codexMCPToolToClaude(eventType string, item codexItem) (ClaudeEvent, bool, error) {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	isStart := eventType == "item.started" || status == "" || status == "in_progress"
	toolName := strings.Trim(strings.Join([]string{item.Server, item.Tool}, "."), ".")
	if toolName == "" {
		toolName = "mcp"
	}

	if isStart {
		input := item.Arguments
		if len(input) == 0 {
			input = []byte("{}")
		}
		return codexToolUseEvent(item.ID, toolName, input), true, nil
	}

	isError := status == "failed"
	if item.Error != nil && strings.TrimSpace(item.Error.Message) != "" {
		isError = true
		msgJSON, err := codexMarshalJSON(item.Error.Message)
		if err != nil {
			return ClaudeEvent{}, false, fmt.Errorf("marshal mcp_tool_call error result: %w", err)
		}
		return codexToolResultEvent(item.ID, true, msgJSON), true, nil
	}
	if len(item.Result) > 0 {
		return codexToolResultEvent(item.ID, isError, item.Result), true, nil
	}
	okJSON, err := codexMarshalJSON("ok")
	if err != nil {
		return ClaudeEvent{}, false, fmt.Errorf("marshal mcp_tool_call default result: %w", err)
	}
	return codexToolResultEvent(item.ID, isError, okJSON), true, nil
}

func codexToolUseEvent(id, name string, input []byte) ClaudeEvent {
	if len(input) == 0 {
		input = []byte("{}")
	}
	return ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &AssistantMessage{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type:  "tool_use",
					Name:  name,
					ID:    id,
					Input: input,
				},
			},
		},
	}
}

func codexToolResultEvent(toolUseID string, isError bool, content []byte) ClaudeEvent {
	if len(content) == 0 {
		content = []byte(`""`)
	}
	return ClaudeEvent{
		Type: "user",
		AssistantMessage: &AssistantMessage{
			Role: "user",
			Content: []ContentBlock{
				{
					Type:        "tool_result",
					ToolUseID:   toolUseID,
					ToolContent: content,
					IsError:     isError,
				},
			},
		},
	}
}

func codexFileChangeSummary(item codexItem) string {
	if len(item.Changes) == 0 {
		if strings.EqualFold(item.Status, "failed") {
			return "File changes failed."
		}
		return "File changes completed."
	}
	parts := make([]string, 0, len(item.Changes))
	for _, ch := range item.Changes {
		if ch.Path == "" {
			continue
		}
		if ch.Kind == "" {
			parts = append(parts, ch.Path)
			continue
		}
		parts = append(parts, ch.Kind+" "+ch.Path)
	}
	if len(parts) == 0 {
		return ""
	}
	summary := strings.Join(parts, ", ")
	if len(summary) > 500 {
		summary = summary[:500] + "..."
	}
	if strings.EqualFold(item.Status, "failed") {
		return "File changes failed: " + summary
	}
	return "File changes: " + summary
}
