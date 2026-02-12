package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

// fake-claude script that emits canned NDJSON matching Claude's actual stream-json format.
const fakeClaude = `#!/usr/bin/env sh
# Emit NDJSON lines simulating Claude stream-json output (actual format).
printf '{"type":"system","subtype":"init","session_id":"test-session","model":"claude-sonnet-4-5-20250929","tools":["Bash","Read"]}\n'
printf '{"type":"assistant","message":{"id":"msg_01","model":"claude-sonnet-4-5-20250929","role":"assistant","content":[{"type":"text","text":"Hello from fake claude!"}],"usage":{"input_tokens":100,"output_tokens":50}}}\n'
printf '{"type":"result","subtype":"success","is_error":false,"total_cost_usd":0.01,"duration_ms":500,"num_turns":1,"result":"Hello from fake claude!","usage":{"input_tokens":100,"output_tokens":50}}\n'
`

func TestClaudeRunStreamJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-claude")
	if err := os.WriteFile(cmdPath, []byte(fakeClaude), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewClaudeAgent().Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "Say hello",
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("Run() result is nil")
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
	}

	// Verify accumulated text output contains the text delta content.
	if !strings.Contains(result.Output, "Hello from fake claude!") {
		t.Errorf("Run() output = %q, want to contain %q", result.Output, "Hello from fake claude!")
	}

	// Verify recording events include claude_stream type.
	events := rec.Events()
	var streamCount int
	for _, ev := range events {
		if ev.Type == "claude_stream" {
			streamCount++
		}
	}
	if streamCount == 0 {
		t.Error("no claude_stream events recorded")
	}
	// We emit 3 NDJSON lines from the fake script.
	if streamCount != 3 {
		t.Errorf("expected 3 claude_stream events, got %d", streamCount)
	}

	// Verify we also recorded meta events.
	var hasMeta bool
	for _, ev := range events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, "agent=claude") {
			hasMeta = true
			break
		}
	}
	if !hasMeta {
		t.Error("missing agent=claude meta event in recording")
	}
}

func TestClaudeRunNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-claude-fail")
	script := `#!/usr/bin/env sh
printf '{"type":"system","subtype":"init","session_id":"fail","model":"test"}\n'
echo "something went wrong" >&2
exit 42
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewClaudeAgent().Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "fail please",
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("Run() exit code = %d, want 42", result.ExitCode)
	}
	if !strings.Contains(result.Error, "something went wrong") {
		t.Errorf("Run() stderr = %q, want to contain error message", result.Error)
	}
}

func TestClaudePromptUsesStdinNotArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-claude-stdin")
	script := `#!/usr/bin/env sh
expected="PROMPT_SENTINEL_123"
stdin_data="$(cat)"
if [ "$stdin_data" != "$expected" ]; then
	echo "stdin mismatch" >&2
	exit 98
fi
for arg in "$@"; do
	if [ "$arg" = "$expected" ]; then
		echo "prompt passed as argv" >&2
		exit 97
	fi
done
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}\n'
printf '{"type":"result","result":"ok"}\n'
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewClaudeAgent().Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "PROMPT_SENTINEL_123",
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
	}
	if !strings.Contains(result.Output, "ok") {
		t.Errorf("Run() output = %q, want to contain %q", result.Output, "ok")
	}

	events := rec.Events()
	for _, ev := range events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, "command=") && strings.Contains(ev.Data, "PROMPT_SENTINEL_123") {
			t.Fatalf("command metadata leaked prompt into argv: %q", ev.Data)
		}
	}
}
