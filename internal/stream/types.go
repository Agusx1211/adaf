package stream

import "encoding/json"

// RawEvent holds both the raw NDJSON line and the parsed event.
type RawEvent struct {
	Raw    []byte
	Parsed ClaudeEvent
	Err    error
}

// ClaudeEvent is the top-level structure for a Claude stream-json event.
//
// In stream-json mode, the Claude CLI emits these top-level event types:
//   - "system"    (subtypes: "init", "status", "hook_started", "hook_progress",
//     "hook_response", "api_error", "informational", "compact_boundary",
//     "task_notification", "turn_duration", "stop_hook_summary")
//   - "assistant" — a message from the assistant with content blocks
//   - "user"      — a tool result or user message
//   - "result"    (subtypes: "success", "error_during_execution",
//     "error_max_turns", "error_max_budget_usd",
//     "error_max_structured_output_retries")
//
// Note: content_block_start/delta/stop are Anthropic API-level events that
// only appear inside "stream_event" wrappers, which the CLI filters out in
// stream-json mode unless --include-partial-messages is set.
type ClaudeEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`

	// Common field across most events.
	SessionID string `json:"session_id,omitempty"`

	// For system/init events (subtype "init").
	Model          string   `json:"model,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	CWD            string   `json:"cwd,omitempty"`
	PermissionMode string   `json:"permissionMode,omitempty"`

	// For assistant and user events: the full message payload.
	AssistantMessage *AssistantMessage `json:"message,omitempty"`

	// For assistant events: parent_tool_use_id (non-empty for sub-agent messages).
	ParentToolUseID *string `json:"parent_tool_use_id,omitempty"`

	// For content_block_start / content_block_delta / content_block_stop
	// (only present when --include-partial-messages is used).
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Delta        *Delta        `json:"delta,omitempty"`

	// For result events.
	TotalCostUSD float64        `json:"total_cost_usd,omitempty"`
	DurationMS   float64        `json:"duration_ms,omitempty"`
	DurationAPMS float64        `json:"duration_api_ms,omitempty"`
	IsError      bool           `json:"is_error,omitempty"`
	NumTurns     int            `json:"num_turns,omitempty"`
	ResultText   string         `json:"result,omitempty"`
	StopReason   *string        `json:"stop_reason,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
	Errors       []string       `json:"errors,omitempty"`
}

// AssistantMessage is the message payload inside an "assistant" event.
type AssistantMessage struct {
	ID      string         `json:"id,omitempty"`
	Model   string         `json:"model,omitempty"`
	Role    string         `json:"role,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
	Usage   *Usage         `json:"usage,omitempty"`
}

// ContentBlock represents a content block within a message.
type ContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	ID    string          `json:"id,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result content blocks (inside "user" events).
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	ToolContent json.RawMessage `json:"content,omitempty"`
	IsError     bool            `json:"is_error,omitempty"`
}

// ToolContentText extracts the text content from a tool_result block.
// ToolContent can be a JSON string or an array of content blocks.
func (cb ContentBlock) ToolContentText() string {
	if len(cb.ToolContent) == 0 {
		return ""
	}

	// Try as a plain JSON string first.
	var s string
	if err := json.Unmarshal(cb.ToolContent, &s); err == nil {
		return s
	}

	// Try as an array of {type, text} blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(cb.ToolContent, &blocks); err == nil {
		var out string
		for _, b := range blocks {
			if b.Text != "" {
				out += b.Text
			}
		}
		return out
	}

	return string(cb.ToolContent)
}

// Delta represents incremental updates within a content block.
type Delta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// Usage holds token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}
