package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

// opencodeEvent is the top-level JSON object emitted by
// `opencode run --format json`. Every line carries the sessionID.
type opencodeEvent struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp,omitempty"`
	SessionID string          `json:"sessionID,omitempty"`
	Part      json.RawMessage `json:"part,omitempty"`
	Error     *opencodeError  `json:"error,omitempty"`
}

type opencodeError struct {
	Name string          `json:"name,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type opencodePart struct {
	ID        string          `json:"id,omitempty"`
	SessionID string          `json:"sessionID,omitempty"`
	MessageID string          `json:"messageID,omitempty"`
	Type      string          `json:"type,omitempty"`
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	CallID    string          `json:"callID,omitempty"`
	Reason    string          `json:"reason,omitempty"`
	State     *opencodeState  `json:"state,omitempty"`
	Cost      float64         `json:"cost,omitempty"`
	Tokens    *opencodeTokens `json:"tokens,omitempty"`
}

type opencodeState struct {
	Status string          `json:"status,omitempty"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output string          `json:"output,omitempty"`
	Title  string          `json:"title,omitempty"`
}

type opencodeTokens struct {
	Input     int            `json:"input,omitempty"`
	Output    int            `json:"output,omitempty"`
	Reasoning int            `json:"reasoning,omitempty"`
	Cache     *opencodeCache `json:"cache,omitempty"`
}

type opencodeCache struct {
	Read  int `json:"read,omitempty"`
	Write int `json:"write,omitempty"`
}

// ParseOpencode reads NDJSON lines from `opencode run --format json` and maps
// them to ClaudeEvent so existing display layers can format them consistently
// with other agents.
func ParseOpencode(ctx context.Context, r io.Reader) <-chan RawEvent {
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
			if len(raw) == 0 {
				continue
			}

			parsed, ok, err := parseOpencodeLine(raw)
			if err != nil {
				offerRawEvent(ctx, ch, RawEvent{Raw: raw, Err: err}, "opencode")
				continue
			}
			if !ok {
				offerRawEvent(ctx, ch, RawEvent{Raw: raw}, "opencode")
				continue
			}
			offerRawEvent(ctx, ch, RawEvent{Raw: raw, Parsed: parsed}, "opencode")
		}

		if err := scanner.Err(); err != nil {
			offerRawEvent(ctx, ch, RawEvent{Err: err}, "opencode")
		}
	}()
	return ch
}

func parseOpencodeLine(raw []byte) (ClaudeEvent, bool, error) {
	var ev opencodeEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ClaudeEvent{}, false, err
	}

	switch ev.Type {
	case "text":
		return parseOpencodeText(ev)
	case "reasoning":
		return parseOpencodeReasoning(ev)
	case "tool_use":
		return parseOpencodeToolUse(ev)
	case "step_start":
		// Map to a system init-like event so the session ID is captured.
		return ClaudeEvent{
			Type:    "system",
			Subtype: "init",
			TurnID:  ev.SessionID,
		}, true, nil
	case "step_finish":
		return parseOpencodeStepFinish(ev)
	case "error":
		return parseOpencodeError(ev)
	default:
		return ClaudeEvent{}, false, nil
	}
}

func parseOpencodeText(ev opencodeEvent) (ClaudeEvent, bool, error) {
	var part opencodePart
	if err := json.Unmarshal(ev.Part, &part); err != nil {
		return ClaudeEvent{}, false, err
	}
	if strings.TrimSpace(part.Text) == "" {
		return ClaudeEvent{}, false, nil
	}
	return ClaudeEvent{
		Type:   "assistant",
		TurnID: ev.SessionID,
		AssistantMessage: &AssistantMessage{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "text", Text: part.Text},
			},
		},
	}, true, nil
}

func parseOpencodeReasoning(ev opencodeEvent) (ClaudeEvent, bool, error) {
	var part opencodePart
	if err := json.Unmarshal(ev.Part, &part); err != nil {
		return ClaudeEvent{}, false, err
	}
	if strings.TrimSpace(part.Text) == "" {
		return ClaudeEvent{}, false, nil
	}
	return ClaudeEvent{
		Type:   "assistant",
		TurnID: ev.SessionID,
		AssistantMessage: &AssistantMessage{
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "thinking", Text: part.Text},
			},
		},
	}, true, nil
}

func parseOpencodeToolUse(ev opencodeEvent) (ClaudeEvent, bool, error) {
	var part opencodePart
	if err := json.Unmarshal(ev.Part, &part); err != nil {
		return ClaudeEvent{}, false, err
	}

	toolName := part.Tool
	if toolName == "" {
		toolName = "tool"
	}

	// Emit both the tool_use and the tool_result since OpenCode only emits
	// tool_use when the tool is already completed.
	id := part.CallID
	if id == "" {
		id = part.ID
	}

	input := []byte("{}")
	if part.State != nil && len(part.State.Input) > 0 {
		input = part.State.Input
	}

	isError := false
	output := ""
	if part.State != nil {
		output = part.State.Output
		isError = part.State.Status == "error"
	}

	// Emit as assistant with tool_use + user with tool_result.
	return ClaudeEvent{
		Type:   "assistant",
		TurnID: ev.SessionID,
		AssistantMessage: &AssistantMessage{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type:  "tool_use",
					Name:  toolName,
					ID:    id,
					Input: input,
				},
				{
					Type:        "tool_result",
					ToolUseID:   id,
					ToolContent: marshalString(output),
					IsError:     isError,
				},
			},
		},
	}, true, nil
}

func parseOpencodeStepFinish(ev opencodeEvent) (ClaudeEvent, bool, error) {
	var part opencodePart
	if err := json.Unmarshal(ev.Part, &part); err != nil {
		return ClaudeEvent{}, false, err
	}

	usage := &Usage{}
	if part.Tokens != nil {
		usage.InputTokens = part.Tokens.Input
		usage.OutputTokens = part.Tokens.Output
		if part.Tokens.Cache != nil {
			usage.CacheReadInputTokens = part.Tokens.Cache.Read
			usage.CacheCreationInputTokens = part.Tokens.Cache.Write
		}
	}

	return ClaudeEvent{
		Type:         "result",
		Subtype:      "success",
		TurnID:       ev.SessionID,
		TotalCostUSD: part.Cost,
		Usage:        usage,
		NumTurns:     1,
	}, true, nil
}

func parseOpencodeError(ev opencodeEvent) (ClaudeEvent, bool, error) {
	msg := "unknown error"
	if ev.Error != nil {
		if ev.Error.Name != "" {
			msg = ev.Error.Name
		}
		if len(ev.Error.Data) > 0 {
			var data struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(ev.Error.Data, &data) == nil && data.Message != "" {
				msg = data.Message
			}
		}
	}
	return ClaudeEvent{
		Type:       "result",
		Subtype:    "error_during_execution",
		TurnID:     ev.SessionID,
		IsError:    true,
		ResultText: msg,
	}, true, nil
}

func marshalString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}
