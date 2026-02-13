//go:build integration

package agent

import (
	"strings"
	"testing"
	"time"
)

// TestCodexRealIntegration runs the shared integration test suite against the
// real codex CLI binary, plus Codex-specific stream parsing verification.
func TestCodexRealIntegration(t *testing.T) {
	suite := &agentTestSuite{
		Name:           "codex",
		Binary:         findBinary("codex"),
		NewAgent:       func() Agent { return NewCodexAgent() },
		BaseArgs:       []string{},
		SupportsResume: true,
		Timeout:        180 * time.Second,
	}

	suite.runAll(t)

	// --- Codex-specific tests ---

	t.Run("TestCodexStreamParsing", func(t *testing.T) {
		suite.skip(t)

		ws := newTestWorkspace(t, "codex")
		ws.writeFile("hello.txt", "STREAM_PARSE_MARKER_99999")

		result := suite.run(t, ws, strings.Join([]string{
			"Read the file hello.txt in the current directory.",
			"Tell me exactly what it contains.",
		}, " "))

		if result.ExitCode != 0 {
			t.Fatalf("exit code %d; stderr:\n%s\nstdout:\n%s", result.ExitCode, result.Error, result.Output)
		}

		// The JSONL output should have been parsed into result.Output.
		if strings.TrimSpace(result.Output) == "" {
			t.Fatal("result.Output is empty; Codex JSONL stream was not parsed into text")
		}
		t.Logf("output length: %d bytes", len(result.Output))

		// The agent should have returned a thread ID (AgentSessionID) from
		// the JSONL stream init event.
		if result.AgentSessionID == "" {
			t.Error("result.AgentSessionID is empty; expected a thread_id from Codex JSONL output")
		} else {
			t.Logf("agent session ID (thread_id): %s", result.AgentSessionID)
		}
	})
}
