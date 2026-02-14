package agent

import (
	"errors"
	"os/exec"
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

func TestExtractExitCode(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		code, err := extractExitCode(nil)
		if err != nil {
			t.Fatalf("extractExitCode(nil) returned error: %v", err)
		}
		if code != 0 {
			t.Fatalf("extractExitCode(nil) = %d, want 0", code)
		}
	})

	t.Run("exit error", func(t *testing.T) {
		// Need a real ExitError.
		cmd := exec.Command("sh", "-c", "exit 42")
		waitErr := cmd.Run()
		code, err := extractExitCode(waitErr)
		if err != nil {
			t.Fatalf("extractExitCode(ExitError) returned error: %v", err)
		}
		if code != 42 {
			t.Fatalf("extractExitCode(ExitError) = %d, want 42", code)
		}
	})

	t.Run("other error", func(t *testing.T) {
		otherErr := errors.New("something else")
		code, err := extractExitCode(otherErr)
		if err != otherErr {
			t.Fatalf("extractExitCode(other) returned %v, want %v", err, otherErr)
		}
		if code != 0 {
			t.Fatalf("extractExitCode(other) code = %d, want 0", code)
		}
	})
}

func TestHasFlag(t *testing.T) {
	tests := []struct {
		args []string
		flag string
		want bool
	}{
		{[]string{"--foo", "--bar"}, "--foo", true},
		{[]string{"--foo", "--bar"}, "--baz", false},
		{[]string{}, "--foo", false},
		{[]string{"--foobar"}, "--foo", false},
	}
	for _, tt := range tests {
		if got := hasFlag(tt.args, tt.flag); got != tt.want {
			t.Errorf("hasFlag(%v, %q) = %v, want %v", tt.args, tt.flag, got, tt.want)
		}
	}
}

func TestWithoutFlag(t *testing.T) {
	tests := []struct {
		args []string
		flag string
		want []string
	}{
		{[]string{"--foo", "--bar"}, "--foo", []string{"--bar"}},
		{[]string{"--foo", "--bar"}, "--baz", []string{"--foo", "--bar"}},
		{[]string{"--foo", "--foo", "--bar"}, "--foo", []string{"--bar"}},
		{[]string{}, "--foo", nil},
	}
	for _, tt := range tests {
		got := withoutFlag(tt.args, tt.flag)
		if len(got) != len(tt.want) {
			t.Errorf("withoutFlag(%v, %q) = %v, want %v", tt.args, tt.flag, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("withoutFlag(%v, %q) = %v, want %v", tt.args, tt.flag, got, tt.want)
				break
			}
		}
	}
}

func TestHasEnvKey(t *testing.T) {
	tests := []struct {
		env  []string
		key  string
		want bool
	}{
		{[]string{"FOO=bar", "BAZ=qux"}, "FOO", true},
		{[]string{"FOO=bar", "BAZ=qux"}, "BAZ", true},
		{[]string{"FOO=bar", "BAZ=qux"}, "BAR", false},
		{[]string{"FOOBAR=x"}, "FOO", false},
		{[]string{}, "FOO", false},
	}
	for _, tt := range tests {
		if got := hasEnvKey(tt.env, tt.key); got != tt.want {
			t.Errorf("hasEnvKey(%v, %q) = %v, want %v", tt.env, tt.key, got, tt.want)
		}
	}
}

func TestSetupEnv(t *testing.T) {
	cmd := &exec.Cmd{}
	env := map[string]string{
		"ADAF_TEST_VAR": "123",
	}
	setupEnv(cmd, env)

	found := false
	for _, kv := range cmd.Env {
		if kv == "ADAF_TEST_VAR=123" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("setupEnv didn't add ADAF_TEST_VAR=123 to cmd.Env")
	}

	// Verify it also inherited from os.Environ()
	if !hasEnvKey(cmd.Env, "PATH") {
		t.Errorf("setupEnv didn't inherit PATH from os.Environ()")
	}
}
