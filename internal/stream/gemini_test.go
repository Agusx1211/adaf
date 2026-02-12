package stream

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Test data uses the actual Gemini CLI stream-json output format.
// See references/gemini-cli/packages/core/src/output/types.ts for event schemas.

const testGeminiNDJSON = `{"type":"init","timestamp":"2025-01-01T00:00:00.000Z","session_id":"gem-123","model":"gemini-2.5-pro"}
{"type":"message","timestamp":"2025-01-01T00:00:01.000Z","role":"assistant","content":"Hello from Gemini!"}
{"type":"result","timestamp":"2025-01-01T00:00:02.000Z","status":"success","stats":{"total_tokens":200,"input_tokens":120,"output_tokens":80,"cached":0,"input":120,"duration_ms":1500,"tool_calls":0}}
`

const testGeminiDelta = `{"type":"init","timestamp":"2025-01-01T00:00:00.000Z","session_id":"gem-delta","model":"gemini-2.5-flash"}
{"type":"message","timestamp":"2025-01-01T00:00:01.000Z","role":"assistant","content":"Hel","delta":true}
{"type":"message","timestamp":"2025-01-01T00:00:01.100Z","role":"assistant","content":"lo ","delta":true}
{"type":"message","timestamp":"2025-01-01T00:00:01.200Z","role":"assistant","content":"world","delta":true}
{"type":"result","timestamp":"2025-01-01T00:00:02.000Z","status":"success","stats":{"total_tokens":50,"input_tokens":30,"output_tokens":20,"cached":0,"input":30,"duration_ms":500,"tool_calls":0}}
`

// testGeminiMultiMessage tests multiple non-delta assistant messages.
// The Gemini CLI's stream-json format does NOT have a "thought" field --
// all assistant content is emitted as regular "message" events with
// role="assistant" and optional delta=true for streaming chunks.
const testGeminiMultiMessage = `{"type":"init","timestamp":"2025-01-01T00:00:00.000Z","session_id":"gem-multi","model":"gemini-2.5-pro"}
{"type":"message","timestamp":"2025-01-01T00:00:01.000Z","role":"assistant","content":"Let me think about this..."}
{"type":"message","timestamp":"2025-01-01T00:00:02.000Z","role":"assistant","content":"The answer is 42."}
{"type":"result","timestamp":"2025-01-01T00:00:03.000Z","status":"success","stats":{"total_tokens":100,"input_tokens":60,"output_tokens":40,"cached":0,"input":60,"duration_ms":2000,"tool_calls":0}}
`

const testGeminiToolUse = `{"type":"init","timestamp":"2025-01-01T00:00:00.000Z","session_id":"gem-tool","model":"gemini-2.5-pro"}
{"type":"tool_use","timestamp":"2025-01-01T00:00:01.000Z","tool_name":"shell","tool_id":"call_1","parameters":{"command":"ls -la"}}
{"type":"tool_result","timestamp":"2025-01-01T00:00:02.000Z","tool_id":"call_1","status":"success","output":"file1.txt\nfile2.txt"}
{"type":"message","timestamp":"2025-01-01T00:00:03.000Z","role":"assistant","content":"I found two files."}
{"type":"result","timestamp":"2025-01-01T00:00:04.000Z","status":"success","stats":{"total_tokens":150,"input_tokens":80,"output_tokens":70,"cached":0,"input":80,"duration_ms":3000,"tool_calls":1}}
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
	if events[0].Parsed.TurnID != "gem-123" {
		t.Errorf("event[0].TurnID = %q, want %q", events[0].Parsed.TurnID, "gem-123")
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

func TestParseGeminiMultiMessage(t *testing.T) {
	ch := ParseGemini(context.Background(), strings.NewReader(testGeminiMultiMessage))

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

	// Both messages should be plain text content blocks (no thinking in Gemini stream-json).
	msg1 := events[1].Parsed
	if msg1.Type != "assistant" {
		t.Errorf("msg1 event type = %q, want %q", msg1.Type, "assistant")
	}
	if msg1.AssistantMessage == nil {
		t.Fatal("msg1 AssistantMessage is nil")
	}
	if len(msg1.AssistantMessage.Content) != 1 {
		t.Fatalf("msg1 content length = %d, want 1", len(msg1.AssistantMessage.Content))
	}
	if msg1.AssistantMessage.Content[0].Type != "text" {
		t.Errorf("msg1 content type = %q, want %q", msg1.AssistantMessage.Content[0].Type, "text")
	}
	if msg1.AssistantMessage.Content[0].Text != "Let me think about this..." {
		t.Errorf("msg1 content text = %q", msg1.AssistantMessage.Content[0].Text)
	}

	// Check regular text event follows.
	msg2 := events[2].Parsed
	if msg2.Type != "assistant" {
		t.Errorf("msg2 event type = %q, want %q", msg2.Type, "assistant")
	}
	if msg2.AssistantMessage.Content[0].Type != "text" {
		t.Errorf("msg2 content type = %q, want %q", msg2.AssistantMessage.Content[0].Type, "text")
	}
	if msg2.AssistantMessage.Content[0].Text != "The answer is 42." {
		t.Errorf("msg2 content text = %q", msg2.AssistantMessage.Content[0].Text)
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

func TestParseGeminiErrorEvent(t *testing.T) {
	ndjson := `{"type":"init","timestamp":"2025-01-01T00:00:00.000Z","session_id":"gem-err","model":"gemini-2.5-pro"}
{"type":"error","timestamp":"2025-01-01T00:00:01.000Z","severity":"warning","message":"Loop detected, stopping execution"}
{"type":"result","timestamp":"2025-01-01T00:00:02.000Z","status":"success","stats":{"total_tokens":10,"input_tokens":5,"output_tokens":5,"cached":0,"input":5,"duration_ms":100,"tool_calls":0}}
`
	ch := ParseGemini(context.Background(), strings.NewReader(ndjson))

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

	// Check error event carries the message.
	errEv := events[1].Parsed
	if errEv.Type != "error" {
		t.Errorf("error event type = %q, want %q", errEv.Type, "error")
	}
	if errEv.ResultText != "Loop detected, stopping execution" {
		t.Errorf("error event ResultText = %q, want %q", errEv.ResultText, "Loop detected, stopping execution")
	}
}

func TestParseGeminiResultError(t *testing.T) {
	ndjson := `{"type":"init","timestamp":"2025-01-01T00:00:00.000Z","session_id":"gem-reserr","model":"gemini-2.5-pro"}
{"type":"result","timestamp":"2025-01-01T00:00:01.000Z","status":"error","error":{"type":"MaxSessionTurnsError","message":"Maximum session turns exceeded"},"stats":{"total_tokens":100,"input_tokens":50,"output_tokens":50,"cached":30,"input":20,"duration_ms":1200,"tool_calls":0}}
`
	ch := ParseGemini(context.Background(), strings.NewReader(ndjson))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	res := events[1].Parsed
	if res.Type != "result" {
		t.Errorf("result type = %q, want %q", res.Type, "result")
	}
	if !res.IsError {
		t.Error("result IsError should be true for status=error")
	}
	if res.ResultText != "Maximum session turns exceeded" {
		t.Errorf("result ResultText = %q, want error message", res.ResultText)
	}
	if res.Usage == nil {
		t.Fatal("result usage is nil")
	}
	if res.Usage.InputTokens != 50 {
		t.Errorf("result input_tokens = %d, want 50", res.Usage.InputTokens)
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

func TestGeminiDisplayErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	ndjson := `{"type":"error","timestamp":"2025-01-01T00:00:01.000Z","severity":"warning","message":"Loop detected, stopping execution"}
`
	ch := ParseGemini(context.Background(), strings.NewReader(ndjson))
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
	if !strings.Contains(output, "[error]") {
		t.Errorf("display output missing [error], got:\n%s", output)
	}
	if !strings.Contains(output, "Loop detected") {
		t.Errorf("display output missing error message, got:\n%s", output)
	}
}
