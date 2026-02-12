//go:build integration

package agent

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

const geminiCmd = "/home/agusx1211/.nvm/versions/node/v22.3.0/bin/gemini"

// TestGeminiIntegrationBasicPrompt sends a simple prompt to the real gemini CLI
// and verifies the response contains the expected marker string.
func TestGeminiIntegrationBasicPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	result, err := NewGeminiAgent().Run(ctx, Config{
		Command: geminiCmd,
		Args:    []string{"-y"},
		WorkDir: t.TempDir(),
		Prompt:  "Respond with exactly: HELLO_GEMINI_TEST_OK",
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	t.Logf("Exit code: %d", result.ExitCode)
	t.Logf("Duration: %s", result.Duration)
	t.Logf("Output: %q", result.Output)
	if result.Error != "" {
		t.Logf("Stderr (first 500 chars): %q", truncate(result.Error, 500))
	}

	if !strings.Contains(result.Output, "HELLO_GEMINI_TEST_OK") {
		t.Errorf("Run() output = %q, want to contain %q", result.Output, "HELLO_GEMINI_TEST_OK")
	}
}

// TestGeminiIntegrationExitCode verifies that a successful gemini run returns
// exit code 0.
func TestGeminiIntegrationExitCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	result, err := NewGeminiAgent().Run(ctx, Config{
		Command: geminiCmd,
		Args:    []string{"-y"},
		WorkDir: t.TempDir(),
		Prompt:  "Say OK",
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	t.Logf("Exit code: %d", result.ExitCode)
	t.Logf("Duration: %s", result.Duration)
	t.Logf("Output: %q", result.Output)

	if result.ExitCode != 0 {
		t.Errorf("Run() exit code = %d, want 0; stderr = %q",
			result.ExitCode, truncate(result.Error, 500))
	}
}

// TestGeminiIntegrationRecordingEvents verifies that meta events and stream
// events are recorded during a real gemini run.
func TestGeminiIntegrationRecordingEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	result, err := NewGeminiAgent().Run(ctx, Config{
		Command: geminiCmd,
		Args:    []string{"-y"},
		WorkDir: t.TempDir(),
		Prompt:  "Say hi",
		Stdout:  io.Discard,
		Stderr:  io.Discard,
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	t.Logf("Exit code: %d, Duration: %s", result.ExitCode, result.Duration)

	events := rec.Events()
	t.Logf("Total recorded events: %d", len(events))

	// Check for meta events.
	var hasAgentMeta, hasCommandMeta, hasWorkdirMeta, hasStdinEvent bool
	var streamCount int
	for _, ev := range events {
		switch ev.Type {
		case "meta":
			if strings.HasPrefix(ev.Data, "agent=gemini") {
				hasAgentMeta = true
			}
			if strings.HasPrefix(ev.Data, "command=") {
				hasCommandMeta = true
			}
			if strings.HasPrefix(ev.Data, "workdir=") {
				hasWorkdirMeta = true
			}
		case "stdin":
			hasStdinEvent = true
		case "claude_stream":
			streamCount++
		}
	}

	if !hasAgentMeta {
		t.Error("missing agent=gemini meta event")
	}
	if !hasCommandMeta {
		t.Error("missing command= meta event")
	}
	if !hasWorkdirMeta {
		t.Error("missing workdir= meta event")
	}
	if !hasStdinEvent {
		t.Error("missing stdin recording event for the prompt")
	}
	if streamCount == 0 {
		t.Error("no claude_stream events recorded; expected at least 1 NDJSON line")
	}
	t.Logf("Stream events recorded: %d", streamCount)

	// Log a few stream events for debugging.
	for i, ev := range events {
		if ev.Type == "claude_stream" && i < 10 {
			t.Logf("  stream event %d: %s", i, truncate(ev.Data, 200))
		}
	}
}

// TestGeminiIntegrationContextCancel verifies that cancelling the context
// causes the gemini process to terminate cleanly without hanging.
func TestGeminiIntegrationContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	start := time.Now()
	result, err := NewGeminiAgent().Run(ctx, Config{
		Command: geminiCmd,
		Args:    []string{"-y"},
		WorkDir: t.TempDir(),
		// Use a prompt that would normally take a long time so we can
		// verify the context cancellation cuts it short.
		Prompt: "Write a 10000 word essay about the history of computing",
		Stdout: io.Discard,
		Stderr: io.Discard,
	}, rec)
	elapsed := time.Since(start)

	t.Logf("Elapsed: %s", elapsed)

	// The function should return within a reasonable time after cancellation.
	// We allow up to 30s total (5s context + process cleanup time).
	if elapsed > 30*time.Second {
		t.Errorf("Run() took %s after context cancel, expected <30s", elapsed)
	}

	// Either an error or a result with non-zero exit is acceptable.
	if err != nil {
		t.Logf("Run() returned error (expected): %v", err)
		return
	}

	if result != nil {
		t.Logf("Run() returned result: exit=%d, duration=%s, output=%q",
			result.ExitCode, result.Duration, truncate(result.Output, 200))
		// A non-zero exit code from a killed process is expected.
		// Exit code 0 is also acceptable if gemini happened to finish
		// before the context expired.
	}
}

// truncate returns s truncated to n characters with "..." appended if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
