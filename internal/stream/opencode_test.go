package stream

import (
	"context"
	"strings"
	"testing"
)

func TestParseOpencode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantSub  string
		wantText string
		wantSess string
		wantSkip bool // true if the event should be ignored (no parsed type)
	}{
		{
			name:     "text event",
			input:    `{"type":"text","timestamp":1700000000000,"sessionID":"sess-123","part":{"id":"p1","type":"text","text":"Hello world"}}`,
			wantType: "assistant",
			wantText: "Hello world",
			wantSess: "sess-123",
		},
		{
			name:     "reasoning event",
			input:    `{"type":"reasoning","timestamp":1700000000000,"sessionID":"sess-123","part":{"id":"p2","type":"reasoning","text":"Let me think..."}}`,
			wantType: "assistant",
			wantText: "Let me think...",
			wantSess: "sess-123",
		},
		{
			name:     "step_start maps to system init",
			input:    `{"type":"step_start","timestamp":1700000000000,"sessionID":"sess-456","part":{"id":"p3","type":"step-start"}}`,
			wantType: "system",
			wantSub:  "init",
			wantSess: "sess-456",
		},
		{
			name:     "step_finish maps to result",
			input:    `{"type":"step_finish","timestamp":1700000000000,"sessionID":"sess-456","part":{"id":"p4","type":"step-finish","reason":"end_turn","cost":0.05,"tokens":{"input":100,"output":50,"cache":{"read":10,"write":5}}}}`,
			wantType: "result",
			wantSub:  "success",
			wantSess: "sess-456",
		},
		{
			name:     "tool_use event",
			input:    `{"type":"tool_use","timestamp":1700000000000,"sessionID":"sess-789","part":{"id":"p5","type":"tool","callID":"call-1","tool":"bash","state":{"status":"completed","input":{"command":"ls"},"output":"file.txt"}}}`,
			wantType: "assistant",
			wantSess: "sess-789",
		},
		{
			name:     "error event",
			input:    `{"type":"error","timestamp":1700000000000,"sessionID":"sess-err","error":{"name":"ProviderAuthError","data":{"message":"Invalid API key"}}}`,
			wantType: "result",
			wantSub:  "error_during_execution",
			wantText: "Invalid API key",
			wantSess: "sess-err",
		},
		{
			name:     "error event with name only",
			input:    `{"type":"error","timestamp":1700000000000,"sessionID":"sess-err2","error":{"name":"UnknownError"}}`,
			wantType: "result",
			wantSub:  "error_during_execution",
			wantText: "UnknownError",
			wantSess: "sess-err2",
		},
		{
			name:     "empty text is skipped",
			input:    `{"type":"text","timestamp":1700000000000,"sessionID":"sess-123","part":{"id":"p6","type":"text","text":"  "}}`,
			wantSkip: true,
		},
		{
			name:     "unknown event type is skipped",
			input:    `{"type":"session.status","timestamp":1700000000000,"sessionID":"sess-123"}`,
			wantSkip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ch := ParseOpencode(ctx, strings.NewReader(tt.input+"\n"))

			var events []RawEvent
			for ev := range ch {
				events = append(events, ev)
			}

			if len(events) == 0 {
				t.Fatal("expected at least one event")
			}

			ev := events[0]
			if ev.Err != nil {
				t.Fatalf("unexpected error: %v", ev.Err)
			}

			if tt.wantSkip {
				if ev.Parsed.Type != "" {
					t.Errorf("expected skipped event, got type=%q", ev.Parsed.Type)
				}
				return
			}

			if ev.Parsed.Type != tt.wantType {
				t.Errorf("type: got %q, want %q", ev.Parsed.Type, tt.wantType)
			}
			if tt.wantSub != "" && ev.Parsed.Subtype != tt.wantSub {
				t.Errorf("subtype: got %q, want %q", ev.Parsed.Subtype, tt.wantSub)
			}
			if tt.wantSess != "" && ev.Parsed.TurnID != tt.wantSess {
				t.Errorf("session ID: got %q, want %q", ev.Parsed.TurnID, tt.wantSess)
			}
			if tt.wantText != "" {
				gotText := ""
				if ev.Parsed.AssistantMessage != nil && len(ev.Parsed.AssistantMessage.Content) > 0 {
					gotText = ev.Parsed.AssistantMessage.Content[0].Text
				}
				if ev.Parsed.ResultText != "" {
					gotText = ev.Parsed.ResultText
				}
				if gotText != tt.wantText {
					t.Errorf("text: got %q, want %q", gotText, tt.wantText)
				}
			}
		})
	}
}

func TestParseOpencodeMultipleEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"step_start","timestamp":1,"sessionID":"s1","part":{"id":"p1","type":"step-start"}}`,
		`{"type":"text","timestamp":2,"sessionID":"s1","part":{"id":"p2","type":"text","text":"Hello"}}`,
		`{"type":"step_finish","timestamp":3,"sessionID":"s1","part":{"id":"p3","type":"step-finish","cost":0.01,"tokens":{"input":10,"output":5}}}`,
	}, "\n") + "\n"

	ctx := context.Background()
	ch := ParseOpencode(ctx, strings.NewReader(input))

	var events []RawEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// step_start -> system init with session ID
	if events[0].Parsed.Type != "system" || events[0].Parsed.TurnID != "s1" {
		t.Errorf("event 0: expected system init with session s1, got type=%q turn=%q",
			events[0].Parsed.Type, events[0].Parsed.TurnID)
	}

	// text -> assistant
	if events[1].Parsed.Type != "assistant" {
		t.Errorf("event 1: expected assistant, got %q", events[1].Parsed.Type)
	}

	// step_finish -> result
	if events[2].Parsed.Type != "result" {
		t.Errorf("event 2: expected result, got %q", events[2].Parsed.Type)
	}
}

func TestParseOpencodeToolUseEmitsUserToolResult(t *testing.T) {
	input := `{"type":"tool_use","timestamp":1700000000000,"sessionID":"sess-789","part":{"id":"p5","type":"tool","callID":"call-1","tool":"bash","state":{"status":"completed","input":{"command":"ls"},"output":"file.txt"}}}`
	ch := ParseOpencode(context.Background(), strings.NewReader(input+"\n"))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events for tool_use line, got %d", len(events))
	}

	if events[0].Parsed.Type != "assistant" {
		t.Fatalf("event[0] type = %q, want assistant", events[0].Parsed.Type)
	}
	if events[0].Parsed.AssistantMessage == nil || len(events[0].Parsed.AssistantMessage.Content) == 0 {
		t.Fatal("event[0] missing assistant content")
	}
	if events[0].Parsed.AssistantMessage.Content[0].Type != "tool_use" {
		t.Fatalf("event[0] block type = %q, want tool_use", events[0].Parsed.AssistantMessage.Content[0].Type)
	}

	if events[1].Parsed.Type != "user" {
		t.Fatalf("event[1] type = %q, want user", events[1].Parsed.Type)
	}
	if events[1].Parsed.AssistantMessage == nil || len(events[1].Parsed.AssistantMessage.Content) == 0 {
		t.Fatal("event[1] missing user content")
	}
	if events[1].Parsed.AssistantMessage.Content[0].Type != "tool_result" {
		t.Fatalf("event[1] block type = %q, want tool_result", events[1].Parsed.AssistantMessage.Content[0].Type)
	}
	if events[1].Parsed.AssistantMessage.Content[0].ToolUseID != events[0].Parsed.AssistantMessage.Content[0].ID {
		t.Fatal("tool_result ToolUseID does not match tool_use ID")
	}

	if len(events[0].Raw) == 0 {
		t.Fatal("event[0] should carry raw line")
	}
	if len(events[1].Raw) != 0 {
		t.Fatal("event[1] should not duplicate raw line")
	}
}
