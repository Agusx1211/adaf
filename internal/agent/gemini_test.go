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

// fake-gemini script that emits canned NDJSON matching Gemini's stream-json format.
const fakeGemini = `#!/usr/bin/env sh
# Emit NDJSON lines simulating Gemini stream-json output.
printf '{"type":"init","session_id":"gem-test","model":"gemini-2.5-pro"}\n'
printf '{"type":"message","role":"assistant","content":"Hello from fake gemini!","thought":false,"delta":false}\n'
printf '{"type":"result","stats":{"total_tokens":200,"input_tokens":120,"output_tokens":80,"duration_ms":1500,"tool_calls":0}}\n'
`

func TestGeminiRunStreamJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-gemini")
	if err := os.WriteFile(cmdPath, []byte(fakeGemini), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewGeminiAgent().Run(context.Background(), Config{
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

	// Verify accumulated text output contains the text content.
	if !strings.Contains(result.Output, "Hello from fake gemini!") {
		t.Errorf("Run() output = %q, want to contain %q", result.Output, "Hello from fake gemini!")
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
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, "agent=gemini") {
			hasMeta = true
			break
		}
	}
	if !hasMeta {
		t.Error("missing agent=gemini meta event in recording")
	}
}

func TestGeminiRunNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-gemini-fail")
	script := `#!/usr/bin/env sh
printf '{"type":"init","session_id":"fail","model":"test"}\n'
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

	result, err := NewGeminiAgent().Run(context.Background(), Config{
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
