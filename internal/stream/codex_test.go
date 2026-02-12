package stream

import (
	"context"
	"strings"
	"testing"
)

const testCodexNDJSON = `{"type":"thread.started","thread_id":"thread-123"}
{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"ls -la","status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"ls -la","aggregated_output":"file.txt\n","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Done."}}
{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":3,"output_tokens":5}}
`

func TestParseCodexNDJSON(t *testing.T) {
	ch := ParseCodex(context.Background(), strings.NewReader(testCodexNDJSON))

	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected parse error: %v", ev.Err)
		}
		events = append(events, ev)
	}

	if len(events) != 5 {
		t.Fatalf("len(events) = %d, want 5", len(events))
	}

	// thread.started -> system/init
	if events[0].Parsed.Type != "system" || events[0].Parsed.Subtype != "init" {
		t.Fatalf("events[0] = %+v, want system/init", events[0].Parsed)
	}
	if events[0].Parsed.TurnID != "thread-123" {
		t.Fatalf("events[0].TurnID = %q, want %q", events[0].Parsed.TurnID, "thread-123")
	}

	// command start -> assistant tool_use
	cmdStart := events[1].Parsed
	if cmdStart.Type != "assistant" || cmdStart.AssistantMessage == nil {
		t.Fatalf("events[1] = %+v, want assistant with message", cmdStart)
	}
	if len(cmdStart.AssistantMessage.Content) != 1 || cmdStart.AssistantMessage.Content[0].Type != "tool_use" {
		t.Fatalf("events[1] content = %+v, want one tool_use block", cmdStart.AssistantMessage.Content)
	}

	// command completion -> user tool_result
	cmdDone := events[2].Parsed
	if cmdDone.Type != "user" || cmdDone.AssistantMessage == nil {
		t.Fatalf("events[2] = %+v, want user with tool_result", cmdDone)
	}
	if len(cmdDone.AssistantMessage.Content) != 1 || cmdDone.AssistantMessage.Content[0].Type != "tool_result" {
		t.Fatalf("events[2] content = %+v, want one tool_result block", cmdDone.AssistantMessage.Content)
	}
	if got := cmdDone.AssistantMessage.Content[0].ToolContentText(); got != "file.txt\n" {
		t.Fatalf("tool result text = %q, want %q", got, "file.txt\n")
	}

	// agent_message -> assistant text
	msg := events[3].Parsed
	if msg.Type != "assistant" || msg.AssistantMessage == nil {
		t.Fatalf("events[3] = %+v, want assistant text", msg)
	}
	if len(msg.AssistantMessage.Content) != 1 || msg.AssistantMessage.Content[0].Text != "Done." {
		t.Fatalf("events[3] content = %+v, want single text block 'Done.'", msg.AssistantMessage.Content)
	}

	// turn.completed -> result usage
	result := events[4].Parsed
	if result.Type != "result" || result.Subtype != "success" || result.Usage == nil {
		t.Fatalf("events[4] = %+v, want result/success with usage", result)
	}
	if result.Usage.InputTokens != 12 || result.Usage.OutputTokens != 5 || result.Usage.CacheReadInputTokens != 3 {
		t.Fatalf("events[4].Usage = %+v, want input=12 output=5 cache_read=3", result.Usage)
	}
}

func TestParseCodexErrors(t *testing.T) {
	ndjson := `{"type":"error","error":{"message":"boom"}}
{"type":"turn.failed","error":{"message":"turn failed"}}
`
	ch := ParseCodex(context.Background(), strings.NewReader(ndjson))
	var events []RawEvent
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("unexpected parse error: %v", ev.Err)
		}
		events = append(events, ev)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Parsed.Type != "error" || events[0].Parsed.ResultText != "boom" {
		t.Fatalf("events[0] = %+v, want error 'boom'", events[0].Parsed)
	}
	if events[1].Parsed.Type != "result" || !events[1].Parsed.IsError || events[1].Parsed.ResultText != "turn failed" {
		t.Fatalf("events[1] = %+v, want error result with text", events[1].Parsed)
	}
}

func TestParseCodexUnknownAndInvalid(t *testing.T) {
	ndjson := `{"type":"unknown.event","x":1}
not json
`
	ch := ParseCodex(context.Background(), strings.NewReader(ndjson))

	first, ok := <-ch
	if !ok {
		t.Fatal("expected first event")
	}
	if first.Err != nil {
		t.Fatalf("first.Err = %v, want nil", first.Err)
	}
	if first.Parsed.Type != "" {
		t.Fatalf("first.Parsed.Type = %q, want empty for unknown events", first.Parsed.Type)
	}

	second, ok := <-ch
	if !ok {
		t.Fatal("expected second event")
	}
	if second.Err == nil {
		t.Fatal("second.Err = nil, want parse error")
	}
}
