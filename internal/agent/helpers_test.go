package agent

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/stream"
)

func TestDefaultAccumulateTextResetsAtToolBoundaries(t *testing.T) {
	var buf strings.Builder

	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "progress"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "tool_use", Name: "Bash"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "final answer"}},
		},
	}, &buf)

	if got := buf.String(); got != "final answer" {
		t.Fatalf("defaultAccumulateText() = %q, want %q", got, "final answer")
	}
}

func TestDefaultAccumulateTextKeepsAssistantSegmentAfterLastTool(t *testing.T) {
	var buf strings.Builder

	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "first"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "tool_use", Name: "Edit"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "second"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "third"}},
		},
	}, &buf)

	if got, want := buf.String(), "second\n\nthird"; got != want {
		t.Fatalf("defaultAccumulateText() = %q, want %q", got, want)
	}
}

func TestDefaultAccumulateTextResetsOnToolResult(t *testing.T) {
	var buf strings.Builder

	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "before"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "user",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "tool_result", ToolUseID: "toolu_1"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "after"}},
		},
	}, &buf)

	if got := buf.String(); got != "after" {
		t.Fatalf("defaultAccumulateText() = %q, want %q", got, "after")
	}
}

func TestDefaultAccumulateTextResultOverrides(t *testing.T) {
	var buf strings.Builder

	defaultAccumulateText(stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{{Type: "text", Text: "before"}},
		},
	}, &buf)
	defaultAccumulateText(stream.ClaudeEvent{
		Type:       "result",
		ResultText: "done",
	}, &buf)

	if got := buf.String(); got != "done" {
		t.Fatalf("defaultAccumulateText() = %q, want %q", got, "done")
	}
}
