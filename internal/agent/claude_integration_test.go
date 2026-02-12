//go:build integration

package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

const claudeBinary = "/home/agusx1211/.bun/bin/claude"

// newTestRecorder creates a store and recorder in a temporary directory.
func newTestRecorder(t *testing.T) *recording.Recorder {
	t.Helper()
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	return recording.New(1, s)
}

// TestClaudeIntegrationBasicPrompt sends a simple prompt to the real claude CLI
// and verifies the output contains the expected marker string.
func TestClaudeIntegrationBasicPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	var stdout, stderr bytes.Buffer

	result, err := NewClaudeAgent().Run(ctx, Config{
		Command: claudeBinary,
		WorkDir: t.TempDir(),
		Args:    []string{"--dangerously-skip-permissions", "--max-turns", "1"},
		Prompt:  "Respond with exactly: HELLO_CLAUDE_TEST_OK",
		Stdout:  &stdout,
		Stderr:  &stderr,
	}, rec)

	if err != nil {
		t.Fatalf("Run() returned error: %v\nstderr: %s", err, stderr.String())
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	t.Logf("Exit code: %d", result.ExitCode)
	t.Logf("Duration: %s", result.Duration)
	t.Logf("Output length: %d", len(result.Output))
	t.Logf("Output: %s", result.Output)
	t.Logf("Stderr: %s", result.Error)

	if !strings.Contains(result.Output, "HELLO_CLAUDE_TEST_OK") {
		t.Errorf("Output does not contain expected marker.\nGot: %q", result.Output)
	}
}

// TestClaudeIntegrationExitCodeZero verifies that a successful run returns
// exit code 0.
func TestClaudeIntegrationExitCodeZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	var stdout, stderr bytes.Buffer

	result, err := NewClaudeAgent().Run(ctx, Config{
		Command: claudeBinary,
		WorkDir: t.TempDir(),
		Args:    []string{"--dangerously-skip-permissions", "--max-turns", "1"},
		Prompt:  "Say OK",
		Stdout:  &stdout,
		Stderr:  &stderr,
	}, rec)

	if err != nil {
		t.Fatalf("Run() returned error: %v\nstderr: %s", err, stderr.String())
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	t.Logf("Exit code: %d", result.ExitCode)
	t.Logf("Duration: %s", result.Duration)

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d\nstderr: %s", result.ExitCode, result.Error)
	}
}

// TestClaudeIntegrationRecordingEvents verifies that the recorder captures
// both meta events and claude_stream events from a real run.
func TestClaudeIntegrationRecordingEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	var stdout, stderr bytes.Buffer

	result, err := NewClaudeAgent().Run(ctx, Config{
		Command: claudeBinary,
		WorkDir: t.TempDir(),
		Args:    []string{"--dangerously-skip-permissions", "--max-turns", "1"},
		Prompt:  "Say hi",
		Stdout:  &stdout,
		Stderr:  &stderr,
	}, rec)

	if err != nil {
		t.Fatalf("Run() returned error: %v\nstderr: %s", err, stderr.String())
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	events := rec.Events()
	t.Logf("Total recorded events: %d", len(events))

	// Check for meta events.
	var metaCount int
	for _, ev := range events {
		if ev.Type == "meta" {
			metaCount++
			t.Logf("Meta event: %s", ev.Data)
		}
	}
	if metaCount == 0 {
		t.Error("No meta events recorded; expected at least agent/command/workdir meta events")
	}

	// We expect at least agent, command, and workdir meta events.
	var hasAgentMeta, hasCommandMeta, hasWorkdirMeta bool
	for _, ev := range events {
		if ev.Type == "meta" {
			if strings.HasPrefix(ev.Data, "agent=") {
				hasAgentMeta = true
			}
			if strings.HasPrefix(ev.Data, "command=") {
				hasCommandMeta = true
			}
			if strings.HasPrefix(ev.Data, "workdir=") {
				hasWorkdirMeta = true
			}
		}
	}
	if !hasAgentMeta {
		t.Error("Missing agent= meta event")
	}
	if !hasCommandMeta {
		t.Error("Missing command= meta event")
	}
	if !hasWorkdirMeta {
		t.Error("Missing workdir= meta event")
	}

	// Check for stdin recording (the prompt).
	var hasStdin bool
	for _, ev := range events {
		if ev.Type == "stdin" {
			hasStdin = true
			break
		}
	}
	if !hasStdin {
		t.Error("No stdin event recorded; expected prompt to be recorded")
	}

	// Check for stream events from the Claude NDJSON output.
	var streamCount int
	for _, ev := range events {
		if ev.Type == "claude_stream" {
			streamCount++
		}
	}
	t.Logf("claude_stream events: %d", streamCount)
	if streamCount == 0 {
		t.Error("No claude_stream events recorded; expected at least system/init and result events")
	}
}

// TestClaudeIntegrationContextCancel starts a run and cancels the context
// after a short delay, verifying the agent returns without hanging.
func TestClaudeIntegrationContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rec := newTestRecorder(t)

	var stdout, stderr bytes.Buffer

	done := make(chan struct{})
	var result *Result
	var runErr error

	go func() {
		defer close(done)
		result, runErr = NewClaudeAgent().Run(ctx, Config{
			Command: claudeBinary,
			WorkDir: t.TempDir(),
			Args:    []string{"--dangerously-skip-permissions", "--max-turns", "1"},
			// Use a prompt that encourages a longer response to ensure we have
			// time to cancel before it finishes.
			Prompt: "Write a very long essay about the history of computing, at least 5000 words",
			Stdout: &stdout,
			Stderr: &stderr,
		}, rec)
	}()

	// Wait for the run to finish. It should return within a reasonable
	// time after the 5-second context deadline expires.
	select {
	case <-done:
		// Good, it returned.
	case <-time.After(30 * time.Second):
		t.Fatal("Agent did not return within 30 seconds after context cancellation; it appears to be hanging")
	}

	t.Logf("Run returned after context cancel")
	if runErr != nil {
		t.Logf("Run error (expected due to cancellation): %v", runErr)
	}
	if result != nil {
		t.Logf("Exit code: %d", result.ExitCode)
		t.Logf("Duration: %s", result.Duration)
	}

	// The key assertion is that we got here at all (no hang).
	// The agent may return an error or a non-zero exit code, both are acceptable.
	t.Log("Context cancellation test passed: agent returned without hanging")
}
