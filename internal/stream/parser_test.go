package stream

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// testNDJSON uses the actual Claude stream-json format:
// system (init), assistant (full message), result (summary).
const testNDJSON = `{"type":"system","subtype":"init","session_id":"abc123","model":"claude-sonnet-4-5-20250929","tools":["Bash","Read","Write"]}
{"type":"assistant","message":{"id":"msg_01","model":"claude-sonnet-4-5-20250929","role":"assistant","content":[{"type":"text","text":"Hello, world!"}],"usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.08,"duration_ms":141000,"num_turns":3,"result":"Hello, world!","usage":{"input_tokens":1200,"output_tokens":3400}}
`

// testNDJSONWithTools uses format for a multi-turn assistant that includes tool use.
const testNDJSONWithTools = `{"type":"system","subtype":"init","session_id":"def456","model":"claude-opus-4-6"}
{"type":"assistant","message":{"id":"msg_02","model":"claude-opus-4-6","role":"assistant","content":[{"type":"tool_use","name":"Bash","id":"tool_1","input":{"command":"ls"}}]}}
{"type":"assistant","message":{"id":"msg_03","model":"claude-opus-4-6","role":"assistant","content":[{"type":"text","text":"Done."}]}}
{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.15,"duration_ms":5000,"num_turns":2}
`

func TestParseNDJSON(t *testing.T) {
	ctx := context.Background()
	ch := Parse(ctx, strings.NewReader(testNDJSON))

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

	// Check first event is system/init
	if events[0].Parsed.Type != "system" {
		t.Errorf("event[0].Type = %q, want %q", events[0].Parsed.Type, "system")
	}
	if events[0].Parsed.TurnID != "abc123" {
		t.Errorf("event[0].TurnID = %q, want %q", events[0].Parsed.TurnID, "abc123")
	}
	if events[0].Parsed.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("event[0].Model = %q, want %q", events[0].Parsed.Model, "claude-sonnet-4-5-20250929")
	}

	// Check assistant event
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
	if ev1.AssistantMessage.Content[0].Text != "Hello, world!" {
		t.Errorf("event[1] content[0].Text = %q, want %q", ev1.AssistantMessage.Content[0].Text, "Hello, world!")
	}

	// Check result
	last := events[2].Parsed
	if last.Type != "result" {
		t.Errorf("last event type = %q, want %q", last.Type, "result")
	}
	if last.TotalCostUSD != 0.08 {
		t.Errorf("result total_cost_usd = %f, want 0.08", last.TotalCostUSD)
	}
	if last.NumTurns != 3 {
		t.Errorf("result num_turns = %d, want 3", last.NumTurns)
	}
	if last.DurationMS != 141000 {
		t.Errorf("result duration_ms = %f, want 141000", last.DurationMS)
	}
}

func TestParseToolUse(t *testing.T) {
	ch := Parse(context.Background(), strings.NewReader(testNDJSONWithTools))

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

	// Check tool_use in assistant message
	toolMsg := events[1].Parsed
	if toolMsg.AssistantMessage == nil {
		t.Fatal("tool_use assistant message is nil")
	}
	if len(toolMsg.AssistantMessage.Content) != 1 {
		t.Fatalf("tool content length = %d, want 1", len(toolMsg.AssistantMessage.Content))
	}
	if toolMsg.AssistantMessage.Content[0].Type != "tool_use" {
		t.Errorf("tool content type = %q, want %q", toolMsg.AssistantMessage.Content[0].Type, "tool_use")
	}
	if toolMsg.AssistantMessage.Content[0].Name != "Bash" {
		t.Errorf("tool name = %q, want %q", toolMsg.AssistantMessage.Content[0].Name, "Bash")
	}
}

func TestParseContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	r := strings.NewReader(`{"type":"system"}` + "\n")
	ch := Parse(ctx, r)

	count := 0
	for range ch {
		count++
	}
	// May get 0 or 1 events depending on timing; just verify it terminates
	if count > 1 {
		t.Fatalf("expected at most 1 event after cancel, got %d", count)
	}
}

func TestParseEmptyLines(t *testing.T) {
	input := "\n\n{\"type\":\"system\"}\n\n\n"
	ch := Parse(context.Background(), strings.NewReader(input))

	var events []RawEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event (skipping blank lines), got %d", len(events))
	}
}

func TestDisplayHandle(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	ch := Parse(context.Background(), strings.NewReader(testNDJSON))
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("parse error: %v", ev.Err)
		}
		d.Handle(ev.Parsed)
	}
	d.Finish()

	output := buf.String()

	if !strings.Contains(output, "[init]") {
		t.Errorf("display output missing [init], got:\n%s", output)
	}
	if !strings.Contains(output, "[text]") {
		t.Errorf("display output missing [text], got:\n%s", output)
	}
	if !strings.Contains(output, "Hello, world!") {
		t.Errorf("display output missing text content, got:\n%s", output)
	}
	if !strings.Contains(output, "[result]") {
		t.Errorf("display output missing [result], got:\n%s", output)
	}
	if !strings.Contains(output, "cost=$0.08") {
		t.Errorf("display output missing cost, got:\n%s", output)
	}
}

func TestDisplayToolUse(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf)

	ch := Parse(context.Background(), strings.NewReader(testNDJSONWithTools))
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("parse error: %v", ev.Err)
		}
		d.Handle(ev.Parsed)
	}
	d.Finish()

	output := buf.String()

	if !strings.Contains(output, "[tool:Bash]") {
		t.Errorf("display output missing [tool:Bash], got:\n%s", output)
	}
	if !strings.Contains(output, "Done.") {
		t.Errorf("display output missing text content, got:\n%s", output)
	}
}
