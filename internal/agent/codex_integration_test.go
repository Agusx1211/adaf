//go:build integration

package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

const codexBinary = "/home/agusx1211/.bun/bin/codex"

// setupCodexTest creates a temp directory, initialises a store, and returns
// the work dir, store, and recorder. It skips the test if the codex binary is
// missing.
func setupCodexTest(t *testing.T) (string, *store.Store, *recording.Recorder) {
	t.Helper()

	if _, err := os.Stat(codexBinary); err != nil {
		t.Skipf("codex binary not found at %s: %v", codexBinary, err)
	}
	workDir := t.TempDir()
	storeDir := t.TempDir()

	s, err := store.New(storeDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	// Initialise the store so that records/ directory exists.
	if err := s.Init(store.ProjectConfig{Name: "codex-integration-test"}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	rec := recording.New(1, s)
	return workDir, s, rec
}

// TestCodexIntegrationBasicPrompt sends a simple prompt to the real codex CLI
// and verifies it runs to completion and produces output.
func TestCodexIntegrationBasicPrompt(t *testing.T) {
	workDir, _, rec := setupCodexTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agent := NewCodexAgent()

	result, err := agent.Run(ctx, Config{
		Command: codexBinary,
		Args:    []string{"--full-auto", "--skip-git-repo-check"},
		WorkDir: workDir,
		Prompt:  "Respond with exactly: HELLO_CODEX_TEST_OK",
	}, rec)

	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}

	t.Logf("Exit code: %d", result.ExitCode)
	t.Logf("Duration: %s", result.Duration)
	t.Logf("Output length: %d bytes", len(result.Output))
	t.Logf("Stderr length: %d bytes", len(result.Error))

	// Log first 500 chars of output for debugging.
	outputPreview := result.Output
	if len(outputPreview) > 500 {
		outputPreview = outputPreview[:500] + "...(truncated)"
	}
	t.Logf("Output preview: %q", outputPreview)

	// The agent should have produced some output (even if the exact string varies).
	combinedOutput := result.Output + result.Error
	if len(strings.TrimSpace(combinedOutput)) == 0 {
		t.Error("Run() produced no output at all (both stdout and stderr empty)")
	}
}

// TestCodexIntegrationExitCode verifies that a normal codex run exits with code 0.
func TestCodexIntegrationExitCode(t *testing.T) {
	workDir, _, rec := setupCodexTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agent := NewCodexAgent()

	result, err := agent.Run(ctx, Config{
		Command: codexBinary,
		Args:    []string{"--full-auto", "--skip-git-repo-check"},
		WorkDir: workDir,
		Prompt:  "Say hello",
	}, rec)

	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
	if result.ExitCode != 0 {
		t.Errorf("Run() exit code = %d, want 0; stderr: %s", result.ExitCode, result.Error)
	}
	if result.Duration <= 0 {
		t.Errorf("Run() duration = %s, want > 0", result.Duration)
	}
}

// TestCodexIntegrationRecordingEvents verifies that the recorder captures
// meta events (agent name, command, workdir) and at least some I/O events.
func TestCodexIntegrationRecordingEvents(t *testing.T) {
	workDir, _, rec := setupCodexTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agent := NewCodexAgent()

	_, err := agent.Run(ctx, Config{
		Command: codexBinary,
		Args:    []string{"--full-auto", "--skip-git-repo-check"},
		WorkDir: workDir,
		Prompt:  "Say OK",
	}, rec)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	events := rec.Events()
	if len(events) == 0 {
		t.Fatal("recorder captured zero events")
	}
	t.Logf("Total recorded events: %d", len(events))

	// Check for expected meta events.
	metaEvents := make(map[string]string) // key -> value
	for _, ev := range events {
		if ev.Type == "meta" {
			parts := strings.SplitN(ev.Data, "=", 2)
			if len(parts) == 2 {
				metaEvents[parts[0]] = parts[1]
			}
		}
	}

	// agent=codex must be present.
	if v, ok := metaEvents["agent"]; !ok {
		t.Error("missing meta event for 'agent'")
	} else if v != "codex" {
		t.Errorf("meta agent = %q, want %q", v, "codex")
	}

	// command should contain "exec".
	if v, ok := metaEvents["command"]; !ok {
		t.Error("missing meta event for 'command'")
	} else if !strings.Contains(v, "exec") {
		t.Errorf("meta command = %q, does not contain 'exec'", v)
	}

	// workdir should be set.
	if v, ok := metaEvents["workdir"]; !ok {
		t.Error("missing meta event for 'workdir'")
	} else if v == "" {
		t.Error("meta workdir is empty")
	}

	// Check that stdin event was recorded (the prompt).
	hasStdin := false
	for _, ev := range events {
		if ev.Type == "stdin" {
			hasStdin = true
			break
		}
	}
	if !hasStdin {
		t.Error("no stdin event recorded (prompt should be captured)")
	}

	// Log event type counts for debugging.
	typeCounts := make(map[string]int)
	for _, ev := range events {
		typeCounts[ev.Type]++
	}
	for evType, count := range typeCounts {
		t.Logf("  event type %q: %d", evType, count)
	}
}

// TestCodexIntegrationContextCancel verifies that cancelling the context
// causes the agent to return promptly rather than hanging.
func TestCodexIntegrationContextCancel(t *testing.T) {
	workDir, _, rec := setupCodexTest(t)

	// Give it 5 seconds then cancel. The prompt is deliberately slow to
	// ensure the agent is still running when we cancel.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent := NewCodexAgent()

	start := time.Now()
	result, err := agent.Run(ctx, Config{
		Command: codexBinary,
		Args:    []string{"--full-auto", "--skip-git-repo-check"},
		WorkDir: workDir,
		Prompt:  "Write a 10000 word essay about the history of computing. Include all major milestones from 1940 to 2024.",
	}, rec)
	elapsed := time.Since(start)

	t.Logf("Elapsed: %s", elapsed)

	// The agent should have returned -- either with an error or a result.
	// Context cancellation typically kills the process and gives a non-zero exit.
	if err != nil {
		// Some error is expected since the process was killed.
		t.Logf("Run() returned error (expected for cancelled context): %v", err)
	} else if result != nil {
		t.Logf("Run() returned result with exit code %d", result.ExitCode)
		// A non-zero exit code is expected since we killed the process.
		if result.ExitCode == 0 {
			t.Logf("Note: process exited 0 despite cancellation (it may have finished quickly)")
		}
	}

	// The key assertion: it should not take much longer than the timeout.
	// Allow a generous 15 second grace period for process cleanup.
	if elapsed > 20*time.Second {
		t.Errorf("Run() took %s after context cancel (expected <= 20s)", elapsed)
	}
}
