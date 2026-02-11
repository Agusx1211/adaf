package stream

import "encoding/json"

// RawEvent holds both the raw NDJSON line and the parsed event.
type RawEvent struct {
	Raw    []byte
	Parsed ClaudeEvent
	Err    error
}

// ClaudeEvent is the top-level structure for a Claude stream-json event.
type ClaudeEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`

	// For system/init events
	SessionID string `json:"session_id,omitempty"`
	Model     string `json:"model,omitempty"`
	Tools     []string `json:"tools,omitempty"`

	// For assistant events: the full message payload
	AssistantMessage *AssistantMessage `json:"message,omitempty"`

	// For content_block_start / content_block_delta / content_block_stop
	// (may appear in future streaming modes)
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Delta        *Delta        `json:"delta,omitempty"`

	// For result events (top-level fields)
	TotalCostUSD float64        `json:"total_cost_usd,omitempty"`
	DurationMS   float64        `json:"duration_ms,omitempty"`
	IsError      bool           `json:"is_error,omitempty"`
	NumTurns     int            `json:"num_turns,omitempty"`
	ResultText   string         `json:"result,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
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
