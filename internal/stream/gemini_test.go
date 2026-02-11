package stream

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

const testGeminiNDJSON = `{"type":"init","session_id":"gem-123","model":"gemini-2.5-pro"}
{"type":"message","role":"assistant","content":"Hello from Gemini!","thought":false,"delta":false}
{"type":"result","stats":{"total_tokens":200,"input_tokens":120,"output_tokens":80,"duration_ms":1500,"tool_calls":0}}
`

const testGeminiDelta = `{"type":"init","session_id":"gem-delta","model":"gemini-2.5-flash"}
{"type":"message","role":"assistant","content":"Hel","thought":false,"delta":true}
{"type":"message","role":"assistant","content":"lo ","thought":false,"delta":true}
{"type":"message","role":"assistant","content":"world","thought":false,"delta":true}
{"type":"result","stats":{"total_tokens":50,"input_tokens":30,"output_tokens":20,"duration_ms":500}}
`

const testGeminiThinking = `{"type":"init","session_id":"gem-think","model":"gemini-2.5-pro"}
{"type":"message","role":"assistant","content":"Let me think about this...","thought":true,"delta":false}
{"type":"message","role":"assistant","content":"The answer is 42.","thought":false,"delta":false}
{"type":"result","stats":{"total_tokens":100,"input_tokens":60,"output_tokens":40,"duration_ms":2000}}
`

const testGeminiToolUse = `{"type":"init","session_id":"gem-tool","model":"gemini-2.5-pro"}
{"type":"tool_use","tool_name":"shell","tool_id":"call_1","parameters":{"command":"ls -la"}}
{"type":"tool_result","tool_id":"call_1","output":"file1.txt\nfile2.txt"}
{"type":"message","role":"assistant","content":"I found two files.","thought":false,"delta":false}
{"type":"result","stats":{"total_tokens":150,"input_tokens":80,"output_tokens":70,"duration_ms":3000,"tool_calls":1}}
`

func TestParseGeminiNDJSON(t *testing.T) {
	ch := ParseGemini(context.Background(), strings.NewReader(testGeminiNDJSON))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Check init event maps to system/init.
	if events[0].Parsed.Type != "system" {
		t.Errorf("event[0].Type = %q, want %q", events[0].Parsed.Type, "system")
	}
	if events[0].Parsed.Subtype != "init" {
		t.Errorf("event[0].Subtype = %q, want %q", events[0].Parsed.Subtype, "init")
	}
	if events[0].Parsed.SessionID != "gem-123" {
		t.Errorf("event[0].SessionID = %q, want %q", events[0].Parsed.SessionID, "gem-123")
	}
	if events[0].Parsed.Model != "gemini-2.5-pro" {
		t.Errorf("event[0].Model = %q, want %q", events[0].Parsed.Model, "gemini-2.5-pro")
	}

	// Check assistant message.
	ev1 := events[1].Parsed
	if ev1.Type != "assistant" {
		t.Errorf("event[1].Type = %q, want %q", ev1.Type, "assistant")
	}
	if ev1.AssistantMessage == nil {
		t.Fatal("event[1].AssistantMessage is nil")
	}
	if len(ev1.AssistantMessage.Content) != 1 {
		t.Fatalf("event[1] content length = %d, want 1", len(ev1.AssistantMessage.Content))
	}
	if ev1.AssistantMessage.Content[0].Type != "text" {
		t.Errorf("event[1] content[0].Type = %q, want %q", ev1.AssistantMessage.Content[0].Type, "text")
	}
	if ev1.AssistantMessage.Content[0].Text != "Hello from Gemini!" {
		t.Errorf("event[1] content[0].Text = %q, want %q", ev1.AssistantMessage.Content[0].Text, "Hello from Gemini!")
	}

	// Check result.
	last := events[2].Parsed
	if last.Type != "result" {
		t.Errorf("last event type = %q, want %q", last.Type, "result")
	}
	if last.DurationMS != 1500 {
		t.Errorf("result duration_ms = %f, want 1500", last.DurationMS)
	}
	if last.Usage == nil {
		t.Fatal("result usage is nil")
	}
	if last.Usage.InputTokens != 120 {
		t.Errorf("result input_tokens = %d, want 120", last.Usage.InputTokens)
	}
	if last.Usage.OutputTokens != 80 {
		t.Errorf("result output_tokens = %d, want 80", last.Usage.OutputTokens)
	}
}

func TestParseGeminiDelta(t *testing.T) {
	ch := ParseGemini(context.Background(), strings.NewReader(testGeminiDelta))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Check delta events map to content_block_delta.
	for i := 1; i <= 3; i++ {
		ev := events[i].Parsed
		if ev.Type != "content_block_delta" {
			t.Errorf("event[%d].Type = %q, want %q", i, ev.Type, "content_block_delta")
		}
		if ev.Delta == nil {
			t.Errorf("event[%d].Delta is nil", i)
		} else if ev.Delta.Type != "text_delta" {
			t.Errorf("event[%d].Delta.Type = %q, want %q", i, ev.Delta.Type, "text_delta")
		}
	}

	// Verify delta text content.
	if events[1].Parsed.Delta.Text != "Hel" {
		t.Errorf("delta[0] text = %q, want %q", events[1].Parsed.Delta.Text, "Hel")
	}
	if events[2].Parsed.Delta.Text != "lo " {
		t.Errorf("delta[1] text = %q, want %q", events[2].Parsed.Delta.Text, "lo ")
	}
	if events[3].Parsed.Delta.Text != "world" {
		t.Errorf("delta[2] text = %q, want %q", events[3].Parsed.Delta.Text, "world")
	}
}

func TestParseGeminiThinking(t *testing.T) {
	ch := ParseGemini(context.Background(), strings.NewReader(testGeminiThinking))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Check thinking event.
	think := events[1].Parsed
	if think.Type != "assistant" {
		t.Errorf("thinking event type = %q, want %q", think.Type, "assistant")
	}
	if think.AssistantMessage == nil {
		t.Fatal("thinking AssistantMessage is nil")
	}
	if len(think.AssistantMessage.Content) != 1 {
		t.Fatalf("thinking content length = %d, want 1", len(think.AssistantMessage.Content))
	}
	if think.AssistantMessage.Content[0].Type != "thinking" {
		t.Errorf("thinking content type = %q, want %q", think.AssistantMessage.Content[0].Type, "thinking")
	}
	if think.AssistantMessage.Content[0].Text != "Let me think about this..." {
		t.Errorf("thinking content text = %q", think.AssistantMessage.Content[0].Text)
	}

	// Check regular text event follows.
	text := events[2].Parsed
	if text.Type != "assistant" {
		t.Errorf("text event type = %q, want %q", text.Type, "assistant")
	}
	if text.AssistantMessage.Content[0].Type != "text" {
		t.Errorf("text content type = %q, want %q", text.AssistantMessage.Content[0].Type, "text")
	}
}

func TestParseGeminiToolUse(t *testing.T) {
	ch := ParseGemini(context.Background(), strings.NewReader(testGeminiToolUse))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	// init + tool_use + tool_result(skipped but raw recorded) + message + result = 5
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}

	// Check tool_use event.
	tool := events[1].Parsed
	if tool.Type != "assistant" {
		t.Errorf("tool event type = %q, want %q", tool.Type, "assistant")
	}
	if tool.AssistantMessage == nil {
		t.Fatal("tool AssistantMessage is nil")
	}
	if tool.AssistantMessage.Content[0].Type != "tool_use" {
		t.Errorf("tool content type = %q, want %q", tool.AssistantMessage.Content[0].Type, "tool_use")
	}
	if tool.AssistantMessage.Content[0].Name != "shell" {
		t.Errorf("tool name = %q, want %q", tool.AssistantMessage.Content[0].Name, "shell")
	}

	// tool_result line should have empty Parsed.Type (skipped) but still have raw data.
	if events[2].Parsed.Type != "" {
		t.Errorf("tool_result should be skipped, got type = %q", events[2].Parsed.Type)
	}
	if len(events[2].Raw) == 0 {
		t.Error("tool_result raw data should be preserved")
	}
}

func TestGeminiDisplayIntegration(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	ch := ParseGemini(context.Background(), strings.NewReader(testGeminiNDJSON))
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("parse error: %v", ev.Err)
		}
		if ev.Parsed.Type != "" {
			d.Handle(ev.Parsed)
		}
	}
	d.Finish()

	output := buf.String()

	if !strings.Contains(output, "[init]") {
		t.Errorf("display output missing [init], got:\n%s", output)
	}
	if !strings.Contains(output, "[text]") {
		t.Errorf("display output missing [text], got:\n%s", output)
	}
	if !strings.Contains(output, "Hello from Gemini!") {
		t.Errorf("display output missing text content, got:\n%s", output)
	}
	if !strings.Contains(output, "[result]") {
		t.Errorf("display output missing [result], got:\n%s", output)
	}
}
