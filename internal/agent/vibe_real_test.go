//go:build integration

package agent

import (
	"strings"
	"testing"
	"time"
)

// TestVibeRealIntegration runs the shared integration test suite against the
// real vibe CLI binary, plus a Vibe-specific buffer output test.
func TestVibeRealIntegration(t *testing.T) {
	suite := &agentTestSuite{
		Name:           "vibe",
		Binary:         findBinary("vibe"),
		NewAgent:       func() Agent { return NewVibeAgent() },
		BaseArgs:       []string{"--max-turns", "10"},
		SupportsResume: true,
		Timeout:        180 * time.Second,
	}

	suite.runAll(t)

	// Vibe-specific: verify buffer-mode output capture.
	t.Run("TestVibeBufferOutput", func(t *testing.T) {
		suite.skip(t)

		ws := newTestWorkspace(t, "vibe")
		result := suite.run(t, ws, "Say exactly: VIBE_OUTPUT_CHECK")

		if result.ExitCode != 0 {
			t.Fatalf("exit code %d; stderr:\n%s\nstdout:\n%s", result.ExitCode, result.Error, result.Output)
		}

		if result.Output == "" {
			t.Fatal("result.Output is empty; vibe is a buffer-mode agent so stdout should be captured")
		}

		if !strings.Contains(result.Output, "VIBE_OUTPUT_CHECK") {
			t.Errorf("result.Output does not contain expected marker VIBE_OUTPUT_CHECK.\nGot: %q", result.Output)
		}

		t.Logf("buffer output length: %d bytes", len(result.Output))
	})
}
