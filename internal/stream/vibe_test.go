package stream

import (
	"context"
	"strings"
	"testing"
)

const testVibeStream = `{"role":"user","content":"list files"}
{"role":"assistant","content":"Let me check.","tool_calls":[{"id":"call_1","index":0,"function":{"name":"bash","arguments":"{\"command\":\"ls -la\"}"},"type":"function"}]}
{"role":"tool","content":"file1.go\nfile2.go","name":"bash","tool_call_id":"call_1"}
{"role":"assistant","content":"Here are the files."}
`

func TestParseVibe(t *testing.T) {
	ch := ParseVibe(context.Background(), strings.NewReader(testVibeStream))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	// 4 raw lines: user(skipped,raw only) + assistant(text+tool_use) + tool(result) + assistant(text)
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// First event: user message — skipped (no Parsed.Type).
	if events[0].Parsed.Type != "" {
		t.Errorf("user message should be skipped, got type = %q", events[0].Parsed.Type)
	}

	// Second event: assistant with text + tool_use content blocks.
	if events[1].Parsed.Type != "assistant" {
		t.Errorf("expected assistant event, got %q", events[1].Parsed.Type)
	}

	// Third event: tool result.
	if events[2].Parsed.Type != "user" {
		t.Errorf("expected user (tool_result) event, got %q", events[2].Parsed.Type)
	}

	// Fourth event: final assistant text.
	if events[3].Parsed.Type != "assistant" {
		t.Errorf("expected assistant event, got %q", events[3].Parsed.Type)
	}
}

func TestParseVibeDetailed(t *testing.T) {
	ch := ParseVibe(context.Background(), strings.NewReader(testVibeStream))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	// Verify event types
	var types []string
	for _, ev := range events {
		types = append(types, ev.Parsed.Type)
	}
	t.Logf("event types: %v", types)

	// Find assistant events with tool_use
	foundToolUse := false
	foundToolResult := false
	foundText := false
	for _, ev := range events {
		if ev.Parsed.Type == "assistant" && ev.Parsed.AssistantMessage != nil {
			for _, block := range ev.Parsed.AssistantMessage.Content {
				if block.Type == "tool_use" {
					foundToolUse = true
					if block.Name != "bash" {
						t.Errorf("tool name = %q, want %q", block.Name, "bash")
					}
					if block.ID != "call_1" {
						t.Errorf("tool ID = %q, want %q", block.ID, "call_1")
					}
				}
				if block.Type == "text" && block.Text == "Here are the files." {
					foundText = true
				}
			}
		}
		if ev.Parsed.Type == "user" && ev.Parsed.AssistantMessage != nil {
			for _, block := range ev.Parsed.AssistantMessage.Content {
				if block.Type == "tool_result" {
					foundToolResult = true
					if block.Name != "bash" {
						t.Errorf("tool_result name = %q, want %q", block.Name, "bash")
					}
					if block.ToolUseID != "call_1" {
						t.Errorf("tool_result tool_call_id = %q, want %q", block.ToolUseID, "call_1")
					}
				}
			}
		}
	}

	if !foundToolUse {
		t.Error("did not find tool_use event")
	}
	if !foundToolResult {
		t.Error("did not find tool_result event")
	}
	if !foundText {
		t.Error("did not find final text event")
	}
}

func TestVibeToClaudeEvent_Reasoning(t *testing.T) {
	vm := VibeMessage{
		Role:             "assistant",
		Content:          "The answer is 42.",
		ReasoningContent: "Let me think step by step...",
	}

	events, ok := VibeToClaudeEvent(vm)
	if !ok {
		t.Fatal("expected events, got none")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (thinking + text), got %d", len(events))
	}

	// First: thinking
	if events[0].AssistantMessage == nil || len(events[0].AssistantMessage.Content) == 0 {
		t.Fatal("thinking event missing content")
	}
	if events[0].AssistantMessage.Content[0].Type != "thinking" {
		t.Errorf("first event type = %q, want thinking", events[0].AssistantMessage.Content[0].Type)
	}
	if events[0].AssistantMessage.Content[0].Text != "Let me think step by step..." {
		t.Errorf("thinking text = %q", events[0].AssistantMessage.Content[0].Text)
	}

	// Second: text
	if events[1].AssistantMessage == nil || len(events[1].AssistantMessage.Content) == 0 {
		t.Fatal("text event missing content")
	}
	if events[1].AssistantMessage.Content[0].Type != "text" {
		t.Errorf("second event type = %q, want text", events[1].AssistantMessage.Content[0].Type)
	}
}

func TestVibeToClaudeEvent_SkipsUser(t *testing.T) {
	vm := VibeMessage{
		Role:    "user",
		Content: "hello",
	}

	events, ok := VibeToClaudeEvent(vm)
	if ok || events != nil {
		t.Error("user message should be skipped")
	}
}
